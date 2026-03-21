package tabs

import (
	"testing"
	"time"

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
	if row[0] != "📜0" {
		t.Fatalf("expected slot 0 indicator, got %q", row[0])
	}
	if row[4] != ":17890 ↑1.2K ↓3.4M" {
		t.Fatalf("expected slot 0 traffic, got %q", row[4])
	}
}
