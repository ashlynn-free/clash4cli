package components

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/node"
	"rocket4cli/internal/subscription"
)

func newTestNodeBrowser() *NodeBrowser {
	nm := node.NewManager(subscription.NewManager(), 1, time.Second)
	cfg := config.DefaultConfig()
	sm := subscription.NewManager()
	return NewNodeBrowser(cfg, nm, sm)
}

func TestNodeBrowser_ShowsSlot0IndicatorAndTraffic(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	b.nodes = []subscription.Node{{
		Name:   "NodeA",
		Server: "example.com",
		Port:   8388,
	}}

	b.SetSlotIndicators(map[string]SlotIndicator{
		"NodeA": {SlotID: 0, Icon: "📜"},
	})
	b.SetSlotTraffic(map[string]string{
		"NodeA": ":17890 ↑1.2K ↓3.4M",
	})

	row := b.table.SelectedRow()
	if len(row) != 5 {
		t.Fatalf("expected 5 columns in selected row, got %#v", row)
	}
	if row[0] != "📜" {
		t.Fatalf("expected slot 0 indicator, got %q", row[0])
	}
	if row[4] != ":17890 ↑1.2K ↓3.4M" {
		t.Fatalf("expected slot 0 traffic, got %q", row[4])
	}
}

func TestNodeBrowser_ViewZeroNodes(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	view := b.View()
	if view == "" {
		t.Fatal("expected non-empty view for zero nodes")
	}
}

func TestNodeBrowser_PickModeSelected(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, PickMode)

	b.nodes = []subscription.Node{
		{Name: "NodeA", Server: "a.example.com", Port: 8388},
		{Name: "NodeB", Server: "b.example.com", Port: 8388},
	}
	b.updateTableRows()

	// Press enter on first node.
	event, result, _ := b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if event != EventSelected {
		t.Fatalf("expected EventSelected, got %d", event)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Node.Name != "NodeA" {
		t.Fatalf("expected NodeA, got %s", result.Node.Name)
	}
	if result.SubID != "sub1" {
		t.Fatalf("expected sub1, got %s", result.SubID)
	}
}

func TestNodeBrowser_BrowseModeEnterNoOp(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	b.nodes = []subscription.Node{
		{Name: "NodeA", Server: "a.example.com", Port: 8388},
	}
	b.updateTableRows()

	event, _, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{13}})
	if event != EventNone {
		t.Fatalf("expected EventNone in browse mode on Enter, got %d", event)
	}
}

func TestNodeBrowser_EscCloses(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	event, _, _ := b.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if event != EventClosed {
		t.Fatalf("expected EventClosed, got %d", event)
	}
}

func TestNodeBrowser_NavigationNoPanic(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	// Navigation on empty list should not panic.
	b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
}

func TestNodeBrowser_RefreshMessage(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	// Receive empty nodes refresh.
	event, _, _ := b.Update(nodeRefreshedMsg{Nodes: nil})
	if event != EventNone {
		t.Fatalf("expected EventNone, got %d", event)
	}
	if len(b.nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(b.nodes))
	}
}

func TestNodeBrowser_BlocksGlobalKeys(t *testing.T) {
	b := newTestNodeBrowser()
	if b.BlocksGlobalKeys() {
		t.Fatal("should not block when inactive")
	}
	b.Open("sub1", nil, BrowseMode)
	if !b.BlocksGlobalKeys() {
		t.Fatal("should block when active")
	}
	b.Close()
	if b.BlocksGlobalKeys() {
		t.Fatal("should not block after close")
	}
}

func TestNodeBrowser_PreselectNode(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)

	preselect := &subscription.Node{
		Name:   "NodeB",
		Server: "b.example.com",
		Port:   9090,
	}

	b.Open("sub1", preselect, BrowseMode)

	// Simulate nodes arriving via refresh.
	b.Update(nodeRefreshedMsg{Nodes: []subscription.Node{
		{Name: "NodeA", Server: "a.example.com", Port: 8080},
		{Name: "NodeB", Server: "b.example.com", Port: 9090},
		{Name: "NodeC", Server: "c.example.com", Port: 7070},
	}})

	// Preselect should have moved cursor to NodeB.
	if b.table.Selected() != 1 {
		t.Fatalf("expected cursor on index 1 (NodeB), got %d", b.table.Selected())
	}
}

func TestNodeBrowser_ServerDisplayBlur(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	b.nodes = []subscription.Node{
		{Name: "NodeA", Server: "longserver.example.com", Port: 8388},
	}
	b.updateTableRows()

	row := b.table.SelectedRow()
	if len(row) < 3 {
		t.Fatal("expected at least 3 columns with server visible")
	}

	// Blur mode.
	b.SetServerDisplay("blur")
	row = b.table.SelectedRow()
	// Server should be masked.
	server := row[2]
	if server == "longserver.example.com:8388" {
		t.Fatal("expected blurred server, got plain")
	}
}

func TestNodeBrowser_ScrollStatePreserved(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	b.nodes = make([]subscription.Node, 20)
	for i := range b.nodes {
		b.nodes[i] = subscription.Node{
			Name:   fmt.Sprintf("Node%02d", i),
			Server: fmt.Sprintf("%d.example.com", i),
			Port:   8080 + i,
		}
	}
	b.updateTableRows()

	// Move cursor to index 5.
	for i := 0; i < 5; i++ {
		b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if b.table.Selected() != 5 {
		t.Fatalf("expected cursor at 5, got %d", b.table.Selected())
	}

	// Switch subscription away and back.
	b.scrollStates["sub1"] = scrollState{selected: b.table.Selected(), offset: b.table.Offset()}
	b.subscriptionID = "sub2"
	b.nodes = nil
	b.updateTableRows()

	// Switch back.
	b.subscriptionID = "sub1"
	b.nodes = make([]subscription.Node, 20)
	for i := range b.nodes {
		b.nodes[i] = subscription.Node{
			Name:   fmt.Sprintf("Node%02d", i),
			Server: fmt.Sprintf("%d.example.com", i),
			Port:   8080 + i,
		}
	}
	b.updateTableRows()

	if ss, ok := b.scrollStates["sub1"]; ok {
		b.table.SetSelected(ss.selected)
		b.table.SetOffset(ss.offset)
	}

	if b.table.Selected() != 5 {
		t.Fatalf("expected cursor restored to 5, got %d", b.table.Selected())
	}
}

func TestNodeBrowser_DetailView(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	b.nodes = []subscription.Node{
		{Name: "NodeA", Server: "a.example.com", Port: 8388},
	}
	b.updateTableRows()

	// Open detail view with 'i'.
	b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !b.showDetail {
		t.Fatal("expected detail view to be open")
	}

	// View should contain detail content.
	view := b.View()
	if view == "" {
		t.Fatal("expected non-empty detail view")
	}
}

func TestNodeBrowser_ShowsTestingImmediately(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)

	b.nodes = []subscription.Node{{
		Name:   "NodeA",
		Server: "example.com",
		Port:   8388,
	}}
	b.pending = map[string]struct{}{nodeTestKey(&b.nodes[0]): {}}
	b.updateTableRows()

	row := b.table.SelectedRow()
	if len(row) < 5 {
		t.Fatalf("unexpected row: %#v", row)
	}
	if row[3] != "..." {
		t.Fatalf("latency=%q, want \"...\"", row[3])
	}
	if row[4] != i18n.M().Nodes.Testing {
		t.Fatalf("status=%q, want %q", row[4], i18n.M().Nodes.Testing)
	}
}

func TestNodeBrowser_TKeyTestsOnlySelectedNode(t *testing.T) {
	b := newTestNodeBrowser()
	b.SetSize(120, 24)
	b.Open("sub1", nil, BrowseMode)
	b.nodes = []subscription.Node{
		{Name: "NodeA", Server: "a.example.com", Port: 8388},
		{Name: "NodeB", Server: "b.example.com", Port: 8389},
	}
	b.updateTableRows()
	b.table.SetSelected(0)

	_, _, cmd := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if cmd == nil {
		t.Fatalf("expected a test command")
	}
	if _, ok := b.pending[nodeTestKey(&b.nodes[0])]; !ok {
		t.Fatalf("expected selected node to be pending")
	}
	if _, ok := b.pending[nodeTestKey(&b.nodes[1])]; ok {
		t.Fatalf("expected non-selected node NOT to be pending")
	}
}
