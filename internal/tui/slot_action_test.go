package tui

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/tabs"
)

func TestResolveSlotActionPrefersSwitchForDifferentActiveNode(t *testing.T) {
	nodeA := &subscription.Node{Name: "n1", Server: "a.example.com", Port: 443}
	nodeB := &subscription.Node{Name: "n2", Server: "b.example.com", Port: 443}

	if got := resolveSlotAction(nil, nil, "sub1", nodeA); got != slotActionQueueGlobal {
		t.Fatalf("expected first inactive press to queue global, got %v", got)
	}

	pending := pendingSlotAction{
		slotID: 0,
		seq:    1,
		subID:  "sub1",
		node:   *nodeB,
	}
	if got := resolveSlotAction(nil, &pending, "sub1", nodeB); got != slotActionStartRule {
		t.Fatalf("expected second matching press to start rule, got %v", got)
	}

	activeSame := &proxy.SlotState{SlotID: 0, NodeName: nodeB.Name, SubID: "sub1", Mode: "global"}
	if got := resolveSlotAction(activeSame, &pending, "sub1", nodeB); got != slotActionStop {
		t.Fatalf("expected active same-node press to stop, got %v", got)
	}

	activeDifferent := &proxy.SlotState{SlotID: 0, NodeName: nodeA.Name, SubID: "sub1", Mode: "global"}
	if got := resolveSlotAction(activeDifferent, nil, "sub1", nodeB); got != slotActionQueueGlobal {
		t.Fatalf("expected active different-node press to queue global, got %v", got)
	}
	if got := resolveSlotAction(activeDifferent, &pending, "sub1", nodeB); got != slotActionStartRule {
		t.Fatalf("expected active different-node double press to start rule, got %v", got)
	}
}

func TestHandleSlotActionActiveDifferentNodeQueuesThenPromotesDoublePress(t *testing.T) {
	m := newSlotActionTestModel(t)

	if cmd := m.handleSlotAction(0); cmd == nil {
		t.Fatalf("expected first press to queue a pending slot switch")
	}
	pending, ok := m.pendingSlots[0]
	if !ok {
		t.Fatalf("expected first press to queue pending slot action")
	}
	if pending.node.Name != "NodeB" {
		t.Fatalf("expected pending slot target NodeB, got %#v", pending)
	}

	if cmd := m.handleSlotAction(0); cmd == nil {
		t.Fatalf("expected second press to return an immediate start command")
	}
	if _, ok := m.pendingSlots[0]; ok {
		t.Fatalf("expected second press to clear pending slot action")
	}
}

func TestSlotActionCommitMsgStartsPendingEvenWhenSlotStillActive(t *testing.T) {
	m := newSlotActionTestModel(t)

	if cmd := m.handleSlotAction(0); cmd == nil {
		t.Fatalf("expected first press to queue a pending slot switch")
	}
	pending, ok := m.pendingSlots[0]
	if !ok {
		t.Fatalf("expected pending slot action after first press")
	}

	model, cmd := m.Update(slotActionCommitMsg{slotID: 0, seq: pending.seq})
	if cmd == nil {
		t.Fatalf("expected commit to start the pending slot action")
	}

	got := model.(*Model)
	if _, ok := got.pendingSlots[0]; ok {
		t.Fatalf("expected commit to clear pending slot action")
	}
}

func newSlotActionTestModel(t *testing.T) *Model {
	t.Helper()
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	subMgr := subscription.NewManager()
	sub, err := subMgr.AddManual("sub", ""+
		"ss://YWVzLTEyOC1nY206cGFzcw==@a.example.com:8388#NodeA\n"+
		"ss://YWVzLTEyOC1nY206cGFzcw==@b.example.com:8389#NodeB\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	nodes := subMgr.GetNodesForSubscription(sub.ID)
	if len(nodes) < 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	lockPath, err := config.GetLockFilePath()
	if err != nil {
		t.Fatalf("GetLockFilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	addr := ln.Addr().(*net.TCPAddr)
	lock := proxy.MultiLockFile{
		Version: 2,
		Instances: map[int]proxy.LockFile{
			0: {
				Port:       addr.Port,
				APIPort:    addr.Port + 1,
				PID:        os.Getpid(),
				SubID:      sub.ID,
				Node:       nodes[0].Name,
				NodeServer: nodes[0].Server,
				NodePort:   nodes[0].Port,
				NodeType:   nodes[0].Type,
				ProxyHost:  "127.0.0.1",
				APIHost:    "127.0.0.1",
			},
		},
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile(lock): %v", err)
	}

	cfg := config.DefaultConfig()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)
	m := New(cfg, subMgr, nodeMgr, slotMgr, "dev", "")
	time.Sleep(100 * time.Millisecond)
	m.activeTab = TabNodes
	m.activeSubID = sub.ID
	m.nodesTab.SetSubscriptionID(sub.ID)
	m.nodesTab.SetPreselect(&nodes[1])
	m.nodesTab, _ = m.nodesTab.Update(tabs.NodesRefreshedMsg{Nodes: nodes})

	selected := m.nodesTab.GetSelectedNode()
	if selected == nil || selected.Name != nodes[1].Name {
		t.Fatalf("expected selected node NodeB, got %#v", selected)
	}

	return m
}

var _ tea.Model = (*Model)(nil)
