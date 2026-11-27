// collector_test.go：验证采集 handler 对原始 JSON 输出、字段作用域、噪声过滤与脱敏的约束。

package logs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestCollectorPreservesOutputAndSanitizesCollection 验证原输出不变，且存储只收到白名单中的安全字段。
func TestCollectorPreservesOutputAndSanitizesCollection(t *testing.T) {
	var output bytes.Buffer
	repo := &memoryRepository{}
	base := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	service := New(repo, base)
	logger := service.WrapLogger(base).With("module", "auth").With("operation", "login")
	logger.Info("authentication operation completed", "outcome", "failed", "duration_ms", int64(75),
		"password", "synthetic-password", "request_id", "req-1", "error_code", "invalid_credentials",
		"math", math.NaN(), "path", "https://example.test/video?sign=private-signature",
		"reason", map[string]string{"unexpected": "nested-secret"}, "total_bytes", math.Inf(1))
	logger.WithGroup("private").With("session_token", "synthetic-session").Info("操作记录",
		"operation", "ignored-in-group", "route", "/not-top-level")
	logger.Info("普通日志", slog.Group("", slog.String("operation", "logout"), slog.String("request_id", "req-2")))
	stopService(t, service)

	if !strings.Contains(output.String(), "synthetic-password") || !strings.Contains(output.String(), "private-signature") ||
		!strings.Contains(output.String(), `"private":{"session_token":"synthetic-session"`) {
		t.Fatalf("原始 JSON 格式或字段发生变化: %s", &output)
	}
	entries := repo.entries
	if len(entries) != 3 {
		t.Fatalf("应存储三条日志，得到 %d", len(entries))
	}
	first := entries[0]
	if first.Message != "认证 · 登录失败" || first.RequestID != "req-1" || first.Module != "auth" ||
		first.ErrorCode != "invalid_credentials" || first.Details["duration_ms"] != int64(75) ||
		first.Details["path"] != redacted {
		t.Fatalf("首条结构化日志不符合预期: %+v", first)
	}
	if _, exists := first.Details["reason"]; exists {
		t.Errorf("任意嵌套对象不应进入日志详情")
	}
	if _, exists := first.Details["total_bytes"]; exists {
		t.Errorf("不可序列化的数值不应进入日志详情")
	}
	if entries[1].Operation != "login" || len(entries[1].Details) != 0 {
		t.Errorf("具名分组应保留外层属性，忽略分组内部字段: %+v", entries[1])
	}
	if entries[2].Operation != "logout" || entries[2].RequestID != "req-2" {
		t.Errorf("匿名组应展开并覆盖同名属性: %+v", entries[2])
	}
}

// TestCollectorFiltersInfrastructureNoise 验证健康探针、日志查询、成功访问与内部诊断不会写入日志表。
func TestCollectorFiltersInfrastructureNoise(t *testing.T) {
	repo := &memoryRepository{}
	service := New(repo, slog.New(slog.DiscardHandler))
	base := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	logger := service.WrapLogger(base)
	logger.Warn("探针诊断", "module", "health")
	logger.Warn("日志页自身请求", "module", "http", "method", "GET", "route", "/api/v1/logs/export", "status", 403)
	logger.Info("常规访问", "module", "http", "status", 302)
	logger.Error("采集诊断", "module", "logging", "operation", "flush")
	logger.Info("http request", "module", "http", "status", 500, "route", "/api/v1/tasks")
	logger.Error("request panic", "module", "http", "operation", "request", "status", 500, "route", "/api/v1/tasks")
	logger.Debug("已取消访问", "module", "http", "outcome", "canceled")
	logger.Info("导出完成", "module", "logging", "operation", "export")
	logger.Debug("级别已禁用", "module", "app")
	stopService(t, service)
	if got := repo.entries; len(got) != 2 || got[0].Level != "error" || got[0].Message != "处理 HTTP 请求时发生内部错误" ||
		got[0].Operation != "request" || got[0].Module != "system" || got[1].Operation != "export" || got[1].Module != "system" {
		t.Fatalf("采集筛选或级别有误: %+v", got)
	}
}

// TestSanitizeText 验证常见凭据、签名地址和内部控制字符在入库前被遮挡或移除。
func TestSanitizeText(t *testing.T) {
	for _, value := range []string{
		"password: synthetic-password",
		"密码：测试值",
		"Cookie: arbitrary=synthetic-cookie",
		`{\"Authorization\":\"synthetic-auth\"}`,
		"CID=synthetic-cookie; SEID=another-cookie",
		"access_token=synthetic-token",
		"Bearer synthetic-token-value",
		"Authorization: Basic YWJjOjEyMw==",
		"hash=$argon2id$synthetic-hash",
		"hash=$2b$synthetic-hash",
		"csrf-token=synthetic-csrf",
		"MEDIAHUB_SESSION=synthetic-session",
		"eyJhbGciOiJ0ZXN0In0.eyJzdWIiOjF9.signature",
		"https://example.test/video?sign=synthetic-url",
		"播放 /p/internal-playback-token",
		"https%3A%2F%2Fexample.test%2Fa",
		strings.Repeat("日志", maxMessageRunes) + "password=hidden-after-limit",
	} {
		if got := sanitizeText(value, maxMessageRunes); got != redacted {
			t.Errorf("敏感文本未遮挡: %q", value)
		}
	}
	value := strings.Repeat("中", maxMessageRunes+1)
	if got := sanitizeText(value, maxMessageRunes); !utf8.ValidString(got) || utf8.RuneCountInString(got) != maxMessageRunes {
		t.Errorf("应在字符边界截断: %d / %d", len(got), utf8.RuneCountInString(got))
	}
	if got := sanitizeText("路径\r\n归属\x00正常", maxMessageRunes); got != "路径  归属 正常" {
		t.Errorf("应移除控制字符: %q", got)
	}
}

// TestCollectorBoundsDetails 验证持久化详情预算按实际 JSON 字节数计算。
func TestCollectorBoundsDetails(t *testing.T) {
	attrs := make(map[string]any)
	for _, key := range detailKeys {
		attrs[key] = strings.Repeat("<", maxAttributeRunes)
	}
	handler := &collectorHandler{attrs: attrs}
	entry, ok := handler.entry(slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0))
	encoded, err := json.Marshal(entry.Details)
	if !ok || err != nil || len(encoded) > maxDetailsBytes || len(entry.Details) == 0 || len(entry.Details) == len(detailKeys) {
		t.Fatalf("结果未遵守详情大小上限: fields=%d size=%d err=%v", len(entry.Details), len(encoded), err)
	}
}
