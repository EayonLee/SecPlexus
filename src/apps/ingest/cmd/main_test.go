package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckConfigDoesNotConnectOrOpenLog(t *testing.T) {
	t.Setenv("SPX_KAFKA_BROKERS", "127.0.0.1:9092")
	t.Setenv("SPX_INGEST_IAM_TOKEN", strings.Repeat("t", 32))
	logfile := filepath.Join(t.TempDir(), "must-not-exist.log")
	t.Setenv("SPX_INGEST_LOG_FILE", logfile)
	cmd := newCommand()
	cmd.SetArgs([]string{"check-config", "--config", "../config.yaml", "--print-endpoints"})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "public_addr") {
		t.Fatal("missing metadata")
	}
	if _, err := os.Stat(logfile); !os.IsNotExist(err) {
		t.Fatal("check created a log", err)
	}
	t.Setenv("SPX_INGEST_IAM_TOKEN", "short")
	cmd = newCommand()
	cmd.SetArgs([]string{"check-config", "--config", "../config.yaml"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("malformed token accepted")
	}
}
