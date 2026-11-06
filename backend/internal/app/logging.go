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

// NewLogger writes structured JSON to stdout and optionally to a rotating file.
// File storage is bounded to a 10 MiB active file plus three backups, aged seven days.
func NewLogger(config Config, stdout io.Writer) (*slog.Logger, io.Closer, error) {
	var closer io.Closer = nopCloser{}
	output := stdout
	if config.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(config.LogFile), 0o750); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
		// Preflight errors before serving; lumberjack otherwise opens lazily.
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

// reportWriter avoids losing rotation failures silently through slog's Handler API.
type reportingWriter struct {
	writer   io.Writer
	fallback io.Writer
}

func (w *reportingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	if err != nil {
		// Emit only a stable diagnostic, never a potentially sensitive filename/error.
		_, fallbackErr := io.WriteString(w.fallback, "{\"level\":\"ERROR\",\"msg\":\"log file write failed\",\"module\":\"logging\",\"operation\":\"write\",\"error_code\":\"log_write_failed\"}\n")
		return n, errors.Join(err, fallbackErr)
	}
	return n, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
