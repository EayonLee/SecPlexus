package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
)

func TestCommandsRejectArgumentsWithoutLoadingDependencies(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"serve", "extra"}, {"reset-password"}} {
		cmd := newCommand()
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}

func TestServeRejectsMissingSecurityBeforeOpeningLog(t *testing.T) {
	cfg, err := config.Load("../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Auth.CSRFKey = ""
	cfg.Logging.File = filepath.Join(t.TempDir(), "must-not-exist.log")
	if err := execute(context.Background(), "serve", cfg, ""); err == nil {
		t.Fatal("missing security configuration accepted")
	}
	if _, err := os.Stat(cfg.Logging.File); !os.IsNotExist(err) {
		t.Fatal("invalid configuration opened a log file", err)
	}
}

func TestCheckConfigHasNoRuntimeSideEffects(t *testing.T) {
	t.Setenv("SPX_IAM_CSRF_KEY", strings.Repeat("c", 32))
	t.Setenv("SPX_CONSOLE_IAM_TOKEN", strings.Repeat("t", 32))
	t.Setenv("SPX_IAM_POSTGRES_HOST", "127.0.0.1")
	t.Setenv("SPX_IAM_POSTGRES_PORT", "1")
	t.Setenv("SPX_IAM_REDIS_HOST", "127.0.0.1")
	t.Setenv("SPX_IAM_REDIS_PORT", "1")
	logfile := filepath.Join(t.TempDir(), "must-not-exist.log")
	t.Setenv("SPX_IAM_LOG_FILE", logfile)
	cmd := newCommand()
	cmd.SetArgs([]string{"check-config", "--config", "../config.yaml", "--print-endpoints"})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "public_addr") {
		t.Fatal("missing endpoint metadata")
	}
	if _, err := os.Stat(logfile); !os.IsNotExist(err) {
		t.Fatal("configuration check opened a log", err)
	}
	t.Setenv("SPX_IAM_CSRF_KEY", "")
	cmd = newCommand()
	cmd.SetArgs([]string{"check-config", "--config", "../config.yaml"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("missing security accepted")
	}
}
