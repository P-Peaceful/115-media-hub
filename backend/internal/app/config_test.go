// config_test.go：验证启动环境配置拒绝非法端口、日志级别、数据库路径、关闭期限和网盘请求策略，并正确解析有效设置。

package app

import (
	"testing"
	"time"
)

// TestConfigValidation 验证非法环境变量被拒绝，合法日志级别、监听地址和关闭期限能被读取。
func TestConfigValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{"MEDIAHUB_HTTP_ADDR", "localhost"}, {"MEDIAHUB_HTTP_ADDR", ":0"}, {"MEDIAHUB_HTTP_ADDR", ":65536"},
		{"MEDIAHUB_115_REQUEST_INTERVAL", "0s"}, {"MEDIAHUB_115_REQUEST_INTERVAL", "61s"}, {"MEDIAHUB_115_REQUEST_INTERVAL", "invalid"}, {"MEDIAHUB_115_FAST_SCAN", "invalid"}, {"MEDIAHUB_COOKIE_SECURE", "invalid"}, {"MEDIAHUB_LOG_LEVEL", "verbose"}, {"MEDIAHUB_DATABASE_PATH", " "}, {"MEDIAHUB_SHUTDOWN_TIMEOUT", "-1s"},
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

// TestProviderConfig 验证可调大读取间隔和关闭快速扫描均按启动参数生效。
func TestProviderConfig(t *testing.T) {
	t.Setenv("MEDIAHUB_115_REQUEST_INTERVAL", "12s")
	t.Setenv("MEDIAHUB_115_FAST_SCAN", "false")
	config, err := LoadConfig()
	if err != nil || config.ProviderRequestInterval != 12*time.Second || config.ProviderFastScan {
		t.Fatalf("provider config: %+v %v", config, err)
	}
}
