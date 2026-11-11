//go:build unix

package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var (
	errAuthorityAlreadyRunning     = errors.New("another c4cd instance is already running")
	ErrDaemonAlreadyRunningHealthy = errors.New("a healthy c4cd is already running")
)

type authorityLock struct {
	path string
	file *os.File
}

func (l *authorityLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
		_ = l.file.Close()
	}
	_ = os.Remove(l.path)
}

func tryAcquireAuthorityLock(lockPath string) (*authorityLock, error) {
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errAuthorityAlreadyRunning
		}
		return nil, err
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.Seek(0, 0)
		_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
		_ = f.Sync()
	}
	return &authorityLock{path: lockPath, file: f}, nil
}

func ensureSingleAuthority(paths *AuthorityPaths, probePort int) (*authorityLock, error) {
	// The authority lock file can go missing while a daemon is still running
	// (e.g. after orphan recovery), so a successful flock alone is not proof
	// that no daemon owns the port. Probe the health endpoint first.
	if probePort > 0 {
		if live := probeDaemonHealth(probePort); live != nil && live.PID > 0 && isProcessAlive(live.PID) {
			fingerprint, fpErr := HomeFingerprint()
			if fpErr == nil && strings.TrimSpace(live.HomeFingerprint) != "" && live.HomeFingerprint != fingerprint {
				return nil, fmt.Errorf("%w (pid %d, port %d)", errDaemonDifferentHome, live.PID, probePort)
			}
			return nil, fmt.Errorf("%w (pid %d, port %d); use 'c4cd --restart' to replace it", ErrDaemonAlreadyRunningHealthy, live.PID, probePort)
		}
	}

	lock, err := tryAcquireAuthorityLock(paths.LockPath)
	if err == nil {
		return lock, nil
	}
	if !errors.Is(err, errAuthorityAlreadyRunning) {
		return nil, err
	}

	pid := ReadDaemonPID()
	if pid <= 0 {
		return nil, fmt.Errorf("cannot determine existing daemon pid from %s", paths.PIDPath)
	}
	info, err := ReadAuthorityInfo()
	if err != nil {
		return nil, fmt.Errorf("cannot validate existing authority info: %w", err)
	}
	expectedFP, err := HomeFingerprint()
	if err != nil {
		return nil, err
	}
	expectedExe, err := ExecutableIdentity()
	if err != nil {
		return nil, err
	}
	if err := MatchesExpectedIdentity(info, expectedFP, expectedExe); err != nil {
		return nil, fmt.Errorf("refusing daemon takeover for pid %d: %w", pid, err)
	}
	if isProcessAlive(pid) {
		// A healthy daemon is not taken over silently. Replacing it requires an
		// explicit `c4cd --restart` (or --stop) so a running c4c session is not
		// disconnected by accident.
		if daemonRespondsAsPID(info, pid) {
			return nil, fmt.Errorf("%w (pid %d, port %d); use 'c4cd --restart' to replace it", ErrDaemonAlreadyRunningHealthy, pid, info.Port)
		}
		if err := terminateExistingProcess(pid, 3*time.Second); err != nil {
			return nil, fmt.Errorf("failed to kill pid %d: %w", pid, err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		lock, err = tryAcquireAuthorityLock(paths.LockPath)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, errAuthorityAlreadyRunning) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed to acquire daemon lock after takeover of pid %d", pid)
}

func terminateExistingProcess(pid int, timeout time.Duration) error {
	if pid <= 0 || pid == os.Getpid() {
		return fmt.Errorf("refusing to kill pid %d", pid)
	}
	_ = unix.Kill(pid, unix.SIGTERM)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = unix.Kill(pid, unix.SIGKILL)
	deadline = time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("process %d did not exit", pid)
}
