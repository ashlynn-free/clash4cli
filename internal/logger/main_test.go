package logger

import (
	"os"
	"testing"
)

// TestMain keeps logger output from tests out of the real ~/.clash4cli/logs.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rocket4cli-logger-test-")
	if err != nil {
		panic(err)
	}
	if os.Getenv("CLASH4CLI_HOME") == "" {
		_ = os.Setenv("CLASH4CLI_HOME", dir)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
