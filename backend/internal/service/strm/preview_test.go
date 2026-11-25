// preview_test.go：验证预览遵守保存的扫描范围、不写产物，并在超时和取消时保留正确的完整性语义。

package strm

import (
	"bytes"
	"context"
	"errors"
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
	if !result.ScanComplete || result.Revision != 3 || result.Output.Status != "not_directory" ||
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
	for _, attribute := range []string{`"operation":"full_sync_preview"`, `"request_id":"preview-test-request"`, `"outcome":"failed"`, `"error_code":"strm_output_not_directory"`, `"candidate_count":2`} {
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

// TestFullSyncPreviewDeadlineKeepsOnlyCompleteDirectories 验证截止时间到达时只保留分页已结束的目录，不把中断目录计入候选。
func TestFullSyncPreviewDeadlineKeepsOnlyCompleteDirectories(t *testing.T) {
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
		if err != nil || result.ScanComplete || result.ScannedVideos != 1 || result.CandidateCount != 1 || result.FailedCount != 0 ||
			result.Output.Status != "ready" || !slices.Equal(result.Samples, []PreviewSample{{SourcePath: "/媒体/样片.mkv", RelativePath: "样片.strm"}}) {
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
