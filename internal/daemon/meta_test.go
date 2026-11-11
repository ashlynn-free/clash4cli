package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthorityPathsAndFingerprintUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)

	paths, err := GetAuthorityPaths()
	if err != nil {
		t.Fatalf("GetAuthorityPaths: %v", err)
	}
	if paths.LockPath != filepath.Join(home, "daemon.lock") {
		t.Fatalf("unexpected lock path: %s", paths.LockPath)
	}
	if paths.PIDPath != filepath.Join(home, "daemon.pid") {
		t.Fatalf("unexpected pid path: %s", paths.PIDPath)
	}
	if paths.InfoPath != filepath.Join(home, "daemon.info.json") {
		t.Fatalf("unexpected info path: %s", paths.InfoPath)
	}
	if paths.LogPath != filepath.Join(home, "logs", "c4cd.log") {
		t.Fatalf("unexpected log path: %s", paths.LogPath)
	}

	fp, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}
	if fp == "" {
		t.Fatal("empty fingerprint")
	}
}

func TestHomeFingerprintDiffersAcrossHomes(t *testing.T) {
	homeA := filepath.Join(t.TempDir(), "a")
	homeB := filepath.Join(t.TempDir(), "b")
	if err := os.MkdirAll(homeA, 0o755); err != nil {
		t.Fatalf("MkdirAll homeA: %v", err)
	}
	if err := os.MkdirAll(homeB, 0o755); err != nil {
		t.Fatalf("MkdirAll homeB: %v", err)
	}

	t.Setenv("CLASH4CLI_HOME", homeA)
	fpA, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint A: %v", err)
	}
	t.Setenv("CLASH4CLI_HOME", homeB)
	fpB, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint B: %v", err)
	}
	if fpA == fpB {
		t.Fatalf("expected different fingerprints, got %s", fpA)
	}
}

func TestWriteAuthorityInfoFailsClosedOnInsecurePermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)
	infoPath := filepath.Join(home, "daemon.info.json")
	if err := os.WriteFile(infoPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(infoPath, 0o666); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	info, err := NewAuthorityInfo(17890, "v1", "test", false)
	if err != nil {
		t.Fatalf("NewAuthorityInfo: %v", err)
	}
	if err := WriteAuthorityInfo(info); err == nil {
		t.Fatal("expected fail-closed on insecure permissions")
	}
}

func TestCleanupStaleAuthorityFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", home)

	lockPath := filepath.Join(home, "daemon.lock")
	pidPath := filepath.Join(home, "daemon.pid")
	infoPath := filepath.Join(home, "daemon.info.json")
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("WriteFile lock: %v", err)
	}
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("WriteFile pid: %v", err)
	}
	if err := os.WriteFile(infoPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile info: %v", err)
	}

	if err := CleanupStaleAuthorityFiles(); err != nil {
		t.Fatalf("CleanupStaleAuthorityFiles: %v", err)
	}
	for _, p := range []string{lockPath, pidPath, infoPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected removed file %s", p)
		}
	}
}

func TestMatchesExpectedIdentity(t *testing.T) {
	info := &AuthorityInfo{HomeFingerprint: "home-a", Executable: "/tmp/c4cd"}
	if err := MatchesExpectedIdentity(info, "home-a", "/tmp/c4cd"); err != nil {
		t.Fatalf("expected identity match: %v", err)
	}
	if err := MatchesExpectedIdentity(info, "home-b", "/tmp/c4cd"); err == nil || !strings.Contains(err.Error(), "home fingerprint mismatch") {
		t.Fatalf("expected home mismatch error, got %v", err)
	}
}
