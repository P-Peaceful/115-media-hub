// rapid_scan.go：将已完整验证的快速快照投影到正式 STRM 扫描结果，复用标准扫描的路径、排除与视频校验规则。

package strm

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// adoptRapidSnapshot 在快照验证完成后才生成候选，任何目录路径错误均不输出半成品。
func (s *Service) adoptRapidSnapshot(ctx context.Context, target library.Library, snapshot provider115.TreeSnapshot, reportDirectory func(string, scanSummary) error, reportFailure func(sourceFile)) ([]sourceFile, scanSummary, error) {
	var summary scanSummary
	if snapshot.AccountID == "" {
		return nil, summary, errAccountChanged
	}
	directories := make(map[string]sourceDirectory)
	rootDepth := 0
	for _, node := range snapshot.Directories {
		if node.ID == target.DirectoryID {
			rootDepth = len(node.Path)
		}
	}
	if rootDepth == 0 {
		return nil, summary, errSourcePath
	}
	pages := make(map[string]provider115.FilePage)
	for _, node := range snapshot.Directories {
		if len(node.Path) < rootDepth || node.Path[rootDepth-1].ID != target.DirectoryID {
			return nil, summary, errSourcePath
		}
		names := make([]string, 0, len(node.Path))
		for _, part := range node.Path {
			if !validPathPart(part.Name) {
				return nil, summary, errSourcePath
			}
			names = append(names, part.Name)
		}
		relative := ""
		if len(names) > rootDepth {
			relative = filepath.Join(names[rootDepth:]...)
		}
		directories[node.ID] = sourceDirectory{id: node.ID, parentID: node.ParentID, relative: relative, display: strings.Join(names, "/")}
		pages[node.ID] = provider115.FilePage{DirectoryID: node.ID, Path: node.Path, AccountID: snapshot.AccountID}
	}
	ids := make([]string, 0, len(directories))
	for id := range directories {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var files []sourceFile
	seen := make(map[string]bool)
	entries := make(map[string][]provider115.FileItem)
	for _, file := range snapshot.Files {
		if _, ok := directories[file.DirectoryID]; !ok {
			return nil, summary, errSourcePath
		}
		entries[file.DirectoryID] = append(entries[file.DirectoryID], file.Item)
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, summary, err
		}
		directory := directories[id]
		page := pages[id]
		display, relative, err := sourceDirectoryPaths(directory, page)
		if err != nil {
			return nil, summary, err
		}
		if excludedDirectory(target.ExcludeDirectoryPath, display, relative) {
			continue
		}
		if reportDirectory != nil {
			if err := reportDirectory(display, summary); err != nil {
				return nil, summary, err
			}
		}
		summary.completedDirectories++
		for _, item := range entries[id] {
			file, selected := sourceCandidate(item, directory.relative, relative, snapshot.AccountID, target.Config)
			if !selected {
				continue
			}
			file.directoryID = id
			if seen[file.key] {
				continue
			}
			seen[file.key] = true
			files = append(files, file)
			if file.isVideo {
				summary.scannedVideos++
			}
			if file.err == nil {
				summary.candidateFiles++
			} else {
				summary.invalidSourceEntries++
				if reportFailure != nil {
					reportFailure(file)
				}
			}
		}
	}
	return files, summary, nil
}
