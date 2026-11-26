// logs_test.go：使用真实 SQLite 和管理员会话验证运行日志的分页、导出、清理及实时流边界。

package http

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/logs"
)

// newRuntimeLogBrowser 给测试浏览器安装运行日志接口，确保清理时先排空采集再关闭测试库。
func newRuntimeLogBrowser(t *testing.T) (*authBrowser, *sqlite.Database, *slog.Logger) {
	t.Helper()
	return newRuntimeLogBrowserAt(t, filepath.Join(t.TempDir(), "logs.db"))
}

// newRuntimeLogBrowserAt 装配指向指定临时库的真实接口，供 SQLite 故障注入测试复用。
func newRuntimeLogBrowserAt(t *testing.T, databasePath string) (*authBrowser, *sqlite.Database, *slog.Logger) {
	t.Helper()
	browser, authentication, database := newAuthBrowserAt(t, databasePath, false, nil)
	base := slog.New(slog.NewJSONHandler(io.Discard, nil))
	service := logs.New(database, base)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := service.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	logger := service.WrapLogger(base)
	router, err := NewRouter(database, logger, nil, AuthOptions{Service: authentication, Logs: service})
	if err != nil {
		t.Fatal(err)
	}
	browser.handler = router
	return browser, database, logger
}

// TestRuntimeLogsManagement 验证认证与同源 CSRF、按相同时间戳行号分页、字面关键词查询和安全清理范围。
func TestRuntimeLogsManagement(t *testing.T) {
	browser, database, _ := newRuntimeLogBrowser(t)
	path := "/api/v1/logs"
	for _, suffix := range []string{"", "/stream", "/export?format=log"} {
		assertAuthError(t, browser.request(t, http.MethodGet, path+suffix, "", nil), http.StatusUnauthorized, generated.AuthenticationRequired)
	}
	browser.setup(t)
	clearAll := `{"scope":"all"}`
	assertAuthError(t, browser.request(t, http.MethodPost, path+"/clear", clearAll, map[string]string{"X-CSRF-Token": ""}), http.StatusForbidden, generated.CsrfInvalid)
	assertAuthError(t, browser.request(t, http.MethodPost, path+"/clear", clearAll, map[string]string{"Origin": "https://another.test"}), http.StatusForbidden, generated.CsrfInvalid)
	assertAuthError(t, browser.request(t, http.MethodPost, path+"/clear", `{"scope":"invalid"}`, nil), http.StatusBadRequest, generated.ValidationFailed)
	for _, query := range []string{"limit=201", "cursor=bad", "range=1d", "level=INFO", "module=fake", "q=" + strings.Repeat("a", 129), "level=info&level=warn"} {
		assertAuthError(t, browser.request(t, http.MethodGet, path+"?"+query, "", nil), http.StatusBadRequest, generated.ValidationFailed)
	}
	now := time.Now().UTC()
	entries, err := database.InsertRuntimeLogs(t.Context(), []domain.Entry{
		{OccurredAt: now, Level: "error", Module: "strm", JobID: "job-1", Message: "转换 Report%_Failed 第一条"},
		{OccurredAt: now, Level: "error", Module: "strm", JobID: "job-1", Message: "转换 REPORT%_FAILED 第二条"},
		{OccurredAt: now, Level: "error", Module: "strm", JobID: "job-1", Message: "转换 reportXXfailed"},
		{OccurredAt: now.Add(-8 * 24 * time.Hour), Level: "error", Module: "strm", JobID: "job-1", Message: "旧日志 report%_failed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	query := "?limit=1&level=error&module=strm&job_id=job-1&q=" + url.QueryEscape("report%_failed")
	first := taskJSON[generated.RuntimeLogPage](t, browser.request(t, http.MethodGet, path+query, "", nil), http.StatusOK)
	if len(first.Items) != 1 || first.Items[0].Id != strconv.FormatInt(entries[1].ID, 10) || first.NextCursor == nil {
		t.Fatalf("first page = %+v", first)
	}
	second := taskJSON[generated.RuntimeLogPage](t, browser.request(t, http.MethodGet, path+query+"&cursor="+url.QueryEscape(*first.NextCursor), "", nil), http.StatusOK)
	if len(second.Items) != 1 || second.Items[0].Id != strconv.FormatInt(entries[0].ID, 10) || second.NextCursor != nil {
		t.Fatalf("second page = %+v", second)
	}
	clear := taskJSON[generated.ClearRuntimeLogsResponse](t, browser.request(t, http.MethodPost, path+"/clear", `{"scope":"before_7d"}`, nil), http.StatusOK)
	if clear.DeletedCount != 1 || clear.Scope != generated.ClearRuntimeLogsScopeBefore7d {
		t.Fatalf("clear = %+v", clear)
	}
	all := taskJSON[generated.RuntimeLogPage](t, browser.request(t, http.MethodGet, path+"?range=all&module=strm", "", nil), http.StatusOK)
	if len(all.Items) != 3 || all.NextCursor != nil {
		t.Fatalf("all = %+v", all)
	}
}

// TestRuntimeLogClearAuditFailure 验证审计写入失败时返回脱敏 503，事务回滚保留原运行日志。
func TestRuntimeLogClearAuditFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "logs.db")
	browser, database, _ := newRuntimeLogBrowserAt(t, dbPath)
	browser.setup(t)
	entries, err := database.InsertRuntimeLogs(t.Context(), []domain.Entry{
		{OccurredAt: time.Now().UTC(), Level: "warn", Module: "strm", Message: "保留原日志"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fault, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fault.Close(); err != nil {
			t.Error(err)
		}
	})
	_, err = fault.ExecContext(t.Context(), `CREATE TRIGGER deny_runtime_clear_audit BEFORE INSERT ON audit_records
		WHEN NEW.action = 'clear_runtime_logs' BEGIN SELECT RAISE(ABORT,'internal audit failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	failed := browser.request(t, http.MethodPost, "/api/v1/logs/clear", `{"scope":"all"}`, nil)
	assertAuthError(t, failed, http.StatusServiceUnavailable, generated.ServiceUnavailable)
	if strings.Contains(failed.Body.String(), "internal audit failure") {
		t.Fatalf("clear exposed storage error: %s", failed.Body)
	}
	response := browser.request(t, http.MethodGet, "/api/v1/logs?module=strm", "", nil)
	page := taskJSON[generated.RuntimeLogPage](t, response, http.StatusOK)
	if len(page.Items) != 1 || page.Items[0].Id != strconv.FormatInt(entries[0].ID, 10) {
		t.Fatalf("original runtime log lost after rollback: %+v", page)
	}
}

// TestRuntimeLogsExport 验证导出上限、倒序分批、JSONL 与纯文本下载头及空结果格式。
func TestRuntimeLogsExport(t *testing.T) {
	browser, database, _ := newRuntimeLogBrowser(t)
	browser.setup(t)
	path := "/api/v1/logs/export"
	assertAuthError(t, browser.request(t, http.MethodGet, path+"?format=csv", "", nil), http.StatusBadRequest, generated.ValidationFailed)
	now := time.Now().UTC()
	seed := make([]domain.Entry, runtimeLogExportLimit+1)
	for i := range seed {
		seed[i] = domain.Entry{OccurredAt: now, Level: "info", Module: "sync", Message: "同步\n完成"}
	}
	entries, err := database.InsertRuntimeLogs(t.Context(), seed)
	if err != nil {
		t.Fatal(err)
	}
	export := browser.request(t, http.MethodGet, path+"?format=jsonl&module=sync", "", nil)
	if export.Code != http.StatusOK || export.Header().Get("X-Export-Truncated") != "true" ||
		!strings.HasPrefix(export.Header().Get("Content-Type"), "application/x-ndjson") ||
		!strings.HasSuffix(export.Header().Get("Content-Disposition"), `.jsonl"`) {
		t.Fatalf("jsonl export status=%d headers=%v", export.Code, export.Header())
	}
	lines := bufio.NewScanner(export.Body)
	count := 0
	for lines.Scan() {
		var entry generated.RuntimeLogEntry
		if err := json.Unmarshal(lines.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Id != strconv.FormatInt(entries[runtimeLogExportLimit-count].ID, 10) || entry.JobId != nil || entry.Message != "同步\n完成" {
			t.Fatalf("export line %d = %+v", count, entry)
		}
		count++
	}
	if lines.Err() != nil || count != runtimeLogExportLimit {
		t.Fatalf("export count=%d err=%v", count, lines.Err())
	}
	logFile := browser.request(t, http.MethodGet, path+"?format=log&module=sync", "", nil)
	if logFile.Code != http.StatusOK || !strings.HasPrefix(logFile.Header().Get("Content-Type"), "text/plain") ||
		!strings.HasSuffix(logFile.Header().Get("Content-Disposition"), `.log"`) || !strings.Contains(logFile.Body.String(), "[INFO] [sync] 同步 完成\n") {
		t.Fatalf("text export status=%d headers=%v", logFile.Code, logFile.Header())
	}
	empty := browser.request(t, http.MethodGet, path+"?format=jsonl&module=upload", "", nil)
	if empty.Code != http.StatusOK || empty.Header().Get("X-Export-Truncated") != "false" || empty.Body.Len() != 0 {
		t.Fatalf("empty export status=%d headers=%v body=%s", empty.Code, empty.Header(), empty.Body)
	}
}

// TestRuntimeLogStreamLifecycle 验证筛选、持久 ID、清理通知和注销后的发送前复核。
func TestRuntimeLogStreamLifecycle(t *testing.T) {
	browser, _, logger := newRuntimeLogBrowser(t)
	browser.setup(t)
	server := httptest.NewServer(browser.handler)
	defer server.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/v1/logs/stream?level=warn&module=strm&q="+url.QueryEscape("验证"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(browser.cookies[sessionCookie])
	client := &http.Client{Timeout: 5 * time.Second}
	stream, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if stream.StatusCode != http.StatusOK || stream.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream status=%d headers=%v", stream.StatusCode, stream.Header)
	}
	reader := bufio.NewReader(stream.Body)
	if frame := readRuntimeLogFrame(t, reader); frame != ": connected\n\n" {
		t.Fatalf("initial frame=%q", frame)
	}
	logger.Warn("另一个模块验证", "module", "jobs")
	logger.Warn("目标日志验证", "module", "strm", "operation", "test", "job_id", "job-test")
	frame := readRuntimeLogFrame(t, reader)
	prefix := "id: 1\nevent: log.created\ndata: "
	if !strings.HasPrefix(frame, prefix) {
		t.Fatalf("created frame=%q", frame)
	}
	var event generated.RuntimeLogEntry
	if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, prefix)), &event); err != nil {
		t.Fatal(err)
	}
	page := taskJSON[generated.RuntimeLogPage](t, browser.request(t, http.MethodGet, "/api/v1/logs?level=warn&module=strm&q="+url.QueryEscape("验证"), "", nil), http.StatusOK)
	if len(page.Items) != 1 || page.Items[0].Id != event.Id || event.JobId == nil || *event.JobId != "job-test" {
		t.Fatalf("stream entry=%+v persisted=%+v", event, page)
	}
	taskJSON[generated.ClearRuntimeLogsResponse](t, browser.request(t, http.MethodPost, "/api/v1/logs/clear", `{"scope":"all"}`, nil), http.StatusOK)
	if frame := readRuntimeLogFrame(t, reader); !strings.HasPrefix(frame, "id: 2\nevent: log.cleared\ndata: ") {
		t.Fatalf("clear frame=%q", frame)
	}
	if response := browser.request(t, http.MethodPost, "/api/v1/auth/logout", "", nil); response.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d: %s", response.Code, response.Body)
	}
	logger.Warn("注销后的验证", "module", "strm")
	if data, err := io.ReadAll(reader); err != nil || len(data) != 0 {
		t.Fatalf("revoked stream data=%q err=%v", data, err)
	}
}

// readRuntimeLogFrame 读取至 SSE 空行分隔符，在断连前得到完整帧。
func readRuntimeLogFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		frame.WriteString(line)
		if line == "\n" {
			return frame.String()
		}
	}
}
