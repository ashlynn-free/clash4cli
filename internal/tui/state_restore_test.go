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
	proxyCtrl := proxy.NewController(cfg2)

	m := New(cfg2, subMgr2, nodeMgr, proxyCtrl, "dev", "")

	if m.activeSubID != sub.ID {
		t.Fatalf("expected activeSubID=%q, got %q", sub.ID, m.activeSubID)
	}
	if m.selectedNode == nil {
		t.Fatalf("expected selectedNode to be restored")
	}
	if m.selectedNode.Name != "TestNode" || m.selectedNode.Server != "example.com" || m.selectedNode.Port != 8388 {
		t.Fatalf("unexpected selectedNode: %#v", m.selectedNode)
	}

	// After a node refresh, the preselect should land on the expected node in the table.
	nodes := subMgr2.GetNodesForSubscription(sub.ID)
	m.nodesTab, _ = m.nodesTab.Update(tabs.NodesRefreshedMsg{Nodes: nodes})
	n := m.nodesTab.GetSelectedNode()
	if n == nil || n.Name != "TestNode" {
		t.Fatalf("expected Nodes tab selected node to be TestNode, got %#v", n)
	}
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
	proxyCtrl := proxy.NewController(cfg)
	if proxyCtrl.GetStatus() != proxy.StatusRunning {
		t.Fatalf("expected proxy controller to detect running core from proxy.lock")
	}

	m := New(cfg, subMgr2, nodeMgr, proxyCtrl, "dev", "")
	if m.activeSubID != sub.ID {
		t.Fatalf("expected activeSubID=%q, got %q", sub.ID, m.activeSubID)
	}
	if m.selectedNode == nil || m.selectedNode.Name != "TestNode" {
		t.Fatalf("expected selectedNode restored from lock, got %#v", m.selectedNode)
	}
	if m.connectedNodeName != "TestNode" {
		t.Fatalf("expected connectedNodeName=TestNode, got %q", m.connectedNodeName)
	}
	if m.proxyMode != ProxyModeRule {
		t.Fatalf("expected proxyMode=Rule, got %v", m.proxyMode)
	}

	// Stop "alive" detection and wait briefly for the monitor goroutine to exit.
	reachable = false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if proxyCtrl.GetStatus() != proxy.StatusRunning {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected proxy controller to stop after listener closed")
}
