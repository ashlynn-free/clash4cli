package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os/exec"

	"rocket4cli/internal/logger"
	"time"
)

var dialTimeout = net.DialTimeout

// SetDialTimeoutForTests overrides the dialer used by internal TCP reachability checks.
// Intended for unit tests only.
func SetDialTimeoutForTests(fn func(network, address string, timeout time.Duration) (net.Conn, error)) (restore func()) {
	prev := dialTimeout
	if fn == nil {
		dialTimeout = net.DialTimeout
	} else {
		dialTimeout = fn
	}
	return func() { dialTimeout = prev }
}

var findMihomoBinaryFunc = findMihomoBinaryDefault

// SetFindMihomoBinaryForTests overrides the mihomo binary lookup.
// Intended for unit tests only.
func SetFindMihomoBinaryForTests(fn func() (string, error)) (restore func()) {
	prev := findMihomoBinaryFunc
	if fn == nil {
		findMihomoBinaryFunc = findMihomoBinaryDefault
	} else {
		findMihomoBinaryFunc = fn
	}
	return func() { findMihomoBinaryFunc = prev }
}

var detachSubprocesses = true

// SetDetachSubprocesses controls whether spawned mihomo processes are detached from the parent process.
func SetDetachSubprocesses(detach bool) {
	detachSubprocesses = detach
}

// startMihomoProcess creates and starts the mihomo subprocess.
// Tests can override this to avoid spawning a real binary.
var startMihomoProcess = defaultStartMihomoProcess

// SetStartMihomoProcessForTests overrides the mihomo process spawner.
// Intended for unit tests only.
func SetStartMihomoProcessForTests(fn func(mihomoPath, workDir, configPath, logPath string) (*exec.Cmd, error)) (restore func()) {
	prev := startMihomoProcess
	if fn == nil {
		startMihomoProcess = defaultStartMihomoProcess
	} else {
		startMihomoProcess = fn
	}
	return func() { startMihomoProcess = prev }
}

var probeLatencyFunc = probeLatencyViaProxy

// SetProbeLatencyForTests overrides the HTTP latency probe.
// Intended for unit tests only.
func SetProbeLatencyForTests(fn func(proxyHost string, proxyPort int, testURL string, timeout time.Duration) (int64, error)) (restore func()) {
	prev := probeLatencyFunc
	if fn == nil {
		probeLatencyFunc = probeLatencyViaProxy
	} else {
		probeLatencyFunc = fn
	}
	return func() { probeLatencyFunc = prev }
}

func defaultStartMihomoProcess(mihomoPath, workDir, configPath, logPath string) (*exec.Cmd, error) {
	cmd := exec.Command(mihomoPath, "-d", workDir, "-f", configPath)
	cmd.Dir = workDir
	if detachSubprocesses {
		configureDetachedCommand(cmd)
	}

	devNull, logFile, err := openProcessFiles(logPath)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = devNull

	if detachSubprocesses {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			_ = devNull.Close()
			_ = logFile.Close()
			return nil, fmt.Errorf("failed to start mihomo: %w", err)
		}
		_ = devNull.Close()
		_ = logFile.Close()
	} else {
		// Daemon mode: read mihomo stdout/stderr through pipes so output
		// appears in both the log file and the SSE log stream.
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			_ = devNull.Close()
			_ = logFile.Close()
			return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			_ = devNull.Close()
			_ = logFile.Close()
			return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}
		if err := cmd.Start(); err != nil {
			_ = devNull.Close()
			_ = logFile.Close()
			return nil, fmt.Errorf("failed to start mihomo: %w", err)
		}
		_ = devNull.Close()
		// Keep logFile open until pipe readers finish (child exit).
		go pipeMihomoToLogAndLogger(stdoutPipe, logFile)
		go pipeMihomoToLogAndLogger(stderrPipe, logFile)
	}

	return cmd, nil
}

// pipeMihomoToLogAndLogger reads lines from r, writes them to the log file,
// and publishes them via the logger. logFile is closed when r is exhausted.
func pipeMihomoToLogAndLogger(r io.ReadCloser, logFile io.WriteCloser) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = logFile.Write([]byte(line + "\n"))
		logger.Info("[mihomo] %s", line)
	}
	_ = r.Close()
	_ = logFile.Close()
}

