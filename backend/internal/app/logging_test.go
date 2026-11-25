// logging_test.go：验证结构化日志关键字段、真实文件轮转及过期备份清理，覆盖配置的存储保留边界。

package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStructuredLogAndRotation 验证 JSON 日志字段、10 MiB 文件触发轮转及七天外备份清理后的活动文件内容。
func TestStructuredLogAndRotation(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "app.log")
	// 预置过期备份，验证真实轮转执行配置的保留策略。
	oldName := filepath.Join(directory, "app-"+time.Now().Add(-8*24*time.Hour).UTC().Format("2006-01-02T15-04-05.000")+".log")
	if err := os.WriteFile(oldName, []byte("expired"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	logger, closer, err := NewLogger(Config{LogFile: filename}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("started", "module", "app", "operation", "startup", "outcome", "succeeded")
	var entry map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["module"] != "app" || entry["operation"] != "startup" || entry["outcome"] != "succeeded" {
		t.Fatalf("missing log fields: %v", entry)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	// 预置达到上限的活动文件，让下一次写入触发真实 lumberjack 轮转。
	if err := os.WriteFile(filename, []byte(strings.Repeat("x", 10*1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	logger, closer, err = NewLogger(Config{LogFile: filename}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("rotated")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := os.Stat(oldName)
		if os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired log was not removed")
		}
		runtime.Gosched()
	}
	entries, err := filepath.Glob(filepath.Join(directory, "app-*.log"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("rotation backups: %v, %v", entries, err)
	}
	active, err := os.ReadFile(filename)
	if err != nil || !strings.Contains(string(active), "rotated") {
		t.Fatalf("new active log: %s, %v", active, err)
	}
}
