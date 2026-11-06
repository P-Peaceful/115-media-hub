package app

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{"MEDIAHUB_HTTP_ADDR", "localhost"}, {"MEDIAHUB_HTTP_ADDR", ":0"}, {"MEDIAHUB_HTTP_ADDR", ":65536"},
		{"MEDIAHUB_LOG_LEVEL", "verbose"}, {"MEDIAHUB_DATABASE_PATH", " "}, {"MEDIAHUB_SHUTDOWN_TIMEOUT", "-1s"},
	} {
		t.Run(test.name+test.value, func(t *testing.T) {
			t.Setenv(test.name, test.value)
			if _, err := LoadConfig(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	t.Setenv("MEDIAHUB_HTTP_ADDR", ":8080")
	t.Setenv("MEDIAHUB_DATABASE_PATH", "./data/mediahub.db")
	t.Setenv("MEDIAHUB_LOG_LEVEL", "DEBUG")
	t.Setenv("MEDIAHUB_SHUTDOWN_TIMEOUT", "3s")
	config, err := LoadConfig()
	if err != nil || config.ShutdownTimeout != 3*time.Second {
		t.Fatalf("config: %+v, %v", config, err)
	}
}
