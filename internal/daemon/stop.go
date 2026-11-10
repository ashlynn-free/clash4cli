package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// stopGracefulTimeout is how long a daemon gets to exit after SIGTERM.
	stopGracefulTimeout = 1 * time.Second
	stopKillTimeout     = 1 * time.Second
	stopHealthTimeout   = 2 * time.Second
)

// StopResult reports what StopDaemon did.
type StopResult struct {
	WasRunning bool
	PID        int
}

// StopDaemon ensures no c4cd process is running for the current CLASH4CLI_HOME.
// It is idempotent: when nothing is running it returns WasRunning=false.
//
// The target PID is verified before signalling (health endpoint or executable
// identity) so a stale pid file can never kill an unrelated process.
func StopDaemon() (*StopResult, error) {
	return stopDaemon(defaultPort)
}

func stopDaemon(probePort int) (*StopResult, error) {
	// A stale/corrupt authority file must not prevent --stop from finding a
	// healthy daemon; fall back to the health probe below.
	info, _ := ReadAuthorityInfo()

	pid := 0
	if info != nil {
		pid = info.PID
	} else {
		pid = ReadDaemonPID()
	}

	// Authority files can be missing even while a healthy daemon is running
	// (e.g. they were removed manually or a crash raced cleanup). Fall back to
	// probing the fixed daemon port so --stop still finds it.
	if info == nil || pid <= 0 || !isProcessAlive(pid) {
		live := probeDaemonHealth(probePort)
		if live == nil || live.PID <= 0 || !isProcessAlive(live.PID) {
			_ = CleanupStaleAuthorityFiles()
			return &StopResult{}, nil
		}
		fingerprint, err := HomeFingerprint()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(live.HomeFingerprint) != "" && live.HomeFingerprint != fingerprint {
			return nil, fmt.Errorf("a c4cd for a different CLASH4CLI_HOME is running on port %d (pid %d); refusing to stop it", probePort, live.PID)
		}
		info = live
		pid = live.PID
	}

	fingerprint, err := HomeFingerprint()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(info.HomeFingerprint) != "" && info.HomeFingerprint != fingerprint {
		return nil, fmt.Errorf("refusing to stop pid %d: authority info belongs to a different CLASH4CLI_HOME", pid)
	}

	if !daemonRespondsAsPID(info, pid) && !processMatchesExecutable(pid, info.Executable) {
		return nil, fmt.Errorf("refusing to stop pid %d: process does not match the c4cd authority info (%s)", pid, info.Executable)
	}

	if err := stopProcess(pid, stopGracefulTimeout, stopKillTimeout); err != nil {
		return nil, err
	}
	_ = CleanupStaleAuthorityFiles()
	return &StopResult{WasRunning: true, PID: pid}, nil
}

// probeDaemonHealth asks a daemon on port for its identity. It deliberately
// does not validate the API version, so an older daemon can still be stopped.
func probeDaemonHealth(port int) *AuthorityInfo {
	if port <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopHealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/health", port), nil)
	if err != nil {
		return nil
	}
	resp, err := (&http.Client{Timeout: stopHealthTimeout}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var out struct {
		Daemon *AuthorityInfo `json:"daemon"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	if out.Daemon == nil || out.Daemon.PID <= 0 {
		return nil
	}
	return out.Daemon
}

// daemonRespondsAsPID asks the daemon's health endpoint whether it is the
// process we intend to stop.
func daemonRespondsAsPID(info *AuthorityInfo, pid int) bool {
	if info == nil {
		return false
	}
	live := probeDaemonHealth(info.Port)
	return live != nil && live.PID == pid
}

// processMatchesExecutable verifies that pid runs the expected executable.
// Linux uses /proc/<pid>/exe; elsewhere `ps -o comm=` is used.
func processMatchesExecutable(pid int, want string) bool {
	if pid <= 0 || strings.TrimSpace(want) == "" {
		return false
	}
	got := processExecutable(pid)
	if got == "" {
		return false
	}
	got = resolveCleanPath(strings.TrimSuffix(got, " (deleted)"))
	want = resolveCleanPath(want)
	if got == want {
		return true
	}
	// `ps -o comm=` on Linux reports only the basename (often truncated).
	return filepath.Base(got) == filepath.Base(want)
}

func processExecutable(pid int) string {
	if runtime.GOOS == "linux" {
		if p, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
			return p
		}
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func resolveCleanPath(path string) string {
	path = strings.TrimSpace(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

// stopProcess sends SIGTERM, waits, then escalates to SIGKILL.
func stopProcess(pid int, graceful, killWait time.Duration) error {
	if pid <= 0 || pid == os.Getpid() {
		return fmt.Errorf("refusing to stop pid %d", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		switch {
		case errors.Is(err, syscall.ESRCH):
			return nil
		case errors.Is(err, syscall.EPERM):
			return fmt.Errorf("permission denied while stopping pid %d; if c4cd runs as root, retry with sudo", pid)
		default:
			return fmt.Errorf("failed to signal daemon pid %d: %w", pid, err)
		}
	}
	if waitProcessExit(pid, graceful) {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("failed to force-kill daemon pid %d: %w", pid, err)
	}
	if waitProcessExit(pid, killWait) {
		return nil
	}
	return fmt.Errorf("daemon pid %d did not exit", pid)
}

func waitProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !isProcessAlive(pid)
}
