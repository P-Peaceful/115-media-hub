// scan.go：负责只读遍历媒体库源目录、校验分页与路径身份，并收集可生成 STRM 的视频；丢弃未完整读取的目录结果。

package strm

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

var (
	errAccountChanged   = errors.New("生成期间 115 授权账号发生变化")
	errSourcePath       = errors.New("115 返回的源路径无法用于生成 STRM")
	errSourcePagination = errors.New("115 目录分页结果不完整")
	errPlaybackCode     = errors.New("115 源文件缺少播放定位信息")
)

// sourceDirectory 保存待遍历目录的稳定身份、预期父目录和输出相对位置。
type sourceDirectory struct {
	id, parentID, relative, display string
}

// sourceFile 保存扫描条目的源身份、输出位置或失败；目录失败也作为任务条目返回。
type sourceFile struct {
	key, path, relative, name, id, accountID, playbackCode string
	directoryID                                            string
	isVideo                                                bool
	err                                                    error
}

// scanSummary 汇总已完整读取目录、失败目录及视频候选的统计。
type scanSummary struct {
	completedDirectories, failedDirectories, scannedVideos, candidateFiles, invalidSourceEntries int
}

// providerID 判断源 ID 是否为 1–32 位十进制数字，供目录和条目身份校验使用。
func providerID(id string) bool {
	return len(id) > 0 && len(id) <= 32 && strings.Trim(id, "0123456789") == ""
}

// scan 广度遍历保存的范围并按稳定身份去重；目录分页失败时丢弃该目录结果，兄弟目录可继续。
// 根目录、授权或账号变更失败终止整轮；取消只返回此前完整读取的目录。
func (s *Service) scan(ctx context.Context, target library.Library, requestID string, reportDirectory func(string, scanSummary) error, reportFailure func(sourceFile)) ([]sourceFile, scanSummary, error) {
	directories := []sourceDirectory{{id: target.DirectoryID, display: target.DirectoryPath}}
	seenDirectories := map[string]bool{target.DirectoryID: true}
	seenFiles := make(map[string]bool)
	var files []sourceFile
	var accountID string
	var summary scanSummary
	for len(directories) > 0 {
		directory := directories[0]
		directories = directories[1:]
		if reportDirectory != nil {
			if err := reportDirectory(directory.display, summary); err != nil {
				return nil, summary, err
			}
		}
		nextDirectories, entries, err := s.scanDirectory(ctx, target.Config, requestID, directory, &accountID)
		if err != nil {
			if ctx.Err() != nil {
				// 预览超时只能展示分页已结束的目录，避免把中断的目录当作完整结果。
				return files, summary, ctx.Err()
			}
			summary.failedDirectories++
			failed := sourceFile{key: "directory:" + directory.id, path: directory.display, directoryID: directory.id, err: err}
			if reportFailure != nil {
				reportFailure(failed)
			}
			if directory.parentID == "" || errors.Is(err, errAccountChanged) || errors.Is(err, provider115.ErrUnconfigured) || errors.Is(err, provider115.ErrInvalidAuthorization) {
				return nil, summary, err
			}
			files = append(files, failed)
			// 分页中断后不采用此目录的任何结果。兄弟目录仍可独立生成。
			continue
		}
		summary.completedDirectories++
		for _, entry := range entries {
			if seenFiles[entry.key] {
				continue
			}
			seenFiles[entry.key] = true
			files = append(files, entry)
			if entry.isVideo {
				summary.scannedVideos++
			}
			if entry.err == nil {
				summary.candidateFiles++
			} else {
				summary.invalidSourceEntries++
				if reportFailure != nil {
					reportFailure(entry)
				}
			}
		}
		for _, next := range nextDirectories {
			if !seenDirectories[next.id] {
				seenDirectories[next.id] = true
				directories = append(directories, next)
			}
		}
	}
	return files, summary, nil
}

// scanDirectory 读取单目录所有分页并核对账号、路径及游标；仅完整成功才返回子目录和视频列表。
func (s *Service) scanDirectory(ctx context.Context, config library.Config, requestID string, directory sourceDirectory, accountID *string) ([]sourceDirectory, []sourceFile, error) {
	query := provider115.FileQuery{DirectoryID: directory.id, Limit: 200, Sort: "name", Order: "asc", Type: "all"}
	var directories []sourceDirectory
	var files []sourceFile
	var firstPath string
	seenCursors := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		page, err := s.provider.Files(ctx, query, requestID)
		if err != nil {
			return nil, nil, err
		}
		if page.AccountID == "" || (*accountID != "" && *accountID != page.AccountID) {
			return nil, nil, errAccountChanged
		}
		*accountID = page.AccountID
		display, relative, err := sourceDirectoryPaths(directory, page)
		if err != nil {
			return nil, nil, err
		}
		if query.Cursor == "" {
			firstPath = display
		} else if firstPath != display {
			return nil, nil, errSourcePath
		}
		if excludedDirectory(config.ExcludeDirectoryPath, display, relative) {
			return nil, nil, nil
		}
		for _, item := range page.Items {
			if item.IsDirectory {
				if !config.IncludeSubdirectories {
					continue
				}
				path := display + "/" + item.Name
				if excludedDirectory(config.ExcludeDirectoryPath, path, relative+"/"+item.Name) {
					continue
				}
				if !validPathPart(item.Name) || !providerID(item.ID) {
					files = append(files, sourceFile{key: "directory:" + item.ID, path: path, directoryID: item.ID, err: errSourcePath})
					continue
				}
				directories = append(directories, sourceDirectory{
					id: item.ID, parentID: directory.id, relative: filepath.Join(directory.relative, item.Name), display: path,
				})
			} else if item.FileType == "video" {
				file := sourceVideo(item, directory.relative, relative, *accountID)
				file.directoryID = directory.id
				files = append(files, file)
			}
		}
		if page.NextCursor == nil {
			return directories, files, nil
		}
		if *page.NextCursor == "" || seenCursors[*page.NextCursor] {
			return nil, nil, errSourcePagination
		}
		seenCursors[*page.NextCursor] = true
		query.Cursor = *page.NextCursor
	}
}

// sourceDirectoryPaths 核对请求目录及父子路径身份，拒绝扫描期间被移动或改名的子目录。
func sourceDirectoryPaths(directory sourceDirectory, page provider115.FilePage) (display, relative string, err error) {
	path := page.Path
	if page.DirectoryID != directory.id || len(path) == 0 || path[0].ID != "0" || path[len(path)-1].ID != directory.id {
		return "", "", errSourcePath
	}
	names := make([]string, 0, len(path))
	for _, part := range path {
		if !validPathPart(part.Name) {
			return "", "", errSourcePath
		}
		names = append(names, part.Name)
	}
	display = strings.Join(names, "/")
	// 扫描过程中移动或改名的目录，交给下次扫描按新的父目录生成。
	if directory.parentID != "" && (len(path) < 2 || path[len(path)-2].ID != directory.parentID || display != directory.display) {
		return "", "", errSourcePath
	}
	return display, strings.Join(names[1:], "/"), nil
}

// excludedDirectory 按配置的完整展示路径或网盘根相对路径排除目录及其后代。
func excludedDirectory(configured, display, relative string) bool {
	if configured == "" {
		return false
	}
	excluded := strings.Trim(configured, "/")
	if excluded == "" {
		return strings.HasPrefix(configured, "/")
	}
	if strings.HasPrefix(configured, "/") {
		display = strings.Trim(relative, "/")
	}
	return display == excluded || strings.HasPrefix(display, excluded+"/")
}

// sourceVideo 校验视频 ID、文件名及定位码，派生同主干 STRM 相对路径，保留错误供条目统计。
func sourceVideo(item provider115.FileItem, outputDir, sourceDir, accountID string) sourceFile {
	file := sourceFile{
		key: "file:" + item.ID, path: "/" + strings.TrimPrefix(sourceDir+"/"+item.Name, "/"),
		name: item.Name, id: item.ID, accountID: accountID, playbackCode: item.PlaybackCode, isVideo: true,
	}
	// 产物名沿用视频主干，链接的「文件原名」使用保留扩展名的 file.name。
	stem := strings.TrimSuffix(item.Name, filepath.Ext(item.Name))
	if !providerID(item.ID) || !validPathPart(item.Name) || !validPathPart(stem) {
		file.err = errSourcePath
		return file
	}
	file.relative = filepath.Join(outputDir, stem+".strm")
	if !validArtifactPath(file.relative) {
		file.err = errSourcePath
	} else if file.playbackCode == "" {
		file.err = errPlaybackCode
	}
	return file
}
