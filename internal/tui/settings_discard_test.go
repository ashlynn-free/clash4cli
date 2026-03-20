package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

func TestLeavingSettingsTab_PreservesState(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	subMgr := subscription.NewManager()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr, nodeMgr, slotMgr, "dev", "")
	m.activeTab = TabSettings

	// Type into the first field (mixed port).
	var cmd tea.Cmd
	m.settingsTab, cmd = m.settingsTab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9999")})
	_ = cmd

	// Settings tab is preserved when switching away (no discard).
	oldSettings := m.settingsTab
	m.nextTab()

	if m.settingsTab != oldSettings {
		t.Fatalf("expected settingsTab to be preserved (instant-save mode), but it was recreated")
	}
}
