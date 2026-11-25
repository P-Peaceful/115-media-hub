// preview_scan.go：对保存的源目录做只读广度抽样，限制目录、实际请求、时间与样例，页面校验成功后才采用其结果。

package strm

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

const previewDirectoryLimit = 3
const previewRequestLimit = 5

// previewScan 只调用标准分页；局部已校验页面可展示，根未验证及授权/账号变化必须报错。
func (s *Service) previewScan(ctx context.Context, target library.Library, requestID string, result *FullSyncPreview) error {
	budget := provider115.NewRequestBudget(previewRequestLimit)
	ctx = provider115.WithRequestBudget(ctx, budget)
	directories := []sourceDirectory{{id: target.DirectoryID, display: target.DirectoryPath}}
	seenDirectories := map[string]bool{target.DirectoryID: true}
	seenFiles := map[string]bool{}
	accountID := ""
	rootValidated, failedDirectory := false, false
	visited, calls := 0, 0
	result.StopReason = "scope_complete"
	for len(directories) > 0 {
		if visited == previewDirectoryLimit {
			result.StopReason = "directory_limit"
			return nil
		}
		directory := directories[0]
		directories = directories[1:]
		visited++
		query := provider115.FileQuery{DirectoryID: directory.id, Limit: 200, Sort: "name", Order: "asc", Type: "all"}
		firstPath := ""
		seenCursors := map[string]bool{}
		for {
			if err := ctx.Err(); err != nil {
				return previewInterrupted(ctx, rootValidated, result, err)
			}
			if calls == previewRequestLimit || budget.Remaining() == 0 {
				result.StopReason = "request_limit"
				return nil
			}
			calls++ // 同时限制未实现发送 hook 的测试/替代 Provider；真实发送次数由 adapter budget 独立约束。
			page, err := s.provider.Files(ctx, query, requestID)
			if ctx.Err() != nil {
				return previewInterrupted(ctx, rootValidated, result, ctx.Err())
			}
			if errors.Is(err, provider115.ErrRequestBudgetExhausted) {
				if !rootValidated {
					return err
				}
				result.StopReason = "request_limit"
				return nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return previewInterrupted(ctx, rootValidated, result, err)
			}
			if err == nil && len(page.Items) > query.Limit {
				err = errSourcePagination
			}
			if err == nil {
				if page.AccountID == "" || (accountID != "" && accountID != page.AccountID) {
					return errAccountChanged
				}
				var display, relative string
				display, relative, err = sourceDirectoryPaths(directory, page)
				if err == nil && query.Cursor != "" && display != firstPath {
					err = errSourcePath
				}
				if err == nil {
					if page.NextCursor != nil && (*page.NextCursor == "" || seenCursors[*page.NextCursor]) {
						err = errSourcePagination
					}
				}
				if err == nil {
					accountID = page.AccountID
					if directory.parentID == "" {
						rootValidated = true
					}
					firstPath = display
					if excludedDirectory(target.ExcludeDirectoryPath, display, relative) {
						break
					}
					for _, item := range page.Items {
						if item.IsDirectory {
							if !target.IncludeSubdirectories {
								continue
							}
							path := display + "/" + item.Name
							if excludedDirectory(target.ExcludeDirectoryPath, path, relative+"/"+item.Name) {
								continue
							}
							if !validPathPart(item.Name) || !providerID(item.ID) {
								result.FailedCount++
								failedDirectory = true
								continue
							}
							if !seenDirectories[item.ID] {
								seenDirectories[item.ID] = true
								directories = append(directories, sourceDirectory{id: item.ID, parentID: directory.id, relative: filepath.Join(directory.relative, item.Name), display: path})
							}
							continue
						}
						if item.FileType != "video" {
							continue
						}
						file := sourceVideo(item, directory.relative, relative, accountID)
						if seenFiles[file.key] {
							continue
						}
						seenFiles[file.key] = true
						result.ScannedVideos++
						if file.err != nil {
							result.FailedCount++
							continue
						}
						result.CandidateCount++
						result.Samples = append(result.Samples, PreviewSample{SourcePath: file.path, RelativePath: filepath.ToSlash(file.relative)})
						if len(result.Samples) == fullSyncPreviewSampleLimit {
							result.StopReason = "sample_limit"
							return nil
						}
					}
					if page.NextCursor == nil {
						break
					}
					seenCursors[*page.NextCursor] = true
					query.Cursor = *page.NextCursor
					continue
				}
			}
			if directory.parentID == "" && !rootValidated || errors.Is(err, provider115.ErrUnconfigured) || errors.Is(err, provider115.ErrInvalidAuthorization) || errors.Is(err, errAccountChanged) || errors.Is(err, provider115.ErrCredentialChanged) {
				return err
			}
			result.FailedCount++
			failedDirectory = true
			break
		}
	}
	if failedDirectory {
		result.StopReason = "source_error"
	} else {
		result.ScanComplete = true
	}
	return nil
}

// previewInterrupted 仅在根已验证后将期限转为部分结果；客户端取消始终保持取消身份。
func previewInterrupted(ctx context.Context, rootValidated bool, result *FullSyncPreview, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if (errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)) && rootValidated {
		result.StopReason = "time_limit"
		return nil
	}
	return err
}
