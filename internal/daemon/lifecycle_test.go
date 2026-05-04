package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

func TestServerWritesAuthorityFilesAndCleansUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)
	cfg := config.DefaultConfig()
	slotMgr := proxy.NewSlotManager(cfg)
	s := NewServer(cfg, slotMgr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for _, path := range []string{
		filepath.Join(home, "daemon.lock"),
		filepath.Join(home, "daemon.pid"),
		filepath.Join(home, "daemon.info.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected authority file %s: %v", path, err)
		}
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	for _, path := range []string{
		filepath.Join(home, "daemon.lock"),
		filepath.Join(home, "daemon.pid"),
		filepath.Join(home, "daemon.info.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected cleaned authority file %s", path)
		}
	}
}
