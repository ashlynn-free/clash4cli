package tabs

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the real ~/.clash4cli home.
// Several tests exercise save paths (config.yaml, subscriptions/*.yaml); without
// this guard they write into the developer's real data directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rocket4cli-tabs-test-")
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
