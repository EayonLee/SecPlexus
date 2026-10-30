package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeclaredVariablesAndStrictDecoding(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("port: '${SPX_TEST_PORT-8081}'\nenabled: true\nwait: 30m\nclients:\n  console:\n    token: '${SPX_TEST_TOKEN-}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPX_TEST_PORT", "9081")
	t.Setenv("SPX_TEST_TOKEN", "private-value")
	v, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}
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
	v, err = Read(file)
	if err != nil {
		t.Fatal(err)
	}
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

// TestDeclaredEnvironmentDefaults 将映射与默认值放在 YAML，保留空值及秘密字面值。
func TestDeclaredEnvironmentDefaults(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("host: '${SPX_TEST_MODULE_HOST-${SPX_TEST_HOST-127.0.0.1}}'\nport: '${SPX_TEST_PORT-5432}'\nsecret: '${SPX_TEST_SECRET-}'\nfile: '${SPX_TEST_FILE-logs/test.log}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	secret := "quotes '\" : #\n${NOT_EXPANDED} $(not-executed) \\"
	t.Setenv("SPX_TEST_HOST", "shared-db")
	t.Setenv("SPX_TEST_SECRET", secret)
	t.Setenv("SPX_TEST_FILE", "")
	var cfg struct {
		Host   string
		Port   int
		Secret string
		File   string
	}
	settings, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := Decode(settings, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "shared-db" || cfg.Port != 5432 || cfg.Secret != secret || cfg.File != "" {
		t.Fatal("YAML declaration did not preserve priority, defaults or literal values")
	}
	t.Setenv("SPX_TEST_MODULE_HOST", "module-db")
	settings, err = Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := Decode(settings, &cfg); err != nil || cfg.Host != "module-db" {
		t.Fatal("module override not applied")
	}
}

// TestVariableExpansionDoesNotExecuteCommands 变量展开不是 Shell 执行入口。
func TestVariableExpansionDoesNotExecuteCommands(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	target := filepath.Join(t.TempDir(), "must-not-exist")
	if err := os.WriteFile(file, []byte("value: '$(touch "+target+")'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(file); err == nil {
		t.Fatal("command substitution accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("configuration executed a command")
	}
}

func TestDiagnosticsIdentifyFieldsWithoutValues(t *testing.T) {
	for _, tc := range []struct{ content, field, hint string }{
		{"db:\n  port: private-invalid-value\n", "db.port", "type"},
		{"db:\n  typo: private-invalid-value\n", "db.typo", "unknown"},
		{"db:\n  port: '$(private-invalid-value)'\n", "db.port", "expression"},
	} {
		t.Run(tc.field+tc.hint, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(file, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			var cfg struct {
				DB struct {
					Port int `yaml:"port"`
				} `yaml:"db"`
			}
			settings, err := Read(file)
			if err == nil {
				err = Decode(settings, &cfg)
			}
			if err == nil || !strings.Contains(err.Error(), tc.field) || !strings.Contains(err.Error(), tc.hint) || strings.Contains(err.Error(), "private-invalid-value") {
				t.Fatalf("expected safe field diagnostic, got %v", err)
			}
		})
	}
	var cfg struct {
		App struct {
			Port int `yaml:"port" validate:"gte=1,lte=65535"`
		} `yaml:"app"`
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "app.port") || !strings.Contains(err.Error(), "gte=1") {
		t.Fatalf("expected YAML field and constraint, got %v", err)
	}
}
