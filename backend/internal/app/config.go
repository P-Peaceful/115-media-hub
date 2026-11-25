// config.go：读取并校验环境变量中的启动配置，提供监听地址、数据库、日志和关闭期限的默认值。

// Package app 负责应用依赖构建与进程生命周期。
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

// Config 保存进程启动设置；媒体库、账号等业务设置由 SQLite 管理。
type Config struct {
	HTTPAddr        string
	DatabasePath    string
	LogLevel        slog.Level
	LogFile         string
	ShutdownTimeout time.Duration
	CookieSecure    bool
}

// LoadConfig 读取环境配置并校验监听端口、日志级别、关闭期限及 Cookie 设置；无效值直接返回错误。
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
	if value, ok := os.LookupEnv("MEDIAHUB_COOKIE_SECURE"); ok {
		secure, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("MEDIAHUB_COOKIE_SECURE must be a boolean")
		}
		config.CookieSecure = secure
	}
	if strings.TrimSpace(config.DatabasePath) == "" {
		return Config{}, fmt.Errorf("MEDIAHUB_DATABASE_PATH must not be blank")
	}
	return config, nil
}

// env 读取已设置的环境变量，包括显式空值；未设置时返回默认值。
func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}
