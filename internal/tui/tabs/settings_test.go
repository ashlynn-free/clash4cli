package tabs

import (
	"runtime"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
)

func TestSettingsTab_FocusSkipsHeaderAndDisabledFields(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	// 0 -> 1 (reset_port button)
	tab.focusNext()
	if tab.focusedField != 1 {
		t.Fatalf("expected focusedField=1, got %d", tab.focusedField)
	}
	if tab.fields[tab.focusedField].key != "reset_port" {
		t.Fatalf("expected key reset_port, got %q", tab.fields[tab.focusedField].key)
	}

	// 1 -> 2 (allow_lan)
	tab.focusNext()
	if tab.fields[tab.focusedField].key != "allow_lan" {
		t.Fatalf("expected key allow_lan, got %q", tab.fields[tab.focusedField].key)
	}

	// 2 -> 3 (update_via_proxy)
	tab.focusNext()
	if tab.fields[tab.focusedField].key != "update_via_proxy" {
		t.Fatalf("expected to focus update_via_proxy, got %q", tab.fields[tab.focusedField].key)
	}

	// 3 -> (skip header) -> edit_slots
	tab.focusNext()
	if tab.fields[tab.focusedField].key != "edit_slots" {
		t.Fatalf("expected to focus edit_slots, got %q", tab.fields[tab.focusedField].key)
	}

	// edit_slots -> (skip linux header and disabled linux-only fields if needed)
	tab.focusNext()
	if runtime.GOOS == "linux" {
		if tab.fields[tab.focusedField].key != "tun_enabled" {
			t.Fatalf("expected to focus tun_enabled on linux, got %q", tab.fields[tab.focusedField].key)
		}
	} else {
		if tab.fields[tab.focusedField].key != "language" {
			t.Fatalf("expected to skip disabled linux-only fields and focus language, got %q", tab.fields[tab.focusedField].key)
		}
	}
}

func TestSettingsTab_LoadSlotUpstreamDraft(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SetSlotUpstream(3, config.UpstreamProxyConfig{
		Enabled:  true,
		Protocol: "socks5",
		Server:   "gateway.example.com",
		Port:     1080,
		Username: "alice",
		Password: "secret",
	})

	tab := NewSettingsTab(cfg, nil, "dev")
	tab.loadSlotUpstreamDraft(3)

	if got := tab.slotFieldByKey("slot_upstream_server").input.Value(); got != "gateway.example.com" {
		t.Fatalf("expected upstream server to load, got %q", got)
	}
	if got := tab.slotFieldByKey("slot_upstream_port").input.Value(); got != "1080" {
		t.Fatalf("expected upstream port to load, got %q", got)
	}
	if got := tab.slotFieldByKey("slot_upstream_protocol").options[tab.slotFieldByKey("slot_upstream_protocol").selectedIdx].value; got != "socks5" {
		t.Fatalf("expected upstream protocol socks5, got %q", got)
	}
}

func TestSettingsTab_EditSlotsNavigation(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.focusedField = fieldIndexByKey(tab.fields, "edit_slots")

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd

	if !tab.showSlotList || tab.showSlotDetail {
		t.Fatalf("expected slot list to open, showSlotList=%v showSlotDetail=%v", tab.showSlotList, tab.showSlotDetail)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	_ = cmd
	if tab.slotListIndex != 1 {
		t.Fatalf("expected slotListIndex=1, got %d", tab.slotListIndex)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	if !tab.showSlotList || !tab.showSlotDetail {
		t.Fatalf("expected slot detail to open on top of slot list, showSlotList=%v showSlotDetail=%v", tab.showSlotList, tab.showSlotDetail)
	}
	if tab.slotUpstreamSlot != 1 {
		t.Fatalf("expected slotUpstreamSlot=1, got %d", tab.slotUpstreamSlot)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if !tab.showSlotList || tab.showSlotDetail {
		t.Fatalf("expected esc to return to slot list, showSlotList=%v showSlotDetail=%v", tab.showSlotList, tab.showSlotDetail)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.showSlotList || tab.showSlotDetail {
		t.Fatalf("expected esc to return to main settings, showSlotList=%v showSlotDetail=%v", tab.showSlotList, tab.showSlotDetail)
	}
}

func fieldIndexByKey(fields []settingField, key string) int {
	for i, field := range fields {
		if field.key == key {
			return i
		}
	}
	return -1
}
