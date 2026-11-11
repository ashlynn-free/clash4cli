package tabs

import (
	"os"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

func TestSettingsTab_FocusSkipsHeadersAndDisabledFields(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	if got := tab.fields[tab.focusedField].key; got != "mixed_port" {
		t.Fatalf("expected initial focus mixed_port, got %q", got)
	}

	tab.focusNext()
	if got := tab.fields[tab.focusedField].key; got != "reset_port" {
		t.Fatalf("expected key reset_port, got %q", got)
	}

	tab.focusNext()
	if got := tab.fields[tab.focusedField].key; got != "allow_lan" {
		t.Fatalf("expected key allow_lan, got %q", got)
	}

	tab.focusNext()
	if got := tab.fields[tab.focusedField].key; got != "update_via_proxy" {
		t.Fatalf("expected to focus update_via_proxy, got %q", got)
	}

	tab.focusNext()
	if got := tab.fields[tab.focusedField].key; got != "edit_upstream_proxies" {
		t.Fatalf("expected headers to be skipped, got %q", got)
	}

	tab.focusPrev()
	if got := tab.fields[tab.focusedField].key; got != "update_via_proxy" {
		t.Fatalf("expected focus to move back to update_via_proxy, got %q", got)
	}
}

func TestSettingsTab_FocusWrapsThroughRestartButton(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	last := -1
	for i := len(tab.fields) - 1; i >= 0; i-- {
		if tab.fields[i].fieldType != FieldTypeHeader && !tab.fields[i].disabled {
			last = i
			break
		}
	}
	if last < 0 {
		t.Fatal("expected a focusable field")
	}
	tab.focusField(last)

	tab.focusNext()
	if tab.focusedField != len(tab.fields) {
		t.Fatalf("expected restart core virtual button, got index %d", tab.focusedField)
	}

	tab.focusNext()
	if got := tab.fields[tab.focusedField].key; got != "mixed_port" {
		t.Fatalf("expected focus to wrap to mixed_port, got %q", got)
	}
}

func TestSettingsTab_ArrowKeysMoveThroughFlatList(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	_ = cmd

	if got := tab.fields[tab.focusedField].key; got != "reset_port" {
		t.Fatalf("expected down to move to reset_port, got %q", got)
	}
}

func TestSettingsTab_SelectFieldKeepsViRightKey(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	speedUnitIndex := tab.fieldIndexByKey("speed_unit")
	if speedUnitIndex < 0 {
		t.Fatal("expected speed_unit field")
	}
	tab.setFocusedField(speedUnitIndex)
	before := tab.fields[speedUnitIndex].selectedIdx

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	if cmd == nil {
		t.Fatal("expected select change to save")
	}
	if got := tab.fields[speedUnitIndex].selectedIdx; got == before {
		t.Fatalf("expected selection to advance, still at %d", got)
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if tab.activeSubPage != settingsSubPageLogs {
		t.Fatalf("expected uppercase L to open logs subpage, got %d", tab.activeSubPage)
	}
}

func TestSettingsTab_MovingFocusSavesChangedInput(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	field := tab.fieldByKey("mixed_port")
	if field == nil {
		t.Fatalf("expected mixed_port field")
	}
	field.input.SetValue("17891")

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatalf("expected changed input to trigger save when moving focus")
	}
	msg := cmd()
	if _, ok := msg.(portChangedSaveMsg); !ok {
		t.Fatalf("expected portChangedSaveMsg, got %#v", msg)
	}
	if cfg.Proxy.MixedPort != 17891 {
		t.Fatalf("expected mixed port saved, got %d", cfg.Proxy.MixedPort)
	}
}

func TestSettingsTab_EnterOnSystemInfoOpensSubpage(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.SetSize(100, 24)

	index := fieldIndexByKey(tab.fields, "system_info")
	if index < 0 {
		t.Fatal("expected system_info field")
	}
	tab.setFocusedField(index)

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected system info fetch command")
	}
	if tab.activeSubPage != settingsSubPageSystemInfo {
		t.Fatalf("expected system info subpage, got %d", tab.activeSubPage)
	}
	if !strings.Contains(tab.View(), "System Info") {
		t.Fatalf("expected System Info to render")
	}
}

func TestSettingsTab_UpstreamButtonOpensList(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "VPN 1",
		Protocol: "http",
		Server:   "vpn1.example.com",
		Port:     8080,
	})
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.SetSize(100, 24)

	index := fieldIndexByKey(tab.fields, "edit_upstream_proxies")
	if index < 0 {
		t.Fatal("expected edit_upstream_proxies field")
	}
	tab.setFocusedField(index)

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected opening upstream list not to trigger a command")
	}
	if tab.activeSubPage != settingsSubPageUpstreamList {
		t.Fatalf("expected upstream list subpage, got %d", tab.activeSubPage)
	}

	view := tab.View()
	if !strings.Contains(view, "VPN 1") || !strings.Contains(view, "[+ Add new upstream proxy]") {
		t.Fatalf("expected upstream list to render\n%s", view)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	_ = cmd
	if tab.upstreamListIndex != 1 {
		t.Fatalf("expected down to move upstream list selection, got %d", tab.upstreamListIndex)
	}
}

func TestSettingsTab_UpstreamDeleteFromSubpage(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	id := cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "VPN 1",
		Protocol: "http",
		Server:   "vpn1.example.com",
		Port:     8080,
	})
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.activeSubPage = settingsSubPageUpstreamList

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if _, ok := cfg.GetUpstreamProxy(id); ok {
		t.Fatal("expected selected upstream proxy to be deleted")
	}
	if tab.activeSubPage != settingsSubPageUpstreamList {
		t.Fatalf("expected delete to remain in upstream list, got %d", tab.activeSubPage)
	}
}

func TestSettingsTab_RestartCoreRendersInFlatView(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.SetSize(100, 100)

	if !strings.Contains(tab.View(), "Restart core") {
		t.Fatalf("expected flat view to include restart core")
	}
}

func TestSettingsTab_FlatViewShowsSections(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.SetSize(100, 60)

	view := tab.View()
	for _, want := range []string{"Proxy", "Upstream Proxies", "Display", "System", "View Logs >"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q\n%s", want, view)
		}
	}
}

func TestSettingsTab_PlatformFieldsAreFiltered(t *testing.T) {
	tab := NewSettingsTab(config.DefaultConfig(), nil, "dev")

	has := func(key string) bool {
		return tab.fieldByKey(key) != nil
	}
	if got, want := has("system_proxy_settings"), runtime.GOOS == "darwin"; got != want {
		t.Fatalf("system proxy fields present=%v, want %v", got, want)
	}
	if got, want := has("docker_redirect_tcp"), runtime.GOOS == "linux"; got != want {
		t.Fatalf("docker field present=%v, want %v", got, want)
	}
	wantTUN := (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && os.Geteuid() == 0
	if got := has("tun_settings"); got != wantTUN {
		t.Fatalf("TUN field present=%v, want %v", got, wantTUN)
	}
}

func TestSettingsTab_SubpageEscReturnsToMain(t *testing.T) {
	tab := NewSettingsTab(config.DefaultConfig(), nil, "dev")
	tab.SetSize(100, 100)
	tab.loadUpstreamDraft("")
	tab.activeSubPage = settingsSubPageUpstreamDetail

	if !strings.Contains(tab.View(), "New Upstream Proxy") {
		t.Fatalf("expected upstream detail to render\n%s", tab.View())
	}

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.activeSubPage != settingsSubPageUpstreamList {
		t.Fatalf("expected Esc from downstream detail to return to list, got %d", tab.activeSubPage)
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.activeSubPage != settingsSubPageNone {
		t.Fatalf("expected Esc from list to return to main view, got %d", tab.activeSubPage)
	}
	if !strings.Contains(tab.View(), "View Logs >") {
		t.Fatalf("expected main flat view to render")
	}
}

func TestSettingsTab_LogsSubpageUsesMainLogsTab(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.SetSize(100, 24)
	tab.logsTab.AddLog("info", "hello from settings logs")

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd != nil {
		t.Fatalf("expected logs shortcut not to trigger a command")
	}
	if tab.activeSubPage != settingsSubPageLogs {
		t.Fatalf("expected logs subpage, got %d", tab.activeSubPage)
	}
	if !strings.Contains(tab.View(), "hello from settings logs") {
		t.Fatalf("expected logs view to render inside settings\n%s", tab.View())
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tab.activeSubPage != settingsSubPageNone {
		t.Fatalf("expected Esc to close logs subpage, got %d", tab.activeSubPage)
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
	tab.activeSubPage = settingsSubPageUpstreamList

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd

	if tab.activeSubPage != settingsSubPageUpstreamDetail {
		t.Fatalf("expected enter on upstream list to edit selected entry")
	}
	if tab.upstreamDetailID == "" {
		t.Fatalf("expected existing upstream ID when editing selected entry")
	}

	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.activeSubPage != settingsSubPageUpstreamList {
		t.Fatalf("expected upstream detail to return to list")
	}

	// Navigate down to "Add new" button.
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	_ = cmd
	if tab.upstreamListIndex != 1 {
		t.Fatalf("expected upstreamListIndex=1, got %d", tab.upstreamListIndex)
	}

	// Enter to add new
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = cmd
	if tab.activeSubPage != settingsSubPageUpstreamDetail {
		t.Fatalf("expected upstream detail to open for new entry")
	}
	if tab.upstreamDetailID != "" {
		t.Fatalf("expected empty ID for new entry, got %q", tab.upstreamDetailID)
	}

	// Back to list
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = cmd
	if tab.activeSubPage != settingsSubPageUpstreamList {
		t.Fatalf("expected upstream detail to return to list")
	}
}

func TestSettingsTab_SaveSettingsPersistsTunAdvancedFields(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	tunEnabled := tab.fieldByKey("tun_enabled")
	if tunEnabled == nil {
		t.Skip("TUN settings are unavailable on this platform/user")
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
	tab.activeSubPage = settingsSubPageTUN
	tab.tunFocusedField = fieldIndexByKey(tab.tunFields, "tun_device")
	field := &tab.tunFields[tab.tunFocusedField]
	field.input.Focus()
	field.input.SetValue("utun123")
	field.input.SetCursor(len(field.input.Value()))

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	_ = cmd

	if tab.activeSubPage != settingsSubPageTUN {
		t.Fatalf("expected TUN subpage to remain open on backspace")
	}
	if got := field.input.Value(); got != "utun12" {
		t.Fatalf("expected backspace to edit input, got %q", got)
	}
}

func TestSettingsTab_TunNavigationWithoutChangesDoesNotRestartOrExit(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.tunFields = tab.newTunSettingsFields()
	tab.loadTunDraft()
	tab.activeSubPage = settingsSubPageTUN

	var cmd tea.Cmd
	tab, cmd = tab.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected navigation to return the deferred change check")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("expected unchanged TUN draft to emit no message, got %#v", msg)
	}
	if tab.activeSubPage != settingsSubPageTUN {
		t.Fatalf("expected TUN subpage to remain open, got %d", tab.activeSubPage)
	}
}

func TestSettingsTab_TunChangeRestartsWithoutClosingSubpage(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.tunFields = tab.newTunSettingsFields()
	tab.loadTunDraft()
	tab.activeSubPage = settingsSubPageTUN

	stack := tab.tunFieldByKey("tun_stack")
	if stack == nil || len(stack.options) < 2 {
		t.Fatal("expected multiple TUN stack options")
	}
	stack.selectedIdx = (stack.selectedIdx + 1) % len(stack.options)

	msg := tab.saveTunSettings()()
	changed, ok := msg.(tunChangedSaveMsg)
	if !ok {
		t.Fatalf("expected tunChangedSaveMsg, got %#v", msg)
	}
	tab, cmd := tab.Update(changed)

	if tab.activeSubPage != settingsSubPageTUN {
		t.Fatalf("expected TUN subpage to remain open, got %d", tab.activeSubPage)
	}
	if cmd == nil {
		t.Fatal("expected changed TUN settings to schedule restart feedback")
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

func TestMoveFieldFocus_AllDisabledKeepsCurrentIndex(t *testing.T) {
	tab := &SettingsTab{}
	fields := []settingField{
		{fieldType: FieldTypeHeader},
		{fieldType: FieldTypeSelect, disabled: true},
	}
	current := 1

	tab.moveFieldFocus(fields, &current, 1)

	if current != 1 {
		t.Fatalf("expected focus to stay at 1, got %d", current)
	}
}

func TestAdjustScrollOffsetClampsToViewport(t *testing.T) {
	if got := adjustScrollOffset(0, -5, 4, 10); got != 0 {
		t.Fatalf("expected negative offset to clamp to 0, got %d", got)
	}
	if got := adjustScrollOffset(9, 0, 4, 10); got != 6 {
		t.Fatalf("expected focus at bottom to scroll to 6, got %d", got)
	}
	if got := adjustScrollOffset(2, 20, 4, 3); got != 0 {
		t.Fatalf("expected short content to clamp to 0, got %d", got)
	}
}

func TestSettingsTab_SaveSettingsTriggersTunRestart(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.TunEnabled = false
	tab := NewSettingsTab(cfg, nil, "dev")

	tunField := tab.fieldByKey("tun_enabled")
	if tunField == nil {
		t.Skip("TUN settings are unavailable on this platform/user")
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

func TestSettingsTab_ConnectionsButtonOpensView(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	idx := tab.fieldIndexByKey("show_connections")
	if idx < 0 {
		t.Fatalf("expected show_connections field")
	}
	tab.focusedField = idx

	tab, cmd := tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if tab.activeSubPage != settingsSubPageConnections {
		t.Fatalf("expected connections sub-page, got %v", tab.activeSubPage)
	}
	if cmd == nil {
		t.Fatalf("expected a connection fetch command")
	}
}

func TestSettingsTab_ConnectionsViewShowsPathAndTraffic(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")
	tab.width = 140
	tab.height = 30
	tab.activeSubPage = settingsSubPageConnections
	tab.connItems = []slotConnection{{
		SlotID: 0,
		ConnectionInfo: proxy.ConnectionInfo{
			ID:              "conn-1",
			Host:            "api.example.com",
			DestinationPort: "443",
			Chains:          []string{"slot-0-upstream", "GLOBAL"},
			Download:        2048,
			Upload:          1024,
		},
		DownRate: 100,
		UpRate:   50,
		Entry:    "0:17890",
		Transit:  "美国-US-1",
		Exit:     "US NY",
	}}
	tab.connDownloadTotal = 2048
	tab.connUploadTotal = 1024
	tab.updateConnectionRows()

	view := tab.viewConnections()
	for _, want := range []string{"Connections", "api.example.com:443", "Entry", "Transit", "Exit", "0:17890", "美国-US-1", "US NY"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestConnectionPath_UpstreamAndDirect(t *testing.T) {
	state := &proxy.SlotState{
		SlotID:          0,
		Port:            17890,
		NodeName:        "美国-US-1",
		UpstreamEnabled: true,
		UpstreamServer:  "63.246.132.189",
		UpstreamPort:    5507,
	}

	entry, transit, exit := connectionPath(state, proxy.ConnectionInfo{
		Chains: []string{"slot-0-upstream", "GLOBAL"},
	}, "US NY")
	if entry != "0:17890" || transit != "美国-US-1" || exit != "US NY" {
		t.Fatalf("upstream path = %q / %q / %q", entry, transit, exit)
	}

	// A connection that bypasses the upstream uses the node as its exit.
	entry, transit, exit = connectionPath(state, proxy.ConnectionInfo{
		Chains: []string{"HK-01", "Proxy"},
	}, "US NY")
	if entry != "0:17890" || transit != "-" || exit != "HK-01" {
		t.Fatalf("direct path = %q / %q / %q", entry, transit, exit)
	}

	// Chain order must not matter when picking the outbound proxy.
	if got := connectionExitProxy([]string{"Proxy", "HK-01"}); got != "HK-01" {
		t.Fatalf("connectionExitProxy group-first = %q, want HK-01", got)
	}
	if got := connectionExitProxy([]string{"GLOBAL", "slot-0-upstream"}); got != "slot-0-upstream" {
		t.Fatalf("connectionExitProxy group-first = %q, want slot-0-upstream", got)
	}
}

func TestSettingsTab_UpperCOpensConnections(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg, nil, "dev")

	tab, cmd := tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if tab.activeSubPage != settingsSubPageConnections {
		t.Fatalf("expected connections sub-page, got %v", tab.activeSubPage)
	}
	if cmd == nil {
		t.Fatalf("expected a connection fetch command")
	}
}
