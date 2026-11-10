package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/node"
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

func TestSlotsTab_UpperD_DisconnectAll(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	updated, cmd := tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if cmd == nil {
		t.Fatalf("expected disconnect-all command")
	}

	msg := cmd()
	if _, ok := msg.(SlotDisconnectAllMsg); !ok {
		t.Fatalf("expected SlotDisconnectAllMsg, got %T", msg)
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
	nodeMgr := node.NewManager(subMgr, 1, 0)
	tab := NewSlotsTab(cfg, subMgr, nil, nodeMgr)
	tab.SetSize(120, 30)

	if tab.detailCursor != cardFocusProxy {
		t.Fatalf("expected default cursor on proxy, got %d", tab.detailCursor)
	}

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	if tab.view != slotsViewProxyPicker {
		t.Fatalf("expected proxy picker, got %v", tab.view)
	}

	// Move from "No Proxy (DIRECT)" to first real node, then select it.
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
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

func TestSlotsTab_ProxyPickerCanClearNodeRef(t *testing.T) {
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
	cfg.SetSlotNodeRef(0, config.SlotNodeRef{SubID: sub.ID, NodeName: "NodeA", NodeServer: "example.com", NodePort: 8388})

	nodeMgr := node.NewManager(subMgr, 1, 0)
	tab := NewSlotsTab(cfg, subMgr, nil, nodeMgr)
	tab.SetSize(120, 30)

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if tab.view != slotsViewProxyPicker {
		t.Fatalf("expected proxy picker, got %v", tab.view)
	}

	// Default row is now No Proxy (DIRECT); select it immediately.
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if tab.view != slotsViewList {
		t.Fatalf("expected to return to list after clearing proxy, got %v", tab.view)
	}
	if _, ok := cfg.GetSlotNodeRef(0); ok {
		t.Fatalf("expected slot node ref to be cleared")
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

func TestSlotsTab_SingleCardNeverExceedsWidth(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	for _, width := range []int{60, 80, 120} {
		for _, selected := range []bool{false, true} {
			tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
			tab.SetSize(width, 24)
			card := tab.renderSlotCard(0, width, selected)
			for i, line := range strings.Split(card, "\n") {
				if dw := lipgloss.Width(line); dw > width {
					t.Errorf("width=%d selected=%v card line %d: display width %d exceeds %d", width, selected, i, dw, width)
				}
			}
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

	if tab.detailCursor != cardFocusProxy {
		t.Fatalf("expected default focus on proxy, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tab.detailCursor != cardFocusUpstream {
		t.Fatalf("expected upstream after right, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tab.detailCursor != cardFocusUpstream {
		t.Fatalf("expected upstream clamp, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusProxy {
		t.Fatalf("expected proxy after left, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusMode {
		t.Fatalf("expected mode after left, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusPort {
		t.Fatalf("expected port after left, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tab.detailCursor != cardFocusPort {
		t.Fatalf("expected port clamp, got %d", tab.detailCursor)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tab.slotListIndex != 1 {
		t.Fatalf("expected slot 1, got %d", tab.slotListIndex)
	}
	if tab.detailCursor != cardFocusPort {
		t.Fatalf("expected port preserved after slot switch, got %d", tab.detailCursor)
	}

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
	tab.detailCursor = cardFocusMode

	if mode := cfg.GetSlotStartMode(0); mode != "rule" {
		t.Fatalf("expected default rule, got %s", mode)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mode := cfg.GetSlotStartMode(0); mode != "global" {
		t.Fatalf("expected global after toggle, got %s", mode)
	}
	if tab.view != slotsViewList {
		t.Fatalf("expected to stay in list view")
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mode := cfg.GetSlotStartMode(0); mode != "rule" {
		t.Fatalf("expected rule after second toggle, got %s", mode)
	}
}

func TestSlotsTab_UpstreamOnlySummary(t *testing.T) {
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

	tab := NewSlotsTab(cfg, subscription.NewManager(), nil, nil)
	if got := tab.slotUpstreamSummary(0); got != "Corp Proxy" {
		t.Fatalf("expected upstream summary, got %q", got)
	}
}

func TestSlotsTab_SlotTrafficSetClear(t *testing.T) {
	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	info := &SlotTrafficInfo{UpRate: 1, DownRate: 2, ConnCount: 3}
	tab.SetSlotTraffic(0, info)
	if tab.slotTraffic[0] == nil || tab.slotTraffic[0].ConnCount != 3 {
		t.Fatalf("expected traffic info to be set")
	}
	tab.ClearSlotTraffic(0)
	if tab.slotTraffic[0] != nil {
		t.Fatalf("expected traffic info to be cleared")
	}
}

func TestSlotsTab_RegionLatencyShowsTesting(t *testing.T) {
	tab := &SlotsTab{cfg: config.DefaultConfig()}

	tab.slotTesting[0] = true
	got := tab.slotRegionLatencyInfo(0)
	if !strings.Contains(got, i18n.M().Nodes.Testing) {
		t.Fatalf("expected testing indicator, got %q", got)
	}

	tab.slotTesting[0] = false
	tab.slotRegions[0] = "Tokyo"
	tab.slotLatencies[0] = 123
	got = tab.slotRegionLatencyInfo(0)
	if !strings.Contains(got, "Tokyo") || !strings.Contains(got, "123") {
		t.Fatalf("expected region+latency, got %q", got)
	}
}

func TestSlotsTab_TestResultClearsTestingFlag(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := &SlotsTab{cfg: config.DefaultConfig()}
	tab.slotTesting[0] = true
	tab, _ = tab.Update(SlotTestResultMsg{SlotID: 0, Latency: 42, Region: "Tokyo"})
	if tab.slotTesting[0] {
		t.Fatalf("expected testing flag to be cleared")
	}
	if tab.slotRegions[0] != "Tokyo" || tab.slotLatencies[0] != 42 {
		t.Fatalf("unexpected result: region=%q latency=%d", tab.slotRegions[0], tab.slotLatencies[0])
	}
}

func TestSlotsTab_TestsSelectedSlotOnly(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)
	tab.slotListIndex = 1

	tab, cmd := tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if cmd == nil {
		t.Fatalf("expected a test command")
	}
	if !tab.slotTesting[1] {
		t.Fatalf("expected selected slot 1 to be testing")
	}
	if tab.slotTesting[0] || tab.slotTesting[2] {
		t.Fatalf("expected other slots not to be testing: %v", tab.slotTesting)
	}
}

func TestSlotsTab_UpperTTestsAllSlots(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	tab := NewSlotsTab(config.DefaultConfig(), subscription.NewManager(), nil, nil)
	tab.SetSize(120, 30)

	tab, cmd := tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if cmd == nil {
		t.Fatalf("expected a test command")
	}
	for i := 0; i < proxy.MaxSlots; i++ {
		if !tab.slotTesting[i] {
			t.Fatalf("expected slot %d to be testing: %v", i, tab.slotTesting)
		}
	}
}
