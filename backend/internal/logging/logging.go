package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

func New(dir string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	name := filepath.Join(dir, "app.log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	if err := f.Close(); err != nil {
		return nil, nil, err
	}
	rotation := &lumberjack.Logger{Filename: name, MaxSize: 20, MaxBackups: 5, MaxAge: 14, Compress: true}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, rotation), nil))
	return logger, rotation, nil
}
