package tabs

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/node"
	"rocket4cli/internal/subscription"
)

func TestNodesTab_ShowsSlot0IndicatorAndTraffic(t *testing.T) {
	tab := NewNodesTab(node.NewManager(subscription.NewManager(), 1, time.Second))
	tab.SetSize(120, 24)
	tab.nodes = []subscription.Node{{
		Name:   "NodeA",
		Server: "example.com",
		Port:   8388,
	}}

	tab.SetSlotIndicators(map[string]SlotIndicator{
		"NodeA": {SlotID: 0, Icon: "📜"},
	})
	tab.SetSlotTraffic(map[string]string{
		"NodeA": ":17890 ↑1.2K ↓3.4M",
	})

	row := tab.table.SelectedRow()
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

func TestNodesTab_ViewZeroNodes(t *testing.T) {
	tab := NewNodesTab(node.NewManager(subscription.NewManager(), 1, time.Second))
	tab.SetSize(120, 24)

	// Ensure nodes is empty (default)
	view := tab.View()

	if view == "" {
		t.Fatal("expected non-empty view for zero nodes")
	}

	// GetSelectedNode should return nil safely
	if n := tab.GetSelectedNode(); n != nil {
		t.Fatalf("expected nil selected node, got %+v", n)
	}
}

func TestNodesTab_UpdateZeroNodes(t *testing.T) {
	tab := NewNodesTab(node.NewManager(subscription.NewManager(), 1, time.Second))
	tab.SetSize(120, 24)

	// Receive empty nodes refresh
	tab, _ = tab.Update(NodesRefreshedMsg{Nodes: nil})
	if len(tab.nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(tab.nodes))
	}

	// Navigation on empty list should not panic
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
}
