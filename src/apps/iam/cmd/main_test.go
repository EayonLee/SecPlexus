package main

import (
	"bytes"
	"testing"
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
