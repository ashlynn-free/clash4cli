//go:build unix

package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var errInstanceAlreadyRunning = errors.New("another c4c instance is already running")

type instanceLock struct {
	path string
	file *os.File
}

func (l *instanceLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
		_ = l.file.Close()
	}
	_ = os.Remove(l.path)
}

func tryAcquireInstanceLock(lockPath string) (*instanceLock, error) {
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errInstanceAlreadyRunning
		}
		return nil, err
	}

	// Record our PID for humans and for the next process to display.
	if err := f.Truncate(0); err == nil {
		_, _ = f.Seek(0, 0)
		_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
		_ = f.Sync()
	}

	return &instanceLock{path: lockPath, file: f}, nil
}

func readInstancePID(lockPath string) int {
	f, err := os.Open(lockPath)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}

func killProcess(pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return fmt.Errorf("refusing to kill pid %d", pid)
	}

	// Try graceful shutdown first.
	_ = unix.Kill(pid, unix.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill.
	_ = unix.Kill(pid, unix.SIGKILL)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("process %d did not exit", pid)
}

func askYesNo(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}

func ensureSingleInstanceInteractive(lockPath string) (*instanceLock, error) {
	lock, err := tryAcquireInstanceLock(lockPath)
	if err == nil {
		return lock, nil
	}
	if !errors.Is(err, errInstanceAlreadyRunning) {
		return nil, err
	}

	pid := readInstancePID(lockPath)
	if pid > 0 {
		if !askYesNo(fmt.Sprintf("检测到已有 c4c 实例正在运行 (PID %d)。是否结束它并继续启动？[y/N]: ", pid)) {
			return nil, errInstanceAlreadyRunning
		}
		if err := killProcess(pid); err != nil {
			return nil, fmt.Errorf("failed to kill pid %d: %w", pid, err)
		}
	} else {
		if !askYesNo("检测到已有 c4c 实例正在运行。是否结束它并继续启动？[y/N]: ") {
			return nil, errInstanceAlreadyRunning
		}
		return nil, fmt.Errorf("cannot determine existing c4c pid from lock file: %s", lockPath)
	}

	// Retry acquiring the lock after the previous process exits.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		lock, err = tryAcquireInstanceLock(lockPath)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, errInstanceAlreadyRunning) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("failed to acquire instance lock after killing pid %d", pid)
}
