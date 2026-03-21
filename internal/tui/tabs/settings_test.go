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

	// 3 -> (skip header) -> edit_upstream_proxies
	tab.focusNext()
	if tab.fields[tab.focusedField].key != "edit_upstream_proxies" {
		t.Fatalf("expected to focus edit_upstream_proxies, got %q", tab.fields[tab.focusedField].key)
	}

	// edit_upstream_proxies -> edit_slots
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
	// Add an upstream proxy to the registry.
	id := cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "Test VPN",
		Protocol: "socks5",
		Server:   "gateway.example.com",
		Port:     1080,
		Username: "alice",
		Password: "secret",
	})
	// Assign it to slot 3.
	cfg.SetSlotUpstreamRef(3, id)

	tab := NewSettingsTab(cfg, nil, "dev")
	tab.loadSlotUpstreamDraft(3)

	field := tab.slotFieldByKey("slot_upstream_select")
	if field == nil {
		t.Fatalf("expected slot_upstream_select field to exist")
	}
	if field.selectedIdx < 0 || field.selectedIdx >= len(field.options) {
		t.Fatalf("selectedIdx out of range: %d", field.selectedIdx)
	}
	selectedValue := field.options[field.selectedIdx].value
	if selectedValue != id {
		t.Fatalf("expected selected upstream ID=%q, got %q", id, selectedValue)
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

func TestSettingsTab_InputKeepsArrowAndBackspaceForEditing(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	tab.focusedField = fieldIndexByKey(tab.fields, "mixed_port")
	field := &tab.fields[tab.focusedField]
	field.input.SetValue("123")
	field.input.SetCursor(3)
	tab.armedReplace = false

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyLeft})
	_ = cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	_ = cmd
	if got := field.input.Value(); got != "1293" {
		t.Fatalf("expected left arrow to move cursor within input, got %q", got)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	_ = cmd
	if got := field.input.Value(); got != "123" {
		t.Fatalf("expected backspace to delete within input, got %q", got)
	}
}

func TestSettingsTab_UpstreamProxiesNavigation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "VPN 1",
		Protocol: "http",
		Server:   "vpn1.example.com",
		Port:     8080,
	})

	tab := NewSettingsTab(cfg, nil, "dev")
	tab.focusedField = fieldIndexByKey(tab.fields, "edit_upstream_proxies")

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd

	if !tab.showUpstreamList {
		t.Fatalf("expected upstream list to open")
	}

	// Navigate down to "Add new" button
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	_ = cmd
	if tab.upstreamListIndex != 1 {
		t.Fatalf("expected upstreamListIndex=1, got %d", tab.upstreamListIndex)
	}

	// Enter to add new
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	if !tab.showUpstreamDetail {
		t.Fatalf("expected upstream detail to open for new entry")
	}
	if tab.upstreamDetailID != "" {
		t.Fatalf("expected empty ID for new entry, got %q", tab.upstreamDetailID)
	}

	// Back to list
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.showUpstreamDetail {
		t.Fatalf("expected upstream detail to close")
	}
	if !tab.showUpstreamList {
		t.Fatalf("expected upstream list to remain open")
	}

	// Back to main settings
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.showUpstreamList {
		t.Fatalf("expected upstream list to close")
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
