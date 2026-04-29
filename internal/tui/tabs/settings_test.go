package tabs

import (
	"runtime"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
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

	// edit_upstream_proxies -> (skip header) -> system_proxy_enabled on darwin, tun_enabled on linux, otherwise language
	tab.focusNext()
	if runtime.GOOS == "darwin" {
		if tab.fields[tab.focusedField].key != "system_proxy_enabled" {
			t.Fatalf("expected to focus system_proxy_enabled on darwin, got %q", tab.fields[tab.focusedField].key)
		}
	} else if runtime.GOOS == "linux" {
		if tab.fields[tab.focusedField].key != "tun_enabled" {
			t.Fatalf("expected to focus tun_enabled on linux, got %q", tab.fields[tab.focusedField].key)
		}
	} else {
		if tab.fields[tab.focusedField].key != "language" {
			t.Fatalf("expected to skip disabled system proxy/tun fields and focus language, got %q", tab.fields[tab.focusedField].key)
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

func TestSettingsTab_SaveSettingsPersistsTunAdvancedFields(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	tunEnabled := tab.fieldByKey("tun_enabled")
	if tunEnabled == nil {
		t.Fatalf("expected tun_enabled field")
	}
	tunEnabled.selectedIdx = 0

	tab.tunFields = tab.newTunSettingsFields()
	tab.loadTunDraft()

	tunStack := tab.tunFieldByKey("tun_stack")
	if tunStack == nil {
		t.Fatalf("expected tun_stack field")
	}
	tunStack.selectedIdx = 1

	tunMTU := tab.tunFieldByKey("tun_mtu")
	if tunMTU == nil {
		t.Fatalf("expected tun_mtu field")
	}
	tunMTU.input.SetValue("9000")

	tunDevice := tab.tunFieldByKey("tun_device")
	if tunDevice == nil {
		t.Fatalf("expected tun_device field")
	}
	tunDevice.input.SetValue("utun123")

	msg := tab.saveTunSettings()()
	if errMsg, ok := msg.(SettingsErrorMsg); ok {
		t.Fatalf("saveTunSettings returned error: %v", errMsg.Error)
	}

	if cfg.Proxy.TunStack != "gvisor" {
		t.Fatalf("expected TunStack=gvisor, got %q", cfg.Proxy.TunStack)
	}
	if cfg.Proxy.TunMTU != 9000 {
		t.Fatalf("expected TunMTU=9000, got %d", cfg.Proxy.TunMTU)
	}
	if cfg.Proxy.TunDevice != "utun123" {
		t.Fatalf("expected TunDevice=utun123, got %q", cfg.Proxy.TunDevice)
	}

	proxyCfg, err := proxy.GenerateConfig(nil, nil, cfg, 0, 17890, 9090, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("GenerateConfig returned error: %v", err)
	}
	if runtime.GOOS == "darwin" && proxyCfg.Tun != nil && proxyCfg.Tun.Device != "" {
		t.Fatalf("expected darwin Tun.Device empty after generation, got %q", proxyCfg.Tun.Device)
	}
}

func TestSettingsTab_SaveSettingsPersistsSystemProxyFields(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	tab.systemProxyFields = tab.newSystemProxySettingsFields()
	tab.loadSystemProxyDraft()

	host := tab.systemProxyFieldByKey("system_proxy_host")
	if host == nil {
		t.Fatalf("expected system_proxy_host field")
	}
	host.input.SetValue("127.0.0.2")

	pac := tab.systemProxyFieldByKey("system_proxy_pac_mode")
	if pac == nil {
		t.Fatalf("expected system_proxy_pac_mode field")
	}
	pac.selectedIdx = 0

	guard := tab.systemProxyFieldByKey("system_proxy_guard")
	if guard == nil {
		t.Fatalf("expected system_proxy_guard field")
	}
	guard.selectedIdx = 0

	duration := tab.systemProxyFieldByKey("system_proxy_guard_duration")
	if duration == nil {
		t.Fatalf("expected system_proxy_guard_duration field")
	}
	duration.input.SetValue("45")

	bypass := tab.systemProxyFieldByKey("system_proxy_bypass")
	if bypass == nil {
		t.Fatalf("expected system_proxy_bypass field")
	}
	bypass.input.SetValue("127.0.0.1, localhost")

	msg := tab.saveSystemProxySettings()()
	if errMsg, ok := msg.(SettingsErrorMsg); ok {
		t.Fatalf("saveSystemProxySettings returned error: %v", errMsg.Error)
	}

	if cfg.Proxy.SystemProxy.Host != "127.0.0.2" {
		t.Fatalf("expected system proxy host to persist, got %q", cfg.Proxy.SystemProxy.Host)
	}
	if !cfg.Proxy.SystemProxy.PACMode || !cfg.Proxy.SystemProxy.ProxyGuard {
		t.Fatalf("expected PAC mode and guard to persist: %#v", cfg.Proxy.SystemProxy)
	}
	if cfg.Proxy.SystemProxy.GuardDurationSeconds != 45 {
		t.Fatalf("expected guard duration 45, got %d", cfg.Proxy.SystemProxy.GuardDurationSeconds)
	}
	if len(cfg.Proxy.SystemProxy.Bypass) != 2 {
		t.Fatalf("expected bypass list to persist, got %#v", cfg.Proxy.SystemProxy.Bypass)
	}
}

func TestSettingsTab_TunSubpageBackspaceDoesNotExit(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.tunFields = tab.newTunSettingsFields()
	tab.loadTunDraft()
	tab.showTunSettings = true
	tab.tunFocusedField = fieldIndexByKey(tab.tunFields, "tun_device")
	field := &tab.tunFields[tab.tunFocusedField]
	field.input.Focus()
	field.input.SetValue("utun123")
	field.input.SetCursor(len(field.input.Value()))

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	_ = cmd

	if !tab.showTunSettings {
		t.Fatalf("expected TUN subpage to remain open on backspace")
	}
	if got := field.input.Value(); got != "utun12" {
		t.Fatalf("expected backspace to edit input, got %q", got)
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

func TestSettingsTab_SaveSettingsTriggersTunRestart(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.TunEnabled = false
	tab := NewSettingsTab(cfg, nil, "dev")

	tunField := tab.fieldByKey("tun_enabled")
	if tunField == nil {
		t.Fatalf("expected tun_enabled field")
	}
	if tunField.selectedIdx != 1 {
		tunField.selectedIdx = 1
	}

	tunField.selectedIdx = 0
	msg := tab.saveSettings()()
	if _, ok := msg.(tunChangedSaveMsg); !ok {
		t.Fatalf("expected tunChangedSaveMsg after toggling TUN on, got %T", msg)
	}
	if !cfg.Proxy.TunEnabled {
		t.Fatalf("expected TunEnabled=true after save")
	}

	tunField.selectedIdx = 1
	msg = tab.saveSettings()()
	if _, ok := msg.(tunChangedSaveMsg); !ok {
		t.Fatalf("expected tunChangedSaveMsg after toggling TUN off, got %T", msg)
	}
	if cfg.Proxy.TunEnabled {
		t.Fatalf("expected TunEnabled=false after save")
	}

	msg = tab.saveSettings()()
	if _, ok := msg.(SettingsSavedMsg); !ok {
		t.Fatalf("expected SettingsSavedMsg when TUN unchanged, got %T", msg)
	}
}
