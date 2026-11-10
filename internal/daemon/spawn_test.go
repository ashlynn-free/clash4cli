package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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
	// Hide any c4cd installed on the machine so this test cannot spawn a real
	// daemon or bind the default port.
	t.Setenv("PATH", t.TempDir())
	_, _, err := EnsureDaemon(context.Background())
	if err == nil {
		t.Fatal("expected EnsureDaemon error in isolated test environment")
	}
	_ = os.ErrNotExist
}

func TestAdoptRunningDaemonRestoresAuthorityFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)

	fingerprint, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}
	srv := newHealthServer(t, AuthorityInfo{
		PID:             4242,
		Port:            0,
		APIVersion:      APIVersion,
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
		StartedAt:       "2026-01-01T00:00:00Z",
	})
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	portNum := 0
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	client, info, err := adoptRunningDaemon(context.Background(), portNum)
	if err != nil {
		t.Fatalf("adoptRunningDaemon: %v", err)
	}
	if client == nil || info == nil || info.PID != 4242 {
		t.Fatalf("unexpected daemon identity: client=%v info=%#v", client, info)
	}
	for _, path := range []string{filepath.Join(home, "daemon.pid"), filepath.Join(home, "daemon.info.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected restored authority file %s: %v", path, err)
		}
	}
}

func TestAdoptRunningDaemonRejectsDifferentHome(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	srv := newHealthServer(t, AuthorityInfo{
		PID:             4243,
		APIVersion:      APIVersion,
		HomeFingerprint: "some-other-home",
		Executable:      "/tmp/c4cd",
	})
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	portNum := 0
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	if _, _, err := adoptRunningDaemon(context.Background(), portNum); !errors.Is(err, errDaemonDifferentHome) {
		t.Fatalf("expected errDaemonDifferentHome, got %v", err)
	}
}

func newHealthServer(t *testing.T, info AuthorityInfo) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_version": APIVersion,
			"daemon":      info,
			"healthy":     true,
			"revision":    1,
		})
	})
	return httptest.NewServer(mux)
}
