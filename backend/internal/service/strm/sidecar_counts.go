// sidecar_counts.go：按 STRM、NFO、图片累计真实任务条目结果，通用任务计数包含全部处理项。

package strm

import (
	"fmt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
)

// itemCounts 保存一个输出类别的成功、跳过和失败数量。
type itemCounts struct{ succeeded, skipped, failed int }

// classify 将实际条目状态计入对应类别；目录读取失败归入原 STRM 处理类别。
func (counts *generationCounts) classify(kind string, status domain.ItemStatus) {
	target := &counts.strm
	switch kind {
	case "nfo":
		target = &counts.nfo
	case "image":
		target = &counts.image
	}
	switch status {
	case domain.ItemSucceeded:
		target.succeeded++
	case domain.ItemSkipped:
		target.skipped++
	default:
		target.failed++
	}
}

// summary 返回不含文件内容、名称或签名链接的任务终态分类汇总。
func (counts generationCounts) summary() string {
	return fmt.Sprintf("STRM：成功 %d，跳过 %d，失败 %d；NFO：成功 %d，跳过 %d，失败 %d；图片：成功 %d，跳过 %d，失败 %d", counts.strm.succeeded, counts.strm.skipped, counts.strm.failed, counts.nfo.succeeded, counts.nfo.skipped, counts.nfo.failed, counts.image.succeeded, counts.image.skipped, counts.image.failed)
}
