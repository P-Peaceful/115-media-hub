package config

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	HTTPAddr        string        `yaml:"http_addr"`
	DatabasePath    string        `yaml:"database_path"`
	LogDir          string        `yaml:"log_dir"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	AdminUsername   string        `yaml:"admin_username"`
	AdminPassword   string        `yaml:"admin_password"`
	CookieSecure    bool          `yaml:"cookie_secure"`
}

// Load applies defaults, an optional YAML file, and finally environment overrides.
func Load(path string) (Config, error) {
	c := Config{HTTPAddr: "127.0.0.1:8080", DatabasePath: "./data/115-media-hub.db", LogDir: "./logs", ShutdownTimeout: 10 * time.Second}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return c, fmt.Errorf("open config: %w", err)
		}
		defer f.Close()
		d := yaml.NewDecoder(f)
		d.KnownFields(true)
		if err := d.Decode(&c); err != nil {
			return c, fmt.Errorf("decode config: %w", err)
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			return c, fmt.Errorf("config must contain exactly one YAML document")
		}
	}
	for key, dest := range map[string]*string{"MEDIA_HUB_HTTP_ADDR": &c.HTTPAddr, "MEDIA_HUB_DATABASE_PATH": &c.DatabasePath, "MEDIA_HUB_LOG_DIR": &c.LogDir, "MEDIA_HUB_ADMIN_USERNAME": &c.AdminUsername, "MEDIA_HUB_ADMIN_PASSWORD": &c.AdminPassword} {
		if v, ok := os.LookupEnv(key); ok {
			*dest = v
		}
	}
	if v, ok := os.LookupEnv("MEDIA_HUB_SHUTDOWN_TIMEOUT"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("invalid MEDIA_HUB_SHUTDOWN_TIMEOUT")
		}
		c.ShutdownTimeout = d
	}
	if v, ok := os.LookupEnv("MEDIA_HUB_COOKIE_SECURE"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid MEDIA_HUB_COOKIE_SECURE")
		}
		c.CookieSecure = b
	}
	if strings.TrimSpace(c.AdminUsername) == "" {
		return c, fmt.Errorf("admin_username must not be empty")
	}
	if strings.TrimSpace(c.AdminPassword) == "" {
		return c, fmt.Errorf("admin_password must not be empty")
	}
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("http_addr must contain a valid host:port")
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		return c, fmt.Errorf("database_path must not be empty")
	}
	if strings.TrimSpace(c.LogDir) == "" {
		return c, fmt.Errorf("log_dir must not be empty")
	}
	if c.ShutdownTimeout <= 0 {
		return c, fmt.Errorf("shutdown_timeout must be positive")
	}
	return c, nil
}
