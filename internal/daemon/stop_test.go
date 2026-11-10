package daemon

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"rocket4cli/internal/config"
)

func TestStopDaemon_NotRunning(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	result, err := stopDaemon(freeTCPPort(t))
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if result.WasRunning {
		t.Fatalf("expected WasRunning=false")
	}
}

func TestStopDaemon_StopsVerifiedProcess(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	writeStopTestAuthority(t, cmd.Process.Pid, 0, mustExecutableIdentity(t))

	result, err := StopDaemon()
	if err != nil {
		t.Fatalf("StopDaemon: %v", err)
	}
	if !result.WasRunning || result.PID != cmd.Process.Pid {
		t.Fatalf("unexpected result: %+v", result)
	}
	if isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("daemon helper still alive")
	}
	if _, err := os.Stat(authorityInfoPath(t)); !os.IsNotExist(err) {
		t.Fatalf("expected authority info to be removed, stat err=%v", err)
	}
}

func TestStopDaemon_RefusesUnverifiedProcess(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	writeStopTestAuthority(t, cmd.Process.Pid, 0, "/usr/bin/not-the-c4cd-binary")

	_, err := StopDaemon()
	if err == nil {
		t.Fatalf("expected refusal")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("helper should not have been killed")
	}
}

func TestStopDaemon_RefusesDifferentHome(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	info := &AuthorityInfo{
		PID:             cmd.Process.Pid,
		Port:            0,
		APIVersion:      APIVersion,
		HomeFingerprint: "different-home",
		Executable:      mustExecutableIdentity(t),
		StartedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := WriteAuthorityInfo(info); err != nil {
		t.Fatalf("WriteAuthorityInfo: %v", err)
	}

	_, err := StopDaemon()
	if err == nil {
		t.Fatalf("expected refusal")
	}
	if !strings.Contains(err.Error(), "different CLASH4CLI_HOME") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("helper should not have been killed")
	}
}

func TestStopDaemon_IgnoresUnverifiedLivePIDWithoutAuthorityInfo(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	if err := WriteDaemonPID(cmd.Process.Pid); err != nil {
		t.Fatalf("WriteDaemonPID: %v", err)
	}

	result, err := stopDaemon(freeTCPPort(t))
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if result.WasRunning {
		t.Fatalf("expected not running when no daemon answers on the port")
	}
	if !isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("helper should not have been killed")
	}
}

func TestStopDaemon_FallsBackOnCorruptAuthorityInfo(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	paths, err := GetAuthorityPaths()
	if err != nil {
		t.Fatalf("GetAuthorityPaths: %v", err)
	}
	if err := os.WriteFile(paths.InfoPath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupt info: %v", err)
	}

	fp, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}
	executable := mustExecutableIdentity(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"daemon": &AuthorityInfo{
				PID:             cmd.Process.Pid,
				APIVersion:      APIVersion,
				HomeFingerprint: fp,
				Executable:      executable,
			},
		})
	}))
	defer server.Close()

	result, err := stopDaemon(server.Listener.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if !result.WasRunning {
		t.Fatalf("expected daemon to be stopped")
	}
	if isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("daemon helper still alive")
	}
}

func TestStopDaemon_FindsHealthyDaemonWithoutAuthorityFiles(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	executable := mustExecutableIdentity(t)
	fp, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"daemon": &AuthorityInfo{
				PID:             cmd.Process.Pid,
				APIVersion:      APIVersion,
				HomeFingerprint: fp,
				Executable:      executable,
			},
		})
	}))
	defer server.Close()

	result, err := stopDaemon(server.Listener.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}
	if !result.WasRunning || result.PID != cmd.Process.Pid {
		t.Fatalf("unexpected result: %+v", result)
	}
	if isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("daemon helper still alive")
	}
}

func TestStopDaemon_RefusesHealthyDaemonFromDifferentHome(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cmd := startStopHelper(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"daemon": &AuthorityInfo{
				PID:             cmd.Process.Pid,
				APIVersion:      APIVersion,
				HomeFingerprint: "another-home",
				Executable:      mustExecutableIdentity(t),
			},
		})
	}))
	defer server.Close()

	_, err := stopDaemon(server.Listener.Addr().(*net.TCPAddr).Port)
	if err == nil {
		t.Fatalf("expected refusal")
	}
	if !strings.Contains(err.Error(), "different CLASH4CLI_HOME") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("helper should not have been killed")
	}
}

func TestStopProcessEscalatesToKill(t *testing.T) {
	cmd := startStopHelperEnv(t, "C4CD_STOP_IGNORE_TERM=1")

	start := time.Now()
	if err := stopProcess(cmd.Process.Pid, 300*time.Millisecond, 2*time.Second); err != nil {
		t.Fatalf("stopProcess: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Fatalf("expected a SIGTERM grace period before SIGKILL, took %v", elapsed)
	}
	if isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("expected process to be force-killed")
	}
}

// TestStopDaemonHelperProcess is re-executed as a fake daemon by startStopHelper.
func TestStopDaemonHelperProcess(t *testing.T) {
	if os.Getenv("C4CD_STOP_HELPER") != "1" {
		t.Skip("helper process only")
	}
	readyFile := os.Getenv("C4CD_STOP_READY_FILE")
	if os.Getenv("C4CD_STOP_IGNORE_TERM") == "1" {
		signal.Ignore(syscall.SIGTERM)
		writeReadyFile(readyFile)
		// time.After keeps a timer registered, avoiding the runtime's
		// "all goroutines are asleep - deadlock!" panic.
		<-time.After(24 * time.Hour)
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	writeReadyFile(readyFile)
	<-ch
	os.Exit(0)
}

func writeReadyFile(path string) {
	if path != "" {
		_ = os.WriteFile(path, []byte("ready"), 0o600)
	}
}

func startStopHelper(t *testing.T) *exec.Cmd {
	t.Helper()
	return startStopHelperEnv(t)
}

func startStopHelperEnv(t *testing.T, extraEnv ...string) *exec.Cmd {
	t.Helper()
	readyFile := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=TestStopDaemonHelperProcess")
	cmd.Env = append(append(os.Environ(), "C4CD_STOP_HELPER=1", "C4CD_STOP_READY_FILE="+readyFile), extraEnv...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
		}
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyFile); err == nil {
			return cmd
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("helper process did not become ready")
	return nil
}

func writeStopTestAuthority(t *testing.T, pid, port int, executable string) {
	t.Helper()
	fp, err := HomeFingerprint()
	if err != nil {
		t.Fatalf("HomeFingerprint: %v", err)
	}
	info := &AuthorityInfo{
		PID:             pid,
		Port:            port,
		APIVersion:      APIVersion,
		HomeFingerprint: fp,
		Executable:      executable,
		StartedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := WriteAuthorityInfo(info); err != nil {
		t.Fatalf("WriteAuthorityInfo: %v", err)
	}
}

func mustExecutableIdentity(t *testing.T) string {
	t.Helper()
	exe, err := ExecutableIdentity()
	if err != nil {
		t.Fatalf("ExecutableIdentity: %v", err)
	}
	return exe
}

func authorityInfoPath(t *testing.T) string {
	t.Helper()
	paths, err := GetAuthorityPaths()
	if err != nil {
		t.Fatalf("GetAuthorityPaths: %v", err)
	}
	return paths.InfoPath
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
