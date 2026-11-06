// Package app constructs application dependencies and owns process lifecycle.
package app

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds startup-only settings; business settings will live in SQLite.
type Config struct {
	HTTPAddr        string
	DatabasePath    string
	LogLevel        slog.Level
	LogFile         string
	ShutdownTimeout time.Duration
}

// LoadConfig reads environment variables, rejecting invalid startup settings.
func LoadConfig() (Config, error) {
	config := Config{HTTPAddr: env("MEDIAHUB_HTTP_ADDR", ":8080"), DatabasePath: env("MEDIAHUB_DATABASE_PATH", "./data/mediahub.db"), LogFile: os.Getenv("MEDIAHUB_LOG_FILE"), ShutdownTimeout: 10 * time.Second}
	if err := config.LogLevel.UnmarshalText([]byte(env("MEDIAHUB_LOG_LEVEL", "INFO"))); err != nil {
		return Config{}, fmt.Errorf("MEDIAHUB_LOG_LEVEL: %w", err)
	}
	_, port, err := net.SplitHostPort(config.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("MEDIAHUB_HTTP_ADDR: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return Config{}, fmt.Errorf("MEDIAHUB_HTTP_ADDR must include a port between 1 and 65535")
	}
	if value, ok := os.LookupEnv("MEDIAHUB_SHUTDOWN_TIMEOUT"); ok {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return Config{}, fmt.Errorf("MEDIAHUB_SHUTDOWN_TIMEOUT must be a positive duration")
		}
		config.ShutdownTimeout = duration
	}
	if strings.TrimSpace(config.DatabasePath) == "" {
		return Config{}, fmt.Errorf("MEDIAHUB_DATABASE_PATH must not be blank")
	}
	return config, nil
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}
