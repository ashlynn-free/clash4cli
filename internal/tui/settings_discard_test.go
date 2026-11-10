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

func TestLeavingSettingsTab_PreservesState(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	subMgr := subscription.NewManager()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr, nodeMgr, slotMgr, nil, "dev", "")
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

func TestSettingsTab_DoesNotCaptureGlobalTabSwitch(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	subMgr := subscription.NewManager()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr, nodeMgr, slotMgr, nil, "dev", "")
	m.activeTab = TabSettings

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := model.(*Model)

	if updated.activeTab != TabSlots {
		t.Fatalf("expected Tab to wrap from settings to slots, activeTab=%d", updated.activeTab)
	}
}

func TestSettingsInputCapturesNumericTabShortcuts(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	subMgr := subscription.NewManager()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr, nodeMgr, slotMgr, nil, "dev", "")
	m.activeTab = TabSettings
	m.settingsTab.SetSize(100, 24)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	updated := model.(*Model)

	if updated.activeTab != TabSettings {
		t.Fatalf("expected numeric input to stay on settings, activeTab=%d", updated.activeTab)
	}
	if got := updated.settingsTab.View(); !strings.Contains(got, "> 1") {
		t.Fatalf("expected typed digit to appear in settings view\n%s", got)
	}
}

func TestSettingsLogsCategoryUsesMainLogsTab(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	subMgr := subscription.NewManager()
	nodeMgr := node.NewManager(subMgr, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	slotMgr := proxy.NewSlotManager(cfg)

	m := New(cfg, subMgr, nodeMgr, slotMgr, nil, "dev", "")
	m.activeTab = TabSettings
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateSizes()
	m.logsTab.AddLog("info", "settings embedded log")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	updated := model.(*Model)

	if !strings.Contains(updated.View(), "settings embedded log") {
		t.Fatalf("expected settings logs subpage to render main log entry")
	}
}
