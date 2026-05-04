//go:build unix

package daemon

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

var errAuthorityAlreadyRunning = errors.New("another c4cd instance is already running")

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

func ensureSingleAuthority(paths *AuthorityPaths) (*authorityLock, error) {
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
