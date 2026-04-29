package tui

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/tabs"
)

func TestNew_RestoreActiveSubscriptionAndSelectedNodeFromConfig(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	subMgr := subscription.NewManager()
	sub, err := subMgr.AddManual("sub", "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#TestNode\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.UI.ActiveSubscriptionID = sub.ID
	cfg.UI.SelectedNodeSubID = sub.ID
	cfg.UI.SelectedNodeName = "TestNode"
	cfg.UI.SelectedNodeServer = "example.com"
	cfg.UI.SelectedNodePort = 8388
	cfg.UI.SelectedNodeType = "ss"
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save: %v", err)
	}

	cfg2, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	subMgr2 := subscription.NewManager()
	if err := subMgr2.Load(); err != nil {
		t.Fatalf("subMgr.Load: %v", err)
	}

	nodeMgr := node.NewManager(subMgr2, cfg2.SpeedTest.Concurrent, cfg2.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg2)

	m := New(cfg2, subMgr2, nodeMgr, slotMgr, "dev", "")

	if m.activeSubID != sub.ID {
		t.Fatalf("expected activeSubID=%q, got %q", sub.ID, m.activeSubID)
	}
	if m.selectedNode == nil {
		t.Fatalf("expected selectedNode to be restored")
	}
	if m.selectedNode.Name != "TestNode" || m.selectedNode.Server != "example.com" || m.selectedNode.Port != 8388 {
		t.Fatalf("unexpected selectedNode: %#v", m.selectedNode)
	}

	// The selected node should be restored into model state; the reusable NodeBrowser
	// now consumes that state when opened from Slots or Subscriptions.
}

func TestNew_RestoreFromProxyLockWhenCoreRunning(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	reachable := true
	restoreDial := proxy.SetDialTimeoutForTests(func(network, address string, timeout time.Duration) (net.Conn, error) {
		if !reachable {
			return nil, os.ErrNotExist
		}
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	})
	defer restoreDial()

	subMgr := subscription.NewManager()
	sub, err := subMgr.AddManual("sub", "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#TestNode\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	lock := proxy.LockFile{
		Port:       12345,
		APIPort:    12346,
		PID:        os.Getpid(),
		SubID:      sub.ID,
		Node:       "TestNode",
		NodeServer: "example.com",
		NodePort:   8388,
		NodeType:   "ss",
		ProxyHost:  "127.0.0.1",
		APIHost:    "127.0.0.1",
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		Detached:   true,
	}

	lockPath, err := config.GetLockFilePath()
	if err != nil {
		t.Fatalf("GetLockFilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	b, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(lockPath, b, 0o600); err != nil {
		t.Fatalf("WriteFile(lock): %v", err)
	}

	cfg := config.DefaultConfig()
	subMgr2 := subscription.NewManager()
	if err := subMgr2.Load(); err != nil {
		t.Fatalf("subMgr.Load: %v", err)
	}

	nodeMgr := node.NewManager(subMgr2, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr2, nodeMgr, slotMgr, "dev", "")
	if m.activeSubID != sub.ID {
		t.Fatalf("expected activeSubID=%q, got %q", sub.ID, m.activeSubID)
	}
	if m.selectedNode == nil || m.selectedNode.Name != "TestNode" {
		t.Fatalf("expected selectedNode restored from lock, got %#v", m.selectedNode)
	}

	state := slotMgr.GetSlotState(0)
	if state == nil {
		t.Fatalf("expected slot 0 to be restored from lock file")
	}
	if state.NodeName != "TestNode" {
		t.Fatalf("expected slot 0 node=TestNode, got %q", state.NodeName)
	}

	reachable = false
	time.Sleep(50 * time.Millisecond)
}

func TestModel_PrefersNonCustomSubscriptionByDefault(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	subMgr := subscription.NewManager()
	if err := subMgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	sub, err := subMgr.AddManual("sub", "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#TestNode\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	cfg := config.DefaultConfig()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)
	m := New(cfg, subMgr, nodeMgr, slotMgr, "dev", "")

	subs := subMgr.List()
	sortSubscriptionsByNameAndID(subs)

	model, _ := m.Update(tabs.SubsRefreshedMsg{Subs: subs})
	got := model.(*Model)
	if got.activeSubID != sub.ID {
		t.Fatalf("expected default active subscription to prefer real sub %q, got %q", sub.ID, got.activeSubID)
	}
}
