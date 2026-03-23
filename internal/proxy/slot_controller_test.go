package proxy

import (
	"net"
	"os/exec"
	"strconv"
	"testing"

	"rocket4cli/internal/config"
)

func TestStartSlotWithMode_DirectOnly_NoMihomo(t *testing.T) {
	setupTestHome(t)

	// Mock listenTCP so port checks pass without binding real ports
	prev := listenTCP
	defer func() { listenTCP = prev }()
	listenTCP = func(network, address string) (net.Listener, error) {
		_, portStr, _ := net.SplitHostPort(address)
		p, _ := strconv.Atoi(portStr)
		return &fakeListener{addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p}}, nil
	}

	// Mock findMihomoBinary to return a fake path
	restoreFMB := SetFindMihomoBinaryForTests(func() (string, error) {
		return "/fake/mihomo", nil
	})
	defer restoreFMB()

	// Mock startMihomoProcess to avoid real exec — use "sleep" as a no-op process
	var capturedConfigPath string
	restoreSMP := SetStartMihomoProcessForTests(func(mihomoPath, workDir, cfgPath, logPath string) (*exec.Cmd, error) {
		capturedConfigPath = cfgPath
		cmd := exec.Command("sleep", "10")
		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start stub process: %v", err)
		}
		return cmd, nil
	})
	defer restoreSMP()

	cfg := config.DefaultConfig()
	c := &Controller{
		status:            StatusStopped,
		logCh:             make(chan LogEntry, 100),
		cfg:               cfg,
		apiHost:           "127.0.0.1",
		proxyHost:         "127.0.0.1",
		lockSlotID:        0,
		multiInstanceLock: true,
	}

	// Start with nil nodes = direct-only mode
	err := c.StartSlotWithMode(0, "", nil, nil, nil, "rule")
	if err != nil {
		t.Fatalf("StartSlotWithMode direct-only: %v", err)
	}
	defer func() {
		c.mu.Lock()
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
		c.status = StatusStopped
		c.mu.Unlock()
	}()

	if c.status != StatusRunning {
		t.Fatalf("expected StatusRunning, got %v", c.status)
	}
	if c.port == 0 {
		t.Fatal("expected non-zero port")
	}
	if c.apiPort == 0 {
		t.Fatal("expected non-zero api port")
	}
	if capturedConfigPath == "" {
		t.Fatal("expected config file to be written")
	}
}
