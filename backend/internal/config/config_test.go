package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAdminCredentialsAndCookieSecurePrecedence(t *testing.T) {
	t.Setenv("MEDIA_HUB_ADMIN_USERNAME", "")
	t.Setenv("MEDIA_HUB_ADMIN_PASSWORD", "")
	t.Setenv("MEDIA_HUB_COOKIE_SECURE", "")
	p := filepath.Join(t.TempDir(), "config.yaml")
	body := "admin_username: yaml-user\nadmin_password: ' yaml-pass ' \ncookie_secure: true\n"
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Fatal("explicit empty environment credentials must override YAML and fail")
	}
	t.Setenv("MEDIA_HUB_ADMIN_USERNAME", "env-user")
	t.Setenv("MEDIA_HUB_ADMIN_PASSWORD", "env-pass")
	t.Setenv("MEDIA_HUB_COOKIE_SECURE", "false")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminUsername != "env-user" || c.AdminPassword != "env-pass" || c.CookieSecure {
		t.Fatal("unexpected environment auth config")
	}
}

func TestLoadReadsAdminCredentialsFromYAML(t *testing.T) {
	for _, key := range []string{"MEDIA_HUB_ADMIN_USERNAME", "MEDIA_HUB_ADMIN_PASSWORD", "MEDIA_HUB_COOKIE_SECURE"} {
		old, present := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("admin_username: yaml-admin\nadmin_password: ' secret ' \ncookie_secure: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminUsername != "yaml-admin" || c.AdminPassword != " secret " || !c.CookieSecure {
		t.Fatal("unexpected YAML auth config")
	}
}

func TestLoadRequiresNonWhitespaceAdminCredentials(t *testing.T) {
	t.Setenv("MEDIA_HUB_ADMIN_USERNAME", "admin")
	t.Setenv("MEDIA_HUB_ADMIN_PASSWORD", "   ")
	if _, err := Load(""); err == nil {
		t.Fatal("expected blank password rejection")
	}
}

func TestLoadDefaultsAndEnvironmentOverride(t *testing.T) {
	t.Setenv("MEDIA_HUB_ADMIN_USERNAME", "admin")
	t.Setenv("MEDIA_HUB_ADMIN_PASSWORD", "password")
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
		t.Fatal("unexpected environment override")
	}
}
func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Setenv("MEDIA_HUB_ADMIN_USERNAME", "admin")
	t.Setenv("MEDIA_HUB_ADMIN_PASSWORD", "password")
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
