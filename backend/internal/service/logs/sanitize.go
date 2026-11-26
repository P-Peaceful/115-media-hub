// sanitize.go：在日志进入 SQLite 与实时订阅前净化文本，并为既有英文事件提供中文摘要。

package logs

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxMessageRunes   = 1024
	maxAttributeRunes = 256
	maxScannedBytes   = 32 * 1024
	redacted          = "[REDACTED]"
)

// protectedText 识别自由文本中的凭据标签、认证方案、密码哈希和跳转地址；命中时遮挡整个字段。
var protectedText = regexp.MustCompile(`(?i)((?:https?|wss?):[/\\]{1,2}|//[a-z0-9.-]+[/?:]|/p/|\bbearer\s+|\bbasic\s+[a-z0-9+/]{8,}={0,2}|\beyJ[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+\b|\$argon2(?:id|i|d)\$|\$2[aby]\$|(?:\b(?:set[ _-]*)?cookie|\b(?:proxy[ _-]*)?authorization|\b(?:current[ _-]*|new[ _-]*|old[ _-]*)?pass(?:word|wd)?(?:[ _-]*hash)?|\b(?:x[ _-]*)?(?:access|refresh|session|csrf|api|auth|playback)[ _-]*(?:token|key|hash|secret|id)|\b(?:token|hash|credential|secret(?:[ _-]*key)?|session|csrf|uid|cid|seid|kid|pick[ _-]*code|mediahub[ _-]*(?:session|preauth))|密码|令牌|凭据|授权头)["'\\\x60\s]*[:=：])`)

// sanitizeText 检查原始与 URL 解码文本，清除控制字符并在完整 UTF-8 字符边界截断。
func sanitizeText(value string, maxRunes int) string {
	if len(value) > maxScannedBytes {
		return redacted
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, value)
	decoded, err := url.PathUnescape(value)
	if protectedText.MatchString(value) || (err == nil && decoded != value && protectedText.MatchString(decoded)) {
		return redacted
	}
	remaining := value
	for range maxRunes {
		if remaining == "" {
			return value
		}
		_, size := utf8.DecodeRuneInString(remaining)
		remaining = remaining[size:]
	}
	return value[:len(value)-len(remaining)]
}

// fixedMessages 对无结果变体的内置事件给出固定摘要，避免重复输出原始自由文本。
var fixedMessages = map[string]string{
	"database initialized":                        "数据库初始化完成",
	"HTTP server started":                         "HTTP 服务已启动",
	"HTTP server stopped":                         "HTTP 服务已停止",
	"request panic":                               "处理 HTTP 请求时发生内部错误",
	"authentication request rejected":             "认证请求被拒绝",
	"authentication state request canceled":       "认证状态查询已取消",
	"authentication state unavailable":            "认证状态暂不可用",
	"task center request failed":                  "任务中心请求失败",
	"task event subscription unavailable":         "任务事件订阅暂不可用",
	"task event authorization failed":             "任务事件授权检查失败",
	"scheduler tick failed":                       "定时任务调度失败",
	"task claim failed":                           "领取待执行任务失败",
	"task execution stopped for restart recovery": "任务已停止，等待重启后恢复",
	"schedule occurrence skipped":                 "已跳过本次调度",
	"schedule occurrence submitted":               "已提交本次调度任务",
	"library deletion started":                    "开始删除媒体库",
	"STRM write reconciliation started":           "开始核对 STRM 写入记录",
	"Sidecar reconciliation started":              "开始核对附属文件记录",
	"STRM generation started":                     "开始生成 STRM",
	"STRM output directory ready":                 "STRM 输出目录已就绪",
	"STRM source scan started":                    "开始扫描 STRM 来源目录",
	"STRM source scan progress":                   "STRM 来源目录扫描进行中",
	"STRM source scan finished":                   "STRM 来源目录扫描结束",
	"STRM file generation started":                "开始写入 STRM 文件",
	"STRM file generation progress":               "STRM 文件生成进行中",
	"STRM source item failed":                     "处理 STRM 源文件失败",
	"STRM full sync preview started":              "开始预览 STRM 全量同步",
	"115 read retry":                              "正在重试读取 115 网盘",
	"115 account throttled":                       "115 网盘请求触发限流",
	"115 fast scan falling back":                  "115 快速扫描不可用，切换到标准扫描",
	"115 take-link fallback":                      "切换 115 下载链接获取方式",
	"115 playback link resolved":                  "115 播放地址已解析",
	"115 playback link unavailable":               "115 播放地址暂不可用",
	"115 sidecar downloaded":                      "已下载 115 附属文件",
	"115 sidecar unavailable":                     "无法下载 115 附属文件",
}

// businessSubjects 将复用的终态事件映射成对应业务名称，并在摘要中采用结构化操作和结果。
var businessSubjects = map[string]string{
	"runtime log export finished":        "导出运行日志",
	"authentication operation completed": "认证",
	"115 operation completed":            "115 网盘",
	"library operation completed":        "媒体库",
	"task operation completed":           "任务",
	"task operation failed":              "任务",
	"schedule operation completed":       "定时计划",
	"schedule operation failed":          "定时计划",
	"task execution state changed":       "任务执行",
	"STRM operation finished":            "STRM",
	"STRM full sync preview finished":    "STRM 全量同步预览",
	"Sidecar reconciliation finished":    "附属文件核对",
}

var operationNames = map[string]string{
	"apply_task_action":       "状态变更",
	"cancel_qr":               "取消扫码",
	"create":                  "创建",
	"create_schedule":         "创建",
	"delete":                  "删除",
	"delete_schedule":         "删除",
	"execute":                 "",
	"full_sync_preview":       "",
	"generate":                "生成",
	"initialize":              "初始化",
	"list":                    "列表查询",
	"list_files":              "目录查询",
	"login":                   "登录",
	"logout":                  "退出登录",
	"poll_qr":                 "确认扫码结果",
	"reconcile_sidecars":      "",
	"remove_account":          "解绑账号",
	"reset_password":          "重置密码",
	"set_enabled":             "更新启用状态",
	"set_schedule_enabled":    "更新启用状态",
	"start_qr":                "获取登录二维码",
	"submit_scheduled":        "提交调度任务",
	"submit_task":             "提交",
	"update":                  "更新",
	"update_credentials":      "更新凭据",
	"update_schedule":         "更新",
	"update_session_lifetime": "更新会话有效期",
}

var outcomeNames = map[string]string{
	"succeeded": "完成",
	"failed":    "失败",
	"canceled":  "已取消",
	"skipped":   "已跳过",
}

// localizeMessage 根据已知事件和脱敏属性生成摘要；未知事件保留有界的安全文本。
func localizeMessage(message, operation string, attrs map[string]any) string {
	if translated, known := fixedMessages[message]; known {
		return translated
	}
	if message == "http request" {
		status, _ := attrs["status"].(int64)
		if status >= 400 {
			return "HTTP 请求返回 " + strconv.FormatInt(status, 10)
		}
	}
	if subject, known := businessSubjects[message]; known {
		outcome := outcomeNames[attrString(attrs, "outcome")]
		if outcome == "" {
			outcome = "结果已记录"
		}
		if op := operationNames[operation]; op != "" {
			return subject + " · " + op + outcome
		}
		return subject + " · " + outcome
	}
	return sanitizeText(message, maxMessageRunes)
}
