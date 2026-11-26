// sidecar_test.go：验证同目录有效 STRM 前置、附件原字节发布、零网络跳过和持久化意图恢复边界。

package strm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jobdomain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	sidecar "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/sidecar"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// sidecarTestRepository 为服务边界测试实现真实阶段验证，保留原 STRM 内存仓储的独立登记。
type sidecarTestRepository struct {
	*generationRepository
	downloads   map[string]sidecar.Intent
	artifactErr error
}

// StrmArtifactByTarget 可注入仓储故障，验证资格检查不会把存储失败当成正常跳过。
func (r *sidecarTestRepository) StrmArtifactByTarget(ctx context.Context, target string) (Artifact, error) {
	if r.artifactErr != nil {
		return Artifact{}, r.artifactErr
	}
	return r.generationRepository.StrmArtifactByTarget(ctx, target)
}

// ReserveSidecarIntent 模拟唯一目标保留，必须提交合法 reserved 快照。
func (r *sidecarTestRepository) ReserveSidecarIntent(_ context.Context, intent sidecar.Intent) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	if _, ok := r.downloads[intent.TargetPath]; ok {
		return sidecar.ErrConflict
	}
	r.downloads[intent.TargetPath] = intent
	return nil
}

// AdvanceSidecarIntent 使用领域相邻阶段与版本检查，防止服务跳过已完成事实。
func (r *sidecarTestRepository) AdvanceSidecarIntent(_ context.Context, before, after sidecar.Intent) error {
	current, ok := r.downloads[before.TargetPath]
	if !ok || current.Revision != before.Revision {
		return sidecar.ErrConflict
	}
	if err := before.CanAdvance(after); err != nil {
		return err
	}
	r.downloads[before.TargetPath] = after
	return nil
}

// SidecarIntentsByLibrary 返回指定库未收尾的附件意图。
func (r *sidecarTestRepository) SidecarIntentsByLibrary(_ context.Context, id string) ([]sidecar.Intent, error) {
	var result []sidecar.Intent
	for _, intent := range r.downloads {
		if intent.LibraryID == id {
			result = append(result, intent)
		}
	}
	return result, nil
}

// DiscardSidecarIntent 按版本释放本条记录，不移除其他执行已推进的意图。
func (r *sidecarTestRepository) DiscardSidecarIntent(_ context.Context, intent sidecar.Intent) error {
	current, ok := r.downloads[intent.TargetPath]
	if !ok || current.Revision != intent.Revision {
		return sidecar.ErrConflict
	}
	delete(r.downloads, intent.TargetPath)
	return nil
}

// sidecarTestProvider 在下载边界统计调用并允许注入发布竞态；不模拟网盘传输重试。
type sidecarTestProvider struct {
	generationProvider
	calls         int
	before        func() error
	credentialErr error
	downloadErr   error
}

// DownloadSidecar 写入固定原始字节并返回可注入的凭据快照检查。
func (p *sidecarTestProvider) DownloadSidecar(ctx context.Context, _ provider115.DownloadRequest, sink provider115.DownloadSink) (provider115.DownloadReceipt, error) {
	p.calls++
	if p.before != nil {
		if err := p.before(); err != nil {
			return provider115.DownloadReceipt{}, err
		}
	}
	if p.downloadErr != nil {
		return provider115.DownloadReceipt{}, p.downloadErr
	}
	if err := ctx.Err(); err != nil {
		return provider115.DownloadReceipt{}, err
	}
	n, err := io.WriteString(sink, "raw\x00bytes\xff")
	return provider115.DownloadReceipt{Bytes: int64(n), CredentialCheck: func(context.Context) error { return p.credentialErr }}, err
}

// newSidecarFixture 复用真实生成流程的执行回调，替换为具备附件边界的可观察替身。
func newSidecarFixture(t *testing.T, items ...provider115.FileItem) (*Service, *sidecarTestRepository, *sidecarTestProvider, jobs.Execution, *generationResult) {
	t.Helper()
	provider := generationProvider{list: func(context.Context, provider115.FileQuery) (provider115.FilePage, error) {
		return rootPage(items...), nil
	}}
	_, base, execution, result := newGenerationFixture(t, provider, io.Discard)
	repo := &sidecarTestRepository{generationRepository: base, downloads: make(map[string]sidecar.Intent)}
	source := &sidecarTestProvider{generationProvider: provider}
	execution.Library.GenerateNFO = true
	execution.Library.GeneratePosterCover = true
	service := New(repo, source, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return service, repo, source, execution, result
}

// attachmentFile 构造保留定位码的附件；文件类型不决定 NFO 的后缀筛选。
func attachmentFile(id, name string) provider115.FileItem {
	return provider115.FileItem{ID: id, Name: name, PlaybackCode: "attachment-code", FileType: "file"}
}

// TestSidecarRequiresSameDirectoryVerifiedSTRM 验证无视频、外部或失效 STRM、生成失败均为零附件请求。
func TestSidecarRequiresSameDirectoryVerifiedSTRM(t *testing.T) {
	for _, mode := range []string{"no_video", "external", "modified", "all_failed", "missing_after_generation", "registry_error"} {
		t.Run(mode, func(t *testing.T) {
			items := []provider115.FileItem{attachmentFile("90", "random.NFO")}
			if mode != "no_video" {
				items = append(items, videoFile("12", "movie.mkv"))
			}
			service, repo, provider, execution, result := newSidecarFixture(t, items...)
			strmPath := filepath.Join(execution.Library.OutputPath, "movie.strm")
			switch mode {
			case "external":
				if err := os.WriteFile(strmPath, []byte("external"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "modified":
				requireGeneration(t, service, execution)
				provider.calls = 0
				result.items = nil
				if err := os.Remove(filepath.Join(execution.Library.OutputPath, "random.NFO")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(strmPath, []byte("changed"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "all_failed":
				repo.completeErr = errors.New("storage interruption")
			case "missing_after_generation", "registry_error":
				original := execution.Item
				execution.Item = func(ctx context.Context, item jobdomain.Item) error {
					if item.Key == "file:12" {
						if mode == "registry_error" {
							repo.artifactErr = errors.New("storage unavailable")
						} else if err := os.Remove(strmPath); err != nil {
							return err
						}
					}
					return original(ctx, item)
				}
			}
			_ = service.Module().Run(t.Context(), execution)
			if provider.calls != 0 {
				t.Fatalf("unqualified directory issued %d requests", provider.calls)
			}
			if len(result.items) == 0 {
				t.Fatal("missing actual item outcomes")
			}
			item := result.items[len(result.items)-1]
			if mode == "registry_error" {
				if item.Status != jobdomain.ItemFailed {
					t.Fatalf("storage failure hidden as %s", item.Status)
				}
			} else if item.Status != jobdomain.ItemSkipped {
				t.Fatalf("unqualified attachment %#v", item)
			}
		})
	}
}

// TestSidecarOriginalBytesAndExistingFileSkip 验证全部所选后缀保留原名原字节，重复执行和超限已有文件均零下载。
func TestSidecarOriginalBytesAndExistingFileSkip(t *testing.T) {
	items := []provider115.FileItem{videoFile("12", "movie.mkv")}
	names := []string{"custom.NFO", "unrelated.JpG", "a.JPEG", "b.PNG", "c.GIF", "d.WEBP", "e.BMP"}
	for index, name := range names {
		items = append(items, attachmentFile(string(rune('1'+index))+"0", name))
	}
	service, repo, provider, execution, result := newSidecarFixture(t, items...)
	requireGeneration(t, service, execution)
	if provider.calls != len(names) || len(repo.artifacts) != 1 || len(repo.downloads) != 0 {
		t.Fatalf("calls=%d artifacts=%d intents=%d", provider.calls, len(repo.artifacts), len(repo.downloads))
	}
	for _, name := range names {
		if requireFile(t, filepath.Join(execution.Library.OutputPath, name)) != "raw\x00bytes\xff" {
			t.Fatal("content transformed")
		}
	}
	if len(result.items) != 8 || *result.progress.TotalItems != 8 || !strings.Contains(result.logs[len(result.logs)-1], "NFO：成功 1") {
		t.Fatalf("actual classification missing %#v", result)
	}
	huge := "999999999"
	items[1].SizeBytes = &huge
	items[1].PlaybackCode = ""
	provider.calls = 0
	requireGeneration(t, service, execution)
	if provider.calls != 0 || result.items[len(result.items)-1].Status != jobdomain.ItemSkipped {
		t.Fatal("existing ordinary attachment caused network request")
	}
	// 视频 STRM 本轮按 skip 策略复用时，仍应被当前登记和本地内容复核为有效资格。
	if err := os.Remove(filepath.Join(execution.Library.OutputPath, names[0])); err != nil {
		t.Fatal(err)
	}
	items[1].SizeBytes = nil
	items[1].PlaybackCode = "attachment-code"
	provider.calls = 0
	requireGeneration(t, service, execution)
	if provider.calls != 1 || requireFile(t, filepath.Join(execution.Library.OutputPath, names[0])) != "raw\x00bytes\xff" {
		t.Fatalf("reused STRM did not qualify sidecar: calls=%d", provider.calls)
	}
}

// TestSidecarPublicationRechecksEligibilityAndNeverOverwrites 验证下载后资格、凭据、路径和最终文件竞争均不暴露半文件。
func TestSidecarPublicationRechecksEligibilityAndNeverOverwrites(t *testing.T) {
	for _, mode := range []string{"strm_removed", "credential_changed", "race_regular", "race_symlink", "root_replaced", "download_failed"} {
		t.Run(mode, func(t *testing.T) {
			service, repo, provider, execution, result := newSidecarFixture(t, videoFile("12", "movie.mkv"), attachmentFile("90", "cover.png"))
			target := filepath.Join(execution.Library.OutputPath, "cover.png")
			provider.before = func() error {
				switch mode {
				case "strm_removed":
					return os.Remove(filepath.Join(execution.Library.OutputPath, "movie.strm"))
				case "race_regular":
					return os.WriteFile(target, []byte("user file"), 0o644)
				case "race_symlink":
					return os.Symlink("movie.strm", target)
				case "root_replaced":
					if err := os.Rename(execution.Library.OutputPath, execution.Library.OutputPath+"-old"); err != nil {
						return err
					}
					return os.Mkdir(execution.Library.OutputPath, 0o755)
				}
				return nil
			}
			if mode == "credential_changed" {
				provider.credentialErr = provider115.ErrCredentialChanged
			}
			if mode == "download_failed" {
				provider.downloadErr = provider115.ErrDownloadLength
			}
			_ = service.Module().Run(t.Context(), execution)
			if provider.calls != 1 {
				t.Fatalf("download calls=%d", provider.calls)
			}
			if mode == "race_regular" {
				if requireFile(t, target) != "user file" {
					t.Fatal("existing file overwritten")
				}
			} else if mode != "race_symlink" {
				if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unsafe final output: %v", err)
				}
			}
			last := result.items[len(result.items)-1]
			if last.Status == jobdomain.ItemSucceeded {
				t.Fatalf("unsafe success %#v", last)
			}
			if mode != "root_replaced" && len(repo.downloads) != 0 {
				t.Fatalf("known failed temp not cleaned after item %#v", repo.downloads)
			}
		})
	}
}

// TestSidecarRecoveryPreservesFormalAndUncertainFiles 验证每个持久化阶段的恢复仅删除身份可证的暂存。
func TestSidecarRecoveryPreservesFormalAndUncertainFiles(t *testing.T) {
	for _, stage := range []sidecar.State{sidecar.StateReserved, sidecar.StateStaging, sidecar.StateStaged, sidecar.StatePublished} {
		for _, finalExists := range []bool{false, true} {
			t.Run(string(stage)+"/"+map[bool]string{false: "missing", true: "existing"}[finalExists], func(t *testing.T) {
				service, repo, provider, execution, _ := newSidecarFixture(t)
				now := time.Now().UTC()
				intent := sidecar.Intent{ID: "intent", JobID: execution.Job.ID, Attempt: 1, LibraryID: execution.Library.ID, AccountID: "account", SourceFileID: "90", SourceDirectoryID: "42", Kind: sidecar.KindNFO, OutputRoot: execution.Library.OutputPath, RelativePath: "movie.nfo", TargetPath: filepath.Join(execution.Library.OutputPath, "movie.nfo"), TemporaryName: ".sidecar-owned.tmp", State: stage, Revision: 1, CreatedAt: now, UpdatedAt: now}
				temp := filepath.Join(intent.OutputRoot, intent.TemporaryName)
				if err := os.WriteFile(temp, []byte("partial"), 0o644); err != nil {
					t.Fatal(err)
				}
				if stage != sidecar.StateReserved {
					info, err := os.Stat(temp)
					if err != nil {
						t.Fatal(err)
					}
					intent.TemporaryDevice, intent.TemporaryInode, err = sidecarIdentity(info)
					if err != nil {
						t.Fatal(err)
					}
				}
				repo.downloads[intent.TargetPath] = intent
				if finalExists {
					if err := os.WriteFile(intent.TargetPath, []byte("existing user data"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				verified, err := service.Module().Recover(t.Context(), execution)
				if err != nil {
					t.Fatal(err)
				}
				if stage == sidecar.StateReserved {
					if verified || len(repo.downloads) != 1 || requireFile(t, temp) != "partial" {
						t.Fatal("uncertain creation gap was deleted")
					}
				} else {
					if !verified || len(repo.downloads) != 0 {
						t.Fatal("owned intent not released")
					}
					if _, err := os.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("owned temp remains")
					}
				}
				if finalExists && requireFile(t, intent.TargetPath) != "existing user data" {
					t.Fatal("recovery modified formal file")
				}
				if provider.calls != 0 {
					t.Fatal("recovery restarted download before STRM qualification")
				}
			})
		}
	}
}
