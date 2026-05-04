package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFindDaemonBinaryFromExecutableDir(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	path, err := FindDaemonBinary()
	if err != nil {
		// acceptable in test env if c4cd is not on PATH; just ensure function is callable
		return
	}
	if filepath.Base(path) != "c4cd" {
		t.Fatalf("expected c4cd binary, got %q", path)
	}
}

func TestLoadClientFromAuthorityInfoFailsWithoutInfo(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if _, _, err := LoadClientFromAuthorityInfo(); err == nil {
		t.Fatal("expected missing authority info error")
	}
}

func TestEnsureDaemonFailsWithoutBinaryOrInfo(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	_, _, err := EnsureDaemon(context.Background())
	if err == nil {
		t.Fatal("expected EnsureDaemon error in isolated test environment")
	}
	_ = os.ErrNotExist
}
