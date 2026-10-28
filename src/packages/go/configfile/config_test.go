package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testConfig 只保留验证环境占位符所需的嵌套配置。
type testConfig struct {
	App struct {
		Host    string `yaml:"host"`
		Port    int    `yaml:"port"`
		Enabled bool   `yaml:"enabled"`
	} `yaml:"app"`
	Password string `yaml:"password"`
}

func TestStructuredEnvironmentValues(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(file, []byte("app:\n  host: '${SPX_TEST_HOST:0.0.0.0}'\n  port: '${SPX_TEST_PORT:8081}'\n  enabled: '${SPX_TEST_ENABLED:true}'\npassword: '${SPX_TEST_PASSWORD:}'\n"), 0600)
	t.Setenv("SPX_TEST_PORT", "8123")
	t.Setenv("SPX_TEST_PASSWORD", "first\napp:\n  port: 9999\n#${KEEP_LITERAL}")
	var cfg testConfig
	if err := Load(file, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.App.Host != "0.0.0.0" || cfg.App.Port != 8123 || !cfg.App.Enabled || cfg.Password != os.Getenv("SPX_TEST_PASSWORD") {
		t.Fatal("incorrect scalar expansion or YAML injection")
	}

	t.Setenv("SPX_TEST_HOST", "")
	if err := Load(file, &cfg); err != nil || cfg.App.Host != "" {
		t.Fatal("explicit empty value must remain empty", err)
	}
}

func TestInvalidConfigurationDoesNotExposeValues(t *testing.T) {
	for _, text := range []string{"app: {port: '${SPX_TEST_INVALID:}'}", "app: {typo: true}", "app: {}\n---\napp: {}", "app: {port: 12, port: 13}"} {
		file := filepath.Join(t.TempDir(), "config.yaml")
		os.WriteFile(file, []byte(text), 0600)
		t.Setenv("SPX_TEST_INVALID", "private-value-must-not-appear")
		var cfg testConfig
		err := Load(file, &cfg)
		if err == nil || strings.Contains(err.Error(), "private-value") {
			t.Fatalf("unsafe configuration error: %v", err)
		}
	}
}
