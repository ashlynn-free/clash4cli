package daemon

import (
	"context"
	"errors"
	"testing"

	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

// A second daemon start must not kill a healthy running daemon; it should
// return ErrDaemonAlreadyRunningHealthy so the CLI can point at --restart.
func TestEnsureSingleAuthorityRefusesHealthyDaemon(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cfg := config.DefaultConfig()
	s := NewServer(cfg, proxy.NewSlotManager(cfg), "test")
	s.port = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	paths, err := GetAuthorityPaths()
	if err != nil {
		t.Fatalf("GetAuthorityPaths: %v", err)
	}
	if _, err := ensureSingleAuthority(paths, s.port); !errors.Is(err, ErrDaemonAlreadyRunningHealthy) {
		t.Fatalf("expected ErrDaemonAlreadyRunningHealthy, got %v", err)
	}
}
