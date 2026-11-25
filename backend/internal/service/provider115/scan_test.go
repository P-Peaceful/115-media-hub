// scan_test.go：验证完整快速快照、目录链完整性、根身份变化及请求次数边界，不请求真实网盘。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// scanTransport 用内存响应模拟各协议端点，虚拟时间验证限速而不进行真实等待。
type scanTransport func(*http.Request) (*http.Response, error)

// RoundTrip 执行测试协议回调。
func (f scanTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestRapidSnapshot 验证完整多层快照为常数次请求，坏祖先链与根移动不采用任何半成品。
func TestRapidSnapshot(t *testing.T) {
	for _, scenario := range []string{"complete", "empty", "orphan", "cycle", "root_moved", "count_changed", "missing_identity", "metadata_paths_identity", "metadata_wrong_id", "metadata_wrong_path", "control_directory", "long_directory", "throttled", "category_auth", "directories_auth", "files_auth", "category_business_throttle", "directories_business_throttle", "files_business_throttle"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				anchors := 0
				service := New(&detailsRepository{credential: "UID=123_A1_secret"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
				service.client.Transport = scanTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					path := `[{"cid":"0","name":"root"},{"cid":"10","name":"Movies"}]`
					body := ""
					status := http.StatusOK
					switch r.URL.Path {
					case "/category/get":
						body = `{"file_id":"10","file_category":"0","pick_code":"dirLocator","paths":[{"file_id":"0","file_name":"root"},{"file_id":"10","file_name":"Movies"}]}`
						if scenario == "metadata_paths_identity" {
							body = strings.Replace(body, `"file_id":"10",`, "", 1)
						}
						if scenario == "metadata_wrong_id" {
							body = strings.Replace(body, `"file_id":"10"`, `"file_id":"999"`, 1)
						}
						if scenario == "metadata_wrong_path" {
							body = strings.Replace(body, `"file_name":"Movies"`, `"file_name":"Moved"`, 1)
						}
						if scenario == "missing_identity" {
							body = `{"pick_code":"dirLocator"}`
						}
					case "/app/chrome/downfolders":
						parent := "10"
						if scenario == "orphan" {
							parent = "99"
						}
						if scenario == "cycle" {
							parent = "20"
						}
						body = `{"state":true,"data":{"list":[{"fid":"20","pid":"` + parent + `","fn":"Season"}],"has_next_page":false}}`
						if scenario == "control_directory" {
							body = strings.Replace(body, `"fn":"Season"`, `"fn":"Bad\nName"`, 1)
						}
						if scenario == "long_directory" {
							body = strings.Replace(body, `"fn":"Season"`, `"fn":"`+strings.Repeat("a", 256)+`"`, 1)
						}
						if scenario == "throttled" {
							status = http.StatusTooManyRequests
						}
					case "/files":
						if r.URL.Query().Get("show_dir") == "0" {
							body = `{"state":true,"cid":"10","count":2,"path":` + path + `,"data":[{"fid":"100","cid":"10","n":"a.mkv","pc":"a"},{"fid":"101","cid":"20","n":"b.mp4","pc":"b"}]}`
							if scenario == "count_changed" {
								body = `{"state":true,"cid":"10","count":3,"path":` + path + `,"data":[]}`
							}
						} else {
							anchors++
							if anchors == 2 && scenario == "root_moved" {
								path = `[{"cid":"0","name":"root"},{"cid":"10","name":"Moved"}]`
							}
							body = `{"state":true,"cid":"10","count":1,"path":` + path + `,"data":[{"cid":"20","n":"Season"}]}`
						}
					default:
						t.Errorf("unexpected endpoint %s", r.URL.Path)
					}
					if scenario == "empty" {
						switch r.URL.Path {
						case "/files":
							body = `{"state":true,"cid":"10","count":0,"path":` + path + `,"data":[]}`
						case "/app/chrome/downfolders":
							body = `{"state":true,"data":{"list":[],"has_next_page":false}}`
						}
					}
					endpointName := map[string]string{"/category/get": "category", "/app/chrome/downfolders": "directories", "/files": "files"}[r.URL.Path]
					if (r.URL.Path != "/files" || r.URL.Query().Get("show_dir") == "0") && strings.HasPrefix(scenario, endpointName+"_") {
						if strings.HasSuffix(scenario, "_auth") {
							body = `{"state":false,"errno":99}`
						} else if strings.HasSuffix(scenario, "_business_throttle") {
							body = `{"state":false,"errno":590075}`
						}
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				snapshot, err := service.RapidScan(context.Background(), "10", "req")
				if scenario == "empty" {
					if err != nil || len(snapshot.Files) != 0 || len(snapshot.Directories) != 1 || calls != 5 {
						t.Fatalf("empty snapshot %+v calls=%d err=%v", snapshot, calls, err)
					}
				} else if scenario == "complete" || scenario == "metadata_paths_identity" {
					if err != nil || len(snapshot.Files) != 2 || len(snapshot.Directories) != 2 || calls != 5 || snapshot.AccountID != "123" {
						t.Fatalf("snapshot %+v calls=%d err=%v", snapshot, calls, err)
					}
				} else if scenario == "throttled" {
					if err == nil || errors.Is(err, ErrFastUnavailable) || calls != 4 {
						t.Fatalf("throttle fell back: calls=%d err=%v", calls, err)
					}
				} else if strings.HasSuffix(scenario, "_auth") {
					if !errors.Is(err, ErrInvalidAuthorization) || errors.Is(err, ErrFastUnavailable) {
						t.Fatalf("auth classified fallback: %v", err)
					}
				} else if strings.HasSuffix(scenario, "_business_throttle") {
					if !errors.Is(err, errPlaybackThrottled) || errors.Is(err, ErrFastUnavailable) {
						t.Fatalf("business throttle classified fallback: %v", err)
					}
				} else if !errors.Is(err, ErrFastUnavailable) || len(snapshot.Files) != 0 {
					t.Fatalf("untrusted snapshot accepted: %+v %v", snapshot, err)
				}
			})
		})
	}
}

// TestInternalPaginationBoundary 验证公开浏览保持 200 上限，只有明确大页收窄才报告受控回退。
func TestInternalPaginationBoundary(t *testing.T) {
	s := New(&detailsRepository{credential: "UID=123_A1_secret"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	calls := 0
	s.client.Transport = scanTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"state":true,"limit":200}`))}, nil
	})
	q := FileQuery{DirectoryID: "10", Limit: 1150, Sort: "name", Order: "asc", Type: "all"}
	if _, err := s.Files(t.Context(), q, "req"); !errors.Is(err, ErrInvalidQuery) || calls != 0 {
		t.Fatal("public large page accepted")
	}
	if _, err := s.ScanFiles(t.Context(), q, "req"); !errors.Is(err, ErrLargePageUnsupported) || calls != 1 {
		t.Fatalf("large page fallback %v calls=%d", err, calls)
	}
}

// TestCredentialChangeStopsRetry 验证暂时故障后替换凭据立即停止尝试，保持既有 503 而不是误报授权失效。
func TestCredentialChangeStopsRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &detailsRepository{credential: "UID=123_A1_old"}
		s := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
		calls := 0
		s.client.Transport = scanTransport(func(*http.Request) (*http.Response, error) {
			calls++
			repo.credential = "UID=123_A1_new"
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		_, err := s.Files(t.Context(), FileQuery{DirectoryID: "10", Limit: 200, Sort: "name", Order: "asc", Type: "all"}, "req")
		if calls != 1 || !errors.Is(err, ErrUpstream) || errors.Is(err, ErrInvalidAuthorization) {
			t.Fatalf("stale retry calls=%d err=%v", calls, err)
		}
	})
}

// TestDirectoryRetryBudget 验证抽样实际发送预算包含重试，第三次请求不发送。
func TestDirectoryRetryBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New(&detailsRepository{credential: "UID=123_A1_old"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
		calls := 0
		s.client.Transport = scanTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		budget := NewRequestBudget(2)
		ctx := WithRequestBudget(t.Context(), budget)
		_, err := s.Files(ctx, FileQuery{DirectoryID: "10", Limit: 200, Sort: "name", Order: "asc", Type: "all"}, "req")
		if calls != 2 || budget.Remaining() != 0 || !errors.Is(err, ErrRequestBudgetExhausted) {
			t.Fatalf("retry budget calls=%d left=%d err=%v", calls, budget.Remaining(), err)
		}
	})
}

// TestRapidPaginationCompleteness 验证 1151 文件与 5001 目录只需两页各自清单，分页变化和重复不能产生可信快照。
func TestRapidPaginationCompleteness(t *testing.T) {
	for _, scenario := range []string{"complete", "second_count_changed", "duplicate_file", "premature_file_end", "premature_directory_end", "duplicate_directory"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := New(&detailsRepository{credential: "UID=123_A1_secret"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
				filesRequests, dirsRequests, anchors := 0, 0, 0
				s.client.Transport = scanTransport(func(r *http.Request) (*http.Response, error) {
					var body any
					path := []map[string]string{{"cid": "0", "name": "root"}, {"cid": "10", "name": "Movies"}}
					switch r.URL.Path {
					case "/category/get":
						body = map[string]any{"file_id": "10", "file_category": 0, "pick_code": "locator", "folder_count": 5001, "paths": []map[string]string{{"file_id": "0", "file_name": "root"}, {"file_id": "10", "file_name": "Movies"}}}
					case "/app/chrome/downfolders":
						dirsRequests++
						page, _ := strconv.Atoi(r.URL.Query().Get("page"))
						start, end := 0, 5000
						if page == 2 {
							start, end = 5000, 5001
						}
						list := make([]map[string]string, 0, end-start)
						for i := start; i < end; i++ {
							id := strconv.Itoa(20000 + i)
							if scenario == "duplicate_directory" && page == 2 {
								id = "20000"
							}
							list = append(list, map[string]string{"fid": id, "pid": "10", "fn": fmt.Sprintf("dir%d", i)})
						}
						next := page == 1
						if scenario == "premature_directory_end" {
							next = false
						}
						body = map[string]any{"state": true, "data": map[string]any{"list": list, "has_next_page": next}}
					case "/files":
						if r.URL.Query().Get("show_dir") != "0" {
							anchors++
							body = map[string]any{"state": true, "cid": "10", "count": 5001, "path": path, "data": []map[string]string{{"cid": "20000", "n": "dir0"}}}
							break
						}
						filesRequests++
						offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
						end := offset + 1150
						if end > 1151 {
							end = 1151
						}
						entries := make([]map[string]string, 0, end-offset)
						for i := offset; i < end; i++ {
							id := strconv.Itoa(10000 + i)
							if scenario == "duplicate_file" && offset > 0 {
								id = "10000"
							}
							entries = append(entries, map[string]string{"fid": id, "cid": "25000", "n": fmt.Sprintf("video%d.mkv", i), "pc": "locator"})
						}
						total := 1151
						if scenario == "second_count_changed" && offset > 0 {
							total = 1152
						}
						if scenario == "premature_file_end" && offset > 0 {
							entries = nil
						}
						body = map[string]any{"state": true, "cid": "10", "count": total, "path": path, "data": entries}
					default:
						t.Fatal("unexpected endpoint")
					}
					data, err := json.Marshal(body)
					if err != nil {
						t.Fatal(err)
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}, nil
				})
				snapshot, err := s.RapidScan(t.Context(), "10", "req")
				if scenario == "complete" {
					if err != nil || len(snapshot.Files) != 1151 || len(snapshot.Directories) != 5002 || filesRequests != 2 || dirsRequests != 2 || anchors != 2 {
						t.Fatalf("files=%d dirs=%d requests=%d/%d/%d err=%v", len(snapshot.Files), len(snapshot.Directories), filesRequests, dirsRequests, anchors, err)
					}
				} else if !errors.Is(err, ErrFastUnavailable) || len(snapshot.Files) > 0 {
					t.Fatalf("incomplete accepted: files=%d err=%v", len(snapshot.Files), err)
				}
			})
		})
	}
}

// TestRapidDisabledAndRootZero 验证关闭快速能力或全网盘根无定位码时不请求快速接口。
func TestRapidDisabledAndRootZero(t *testing.T) {
	s := New(&detailsRepository{credential: "UID=123_A1_secret"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	for _, root := range []string{"0", "10"} {
		s.fastScan = root == "0"
		_, err := s.RapidScan(t.Context(), root, "req")
		if !errors.Is(err, ErrFastUnavailable) || s.adapterClient != nil {
			t.Fatal("unavailable rapid sent request")
		}
	}
}

// TestFastErrorPreservesCredentialCause 验证多层协议包装不能把凭据变化或仓储故障归为允许重扫。
func TestFastErrorPreservesCredentialCause(t *testing.T) {
	for _, cause := range []error{ErrCredentialChanged, errors.Join(ErrUpstream, errors.New("repository unavailable")), context.Canceled, ErrRequestBudgetExhausted} {
		wrapped := errors.Join(&adapter.Error{Kind: adapter.Protocol}, fmt.Errorf("credential check: %w", cause))
		result := fastRequestError(wrapped)
		if errors.Is(result, ErrFastUnavailable) || !errors.Is(result, cause) {
			t.Fatalf("cause lost: %v", result)
		}
	}
}

// finalReadCredentialRepository 精确模拟传输采用检查结束后、服务返回前的最后一次凭据切换。
type finalReadCredentialRepository struct {
	detailsRepository
	reads    int
	changeAt int
}

// ProviderCredential 第四次读取返回替换凭据，覆盖 Files 最外层 defer 的窄竞态窗口。
func (r *finalReadCredentialRepository) ProviderCredential(context.Context) (string, error) {
	r.reads++
	threshold := r.changeAt
	if threshold == 0 {
		threshold = 4
	}
	if r.reads >= threshold {
		return "UID=123_A1_replaced", nil
	}
	return "UID=123_A1_original", nil
}

// TestFilesFinalCredentialCheck 验证最后一次凭据检查仍返回扫描致命的变更身份，并丢弃完整响应。
func TestFilesFinalCredentialCheck(t *testing.T) {
	repo := &finalReadCredentialRepository{}
	s := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	s.client.Transport = scanTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"state":true,"cid":"10","count":0,"path":[{"cid":"0","name":"root"},{"cid":"10","name":"Movies"}],"data":[]}`))}, nil
	})
	page, err := s.Files(t.Context(), FileQuery{DirectoryID: "10", Limit: 200, Sort: "name", Order: "asc", Type: "all"}, "req")
	if repo.reads != 4 || !errors.Is(err, ErrCredentialChanged) || !errors.Is(err, ErrUpstream) || errors.Is(err, ErrInvalidAuthorization) || page.DirectoryID != "" {
		t.Fatalf("final check reads=%d page=%+v err=%v", repo.reads, page, err)
	}
}

// TestResolveTreePathBoundsCachedDepth 验证完整祖先深度含缓存前缀，逐层解析也不能越过 256 层上限。
func TestResolveTreePathBoundsCachedDepth(t *testing.T) {
	const root = "10"
	nodes := map[string]treeNode{root: {name: "Movies"}}
	cache := map[string][]FilePath{root: {{ID: "0", Name: "root"}, {ID: root, Name: "Movies"}}}
	parent := root
	for depth := 3; depth <= 257; depth++ {
		id := strconv.Itoa(1000 + depth)
		nodes[id] = treeNode{name: "nested", parent: parent}
		path, ok := resolveTreePath(id, root, nodes, cache)
		if depth <= 256 {
			if !ok || len(path) != depth {
				t.Fatalf("valid depth %d rejected: len=%d ok=%v", depth, len(path), ok)
			}
		} else if ok || path != nil || cache[id] != nil {
			t.Fatalf("cached ancestor bypassed depth: len=%d ok=%v", len(path), ok)
		}
		parent = id
	}
}

// TestFastTransportFailureDoesNotFallback 验证截断等临时传输故障保留分类，不被认作快速协议不支持。
func TestFastTransportFailureDoesNotFallback(t *testing.T) {
	upstream := &adapter.Error{Operation: adapter.FastFiles, Kind: adapter.Temporary, StatusCode: http.StatusOK}
	err := fastRequestError(upstream)
	if errors.Is(err, ErrFastUnavailable) || !errors.Is(err, upstream) || !errors.Is(err, ErrUpstream) {
		t.Fatalf("temporary failure changed scan path: %v", err)
	}
}
