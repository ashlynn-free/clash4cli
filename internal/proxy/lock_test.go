package proxy

import (
	"os"
	"testing"

	"rocket4cli/internal/config"
)

func setupTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}
}

func TestLockFile_RoundTrip(t *testing.T) {
	setupTestHome(t)

	want := LockFile{
		Port:       17890,
		APIPort:    19090,
		RedirPort:  0,
		PID:        12345,
		SubID:      "sub",
		Node:       "node",
		NodeServer: "example.com",
		NodePort:   8388,
		NodeType:   "ss",
		ProxyHost:  "127.0.0.1",
		APIHost:    "127.0.0.1",
		ConfigPath: "/tmp/config.yaml",
		WorkDir:    "/tmp/mihomo",
		LogPath:    "/tmp/mihomo.log",
		Secret:     "secret",
		StartedAt:  "2026-02-01T00:00:00Z",
		Detached:   true,
	}

	if err := writeLockFile(want); err != nil {
		t.Fatalf("writeLockFile: %v", err)
	}

	got, err := ReadLockFile()
	if err != nil {
		t.Fatalf("ReadLockFile: %v", err)
	}

	if got.Port != want.Port || got.APIPort != want.APIPort || got.PID != want.PID {
		t.Fatalf("unexpected lock core fields: got port=%d api=%d pid=%d", got.Port, got.APIPort, got.PID)
	}
	if got.Node != want.Node || got.NodeServer != want.NodeServer || got.NodePort != want.NodePort {
		t.Fatalf("unexpected lock node fields: got node=%q %s:%d", got.Node, got.NodeServer, got.NodePort)
	}

	lockPath, err := config.GetLockFilePath()
	if err != nil {
		t.Fatalf("GetLockFilePath: %v", err)
	}
	st, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Stat(lock): %v", err)
	}
	// Ensure not group/other readable/writable.
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected lock file to be private (perm=%#o)", st.Mode().Perm())
	}
}

func TestLockFile_Update(t *testing.T) {
	setupTestHome(t)

	if err := writeLockFile(LockFile{Port: 1, APIPort: 2, PID: 3, Node: "a"}); err != nil {
		t.Fatalf("writeLockFile: %v", err)
	}

	if err := updateLockFile(func(l *LockFile) {
		l.Node = "b"
		l.NodeServer = "1.2.3.4"
		l.NodePort = 443
	}); err != nil {
		t.Fatalf("updateLockFile: %v", err)
	}

	got, err := ReadLockFile()
	if err != nil {
		t.Fatalf("ReadLockFile: %v", err)
	}
	if got.Node != "b" || got.NodeServer != "1.2.3.4" || got.NodePort != 443 {
		t.Fatalf("unexpected updated lock: %#v", got)
	}
}

func TestReadLockFile_ReadsSlot0FromMultiLock(t *testing.T) {
	setupTestHome(t)

	mlf := &MultiLockFile{
		Version: 2,
		Instances: map[int]LockFile{
			0: {Port: 17890, APIPort: 19090, PID: 11, Node: "slot0"},
			2: {Port: 17892, APIPort: 19092, PID: 22, Node: "slot2"},
		},
	}
	if err := writeMultiLockFile(mlf); err != nil {
		t.Fatalf("writeMultiLockFile: %v", err)
	}

	got, err := ReadLockFile()
	if err != nil {
		t.Fatalf("ReadLockFile: %v", err)
	}
	if got.Node != "slot0" || got.Port != 17890 {
		t.Fatalf("expected slot0 lock, got %#v", got)
	}
}
