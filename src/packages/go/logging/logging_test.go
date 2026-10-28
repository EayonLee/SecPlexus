package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredFileLogsRespectLevelAndRotate(t *testing.T) {
	directory := t.TempDir()
	cfg := Config{
		Level:      "info",
		File:       filepath.Join(directory, "iam.log"),
		MaxSizeMB:  1,
		MaxBackups: 2,
		MaxAgeDays: 1,
	}
	logger, closeLog, err := Open(cfg, "iam")
	if err != nil {
		t.Fatal(err)
	}

	logger.Debug("not recorded")
	logger.Info("first", "payload", strings.Repeat("a", 600000))
	logger.Info("second", "payload", strings.Repeat("b", 600000))
	if err = closeLog(); err != nil {
		t.Fatal(err)
	}

	paths, err := filepath.Glob(filepath.Join(directory, "*.log"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("expected current and rotated file: %v %v", paths, err)
	}

	for _, file := range paths {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		var record map[string]any
		if err = json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}

		if record["service"] != "iam" || record["level"] != "INFO" {
			t.Fatal("invalid structured log metadata")
		}
	}
}

func TestInvalidLoggingConfiguration(t *testing.T) {
	for _, cfg := range []Config{{Level: "unknown", Stdout: true}, {Level: "info"}, {Level: "info", File: "test.log", MaxSizeMB: -1, MaxBackups: 1, MaxAgeDays: 1}} {
		if _, _, err := Open(cfg, "iam"); err == nil {
			t.Fatal("invalid logger configuration accepted")
		}
	}
}
