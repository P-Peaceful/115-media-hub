// preview_test.go：验证预览遵守保存的扫描范围、不写产物，并在超时和取消时保留正确的完整性语义。

package strm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// TestFullSyncPreviewScansSavedScopeEvenWhenOutputIsBlocked 验证输出受阻时仍按已保存范围扫描并给出样例和失败统计，预览不修改产物或登记。
func TestFullSyncPreviewScansSavedScopeEvenWhenOutputIsBlocked(t *testing.T) {
	var requests []string
	var logs bytes.Buffer
	provider := generationProvider{list: func(_ context.Context, query provider115.FileQuery) (provider115.FilePage, error) {
		requests = append(requests, query.DirectoryID+":"+query.Cursor)
		switch query.DirectoryID + ":" + query.Cursor {
		case "42:":
			page := rootPage(videoFile("11", "外部媒体名称.mkv"),
				provider115.FileItem{ID: "51", Name: "电影", IsDirectory: true},
				provider115.FileItem{ID: "52", Name: "排除", IsDirectory: true},
				provider115.FileItem{ID: "53", Name: "无法访问", IsDirectory: true})
			cursor := "next"
			page.NextCursor = &cursor
			return page, nil
		case "42:next":
			return rootPage(videoFile("11", "外部媒体名称.mkv"),
				provider115.FileItem{ID: "12", Name: "无播放码.mp4", FileType: "video"},
				provider115.FileItem{ID: "15", Name: "海报.png", FileType: "image"}), nil
		case "51:":
			page := rootPage(videoFile("13", "子目录.mp4"))
			page.DirectoryID = "51"
			page.Path = append(page.Path, provider115.FilePath{ID: "51", Name: "电影"})
			return page, nil
		case "53:":
			return provider115.FilePage{}, provider115.ErrDirectoryNotFound
		default:
			t.Fatalf("excluded or unexpected directory queried: %q", query.DirectoryID)
			return provider115.FilePage{}, nil
		}
	}}
	service, repo, execution, _ := newGenerationFixture(t, provider, &logs)
	target := execution.Library
	target.IncludeSubdirectories = true
	target.ExcludeDirectoryPath = "/媒体/排除"
	target.Revision = 3
	target.OutputPath = filepath.Join(target.OutputPath, "blocking-local-file")
	if err := os.WriteFile(target.OutputPath, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := service.PreviewFullSync(t.Context(), target, "preview-test-request")
	if err != nil {
		t.Fatal(err)
	}
	if result.ScanComplete || result.StopReason != "source_error" || result.Revision != 3 || result.Output.Status != "not_directory" ||
		result.ScannedVideos != 3 || result.CandidateCount != 2 || result.FailedCount != 2 {
		t.Fatalf("unexpected preview: %+v", result)
	}
	wantSamples := []PreviewSample{
		{SourcePath: "/媒体/外部媒体名称.mkv", RelativePath: "外部媒体名称.strm"},
		{SourcePath: "/媒体/电影/子目录.mp4", RelativePath: "电影/子目录.strm"},
	}
	if !slices.Equal(requests, []string{"42:", "42:next", "51:", "53:"}) || !slices.Equal(result.Samples, wantSamples) {
		t.Fatalf("unexpected scope: requests=%v samples=%+v", requests, result.Samples)
	}
	if len(repo.intents) != 0 || len(repo.artifacts) != 0 || requireFile(t, target.OutputPath) != "external" {
		t.Fatal("preview changed local or registered output")
	}
	for _, attribute := range []string{`"operation":"full_sync_preview"`, `"request_id":"preview-test-request"`, `"outcome":"failed"`, `"error_code":"full_sync_preview_source_error"`, `"candidate_count":2`} {
		if !strings.Contains(logs.String(), attribute) {
			t.Errorf("missing preview log attribute %s", attribute)
		}
	}
	for _, secret := range []string{target.OutputPath, "外部媒体名称.mkv", "private-playback-code", "private-account"} {
		if strings.Contains(logs.String(), secret) {
			t.Error("sensitive preview data appeared in the logs")
		}
	}
}

// TestFullSyncPreviewDeadlineKeepsValidatedPages 验证截止时间到达时保留已验证页面，并明确标记部分抽样。
func TestFullSyncPreviewDeadlineKeepsValidatedPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := generationProvider{list: func(ctx context.Context, query provider115.FileQuery) (provider115.FilePage, error) {
			switch query.DirectoryID + ":" + query.Cursor {
			case "42:":
				return rootPage(videoFile("11", "样片.mkv"), provider115.FileItem{ID: "51", Name: "电影", IsDirectory: true}), nil
			case "51:":
				page := rootPage(videoFile("12", "未完成扫描.mp4"))
				cursor := "next"
				page.NextCursor, page.DirectoryID = &cursor, "51"
				page.Path = append(page.Path, provider115.FilePath{ID: "51", Name: "电影"})
				return page, nil
			case "51:next":
				<-ctx.Done()
				return provider115.FilePage{}, provider115.ErrUpstream
			default:
				t.Fatalf("unexpected directory query: %+v", query)
				return provider115.FilePage{}, nil
			}
		}}
		service, _, execution, _ := newGenerationFixture(t, provider, io.Discard)
		execution.Library.IncludeSubdirectories = true
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		result, err := service.PreviewFullSync(ctx, execution.Library, "preview-test-timeout")
		if err != nil || result.ScanComplete || result.ScannedVideos != 2 || result.CandidateCount != 2 || result.FailedCount != 0 ||
			result.Output.Status != "ready" || !slices.Equal(result.Samples, []PreviewSample{{SourcePath: "/媒体/样片.mkv", RelativePath: "样片.strm"}, {SourcePath: "/媒体/电影/未完成扫描.mp4", RelativePath: "电影/未完成扫描.strm"}}) {
			t.Fatalf("unexpected bounded preview: %+v err=%v", result, err)
		}
	})
}

// TestFullSyncPreviewRespectsClientCancellation 验证客户端取消返回取消错误，不被当作可展示的超时部分结果。
func TestFullSyncPreviewRespectsClientCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := generationProvider{list: func(context.Context, provider115.FileQuery) (provider115.FilePage, error) {
		cancel()
		return provider115.FilePage{}, provider115.ErrUpstream
	}}
	service, _, execution, _ := newGenerationFixture(t, provider, io.Discard)
	if _, err := service.PreviewFullSync(ctx, execution.Library, "preview-test-cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

// TestFullSyncPreviewStopsAtIndependentBounds 验证大根、目录和样例上限先触及时停止，不发额外请求。
func TestFullSyncPreviewStopsAtIndependentBounds(t *testing.T) {
	for _, scenario := range []struct {
		name, reason         string
		directories, samples int
	}{
		{"request", "request_limit", 0, 0}, {"directory", "directory_limit", 4, 0}, {"sample", "sample_limit", 0, 25},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			provider := generationProvider{list: func(_ context.Context, q provider115.FileQuery) (provider115.FilePage, error) {
				calls++
				if q.Limit != 200 {
					t.Fatalf("preview page limit=%d", q.Limit)
				}
				page := rootPage()
				if q.DirectoryID != "42" {
					page.DirectoryID = q.DirectoryID
					page.Path = append(page.Path, provider115.FilePath{ID: q.DirectoryID, Name: "子" + q.DirectoryID})
				}
				if scenario.name == "request" {
					cursor := fmt.Sprint(calls)
					page.NextCursor = &cursor
				}
				if q.DirectoryID == "42" {
					for i := range scenario.directories {
						id := fmt.Sprint(50 + i)
						page.Items = append(page.Items, provider115.FileItem{ID: id, Name: "子" + id, IsDirectory: true})
					}
					for i := range scenario.samples {
						page.Items = append(page.Items, videoFile(fmt.Sprint(100+i), fmt.Sprintf("video%d.mkv", i)))
					}
				}
				return page, nil
			}}
			service, repo, execution, _ := newGenerationFixture(t, provider, io.Discard)
			execution.Library.IncludeSubdirectories = true
			result, err := service.PreviewFullSync(t.Context(), execution.Library, "")
			if err != nil || result.StopReason != scenario.reason || result.ScanComplete {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			wantCalls := map[string]int{"request": 5, "directory": 3, "sample": 1}[scenario.name]
			if calls != wantCalls || len(result.Samples) > 20 || len(repo.artifacts) != 0 || len(repo.intents) != 0 {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
		})
	}
}

// TestFullSyncPreviewRequiresVerifiedRoot 验证首个目录未验证时超时、路径错误和上游失败均不可成为空成功。
func TestFullSyncPreviewRequiresVerifiedRoot(t *testing.T) {
	for _, scenario := range []string{"timeout", "path", "upstream", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				provider := generationProvider{list: func(ctx context.Context, _ provider115.FileQuery) (provider115.FilePage, error) {
					switch scenario {
					case "timeout":
						<-ctx.Done()
						return provider115.FilePage{}, ctx.Err()
					case "budget":
						return provider115.FilePage{}, provider115.ErrRequestBudgetExhausted
					case "path":
						return provider115.FilePage{AccountID: "account"}, nil
					default:
						return provider115.FilePage{}, provider115.ErrUpstream
					}
				}}
				service, _, execution, _ := newGenerationFixture(t, provider, io.Discard)
				if result, err := service.PreviewFullSync(t.Context(), execution.Library, ""); err == nil {
					t.Fatalf("unverified root succeeded: %+v", result)
				}
			})
		})
	}
}

// TestFullSyncPreviewEmptyScopeAndAccountSwitch 验证空范围自然结束，后续页面账号变化仍拒绝部分成功。
func TestFullSyncPreviewEmptyScopeAndAccountSwitch(t *testing.T) {
	calls := 0
	provider := generationProvider{list: func(_ context.Context, q provider115.FileQuery) (provider115.FilePage, error) {
		calls++
		page := rootPage()
		if calls > 1 {
			page.AccountID = "changed"
		}
		if q.Cursor == "" && calls > 1 {
			cursor := "next"
			page.NextCursor = &cursor
		}
		return page, nil
	}}
	service, _, execution, _ := newGenerationFixture(t, provider, io.Discard)
	result, err := service.PreviewFullSync(t.Context(), execution.Library, "")
	if err != nil || !result.ScanComplete || result.StopReason != "scope_complete" || result.Mode != "sample" {
		t.Fatalf("empty=%+v err=%v", result, err)
	}
	calls = 1
	provider.list = func(_ context.Context, q provider115.FileQuery) (provider115.FilePage, error) {
		page := rootPage()
		if q.Cursor == "" {
			cursor := "next"
			page.NextCursor = &cursor
		} else {
			page.AccountID = "changed"
		}
		return page, nil
	}
	service.provider = provider
	if _, err = service.PreviewFullSync(t.Context(), execution.Library, ""); !errors.Is(err, errAccountChanged) {
		t.Fatalf("account switch err=%v", err)
	}
}

// TestFullSyncPreviewRequestBudgetIncludesRetries 验证实际发送预算由所有重试共享，页面调用不再重复扣减。
func TestFullSyncPreviewRequestBudgetIncludesRetries(t *testing.T) {
	attempts, calls := 0, 0
	provider := generationProvider{list: func(ctx context.Context, _ provider115.FileQuery) (provider115.FilePage, error) {
		calls++
		for range 3 {
			if !provider115.RequestBudgetFrom(ctx).Take() {
				return provider115.FilePage{}, provider115.ErrRequestBudgetExhausted
			}
			attempts++
		}
		page := rootPage(videoFile("11", "样片.mkv"))
		cursor := "next"
		page.NextCursor = &cursor
		return page, nil
	}}
	service, _, execution, _ := newGenerationFixture(t, provider, io.Discard)
	result, err := service.PreviewFullSync(t.Context(), execution.Library, "")
	if err != nil || attempts != 5 || calls != 2 || result.StopReason != "request_limit" || result.CandidateCount != 1 {
		t.Fatalf("attempts=%d calls=%d result=%+v err=%v", attempts, calls, result, err)
	}
}

// TestFullSyncPreviewLimitLogsSuccessfulSample 验证正常达到样例上限记录成功抽样，避免误报扫描故障。
func TestFullSyncPreviewLimitLogsSuccessfulSample(t *testing.T) {
	var logs bytes.Buffer
	provider := generationProvider{list: func(context.Context, provider115.FileQuery) (provider115.FilePage, error) {
		page := rootPage()
		for i := range 20 {
			page.Items = append(page.Items, videoFile(fmt.Sprint(100+i), fmt.Sprintf("sample%d.mkv", i)))
		}
		return page, nil
	}}
	service, _, execution, _ := newGenerationFixture(t, provider, &logs)
	if _, err := service.PreviewFullSync(t.Context(), execution.Library, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), `"outcome":"succeeded"`) || !strings.Contains(logs.String(), `"stop_reason":"sample_limit"`) || strings.Contains(logs.String(), `"error_code"`) {
		t.Fatalf("sample log=%s", logs.String())
	}
}

// TestFullSyncPreviewCancellationAfterRootRejectsPartialSuccess 验证已有有效根页面后用户取消仍返回取消，不放行部分成功。
func TestFullSyncPreviewCancellationAfterRootRejectsPartialSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := generationProvider{list: func(_ context.Context, q provider115.FileQuery) (provider115.FilePage, error) {
		if q.Cursor == "" {
			page := rootPage(videoFile("11", "样片.mkv"))
			cursor := "next"
			page.NextCursor = &cursor
			return page, nil
		}
		cancel()
		return provider115.FilePage{}, provider115.ErrUpstream
	}}
	service, _, execution, _ := newGenerationFixture(t, provider, io.Discard)
	if _, err := service.PreviewFullSync(ctx, execution.Library, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel returned %v", err)
	}
}
