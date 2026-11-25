// tasks_test.go：验证任务与计划管理 API 的认证、CSRF、持久结果、失败项重试和会话撤销后的 SSE 关闭。

package http

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

// newTaskBrowser 在临时认证浏览器上装配媒体库与指定任务模块，返回可直接控制生命周期的任务服务。
func newTaskBrowser(t *testing.T, modules map[domain.Type]jobs.Module) (*authBrowser, *jobs.Service) {
	t.Helper()
	browser, authentication, db := newAuthBrowser(t, false, nil)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	libraries := library.New(db, logger)
	service := jobs.New(db, libraries, modules, logger)
	router, err := NewRouter(db, logger, nil, AuthOptions{Service: authentication, Library: libraries, Jobs: service})
	if err != nil {
		t.Fatal(err)
	}
	browser.handler = router
	return browser, service
}

// taskJSON 检查响应状态并解码指定传输类型，失败时提供原始测试响应帮助定位。
func taskJSON[T any](t *testing.T, response *httptest.ResponseRecorder, status int) T {
	t.Helper()
	var data T
	if response.Code != status {
		t.Fatalf("status %d: %s, want %d", response.Code, response.Body, status)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

const allLibrariesDailySchedule = `{"name":"每日扫描","type":"scan","library_id":null,"enabled":true,"period":"daily","timezone":"Asia/Shanghai","time":"03:00","notify_on":["partial_failure"]}`

// TestTaskCenterManagementBoundaryAndUnavailableTypes 验证任务中心读写权限、空集合合同、未注册类型限制和非法查询或执行参数拒绝。
func TestTaskCenterManagementBoundaryAndUnavailableTypes(t *testing.T) {
	b, _ := newTaskBrowser(t, nil)
	for _, route := range []string{"/api/v1/task-types", "/api/v1/tasks", "/api/v1/tasks/missing", "/api/v1/tasks/missing/failures", "/api/v1/tasks/missing/logs", "/api/v1/schedules", "/api/v1/events"} {
		assertAuthError(t, b.request(t, http.MethodGet, route, "", nil), http.StatusUnauthorized, generated.AuthenticationRequired)
	}
	b.setup(t)
	kinds := taskJSON[generated.TaskTypeCapabilities](t, b.request(t, http.MethodGet, "/api/v1/task-types", "", nil), http.StatusOK)
	if len(kinds.Items) != len(domain.Types()) {
		t.Fatalf("capabilities: %+v", kinds.Items)
	}
	seen := make(map[generated.TaskType]bool)
	for _, kind := range kinds.Items {
		if seen[kind.Type] || !kind.Type.Valid() || kind.Available || kind.Reason == nil || kind.CanPause || kind.CanRetryFailed {
			t.Fatalf("capability: %+v", kind)
		}
		seen[kind.Type] = true
	}
	page := taskJSON[generated.TaskPage](t, b.request(t, http.MethodGet, "/api/v1/tasks", "", nil), http.StatusOK)
	if page.Items == nil || len(page.Items) != 0 || page.ByStatus == nil || page.ByType == nil || page.Total != 0 || page.NextCursor != nil {
		t.Fatalf("empty task page: %+v", page)
	}
	plans := taskJSON[generated.SchedulePage](t, b.request(t, http.MethodGet, "/api/v1/schedules", "", nil), http.StatusOK)
	if plans.Items == nil || len(plans.Items) != 0 || plans.NextCursor != nil {
		t.Fatalf("empty schedule page: %+v", plans)
	}

	submission := `{"type":"scan","library_id":"library"}`
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/tasks", submission},
		{http.MethodPost, "/api/v1/tasks/id/actions", `{"action":"cancel"}`},
		{http.MethodPost, "/api/v1/schedules", allLibrariesDailySchedule},
		{http.MethodPut, "/api/v1/schedules/id", allLibrariesDailySchedule},
		{http.MethodPatch, "/api/v1/schedules/id/enabled", `{"enabled":true}`},
		{http.MethodDelete, "/api/v1/schedules/id", ""},
	} {
		assertAuthError(t, b.request(t, request.method, request.path, request.body, map[string]string{"X-CSRF-Token": "wrong"}), http.StatusForbidden, generated.CsrfInvalid)
	}
	assertAuthError(t, b.request(t, http.MethodPost, "/api/v1/tasks", submission, nil), http.StatusConflict, generated.TaskTypeUnavailable)
	assertAuthError(t, b.request(t, http.MethodPost, "/api/v1/tasks", strings.TrimSuffix(submission, "}")+`,"parameters":{}}`, nil), http.StatusBadRequest, generated.ValidationFailed)
	assertAuthError(t, b.request(t, http.MethodPost, "/api/v1/schedules", allLibrariesDailySchedule, nil), http.StatusConflict, generated.TaskTypeUnavailable)
	invalidSchedule := strings.Replace(allLibrariesDailySchedule, `"library_id":null,`, "", 1)
	assertAuthError(t, b.request(t, http.MethodPost, "/api/v1/schedules", invalidSchedule, nil), http.StatusBadRequest, generated.ValidationFailed)
	assertAuthError(t, b.request(t, http.MethodGet, "/api/v1/tasks?limit=201", "", nil), http.StatusBadRequest, generated.ValidationFailed)
	assertAuthError(t, b.request(t, http.MethodGet, "/api/v1/tasks?status=unknown", "", nil), http.StatusBadRequest, generated.ValidationFailed)
	assertAuthError(t, b.request(t, http.MethodGet, "/api/v1/schedules?enabled=yes", "", nil), http.StatusBadRequest, generated.ValidationFailed)
	assertAuthError(t, b.request(t, http.MethodGet, "/api/v1/tasks/missing/failures", "", nil), http.StatusNotFound, generated.NotFound)
}

// TestTaskCenterReadsPersistedResultsAndRetriesFailures 验证后台执行后的进度、部分失败、日志与筛选统计，并确认失败项重试创建关联新任务。
func TestTaskCenterReadsPersistedResultsAndRetriesFailures(t *testing.T) {
	b, service := newTaskBrowser(t, map[domain.Type]jobs.Module{
		domain.Scan: {
			CanRetryFailed: true,
			Run: func(ctx context.Context, run jobs.Execution) error {
				progress := 100.0
				if err := run.Report(ctx, domain.Progress{ProgressPercent: &progress}); err != nil {
					return err
				}
				if err := run.Log(ctx, "info", "scan complete"); err != nil {
					return err
				}
				if err := run.Item(ctx, domain.Item{Key: "first", Path: "/电影/one.mkv", Status: domain.ItemSucceeded}); err != nil {
					return err
				}
				return run.Item(ctx, domain.Item{Key: "second", Path: "/电影/two.mkv", Status: domain.ItemFailed, Reason: "文件读取失败"})
			},
		},
	})
	b.setup(t)
	libraryInput := `{"name":"电影","directory_id":"123","directory_path":"/电影","media_type":"movie","output_path":"/media","generate_strm":true,"strm_existing_file_policy":"skip","generate_nfo":true,"generate_poster_cover":true,"language_preference":"zh-CN","scrape_source":"tmdb","exclude_directory_path":"","include_subdirectories":true,"follow_directory_changes":true,"file_encoding":"UTF-8","link_type":"proxy","strm_direct_domain":"","strm_link_format":"original_name"}`
	target := taskJSON[generated.Library](t, b.request(t, http.MethodPost, "/api/v1/libraries", libraryInput, nil), http.StatusCreated)
	submission := `{"type":"scan","library_id":"` + target.Id + `"}`
	created := taskJSON[generated.Task](t, b.request(t, http.MethodPost, "/api/v1/tasks", submission, nil), http.StatusAccepted)
	if created.Status != generated.TaskStatusQueued || created.LibraryName != "电影" || created.ProgressPercent != nil || created.Actions == nil {
		t.Fatalf("initial task: %+v", created)
	}
	url := "/api/v1/tasks/" + created.Id
	assertAuthError(t, b.request(t, http.MethodPost, url+"/actions", `{"action":"retry_failed"}`, nil), http.StatusConflict, generated.TaskStateConflict)

	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := service.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	deadline := time.Now().Add(4 * time.Second)
	finished := generated.Task{}
	for {
		finished = taskJSON[generated.Task](t, b.request(t, http.MethodGet, url, "", nil), http.StatusOK)
		if finished.FinishedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not finish: %+v", finished)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if finished.Status != generated.TaskStatusCompleted || finished.Outcome == nil || *finished.Outcome != generated.PartialFailure ||
		finished.SucceededItems != 1 || finished.FailedItems != 1 || finished.ProgressPercent == nil || *finished.ProgressPercent != 100 {
		t.Fatalf("completed task: %+v", finished)
	}
	failures := taskJSON[generated.TaskFailurePage](t, b.request(t, http.MethodGet, url+"/failures", "", nil), http.StatusOK)
	if len(failures.Items) != 1 || failures.Items[0].Path != "/电影/two.mkv" || failures.Items[0].Reason != "文件读取失败" {
		t.Fatalf("failures: %+v", failures)
	}
	logs := taskJSON[generated.TaskLogPage](t, b.request(t, http.MethodGet, url+"/logs", "", nil), http.StatusOK)
	if len(logs.Items) == 0 || logs.Items[0].CreatedAt.IsZero() {
		t.Fatalf("logs: %+v", logs)
	}
	page := taskJSON[generated.TaskPage](t, b.request(t, http.MethodGet, "/api/v1/tasks?type=scan&status=completed&library_id="+target.Id, "", nil), http.StatusOK)
	if page.Total != 1 || len(page.Items) != 1 || page.ByStatus["completed"] != 1 || page.ByType["scan"] != 1 {
		t.Fatalf("filtered tasks: %+v", page)
	}
	retry := taskJSON[generated.Task](t, b.request(t, http.MethodPost, url+"/actions", `{"action":"retry_failed"}`, nil), http.StatusAccepted)
	if retry.ParentId == nil || *retry.ParentId != finished.Id || retry.Id == finished.Id || string(retry.Source) != "retry" {
		t.Fatalf("retry: %+v", retry)
	}
}

// TestTaskScheduleCRUDAndAuthenticatedEventStream 验证计划创建、启停和版本冲突，SSE 使用连接内序号并在会话注销后停止发送数据。
func TestTaskScheduleCRUDAndAuthenticatedEventStream(t *testing.T) {
	b, service := newTaskBrowser(t, map[domain.Type]jobs.Module{
		domain.Scan: {Run: func(context.Context, jobs.Execution) error { return nil }},
	})
	b.setup(t)

	server := httptest.NewServer(b.handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(b.cookies[sessionCookie])
	request.Header.Set("Last-Event-ID", "700")
	stream, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if stream.StatusCode != http.StatusOK || stream.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream: %d %v", stream.StatusCode, stream.Header)
	}
	reader := bufio.NewReader(stream.Body)
	if connected := readTaskFrame(t, reader); connected != ": connected\n\n" {
		t.Fatalf("initial event frame: %s", connected)
	}

	plan := taskJSON[generated.Schedule](t, b.request(t, http.MethodPost, "/api/v1/schedules", allLibrariesDailySchedule, nil), http.StatusCreated)
	if plan.Id == "" || plan.LibraryId != nil || plan.NextRunAt == nil || plan.Revision != 1 || len(plan.NotifyOn) != 1 {
		t.Fatalf("created plan: %+v", plan)
	}
	frame := readTaskFrame(t, reader)
	wantFrame := "id: 1\nevent: schedule.changed\ndata: {\"id\":\"" + plan.Id + "\"}\n\n"
	if frame != wantFrame {
		t.Fatalf("event frame: %s", frame)
	}
	planPath := "/api/v1/schedules/" + plan.Id
	fetched := taskJSON[generated.Schedule](t, b.request(t, http.MethodGet, planPath, "", nil), http.StatusOK)
	if fetched.Name != plan.Name || fetched.Revision != plan.Revision {
		t.Fatalf("stored plan: %+v", fetched)
	}
	assertAuthError(t, b.request(t, http.MethodPut, planPath, allLibrariesDailySchedule, nil), http.StatusBadRequest, generated.ValidationFailed)
	disabled := taskJSON[generated.Schedule](t, b.request(t, http.MethodPatch, planPath+"/enabled", `{"enabled":false}`, nil), http.StatusOK)
	if disabled.Enabled || disabled.NextRunAt != nil || disabled.Revision != 2 {
		t.Fatalf("disabled plan: %+v", disabled)
	}
	if next := readTaskFrame(t, reader); !strings.HasPrefix(next, "id: 2\n") {
		t.Fatalf("second event: %s", next)
	}
	assertAuthError(t, b.request(t, http.MethodPut, planPath, allLibrariesDailySchedule, map[string]string{"X-Schedule-Revision": "1"}), http.StatusConflict, generated.Conflict)
	page := taskJSON[generated.SchedulePage](t, b.request(t, http.MethodGet, "/api/v1/schedules?enabled=false&type=scan", "", nil), http.StatusOK)
	if len(page.Items) != 1 || page.Items[0].Id != plan.Id {
		t.Fatalf("filtered plans: %+v", page)
	}
	updatedInput := strings.Replace(allLibrariesDailySchedule, "每日扫描", "更新扫描计划", 1)
	updatedInput = strings.Replace(updatedInput, `"enabled":true`, `"enabled":false`, 1)
	updated := taskJSON[generated.Schedule](t, b.request(t, http.MethodPut, planPath, updatedInput, map[string]string{"X-Schedule-Revision": "2"}), http.StatusOK)
	if updated.Name != "更新扫描计划" || updated.Revision != 3 || updated.NextRunAt != nil {
		t.Fatalf("updated plan: %+v", updated)
	}
	if next := readTaskFrame(t, reader); !strings.HasPrefix(next, "id: 3\n") {
		t.Fatalf("third event: %s", next)
	}
	if response := b.request(t, http.MethodPost, "/api/v1/auth/logout", "", nil); response.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", response.Code, response.Body)
	}
	if _, err := service.SetScheduleEnabled(t.Context(), plan.Id, true, "new-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("revoked session streamed data or did not close: %v", err)
	}
	b.bootstrap(t)
	loggedIn := taskJSON[generated.AuthSession](t, b.request(t, http.MethodPost, "/api/v1/auth/login", loginJSON, nil), http.StatusOK)
	b.csrf = loggedIn.CsrfToken
	if response := b.request(t, http.MethodDelete, planPath, "", nil); response.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}
	assertAuthError(t, b.request(t, http.MethodGet, planPath, "", nil), http.StatusNotFound, generated.NotFound)
}

// readTaskFrame 读取至 SSE 空行分隔符并返回完整帧，流读取失败立即使测试失败。
func readTaskFrame(t *testing.T, reader *bufio.Reader) string {
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
