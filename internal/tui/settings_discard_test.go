package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

func TestLeavingSettingsTab_DiscardsUnsavedChanges(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	subMgr := subscription.NewManager()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr, nodeMgr, slotMgr, "dev", "")
	m.activeTab = TabSettings

	// Type into the first field (mixed port) without saving.
	var cmd tea.Cmd
	m.settingsTab, cmd = m.settingsTab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9999")})
	_ = cmd

	if !strings.Contains(m.settingsTab.View(), "9999") {
		t.Fatalf("expected Settings view to contain unsaved value 9999")
	}

	oldSettings := m.settingsTab
	m.nextTab()

	if m.settingsTab == oldSettings {
		t.Fatalf("expected settingsTab to be recreated when leaving Settings tab")
	}
	view := m.settingsTab.View()
	if strings.Contains(view, "9999") {
		t.Fatalf("expected unsaved value 9999 to be discarded")
	}
	if !strings.Contains(view, "17890") {
		t.Fatalf("expected Settings view to revert to config value 17890")
	}
}
