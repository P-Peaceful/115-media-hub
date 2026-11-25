// logging.go：建立统一 JSON 日志输出和可选轮转文件，预检文件权限并在轮转写入失败时输出脱敏诊断。

package app

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger 创建标准输出及可选轮转文件的 JSON 日志器，并返回由调用方关闭的文件资源。
// 活动文件上限为 10 MiB，最多保留三份备份并按七天清理。
func NewLogger(config Config, stdout io.Writer) (*slog.Logger, io.Closer, error) {
	var closer io.Closer = nopCloser{}
	output := stdout
	if config.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(config.LogFile), 0o750); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
		// 在服务启动前暴露文件访问错误，避免推迟到 lumberjack 首次写入时才发现。
		file, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		if err := file.Close(); err != nil {
			return nil, nil, fmt.Errorf("close log preflight: %w", err)
		}
		rotating := &lumberjack.Logger{Filename: config.LogFile, MaxSize: 10, MaxAge: 7, MaxBackups: 3}
		closer = rotating
		output = io.MultiWriter(stdout, &reportingWriter{writer: rotating, fallback: stdout})
	}
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: config.LogLevel})), closer, nil
}

// reportingWriter 在 slog 不传播写入错误时向备用输出报告稳定诊断。
type reportingWriter struct {
	writer   io.Writer
	fallback io.Writer
}

// Write 向日志文件写入内容；失败时向备用输出写稳定诊断并合并错误，避免泄露文件名和底层错误。
func (w *reportingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	if err != nil {
		// 仅输出稳定诊断，不记录可能包含敏感信息的文件名或底层错误。
		_, fallbackErr := io.WriteString(w.fallback, "{\"level\":\"ERROR\",\"msg\":\"log file write failed\",\"module\":\"logging\",\"operation\":\"write\",\"error_code\":\"log_write_failed\"}\n")
		return n, errors.Join(err, fallbackErr)
	}
	return n, nil
}

// nopCloser 为仅使用标准输出的日志器提供统一关闭接口。
type nopCloser struct{}

// Close 在未启用文件日志时提供无需释放资源的关闭操作。
func (nopCloser) Close() error { return nil }
