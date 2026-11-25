// strm_test.go：以临时 SQLite、输出目录和 HTTP 服务验证 STRM 预览、生成登记与 302 播放，覆盖凭据及日志安全边界。

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/strm"
)

// lockedBuffer 为后台任务与 HTTP 并发日志提供可安全读取的测试缓冲区。
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

// Write 在互斥保护下捕获并发任务与请求日志，供集成测试统一断言。
func (b *lockedBuffer) Write(content []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(content)
}

// String 在互斥保护下读取日志快照，避免与后台写入竞争。
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// testStrmProvider 用固定清单和取链条件替代真实网盘，保持测试可重复。
type testStrmProvider struct{ targetURL string }

// Files 仅为测试目录返回固定视频清单，其他目录模拟源不存在。
func (testStrmProvider) Files(_ context.Context, query provider115.FileQuery, _ string) (provider115.FilePage, error) {
	if query.DirectoryID != "12" {
		return provider115.FilePage{}, provider115.ErrDirectoryNotFound
	}
	return provider115.FilePage{
		AccountID: "11501", DirectoryID: "12", Total: 1,
		Path:  []provider115.FilePath{{ID: "0", Name: "网盘"}, {ID: "12", Name: "电影"}},
		Items: []provider115.FileItem{{ID: "34", Name: "示例影片.mp4", FileType: "video", PlaybackCode: "provider-private-code"}},
	}, nil
}

// ResolvePlayback 校验账号、源文件、取链码及播放器标识，匹配时返回测试媒体地址。
func (p testStrmProvider) ResolvePlayback(_ context.Context, accountID, fileID, code, userAgent, _ string) (string, error) {
	if accountID != "11501" || fileID != "34" || code != "provider-private-code" || userAgent != "TestPlayer" {
		return "", provider115.ErrUpstream
	}
	return p.targetURL, nil
}

// TestSTRMGenerationRegistryAndPlaybackBoundary 验证只读预览、任务生成和重复跳过、登记详情、GET/HEAD 重定向及修改文件和敏感日志保护。
func TestSTRMGenerationRegistryAndPlaybackBoundary(t *testing.T) {
	browser, administrator, db := newAuthBrowser(t, false, nil)
	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	libraries := library.New(db, logger)
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("signature") != "temporary-signature" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("sample video"))
	}))
	defer media.Close()
	targetURL := media.URL + "/video?signature=temporary-signature"
	app := httptest.NewUnstartedServer(nil)
	strmFiles := strm.New(db, testStrmProvider{targetURL: targetURL}, logger)
	tasks := jobs.New(db, libraries, map[domain.Type]jobs.Module{domain.STRM: strmFiles.Module()}, logger)
	router, err := NewRouter(db, logger, nil, AuthOptions{Service: administrator, Library: libraries, Jobs: tasks, STRM: strmFiles})
	if err != nil {
		t.Fatal(err)
	}
	browser.handler = router
	app.Config.Handler = router
	app.Start()
	defer app.Close()
	assertAuthError(t, browser.request(t, http.MethodGet, "/api/v1/strm", "", nil), http.StatusUnauthorized, generated.AuthenticationRequired)
	assertAuthError(t, browser.request(t, http.MethodGet, "/api/v1/libraries/example/full-sync-preview", "", nil), http.StatusUnauthorized, generated.AuthenticationRequired)

	browser.setup(t)
	assertAuthError(t, browser.request(t, http.MethodGet, "/api/v1/libraries/unknown/full-sync-preview", "", nil), http.StatusNotFound, generated.NotFound)
	root := filepath.Join(t.TempDir(), "new-output")
	config := generated.LibraryConfig{
		Name: "电影", DirectoryId: "12", DirectoryPath: "/电影", MediaType: "movie",
		OutputPath: root, GenerateStrm: true, StrmExistingFilePolicy: "skip",
		LanguagePreference: "zh-CN", ScrapeSource: "none", IncludeSubdirectories: true,
		FileEncoding: "UTF-8", LinkType: "proxy", StrmLinkFormat: "original_name",
	}
	createLibrary := func(config generated.LibraryConfig) generated.Library {
		t.Helper()
		input, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		return taskJSON[generated.Library](t, browser.request(t, http.MethodPost, "/api/v1/libraries", string(input), nil), http.StatusCreated)
	}
	unconfigured := createLibrary(config)
	request := `{"type":"strm","library_id":"` + unconfigured.Id + `"}`
	assertAuthError(t, browser.request(t, http.MethodPost, "/api/v1/tasks", request, nil), http.StatusConflict, generated.Conflict)
	assertAuthError(t, browser.request(t, http.MethodGet, "/api/v1/libraries/"+unconfigured.Id+"/full-sync-preview", "", nil), http.StatusConflict, generated.FullSyncUnavailable)
	config.StrmDirectDomain = app.URL + "/"
	config.Name = "可同步电影"
	target := createLibrary(config)
	previewURL := "/api/v1/libraries/" + target.Id + "/full-sync-preview"
	preview := taskJSON[generated.FullSyncPreview](t, browser.request(t, http.MethodGet, previewURL, "", nil), http.StatusOK)
	if preview.Mode != generated.Sample || preview.StopReason != generated.ScopeComplete || !preview.ScanComplete || preview.Revision != target.Revision || preview.Output.Status != generated.FullSyncOutputCheckStatusWillCreate ||
		preview.ScannedVideos != 1 || preview.CandidateCount != 1 || preview.FailedCount != 0 ||
		!slices.Equal(preview.Samples, []generated.FullSyncPreviewSample{{SourcePath: "/电影/示例影片.mp4", RelativePath: "示例影片.strm"}}) {
		t.Fatalf("unexpected full-sync preview: %+v", preview)
	}
	if _, err := os.Stat(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read-only preview touched the output directory: %v", err)
	}
	initialHistory := taskJSON[generated.TaskPage](t, browser.request(t, http.MethodGet, "/api/v1/tasks?type=strm&library_id="+target.Id, "", nil), http.StatusOK)
	initialArtifacts := taskJSON[generated.StrmPage](t, browser.request(t, http.MethodGet, "/api/v1/strm?library_id="+target.Id, "", nil), http.StatusOK)
	if initialHistory.Total != 0 || initialArtifacts.Total != 0 {
		t.Fatalf("read-only preview persisted a task or artifact: history=%d artifacts=%d", initialHistory.Total, initialArtifacts.Total)
	}
	missingConfig := config
	missingConfig.Name, missingConfig.DirectoryId = "已移动的电影", "99"
	missingSource := createLibrary(missingConfig)
	assertAuthError(t, browser.request(t, http.MethodGet, "/api/v1/libraries/"+missingSource.Id+"/full-sync-preview", "", nil), http.StatusNotFound, generated.ProviderDirectoryNotFound)
	types := taskJSON[generated.TaskTypeCapabilities](t, browser.request(t, http.MethodGet, "/api/v1/task-types", "", nil), http.StatusOK)
	fullSync := slices.IndexFunc(types.Items, func(capability generated.TaskTypeCapability) bool {
		return capability.Type == generated.Strm
	})
	if fullSync == -1 || !types.Items[fullSync].Available || types.Items[fullSync].CanSchedule {
		t.Fatalf("expected an available, on-demand STRM full sync capability: %+v", types.Items)
	}
	assertAuthError(t, browser.request(t, http.MethodPost, "/api/v1/schedules",
		strings.Replace(allLibrariesDailySchedule, `"scan"`, `"strm"`, 1), nil), http.StatusConflict, generated.TaskTypeUnavailable)
	if err := tasks.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := tasks.Stop(ctx); err != nil {
			t.Error(err)
		}
	})

	submit := func() generated.Task {
		t.Helper()
		request := `{"type":"strm","library_id":"` + target.Id + `"}`
		task := taskJSON[generated.Task](t, browser.request(t, http.MethodPost, "/api/v1/tasks", request, nil), http.StatusAccepted)
		deadline := time.Now().Add(5 * time.Second)
		for {
			task = taskJSON[generated.Task](t, browser.request(t, http.MethodGet, "/api/v1/tasks/"+task.Id, "", nil), http.StatusOK)
			if task.FinishedAt != nil {
				return task
			}
			if time.Now().After(deadline) {
				t.Fatal("STRM task did not finish")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	first := submit()
	if first.Status != generated.TaskStatusCompleted || first.SucceededItems != 1 {
		t.Fatalf("generation failed: %+v; %s", first, logs.String())
	}
	preview = taskJSON[generated.FullSyncPreview](t, browser.request(t, http.MethodGet, previewURL, "", nil), http.StatusOK)
	if preview.Output.Status != generated.FullSyncOutputCheckStatusReady || preview.CandidateCount != 1 {
		t.Fatalf("unexpected preview after generation: %+v", preview)
	}
	second := submit()
	if second.Status != generated.TaskStatusCompleted || second.SkippedItems != 1 {
		t.Fatalf("rerun failed: %+v", second)
	}
	history := taskJSON[generated.TaskPage](t, browser.request(t, http.MethodGet, "/api/v1/tasks?type=strm&library_id="+target.Id, "", nil), http.StatusOK)
	if history.Total != 2 || len(history.Items) != 2 || history.Items[0].Id != second.Id || history.Items[1].Id != first.Id {
		t.Fatalf("unexpected library full-sync history: %+v", history)
	}
	list := taskJSON[generated.StrmPage](t, browser.request(t, http.MethodGet, "/api/v1/strm?library_id="+target.Id+"&limit=1", "", nil), http.StatusOK)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].LibraryName == nil ||
		*list.Items[0].LibraryName != target.Name || list.Items[0].SourceFileId != "34" {
		t.Fatalf("unexpected registry list: %+v", list)
	}
	assertAuthError(t, browser.request(t, http.MethodGet, "/api/v1/strm?sort=invalid", "", nil), http.StatusBadRequest, generated.ValidationFailed)
	file := list.Items[0]
	detailURL := "/api/v1/strm/" + file.Id
	detail := taskJSON[generated.StrmDetail](t, browser.request(t, http.MethodGet, detailURL, "", nil), http.StatusOK)
	if detail.FileState != "present" || detail.Content == nil || detail.OutputPath != filepath.Join(root, "示例影片.strm") {
		t.Fatalf("unexpected STRM detail state: %s", detail.FileState)
	}
	playback := strings.TrimSpace(*detail.Content)
	if !strings.HasPrefix(playback, app.URL+"/p/") {
		t.Fatal("STRM did not use the media library playback address")
	}
	parts := strings.Split(playback, "/")
	if parts[len(parts)-1] != url.PathEscape("示例影片.mp4") {
		t.Fatal("original-name playback URL must preserve the source filename and extension")
	}
	token := parts[len(parts)-2]
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, playback, nil)
		req.Header.Set("User-Agent", "TestPlayer")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != targetURL ||
			recorder.Header().Get("Cache-Control") != "no-store" || recorder.Body.Len() != 0 {
			t.Fatalf("expected an uncached 302 with no body, got %d", recorder.Code)
		}
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, playback, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "TestPlayer")
	response, err := app.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	mediaBytes, readErr := io.ReadAll(response.Body)
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if readErr != nil || response.StatusCode != http.StatusOK || string(mediaBytes) != "sample video" {
		t.Fatalf("redirect did not deliver media: %d, %v", response.StatusCode, readErr)
	}
	bad := httptest.NewRecorder()
	router.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, strings.Replace(playback, token, "bad-token", 1), nil))
	if bad.Code != http.StatusNotFound {
		t.Fatalf("invalid token status: %d", bad.Code)
	}
	malformed := httptest.NewRecorder()
	router.ServeHTTP(malformed, httptest.NewRequest(http.MethodGet, playback+"/", nil))
	if malformed.Code != http.StatusNotFound || malformed.Header().Get("Location") != "" {
		t.Fatalf("malformed playback path should return 404 without redirect, got %d", malformed.Code)
	}
	if err := os.WriteFile(detail.OutputPath, []byte("manually-modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	modified := taskJSON[generated.StrmDetail](t, browser.request(t, http.MethodGet, detailURL, "", nil), http.StatusOK)
	if modified.FileState != "modified" || modified.Content != nil {
		t.Fatalf("modified output not flagged: %s", modified.FileState)
	}
	for _, secret := range []string{token, "bad-token", "provider-private-code", "temporary-signature", "manually-modified"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("logs exposed sensitive playback data")
		}
	}
	if !strings.Contains(logs.String(), `"operation":"resolve_playback"`) {
		t.Fatal("playback resolution was not logged")
	}
}
