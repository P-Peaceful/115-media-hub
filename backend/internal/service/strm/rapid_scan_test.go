// rapid_scan_test.go：比较快速与标准扫描的候选、排除和相对路径，并验证只有能力不可用时执行一次回退。

package strm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"testing"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// treeProvider 明确实现扫描和播放边界，避免 nil 嵌入接口掩盖新扫描能力。
type treeProvider struct {
	snapshot              provider115.TreeSnapshot
	rapidErr              error
	rapidCalls, listCalls int
}

// Files 提供相同树的逐目录视图，断言标准内部扫描不使用公共分页入口。
func (p *treeProvider) Files(context.Context, provider115.FileQuery, string) (provider115.FilePage, error) {
	return provider115.FilePage{}, errors.New("unexpected public scan")
}

// ScanFiles 提供真实内部大批次合同的模拟目录页。
func (p *treeProvider) ScanFiles(_ context.Context, q provider115.FileQuery, _ string) (provider115.FilePage, error) {
	p.listCalls++
	if q.Limit != 1150 {
		return provider115.FilePage{}, errors.New("unexpected internal limit")
	}
	page := provider115.FilePage{DirectoryID: q.DirectoryID, AccountID: "123"}
	for _, directory := range p.snapshot.Directories {
		if directory.ID == q.DirectoryID {
			page.Path = directory.Path
		}
		if directory.ParentID == q.DirectoryID {
			page.Items = append(page.Items, provider115.FileItem{ID: directory.ID, Name: directory.Path[len(directory.Path)-1].Name, IsDirectory: true})
		}
	}
	for _, file := range p.snapshot.Files {
		if file.DirectoryID == q.DirectoryID {
			page.Items = append(page.Items, file.Item)
		}
	}
	page.Total = int64(len(page.Items))
	return page, nil
}

// RapidScan 返回完整快照或指定错误，并记录是否发生重复快速尝试。
func (p *treeProvider) RapidScan(context.Context, string, string) (provider115.TreeSnapshot, error) {
	p.rapidCalls++
	return p.snapshot, p.rapidErr
}

// ResolvePlayback 此扫描测试不允许调用取链。
func (p *treeProvider) ResolvePlayback(context.Context, string, string, string, string, string) (string, error) {
	return "", errors.New("unexpected playback")
}

// TestRapidAndStandardEquivalent 验证多层目录和排除规则产出相同源身份及相对输出路径。
func TestRapidAndStandardEquivalent(t *testing.T) {
	root := []provider115.FilePath{{ID: "0", Name: "root"}, {ID: "10", Name: "Movies"}}
	season := append(append([]provider115.FilePath(nil), root...), provider115.FilePath{ID: "20", Name: "Season"})
	excluded := append(append([]provider115.FilePath(nil), root...), provider115.FilePath{ID: "30", Name: "Excluded"})
	snapshot := provider115.TreeSnapshot{AccountID: "123", Directories: []provider115.TreeDirectory{{ID: "10", Path: root}, {ID: "20", ParentID: "10", Path: season}, {ID: "30", ParentID: "10", Path: excluded}}, Files: []provider115.TreeFile{{DirectoryID: "10", Item: provider115.FileItem{ID: "100", Name: "a.mkv", FileType: "video", PlaybackCode: "a"}}, {DirectoryID: "20", Item: provider115.FileItem{ID: "101", Name: "b.mp4", FileType: "video", PlaybackCode: "b"}}, {DirectoryID: "30", Item: provider115.FileItem{ID: "102", Name: "c.mp4", FileType: "video", PlaybackCode: "c"}}}}
	target := library.Library{Config: library.Config{DirectoryID: "10", DirectoryPath: "root/Movies", IncludeSubdirectories: true, ExcludeDirectoryPath: "/Movies/Excluded"}}
	run := func(err error) ([]sourceFile, scanSummary, *treeProvider) {
		p := &treeProvider{snapshot: snapshot, rapidErr: err}
		s := &Service{provider: p, logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
		files, summary, e := s.scan(t.Context(), target, "req", nil, nil)
		if e != nil {
			t.Fatal(e)
		}
		sort.Slice(files, func(i, j int) bool { return files[i].key < files[j].key })
		return files, summary, p
	}
	rapid, rapidSummary, r := run(nil)
	standard, standardSummary, p := run(provider115.ErrFastUnavailable)
	if !reflect.DeepEqual(rapid, standard) || rapidSummary != standardSummary || len(rapid) != 2 || r.listCalls != 0 || r.rapidCalls != 1 || p.listCalls != 2 || p.rapidCalls != 1 {
		t.Fatalf("rapid=%+v standard=%+v summaries=%+v/%+v calls=%+v/%+v", rapid, standard, rapidSummary, standardSummary, r, p)
	}
	target.IncludeSubdirectories = false
	nonrecursive := &treeProvider{snapshot: snapshot}
	s := &Service{provider: nonrecursive, logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	files, _, err := s.scan(t.Context(), target, "req", nil, nil)
	if err != nil || len(files) != 1 || nonrecursive.rapidCalls != 0 || nonrecursive.listCalls != 1 {
		t.Fatalf("nonrecursive: %+v %v", files, err)
	}
}

// TestRapidErrorsNeverRescan 验证凭据变更、授权、限流和取消均不触发标准重扫。
func TestRapidErrorsNeverRescan(t *testing.T) {
	for _, err := range []error{provider115.ErrCredentialChanged, provider115.ErrInvalidAuthorization, context.Canceled, context.DeadlineExceeded, provider115.ErrUpstream} {
		p := &treeProvider{rapidErr: err}
		s := &Service{provider: p, logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
		_, _, got := s.scan(t.Context(), library.Library{Config: library.Config{DirectoryID: "10", IncludeSubdirectories: true}}, "req", nil, nil)
		if !errors.Is(got, err) || p.listCalls != 0 || p.rapidCalls != 1 {
			t.Fatalf("rescanned after %v: %+v %v", err, p, got)
		}
	}
}

// controlledScanner 为内部标准分页注入明确能力限制或分页变化，不嵌入未实现接口。
type controlledScanner struct {
	treeProvider
	list func(provider115.FileQuery) (provider115.FilePage, error)
}

// ScanFiles 转发受控的内部标准页，用于验证大页降级和完整目录提交。
func (p *controlledScanner) ScanFiles(_ context.Context, q provider115.FileQuery, _ string) (provider115.FilePage, error) {
	return p.list(q)
}

// TestStandardPageFallbackAndIntegrity 验证明确能力限制只回退一次，计数变化、重复或限流均不能采用不完整目录。
func TestStandardPageFallbackAndIntegrity(t *testing.T) {
	for _, scenario := range []string{"unsupported", "throttled", "count_changed", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			provider := &controlledScanner{}
			provider.list = func(q provider115.FileQuery) (provider115.FilePage, error) {
				calls++
				page := provider115.FilePage{AccountID: "123", DirectoryID: "10", Path: []provider115.FilePath{{ID: "0", Name: "root"}, {ID: "10", Name: "Movies"}}, Total: 1, Items: []provider115.FileItem{{ID: "100", Name: "video.mkv", FileType: "video", PlaybackCode: "code"}}}
				if scenario == "unsupported" {
					if calls == 1 {
						if q.Limit != 1150 {
							t.Fatal("no internal large batch")
						}
						return provider115.FilePage{}, provider115.ErrLargePageUnsupported
					}
					if calls != 2 || q.Limit != 200 || q.Cursor != "" {
						t.Fatal("fallback did not restart bounded smaller batch")
					}
					return page, nil
				}
				if scenario == "throttled" {
					return provider115.FilePage{}, provider115.ErrUpstream
				}
				page.Total = 2
				if calls == 1 {
					cursor := "second"
					page.NextCursor = &cursor
				} else if scenario == "count_changed" {
					page.Total = 3
					page.Items[0].ID = "101"
				}
				return page, nil
			}
			s := &Service{provider: provider}
			account := ""
			_, files, err := s.scanDirectory(t.Context(), library.Config{}, "req", sourceDirectory{id: "10"}, &account)
			if scenario == "unsupported" {
				if err != nil || len(files) != 1 || calls != 2 {
					t.Fatalf("fallback: %+v %v calls=%d", files, err, calls)
				}
			} else if err == nil || len(files) > 0 || (scenario == "throttled" && calls != 1) {
				t.Fatalf("incomplete directory adopted: %+v %v calls=%d", files, err, calls)
			}
		})
	}
}

// TestStandardPaginationBound 验证每页游标均不同也受目录页数上限约束，超限丢弃整个目录结果。
func TestStandardPaginationBound(t *testing.T) {
	calls := 0
	p := &controlledScanner{}
	p.list = func(q provider115.FileQuery) (provider115.FilePage, error) {
		calls++
		next := fmt.Sprint(calls)
		return provider115.FilePage{DirectoryID: "10", AccountID: "123", Total: 3, Path: []provider115.FilePath{{ID: "0", Name: "root"}, {ID: "10", Name: "Movies"}}, NextCursor: &next, Items: []provider115.FileItem{{ID: fmt.Sprint(100 + calls), Name: "video.mkv", FileType: "video", PlaybackCode: "code"}}}, nil
	}
	s := &Service{provider: p}
	account := ""
	dirs, files, err := s.scanDirectoryPages(t.Context(), library.Config{}, "req", sourceDirectory{id: "10"}, &account, 2)
	if !errors.Is(err, errSourcePagination) || calls != 2 || len(dirs) > 0 || len(files) > 0 {
		t.Fatalf("unbounded directory calls=%d dirs=%v files=%v err=%v", calls, dirs, files, err)
	}
}

// TestStandardCredentialChangeStopsSiblings 验证子目录最后凭据检查失败后立即终止，不能继续扫描相邻目录。
func TestStandardCredentialChangeStopsSiblings(t *testing.T) {
	calls := 0
	p := &controlledScanner{treeProvider: treeProvider{rapidErr: provider115.ErrFastUnavailable}}
	p.list = func(q provider115.FileQuery) (provider115.FilePage, error) {
		calls++
		if q.DirectoryID == "20" {
			return provider115.FilePage{}, provider115.ErrCredentialChanged
		}
		if q.DirectoryID != "10" {
			t.Fatal("scanned sibling after credential change")
		}
		return provider115.FilePage{DirectoryID: "10", AccountID: "123", Total: 2, Path: []provider115.FilePath{{ID: "0", Name: "root"}, {ID: "10", Name: "Movies"}}, Items: []provider115.FileItem{{ID: "20", Name: "A", IsDirectory: true}, {ID: "30", Name: "B", IsDirectory: true}}}, nil
	}
	s := &Service{provider: p, logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	files, _, err := s.scan(t.Context(), library.Library{Config: library.Config{DirectoryID: "10", DirectoryPath: "root/Movies", IncludeSubdirectories: true}}, "req", nil, nil)
	if !errors.Is(err, provider115.ErrCredentialChanged) || calls != 2 || len(files) > 0 {
		t.Fatalf("credential change calls=%d files=%+v err=%v", calls, files, err)
	}
}
