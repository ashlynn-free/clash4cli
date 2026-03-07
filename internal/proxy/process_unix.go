//go:build unix

package proxy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func configureDetachedCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Detach from the controlling terminal/session so that quitting c4c does not
	// send signals (e.g. Ctrl+C) to the proxy core.
	cmd.SysProcAttr.Setsid = true
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func terminateProcess(pid int, timeout time.Duration) error {
	if pid <= 0 || pid == os.Getpid() {
		return fmt.Errorf("refusing to kill pid %d", pid)
	}

	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = syscall.Kill(pid, syscall.SIGKILL)
	deadline = time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("process %d did not exit", pid)
}

func readProcessCommandLine(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid pid: %d", pid)
	}

	// Linux: /proc is reliable and fast.
	if data, err := os.ReadFile(filepath.Join("/proc", fmt.Sprintf("%d", pid), "cmdline")); err == nil && len(data) > 0 {
		// /proc/<pid>/cmdline is NUL-separated.
		s := strings.ReplaceAll(string(data), "\x00", " ")
		return strings.TrimSpace(s), nil
	}

	// macOS: fall back to ps.
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command=").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ps failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// findPIDOnPort uses lsof to find the PID of the process listening on a TCP port.
// Returns 0 if no process is found or on error.
func findPIDOnPort(port int) int {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", port)).CombinedOutput()
	if err != nil || len(out) == 0 {
		return 0
	}
	// lsof may return multiple PIDs; take the first one.
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	var pid int
	if _, err := fmt.Sscanf(line, "%d", &pid); err != nil {
		return 0
	}
	return pid
}

// killStaleProcessOnPort attempts to kill a stale mihomo process occupying the given port.
// Returns true if the port was successfully freed.
func killStaleProcessOnPort(port int) bool {
	pid := findPIDOnPort(port)
	if pid <= 0 {
		return false
	}
	if !processLooksLikeMihomo(pid, "") {
		return false
	}
	if err := terminateProcess(pid, 2*time.Second); err != nil {
		return false
	}
	return true
}

func processLooksLikeMihomo(pid int, configPath string) bool {
	cmdline, err := readProcessCommandLine(pid)
	if err != nil || cmdline == "" {
		return false
	}
	lower := strings.ToLower(cmdline)
	if !strings.Contains(lower, "mihomo") && !strings.Contains(lower, "clash") {
		return false
	}
	if configPath == "" {
		return true
	}
	abs, err := filepath.Abs(configPath)
	if err != nil {
		abs = configPath
	}
	return strings.Contains(cmdline, abs) || strings.Contains(cmdline, configPath)
}
