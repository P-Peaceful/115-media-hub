package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsAndEnvironmentOverride(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("http_addr: '127.0.0.1:9090'\ndatabase_path: './test.db'\nshutdown_timeout: 3s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIA_HUB_HTTP_ADDR", "127.0.0.1:9191")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != "127.0.0.1:9191" || c.DatabasePath != "./test.db" || c.ShutdownTimeout != 3*time.Second {
		t.Fatalf("unexpected config: %+v", c)
	}
}
func TestLoadRejectsInvalidConfig(t *testing.T) {
	for _, body := range []string{"unknown: true", "shutdown_timeout: -1s", "http_addr: invalid", "database_path: ''"} {
		t.Run(body, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(p, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(p); err == nil {
				t.Fatal("expected invalid configuration error")
			}
		})
	}
}
