package proxy

import (
	"fmt"
	"net"
	"os/exec"
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
	configureDetachedCommand(cmd)

	devNull, logFile, err := openProcessFiles(logPath)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = devNull.Close()
		_ = logFile.Close()
		return nil, fmt.Errorf("failed to start mihomo: %w", err)
	}

	_ = devNull.Close()
	_ = logFile.Close()
	return cmd, nil
}

