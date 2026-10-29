package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestViperBindingAndStrictDecoding(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("port: 8081\nenabled: true\nwait: 30m\nclients:\n  console:\n    token: ''\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPX_TEST_PORT", "9081")
	t.Setenv("SPX_TEST_TOKEN", "private-value")
	v, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}
	_ = v.BindEnv("port", "SPX_TEST_PORT")
	_ = v.BindEnv("clients.console.token", "SPX_TEST_TOKEN")
	var cfg struct {
		Port    int
		Enabled bool
		Wait    time.Duration
		Clients map[string]struct{ Token string }
	}
	if err := Decode(v, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9081 || !cfg.Enabled || cfg.Wait != 30*time.Minute || cfg.Clients["console"].Token != "private-value" {
		t.Fatal("typed environment binding was not applied")
	}
	t.Setenv("SPX_TEST_PORT", "private-invalid-value")
	if err := Decode(v, &cfg); err == nil || strings.Contains(err.Error(), "private-") {
		t.Fatal("configuration errors must reject invalid values without exposing them")
	}
}

func TestConfigurationRejectsAdditionalDocumentsAndDuplicateKeys(t *testing.T) {
	for _, content := range []string{"port: 1\n---\nport: 2\n", "port: 1\nport: 2\n"} {
		file := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(file); err == nil {
			t.Fatal("ambiguous YAML configuration accepted")
		}
	}
}
