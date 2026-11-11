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
	"strings"
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
	_, _, err := EnsureDaemon(context.Background(), "test")
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

func TestDaemonVersionMatches(t *testing.T) {
	cases := []struct {
		name   string
		client string
		info   *AuthorityInfo
		want   bool
	}{
		{name: "same version", client: "1.2.3", info: &AuthorityInfo{Version: "1.2.3"}, want: true},
		{name: "different version", client: "1.2.3", info: &AuthorityInfo{Version: "1.2.2"}, want: false},
		{name: "unversioned daemon", client: "1.2.3", info: &AuthorityInfo{}, want: false},
		{name: "nil info", client: "1.2.3", info: nil, want: false},
		{name: "unversioned client", client: "", info: &AuthorityInfo{Version: "1.2.3"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := daemonVersionMatches(tc.client, tc.info); got != tc.want {
				t.Fatalf("daemonVersionMatches()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnsureDaemonRestartsOnVersionMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)
	fingerprint, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}

	srv := newHealthServer(t, AuthorityInfo{
		PID:             4242,
		APIVersion:      APIVersion,
		Version:         "old",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	})
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	if err := WriteAuthorityInfo(&AuthorityInfo{
		PID:             4242,
		Port:            port,
		APIVersion:      APIVersion,
		Version:         "old",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	}); err != nil {
		t.Fatalf("WriteAuthorityInfo: %v", err)
	}

	restarted := false
	oldRestart := restartDaemonForVersion
	t.Cleanup(func() { restartDaemonForVersion = oldRestart })
	restartDaemonForVersion = func(context.Context) (*Client, *AuthorityInfo, error) {
		restarted = true
		return NewClientFromPort(port), &AuthorityInfo{PID: 4242, Port: port, APIVersion: APIVersion, Version: "new"}, nil
	}

	_, info, err := EnsureDaemon(context.Background(), "new")
	if err != nil {
		t.Fatalf("EnsureDaemon: %v", err)
	}
	if !restarted {
		t.Fatal("expected the daemon to be restarted on version mismatch")
	}
	if info == nil || info.Version != "new" {
		t.Fatalf("unexpected daemon info: %#v", info)
	}
}

func TestEnsureDaemonExitsWhenVersionMismatchPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)
	fingerprint, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}

	srv := newHealthServer(t, AuthorityInfo{
		PID:             4242,
		APIVersion:      APIVersion,
		Version:         "old",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	})
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	if err := WriteAuthorityInfo(&AuthorityInfo{
		PID:             4242,
		Port:            port,
		APIVersion:      APIVersion,
		Version:         "old",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	}); err != nil {
		t.Fatalf("WriteAuthorityInfo: %v", err)
	}

	oldRestart := restartDaemonForVersion
	t.Cleanup(func() { restartDaemonForVersion = oldRestart })
	restartDaemonForVersion = func(context.Context) (*Client, *AuthorityInfo, error) {
		return NewClientFromPort(port), &AuthorityInfo{PID: 4242, Port: port, APIVersion: APIVersion, Version: "still-old"}, nil
	}

	_, _, err = EnsureDaemon(context.Background(), "new")
	if err == nil || !strings.Contains(err.Error(), "still does not match") {
		t.Fatalf("expected persistent version mismatch error, got %v", err)
	}
}

func TestEnsureDaemonSkipsRestartWhenVersionsMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)
	fingerprint, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}

	srv := newHealthServer(t, AuthorityInfo{
		PID:             4242,
		APIVersion:      APIVersion,
		Version:         "same",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	})
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	if err := WriteAuthorityInfo(&AuthorityInfo{
		PID:             4242,
		Port:            port,
		APIVersion:      APIVersion,
		Version:         "same",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	}); err != nil {
		t.Fatalf("WriteAuthorityInfo: %v", err)
	}

	oldRestart := restartDaemonForVersion
	t.Cleanup(func() { restartDaemonForVersion = oldRestart })
	restartDaemonForVersion = func(context.Context) (*Client, *AuthorityInfo, error) {
		t.Fatal("daemon must not be restarted when versions match")
		return nil, nil, nil
	}

	if _, _, err := EnsureDaemon(context.Background(), "same"); err != nil {
		t.Fatalf("EnsureDaemon: %v", err)
	}
}

func newHealthServer(t *testing.T, info AuthorityInfo) *httptest.Server {
	t.Helper()
	return newHealthServerWithAPIVersion(t, info, APIVersion)
}

func newHealthServerWithAPIVersion(t *testing.T, info AuthorityInfo, apiVersion string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_version": apiVersion,
			"daemon":      info,
			"healthy":     true,
			"revision":    1,
		})
	})
	return httptest.NewServer(mux)
}

func TestEnsureDaemonRestartsOnAPIVersionMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)
	fingerprint, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}

	// The health probe only adopts a daemon whose reported PID is alive.
	pid := os.Getpid()
	srv := newHealthServerWithAPIVersion(t, AuthorityInfo{
		PID:             pid,
		APIVersion:      "v0",
		Version:         "same",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	}, "v0")
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	if err := WriteAuthorityInfo(&AuthorityInfo{
		PID:             pid,
		Port:            port,
		APIVersion:      "v0",
		Version:         "same",
		HomeFingerprint: fingerprint,
		Executable:      "/tmp/c4cd",
	}); err != nil {
		t.Fatalf("WriteAuthorityInfo: %v", err)
	}

	restarted := false
	oldRestart := restartDaemonForVersion
	t.Cleanup(func() { restartDaemonForVersion = oldRestart })
	restartDaemonForVersion = func(context.Context) (*Client, *AuthorityInfo, error) {
		restarted = true
		return NewClientFromPort(port), &AuthorityInfo{PID: 4242, Port: port, APIVersion: APIVersion, Version: "same"}, nil
	}

	if _, _, err := EnsureDaemon(context.Background(), "same"); err != nil {
		t.Fatalf("EnsureDaemon: %v", err)
	}
	if !restarted {
		t.Fatal("expected the daemon to be restarted on API version mismatch")
	}
}
