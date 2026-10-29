package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
