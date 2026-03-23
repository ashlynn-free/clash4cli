package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

func TestSlotsTab_SpaceToggleReturnsRequest(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	updated, cmd := tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if cmd == nil {
		t.Fatalf("expected toggle command")
	}

	msg := cmd()
	req, ok := msg.(SlotToggleRequestMsg)
	if !ok {
		t.Fatalf("expected SlotToggleRequestMsg, got %T", msg)
	}
	if req.SlotID != 0 {
		t.Fatalf("expected slot 0, got %d", req.SlotID)
	}

	if updated.view != slotsViewList {
		t.Fatalf("expected list view to remain active")
	}
}

func TestSlotsTab_SelectProxyPersistsNodeRef(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	subMgr := subscription.NewManager()
	sub, err := subMgr.AddManual("sub", "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#NodeA\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	cfg := config.DefaultConfig()
	tab := NewSlotsTab(cfg, subMgr, nil, nil)
	tab.SetSize(120, 30)

	// Default focus is on Proxy; Enter opens picker directly
	if tab.detailCursor != cardFocusProxy {
		t.Fatalf("expected default cursor on proxy, got %d", tab.detailCursor)
	}

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	if tab.view != slotsViewProxyPicker {
		t.Fatalf("expected proxy picker, got %v", tab.view)
	}

	// Select first node
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	if tab.view != slotsViewList {
		t.Fatalf("expected to return to list after save, got %v", tab.view)
	}

	ref, ok := cfg.GetSlotNodeRef(0)
	if !ok {
		t.Fatalf("expected slot node ref to be saved")
	}
	if ref.SubID != sub.ID || ref.NodeName != "NodeA" || ref.NodeServer != "example.com" || ref.NodePort != 8388 {
		t.Fatalf("unexpected slot node ref: %#v", ref)
	}
}

func TestSlotsTab_ListViewScrollsSelectedCardIntoView(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	tab.SetSize(90, 16)
	tab.slotListIndex = 8

	view := tab.View()
	if !strings.Contains(view, "Slot 8") {
		t.Fatalf("expected view to include selected slot 8")
	}
	if strings.Contains(view, "Slot 0") {
		t.Fatalf("expected scrolled view not to include slot 0")
	}
}

func TestSlotsTab_ViewListNeverExceedsHeight(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	for _, height := range []int{12, 16, 24, 40, 80} {
		tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
		tab.SetSize(90, height)
		view := tab.View()
		lines := strings.Split(view, "\n")
		if len(lines) > height {
			t.Errorf("height=%d: rendered %d lines, exceeds budget", height, len(lines))
		}
	}
}

func TestSlotsTab_UnifiedNavigation(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	// Default focus on proxy
	if tab.detailCursor != cardFocusProxy {
		t.Fatalf("expected default focus on proxy, got %d", tab.detailCursor)
	}

	// Right → Upstream
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tab.detailCursor != cardFocusUpstream {
		t.Fatalf("expected upstream after right, got %d", tab.detailCursor)
	}

	// Right again → clamp at upstream
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tab.detailCursor != cardFocusUpstream {
		t.Fatalf("expected upstream clamp, got %d", tab.detailCursor)
	}

	// Left → Proxy
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusProxy {
		t.Fatalf("expected proxy after left, got %d", tab.detailCursor)
	}

	// Left → Mode (flow order: Port, Mode, Proxy, Upstream)
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusMode {
		t.Fatalf("expected mode after left, got %d", tab.detailCursor)
	}

	// Left → Port
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusPort {
		t.Fatalf("expected port after left, got %d", tab.detailCursor)
	}

	// Left → clamp at port
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusPort {
		t.Fatalf("expected port clamp, got %d", tab.detailCursor)
	}

	// Down → next slot, cursor stays on Port
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tab.slotListIndex != 1 {
		t.Fatalf("expected slot 1, got %d", tab.slotListIndex)
	}
	if tab.detailCursor != cardFocusPort {
		t.Fatalf("expected port preserved after slot switch, got %d", tab.detailCursor)
	}

	// Up → previous slot
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tab.slotListIndex != 0 {
		t.Fatalf("expected slot 0, got %d", tab.slotListIndex)
	}
}

func TestSlotsTab_ModeDirectToggle(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cfg := config.DefaultConfig()
	tab := NewSlotsTab(cfg, subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	// Navigate to Mode
	tab.detailCursor = cardFocusMode

	// Default mode should be "rule"
	if mode := cfg.GetSlotStartMode(0); mode != "rule" {
		t.Fatalf("expected default rule, got %s", mode)
	}

	// Enter toggles to global
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mode := cfg.GetSlotStartMode(0); mode != "global" {
		t.Fatalf("expected global after toggle, got %s", mode)
	}
	if tab.view != slotsViewList {
		t.Fatalf("expected to stay in list view")
	}

	// Enter toggles back to rule
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mode := cfg.GetSlotStartMode(0); mode != "rule" {
		t.Fatalf("expected rule after second toggle, got %s", mode)
	}
}

func TestSlotsTab_RenderSlotCardShowsLatencyAfterTest(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.SetSlotLastLatency(0, 42)
	cfg.SetSlotLastRegion(0, "US, Test City")

	tab := NewSlotsTab(cfg, subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	card := tab.renderSlotCard(0, 60, true)
	if !strings.Contains(card, "ms") {
		t.Fatalf("expected slot card to show latency, got %q", card)
	}
	if !strings.Contains(card, "US, Test City") {
		t.Fatalf("expected slot card to show region, got %q", card)
	}
}

func TestSlotsTab_UpstreamOnlySlotShowsReady(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cfg := config.DefaultConfig()
	upID := cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "Corp Proxy",
		Protocol: "http",
		Server:   "proxy.corp.com",
		Port:     8080,
	})
	cfg.SetSlotUpstreamRef(0, upID)
	// No node ref set — upstream-only

	tab := NewSlotsTab(cfg, subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	ready, reason := tab.slotReady(0)
	if !ready {
		t.Fatalf("expected upstream-only slot to be ready, got reason: %s", reason)
	}

	card := tab.renderSlotCard(0, 60, true)
	if strings.Contains(card, "MISSING PROXY") {
		t.Fatalf("expected no 'MISSING PROXY' for upstream-only slot, got %q", card)
	}
}

func TestSlotsTab_AllSlotsReadyByDefault(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cfg := config.DefaultConfig()
	tab := NewSlotsTab(cfg, subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	for slotID := 0; slotID < proxy.MaxSlots; slotID++ {
		ready, reason := tab.slotReady(slotID)
		if !ready {
			t.Errorf("slot %d: expected ready, got not ready (%s)", slotID, reason)
		}
	}
}
