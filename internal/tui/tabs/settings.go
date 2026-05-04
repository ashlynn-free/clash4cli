package tabs

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/netctl"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/tui/styles"
)

// SystemInfoData holds data for the System Info secondary page.
type SystemInfoData struct {
	CoreVersion string
	MixedPort   int
	Uptime      string
	RulesCount  int
	OSInfo      string
	RunningMode string
	AppVersion  string
}

type systemInfoFetchedMsg struct {
	info SystemInfoData
}

// SettingsSavedMsg is sent when settings are saved
type SettingsSavedMsg struct {
	Message string
}

// SettingsErrorMsg is sent when settings could not be saved or validated
type SettingsErrorMsg struct {
	Error error
}

// RestartCoreMsg requests restarting the proxy core (mihomo).
type RestartCoreMsg struct{}

// RestartAllSlotsMsg requests restarting ALL running slots (e.g. after port change).
type RestartAllSlotsMsg struct{}

// RestartSlotMsg requests restarting a single running slot.
type RestartSlotMsg struct {
	SlotID int
}

// portChangedSaveMsg is an internal message indicating settings were saved and the port changed.
type portChangedSaveMsg struct {
	notice string
}

// tunChangedSaveMsg is an internal message indicating settings were saved and TUN was toggled.
type tunChangedSaveMsg struct {
	notice string
}

type slotUpstreamSavedMsg struct {
	slotID  int
	restart bool
	message string
}

type settingsFlashTickMsg struct {
	id int
}

// FieldType represents the type of setting field
type FieldType int

const (
	FieldTypeInput FieldType = iota
	FieldTypeSelect
	FieldTypeHeader
	FieldTypeButton
)

// SettingsTab represents the settings tab
type SettingsTab struct {
	cfg              *config.Config
	slotMgr          proxy.SlotService
	appVersion       string
	width            int
	height           int
	focusedField     int
	fields           []settingField
	armedReplace     bool
	pendingSave      bool
	message          string
	messageType      string
	messageShow      bool
	flashID          int
	flashTicks       int
	scrollOffset     int
	slotUpstreamSlot int
	slotListIndex    int
	slotFields       []settingField
	slotFocusedField int
	slotScrollOffset int
	// System Info secondary page
	showSystemInfo bool
	systemInfo     *SystemInfoData
	showSlotList   bool
	showSlotDetail bool
	// Upstream proxy registry views
	showUpstreamList     bool
	showUpstreamDetail   bool
	upstreamListIndex    int
	upstreamDetailID     string // "" means creating new
	upstreamFields       []settingField
	upstreamFocusedField int
	upstreamScrollOffset int
	// TUN / System Proxy settings secondary pages
	showTunSettings         bool
	tunFields               []settingField
	tunFocusedField         int
	tunScrollOffset         int
	showSystemProxySettings bool
	systemProxyFields       []settingField
	systemProxyFocusedField int
	systemProxyScrollOffset int
	systemProxyStatus       *netctl.SystemProxyStatus
}

type settingField struct {
	label       string
	helpText    string
	fieldType   FieldType
	key         string
	disabled    bool
	numericOnly bool
	manualApply bool
	// For input type
	input textinput.Model
	// For select type
	options     []selectOption
	selectedIdx int
}

type selectOption struct {
	label string
	value string
}

var languageOptions = []selectOption{
	{label: "🇺🇸 English", value: "en_us"},
	{label: "🇨🇳 简体中文", value: "zh_cn"},
	{label: "🇨🇳 繁體中文", value: "zh_tw"},
	{label: "🇷🇺 Русский", value: "ru_ru"},
	{label: "🇯🇵 日本語", value: "ja_jp"},
	{label: "🇰🇷 한국어", value: "ko_kr"},
	{label: "🇪🇸 Español", value: "es_es"},
	{label: "🇧🇷 Português (Brasil)", value: "pt_br"},
	{label: "🇫🇷 Français", value: "fr_fr"},
	{label: "🇩🇪 Deutsch", value: "de_de"},
	{label: "🇮🇹 Italiano", value: "it_it"},
	{label: "🇮🇩 Bahasa Indonesia", value: "id_id"},
	{label: "🇻🇳 Tiếng Việt", value: "vi_vn"},
	{label: "🇹🇷 Türkçe", value: "tr_tr"},
}

var serverDisplayOptions = []selectOption{
	{label: "Show", value: "show"},
	{label: "Blur", value: "blur"},
	{label: "Off", value: "off"},
}

var speedUnitOptions = []selectOption{
	{label: "MB/s (Bytes)", value: "bytes"},
	{label: "Mbps (Bits)", value: "bits"},
}

var upstreamProtocolOptions = []selectOption{
	{label: "HTTP", value: "http"},
	{label: "HTTPS", value: "https"},
	{label: "SOCKS5", value: "socks5"},
}

// NewSettingsTab creates a new settings tab
func NewSettingsTab(cfg *config.Config, slotMgr proxy.SlotService, version string) *SettingsTab {
	mixedPortInput := textinput.New()
	mixedPortInput.SetValue(fmt.Sprintf("%d", cfg.Proxy.MixedPort))
	mixedPortInput.CharLimit = 5

	allowLanIdx := 0
	if !cfg.Proxy.AllowLan {
		allowLanIdx = 1
	}
	allowLanOptions := []selectOption{
		{label: i18n.M().Settings.On, value: "true"},
		{label: i18n.M().Settings.Off, value: "false"},
	}

	updateViaProxyIdx := 1
	if cfg.Subscription.UpdateViaProxy {
		updateViaProxyIdx = 0
	}
	updateViaProxyOptions := []selectOption{
		{label: i18n.M().Settings.On, value: "true"},
		{label: i18n.M().Settings.Off, value: "false"},
	}

	tunIdx := 1
	if cfg.Proxy.TunEnabled {
		tunIdx = 0
	}
	tunOptions := []selectOption{
		{label: i18n.M().Settings.On, value: "true"},
		{label: i18n.M().Settings.Off, value: "false"},
	}

	dockerTCPIdx := 1
	if cfg.Proxy.DockerRedirectTCP {
		dockerTCPIdx = 0
	}
	dockerTCPOptions := []selectOption{
		{label: i18n.M().Settings.On, value: "true"},
		{label: i18n.M().Settings.Off, value: "false"},
	}

	concurrentInput := textinput.New()
	concurrentInput.SetValue(fmt.Sprintf("%d", cfg.SpeedTest.Concurrent))
	concurrentInput.CharLimit = 3

	// Find current language index
	langIdx := 0
	for i, opt := range languageOptions {
		if opt.value == cfg.Language {
			langIdx = i
			break
		}
	}

	serverDisplayIdx := 0
	for i, opt := range serverDisplayOptions {
		if opt.value == cfg.UI.ServerDisplay {
			serverDisplayIdx = i
			break
		}
	}

	speedUnitIdx := 0
	for i, opt := range speedUnitOptions {
		if opt.value == cfg.UI.SpeedUnit {
			speedUnitIdx = i
			break
		}
	}

	fields := []settingField{
		{
			label:       i18n.M().Settings.MixedPortLabel,
			helpText:    i18n.M().Settings.MixedPortHelp,
			fieldType:   FieldTypeInput,
			input:       mixedPortInput,
			key:         "mixed_port",
			numericOnly: true,
		},
		{
			label:     "Reset Port (17890)",
			fieldType: FieldTypeButton,
			key:       "reset_port",
		},
		{
			label:       i18n.M().Settings.AllowLan,
			fieldType:   FieldTypeSelect,
			key:         "allow_lan",
			options:     allowLanOptions,
			selectedIdx: allowLanIdx,
		},
		{
			label:       "Update subscriptions via core proxy",
			fieldType:   FieldTypeSelect,
			key:         "update_via_proxy",
			options:     updateViaProxyOptions,
			selectedIdx: updateViaProxyIdx,
		},
	}

	// Advanced network features: TUN is available on Linux and macOS; Docker hijack remains Linux-only.
	linuxSection := i18n.M().Settings.LinuxFeatures
	if linuxSection == "" {
		linuxSection = "Advanced Network Features"
	}
	fields = append(fields, settingField{
		label:     "Upstream Proxies",
		fieldType: FieldTypeHeader,
	})

	fields = append(fields, settingField{
		label:     "Edit Upstream Proxies >",
		helpText:  "Manage reusable upstream proxy definitions.",
		fieldType: FieldTypeButton,
		key:       "edit_upstream_proxies",
	})

	fields = append(fields, settingField{
		label:     linuxSection,
		fieldType: FieldTypeHeader,
	})

	tunDisabled := (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || os.Geteuid() != 0
	dockerDisabled := runtime.GOOS != "linux"

	fields = append(fields, settingField{
		label:     "System Proxy",
		fieldType: FieldTypeSelect,
		key:       "system_proxy_enabled",
		options:   tunOptions,
		selectedIdx: func() int {
			if cfg.Proxy.SystemProxy.Enabled {
				return 0
			}
			return 1
		}(),
		disabled: runtime.GOOS != "darwin",
	})

	fields = append(fields, settingField{
		label:     "System Proxy Settings >",
		helpText:  "macOS only. Applies to all network services.",
		fieldType: FieldTypeButton,
		key:       "system_proxy_settings",
		disabled:  runtime.GOOS != "darwin",
	})

	tunLabel := i18n.M().Settings.Tun
	if tunLabel == "" {
		tunLabel = "TUN mode"
	}
	fields = append(fields, settingField{
		label:       tunLabel,
		fieldType:   FieldTypeSelect,
		key:         "tun_enabled",
		options:     tunOptions,
		selectedIdx: tunIdx,
		disabled:    tunDisabled,
	})

	fields = append(fields, settingField{
		label:     "TUN Settings >",
		helpText:  "Main slot only. Advanced TUN configuration.",
		fieldType: FieldTypeButton,
		key:       "tun_settings",
		disabled:  tunDisabled,
	})

	dockerLabel := i18n.M().Settings.DockerTCP
	if dockerLabel == "" {
		dockerLabel = "Docker hijack (TCP, Linux)"
	}
	fields = append(fields, settingField{
		label:       dockerLabel,
		fieldType:   FieldTypeSelect,
		key:         "docker_redirect_tcp",
		options:     dockerTCPOptions,
		selectedIdx: dockerTCPIdx,
		disabled:    dockerDisabled,
	})

	fields = append(fields,
		settingField{
			label:       i18n.M().Settings.Language,
			fieldType:   FieldTypeSelect,
			key:         "language",
			options:     languageOptions,
			selectedIdx: langIdx,
		},
		settingField{
			label:       i18n.M().Settings.ServerDisplay,
			fieldType:   FieldTypeSelect,
			key:         "server_display",
			options:     serverDisplayOptions,
			selectedIdx: serverDisplayIdx,
		},
		settingField{
			label:       i18n.M().Settings.SpeedUnit,
			fieldType:   FieldTypeSelect,
			key:         "speed_unit",
			options:     speedUnitOptions,
			selectedIdx: speedUnitIdx,
		},
		settingField{
			label:       i18n.M().Settings.Concurrent,
			fieldType:   FieldTypeInput,
			input:       concurrentInput,
			key:         "concurrent",
			numericOnly: true,
		},
		settingField{
			label:     "System Info >",
			fieldType: FieldTypeButton,
			key:       "system_info",
		},
	)

	// Focus first field
	fields[0].input.Focus()

	tab := &SettingsTab{
		cfg:          cfg,
		slotMgr:      slotMgr,
		appVersion:   version,
		fields:       fields,
		armedReplace: true,
	}
	tab.slotFields = tab.newSlotSettingsFields()
	tab.loadSlotUpstreamDraft(0)
	tab.upstreamFields = newUpstreamDetailFields()
	tab.tunFields = tab.newTunSettingsFields()
	tab.loadTunDraft()
	tab.systemProxyFields = tab.newSystemProxySettingsFields()
	tab.loadSystemProxyDraft()
	return tab
}

func (t *SettingsTab) newSlotSettingsFields() []settingField {
	options := t.buildUpstreamSelectOptions()
	return []settingField{
		{
			label:     "Upstream Proxy",
			fieldType: FieldTypeHeader,
		},
		{
			label:       "Select upstream proxy",
			helpText:    "Choose an upstream proxy from the registry, or None.",
			fieldType:   FieldTypeSelect,
			key:         "slot_upstream_select",
			options:     options,
			selectedIdx: 0,
			manualApply: true,
		},
		{
			label:       "Apply Slot Settings",
			fieldType:   FieldTypeButton,
			key:         "slot_upstream_apply",
			manualApply: true,
		},
	}
}

func (t *SettingsTab) buildUpstreamSelectOptions() []selectOption {
	options := []selectOption{
		{label: "(None)", value: ""},
	}
	for _, entry := range t.cfg.ListUpstreamProxies() {
		label := entry.Label
		if label == "" {
			label = entry.ID
		}
		detail := fmt.Sprintf("%s://%s:%d", entry.Protocol, entry.Server, entry.Port)
		options = append(options, selectOption{
			label: fmt.Sprintf("%s (%s)", label, detail),
			value: entry.ID,
		})
	}
	return options
}

func (t *SettingsTab) newTunSettingsFields() []settingField {
	tunDefaultStack := "system"
	if runtime.GOOS == "darwin" {
		tunDefaultStack = "mixed"
	}
	tunStackOptions := []selectOption{{label: "mixed", value: "mixed"}, {label: "gvisor", value: "gvisor"}, {label: "system", value: "system"}}
	tunStackIdx := 0
	for i, opt := range tunStackOptions {
		if opt.value == t.cfg.Proxy.TunStack || (t.cfg.Proxy.TunStack == "" && opt.value == tunDefaultStack) {
			tunStackIdx = i
			break
		}
	}
	tunDeviceInput := textinput.New()
	tunDeviceInput.CharLimit = 64
	tunDeviceInput.SetValue(t.cfg.Proxy.TunDevice)
	tunMTUInput := textinput.New()
	tunMTUInput.CharLimit = 5
	if t.cfg.Proxy.TunMTU > 0 {
		tunMTUInput.SetValue(fmt.Sprintf("%d", t.cfg.Proxy.TunMTU))
	}
	dnsHijackInput := textinput.New()
	dnsHijackInput.CharLimit = 255
	dnsHijackInput.SetValue(strings.Join(t.cfg.Proxy.TunDNSHijack, ", "))
	routeExcludeInput := textinput.New()
	routeExcludeInput.CharLimit = 255
	routeExcludeInput.SetValue(strings.Join(t.cfg.Proxy.TunRouteExcludeAddress, ", "))
	autoRouteIdx := 1
	if t.cfg.Proxy.TunAutoRoute {
		autoRouteIdx = 0
	}
	strictRouteIdx := 1
	if t.cfg.Proxy.TunStrictRoute {
		strictRouteIdx = 0
	}
	autoDetectIdx := 1
	if t.cfg.Proxy.TunAutoDetectInterface {
		autoDetectIdx = 0
	}
	onOff := []selectOption{{label: i18n.M().Settings.On, value: "true"}, {label: i18n.M().Settings.Off, value: "false"}}
	return []settingField{
		{label: "Tun Mode", fieldType: FieldTypeHeader},
		{label: "Tun Stack", helpText: fmt.Sprintf("Main slot only. Default: %s", tunDefaultStack), fieldType: FieldTypeSelect, key: "tun_stack", options: tunStackOptions, selectedIdx: tunStackIdx},
		{label: "Device Name", helpText: "macOS: leave empty and let the system assign utun automatically.", fieldType: FieldTypeInput, key: "tun_device", input: tunDeviceInput},
		{label: "Auto Route", fieldType: FieldTypeSelect, key: "tun_auto_route", options: onOff, selectedIdx: autoRouteIdx},
		{label: "Strict Route", fieldType: FieldTypeSelect, key: "tun_strict_route", options: onOff, selectedIdx: strictRouteIdx},
		{label: "Auto Detect Interface", fieldType: FieldTypeSelect, key: "tun_auto_detect_interface", options: onOff, selectedIdx: autoDetectIdx},
		{label: "DNS Hijack", helpText: "Comma-separated values like any:53", fieldType: FieldTypeInput, key: "tun_dns_hijack", input: dnsHijackInput},
		{label: "Max Transmission Unit", helpText: "Optional. Leave empty to use mihomo default.", fieldType: FieldTypeInput, key: "tun_mtu", input: tunMTUInput, numericOnly: true},
		{label: "Route Exclude Address", helpText: "Comma-separated CIDRs.", fieldType: FieldTypeInput, key: "tun_route_exclude_address", input: routeExcludeInput},
	}
}

func (t *SettingsTab) loadTunDraft() {
	t.tunFocusedField = 1
	t.tunScrollOffset = 0
	for i := range t.tunFields {
		if t.tunFields[i].fieldType == FieldTypeInput {
			t.tunFields[i].input.Blur()
		}
	}
	if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeInput {
		t.tunFields[t.tunFocusedField].input.Focus()
	}
}

func (t *SettingsTab) newSystemProxySettingsFields() []settingField {
	hostInput := textinput.New()
	hostInput.CharLimit = 255
	hostInput.SetValue(t.cfg.Proxy.SystemProxy.Host)
	guardInput := textinput.New()
	guardInput.CharLimit = 4
	guardInput.SetValue(fmt.Sprintf("%d", t.cfg.Proxy.SystemProxy.GuardDurationSeconds))
	bypassInput := textinput.New()
	bypassInput.CharLimit = 255
	bypassInput.SetValue(strings.Join(t.cfg.Proxy.SystemProxy.Bypass, ", "))
	onOff := []selectOption{{label: i18n.M().Settings.On, value: "true"}, {label: i18n.M().Settings.Off, value: "false"}}
	pacIdx := 1
	if t.cfg.Proxy.SystemProxy.PACMode {
		pacIdx = 0
	}
	guardIdx := 1
	if t.cfg.Proxy.SystemProxy.ProxyGuard {
		guardIdx = 0
	}
	alwaysBypassIdx := 1
	if t.cfg.Proxy.SystemProxy.AlwaysUseDefaultBypass {
		alwaysBypassIdx = 0
	}
	validateIdx := 1
	if t.cfg.Proxy.SystemProxy.ValidateBypassFormat {
		validateIdx = 0
	}
	return []settingField{
		{label: "System Proxy Setting", fieldType: FieldTypeHeader},
		{label: "Proxy Host", fieldType: FieldTypeInput, key: "system_proxy_host", input: hostInput},
		{label: "Use PAC Mode", fieldType: FieldTypeSelect, key: "system_proxy_pac_mode", options: onOff, selectedIdx: pacIdx},
		{label: "Proxy Guard", fieldType: FieldTypeSelect, key: "system_proxy_guard", options: onOff, selectedIdx: guardIdx},
		{label: "Guard Duration", helpText: "Seconds", fieldType: FieldTypeInput, key: "system_proxy_guard_duration", input: guardInput, numericOnly: true},
		{label: "Always use Default Bypass", fieldType: FieldTypeSelect, key: "system_proxy_default_bypass", options: onOff, selectedIdx: alwaysBypassIdx},
		{label: "Validate Proxy Bypass Format", fieldType: FieldTypeSelect, key: "system_proxy_validate_bypass", options: onOff, selectedIdx: validateIdx},
		{label: "Bypass", helpText: "Comma-separated values.", fieldType: FieldTypeInput, key: "system_proxy_bypass", input: bypassInput},
		{label: "Save", fieldType: FieldTypeButton, key: "system_proxy_save"},
	}
}

func (t *SettingsTab) loadSystemProxyDraft() {
	t.systemProxyFocusedField = 1
	t.systemProxyScrollOffset = 0
	for i := range t.systemProxyFields {
		if t.systemProxyFields[i].fieldType == FieldTypeInput {
			t.systemProxyFields[i].input.Blur()
		}
	}
	if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeInput {
		t.systemProxyFields[t.systemProxyFocusedField].input.Focus()
	}
}

func newUpstreamDetailFields() []settingField {
	labelInput := textinput.New()
	labelInput.Placeholder = "My VPN"
	labelInput.CharLimit = 64

	serverInput := textinput.New()
	serverInput.Placeholder = "gateway.example.com"
	serverInput.CharLimit = 255

	portInput := textinput.New()
	portInput.CharLimit = 5

	usernameInput := textinput.New()
	usernameInput.CharLimit = 255

	passwordInput := textinput.New()
	passwordInput.CharLimit = 255

	return []settingField{
		{
			label:     "Upstream Proxy Details",
			fieldType: FieldTypeHeader,
		},
		{
			label:       "Label",
			helpText:    "A display name for this upstream proxy.",
			fieldType:   FieldTypeInput,
			input:       labelInput,
			key:         "upstream_label",
			manualApply: true,
		},
		{
			label:       "Protocol",
			fieldType:   FieldTypeSelect,
			key:         "upstream_protocol",
			options:     upstreamProtocolOptions,
			selectedIdx: 0,
			manualApply: true,
		},
		{
			label:       "Server",
			fieldType:   FieldTypeInput,
			input:       serverInput,
			key:         "upstream_server",
			manualApply: true,
		},
		{
			label:       "Port",
			fieldType:   FieldTypeInput,
			input:       portInput,
			key:         "upstream_port",
			numericOnly: true,
			manualApply: true,
		},
		{
			label:       "Username",
			fieldType:   FieldTypeInput,
			input:       usernameInput,
			key:         "upstream_username",
			manualApply: true,
		},
		{
			label:       "Password",
			fieldType:   FieldTypeInput,
			input:       passwordInput,
			key:         "upstream_password",
			manualApply: true,
		},
		{
			label:       "Save",
			fieldType:   FieldTypeButton,
			key:         "upstream_save",
			manualApply: true,
		},
	}
}

// Init initializes the tab
func (t *SettingsTab) Init() tea.Cmd {
	return textinput.Blink
}

func (t *SettingsTab) focusedInputField() *settingField {
	if t.focusedField < 0 || t.focusedField >= len(t.fields) {
		return nil
	}
	if t.fields[t.focusedField].fieldType != FieldTypeInput || t.fields[t.focusedField].disabled {
		return nil
	}
	return &t.fields[t.focusedField]
}

func (t *SettingsTab) focusedSlotInputField() *settingField {
	if t.slotFocusedField < 0 || t.slotFocusedField >= len(t.slotFields) {
		return nil
	}
	if t.slotFields[t.slotFocusedField].fieldType != FieldTypeInput || t.slotFields[t.slotFocusedField].disabled {
		return nil
	}
	return &t.slotFields[t.slotFocusedField]
}

func (t *SettingsTab) updateInputField(field *settingField, msg tea.KeyMsg) tea.Cmd {
	if field == nil {
		return nil
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		if field.numericOnly {
			digits := filterDigits(msg.Runes)
			if len(digits) == 0 {
				return nil
			}
			msg.Runes = digits
		}
		if t.armedReplace {
			field.input.SetValue("")
			field.input.SetCursor(0)
			t.armedReplace = false
		}
	} else {
		// Any non-rune edit/navigation key means we are modifying the current value in place.
		t.armedReplace = false
	}

	var cmd tea.Cmd
	field.input, cmd = field.input.Update(msg)
	return cmd
}

// Update handles messages
func (t *SettingsTab) Update(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	// System Info secondary page intercepts all keys
	if t.showSystemInfo {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))) {
				t.showSystemInfo = false
				t.systemInfo = nil
			}
			return t, nil
		case systemInfoFetchedMsg:
			t.systemInfo = &msg.info
			return t, nil
		}
		return t, nil
	}

	if t.showUpstreamDetail {
		return t.updateUpstreamDetail(msg)
	}

	if t.showUpstreamList {
		return t.updateUpstreamList(msg)
	}

	if t.showSlotDetail {
		return t.updateSlotDetail(msg)
	}

	if t.showSlotList {
		return t.updateSlotList(msg)
	}

	if t.showTunSettings {
		return t.updateTunSettings(msg)
	}

	if t.showSystemProxySettings {
		return t.updateSystemProxySettings(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if field := t.focusedInputField(); field != nil {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				t.focusPrev()
				if t.pendingSave {
					t.pendingSave = false
					return t, t.saveSettings()
				}
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				t.focusNext()
				if t.pendingSave {
					t.pendingSave = false
					return t, t.saveSettings()
				}
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
				return t, func() tea.Msg { return RestartCoreMsg{} }
			default:
				return t, t.updateInputField(field, msg)
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.focusPrev()
			if t.pendingSave {
				t.pendingSave = false
				return t, t.saveSettings()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.focusNext()
			if t.pendingSave {
				t.pendingSave = false
				return t, t.saveSettings()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if t.focusedField < len(t.fields) {
				field := &t.fields[t.focusedField]
				if field.fieldType == FieldTypeButton && field.key == "reset_port" {
					// Reset port to default 17890
					for i := range t.fields {
						if t.fields[i].key == "mixed_port" {
							t.fields[i].input.SetValue("17890")
							break
						}
					}
					return t, t.saveSettings()
				}
				if field.fieldType == FieldTypeButton && field.key == "system_info" {
					t.showSystemInfo = true
					t.systemInfo = nil
					return t, t.fetchSystemInfo()
				}
				if field.fieldType == FieldTypeButton && field.key == "edit_upstream_proxies" {
					t.showUpstreamList = true
					t.showUpstreamDetail = false
					return t, nil
				}
				if field.fieldType == FieldTypeButton && field.key == "edit_slots" {
					// Refresh slot fields in case upstream proxies were added/removed.
					t.slotFields = t.newSlotSettingsFields()
					t.loadSlotUpstreamDraft(t.slotListIndex)
					t.showSlotList = true
					t.showSlotDetail = false
					return t, nil
				}
				if field.fieldType == FieldTypeButton && field.key == "tun_settings" {
					t.tunFields = t.newTunSettingsFields()
					t.loadTunDraft()
					t.showTunSettings = true
					return t, nil
				}
				if field.fieldType == FieldTypeButton && field.key == "system_proxy_settings" {
					t.systemProxyFields = t.newSystemProxySettingsFields()
					t.loadSystemProxyDraft()
					t.showSystemProxySettings = true
					return t, nil
				}
			}
			if t.focusedField == len(t.fields) {
				// Restart core button
				return t, func() tea.Msg { return RestartCoreMsg{} }
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
			return t, func() tea.Msg { return RestartCoreMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			// For select fields, move selection left and save immediately
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeSelect && !t.fields[t.focusedField].disabled {
				field := &t.fields[t.focusedField]
				field.selectedIdx--
				if field.selectedIdx < 0 {
					field.selectedIdx = len(field.options) - 1
				}
				if field.key == "tun_enabled" {
					t.applyTunFieldState()
				}
				return t, t.saveSettings()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			// For select fields, move selection right and save immediately
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeSelect && !t.fields[t.focusedField].disabled {
				field := &t.fields[t.focusedField]
				field.selectedIdx++
				if field.selectedIdx >= len(field.options) {
					field.selectedIdx = 0
				}
				if field.key == "tun_enabled" {
					t.applyTunFieldState()
				}
				return t, t.saveSettings()
			}
		}

	case SettingsSavedMsg:
		if msg.Message != "" {
			t.message = msg.Message
		} else {
			t.message = i18n.M().Settings.Saved
		}
		t.messageType = "success"
		t.messageShow = true
		return t, t.startFlash(3 * time.Second)

	case tunChangedSaveMsg:
			t.message = "TUN setting changed — restarting all slots..."
			if msg.notice != "" {
				t.message = msg.notice + " TUN setting changed — restarting all slots..."
			}
			t.messageType = "success"
			t.messageShow = true
			return t, tea.Batch(
				t.startFlash(3*time.Second),
				func() tea.Msg { return RestartAllSlotsMsg{} },
			)

		case portChangedSaveMsg:
		t.message = "Port changed — restarting all slots..."
		if msg.notice != "" {
			t.message = msg.notice + " Port changed — restarting all slots..."
		}
		t.messageType = "success"
		t.messageShow = true
		return t, tea.Batch(
			t.startFlash(3*time.Second),
			func() tea.Msg { return RestartAllSlotsMsg{} },
		)

	case slotUpstreamSavedMsg:
		if msg.message != "" {
			t.message = msg.message
		} else {
			t.message = fmt.Sprintf("Saved upstream settings for slot %d", msg.slotID)
		}
		t.messageType = "success"
		t.messageShow = true
		if msg.restart {
			return t, tea.Batch(
				t.startFlash(3*time.Second),
				func() tea.Msg { return RestartSlotMsg{SlotID: msg.slotID} },
			)
		}
		return t, t.startFlash(3 * time.Second)

	case SettingsErrorMsg:
		if msg.Error != nil {
			t.message = msg.Error.Error()
		} else {
			t.message = "unknown error"
		}
		t.messageType = "error"
		t.messageShow = true

	case settingsFlashTickMsg:
		if msg.id != t.flashID {
			return t, nil
		}
		t.flashTicks--
		if t.flashTicks <= 0 {
			t.message = ""
			t.messageType = ""
			t.messageShow = false
			return t, nil
		}
		t.messageShow = !t.messageShow
		return t, t.nextFlashTick()
	}

	return t, nil
}

func (t *SettingsTab) focusNext() {
	if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput {
		t.fields[t.focusedField].input.Blur()
		if !t.fields[t.focusedField].manualApply {
			t.pendingSave = true
		}
	}
	for {
		t.focusedField++
		if t.focusedField > len(t.fields) {
			t.focusedField = 0
		}
		if t.focusedField >= len(t.fields) || t.isFieldFocusable(t.focusedField) {
			break
		}
	}
	if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput {
		t.fields[t.focusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) focusPrev() {
	if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput {
		t.fields[t.focusedField].input.Blur()
		if !t.fields[t.focusedField].manualApply {
			t.pendingSave = true
		}
	}
	for {
		t.focusedField--
		if t.focusedField < 0 {
			t.focusedField = len(t.fields)
		}
		if t.focusedField >= len(t.fields) || t.isFieldFocusable(t.focusedField) {
			break
		}
	}
	if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput {
		t.fields[t.focusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) isFieldFocusable(index int) bool {
	if index < 0 || index >= len(t.fields) {
		return false
	}
	f := t.fields[index]
	if f.fieldType == FieldTypeHeader {
		return false
	}
	if f.disabled {
		return false
	}
	return true
}

func (t *SettingsTab) applyTunFieldState() {
	enabled := false
	if field := t.fieldByKey("tun_enabled"); field != nil && len(field.options) > 0 {
		enabled = field.options[field.selectedIdx].value == "true"
	}
	for _, key := range []string{"tun_stack", "tun_mtu", "tun_device"} {
		if field := t.fieldByKey(key); field != nil {
			field.disabled = !enabled
		}
	}
}
func filterDigits(runes []rune) []rune {
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return out
}

func (t *SettingsTab) fieldByKey(key string) *settingField {
	for i := range t.fields {
		if t.fields[i].key == key {
			return &t.fields[i]
		}
	}
	for i := range t.slotFields {
		if t.slotFields[i].key == key {
			return &t.slotFields[i]
		}
	}
	return nil
}

func (t *SettingsTab) slotFieldByKey(key string) *settingField {
	for i := range t.slotFields {
		if t.slotFields[i].key == key {
			return &t.slotFields[i]
		}
	}
	return nil
}

func (t *SettingsTab) tunFieldByKey(key string) *settingField {
	for i := range t.tunFields {
		if t.tunFields[i].key == key {
			return &t.tunFields[i]
		}
	}
	return nil
}

func (t *SettingsTab) systemProxyFieldByKey(key string) *settingField {
	for i := range t.systemProxyFields {
		if t.systemProxyFields[i].key == key {
			return &t.systemProxyFields[i]
		}
	}
	return nil
}

func (t *SettingsTab) selectedSlotDraft() int {
	return t.slotUpstreamSlot
}

func (t *SettingsTab) loadSlotUpstreamDraft(slotID int) {
	t.slotUpstreamSlot = slotID

	// Refresh the options in case registry changed.
	if field := t.slotFieldByKey("slot_upstream_select"); field != nil {
		field.options = t.buildUpstreamSelectOptions()
		field.selectedIdx = 0 // default to (None)
		ref, _ := t.cfg.GetSlotUpstreamRef(slotID)
		if ref != "" {
			for i, opt := range field.options {
				if opt.value == ref {
					field.selectedIdx = i
					break
				}
			}
		}
	}

	t.slotFocusedField = 1
	t.slotScrollOffset = 0
	for i := range t.slotFields {
		if t.slotFields[i].fieldType == FieldTypeInput {
			t.slotFields[i].input.Blur()
		}
	}
}

func parseCSVList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (t *SettingsTab) focusedTunInputField() *settingField {
	if t.tunFocusedField < 0 || t.tunFocusedField >= len(t.tunFields) {
		return nil
	}
	if t.tunFields[t.tunFocusedField].fieldType != FieldTypeInput || t.tunFields[t.tunFocusedField].disabled {
		return nil
	}
	return &t.tunFields[t.tunFocusedField]
}

func (t *SettingsTab) focusedSystemProxyInputField() *settingField {
	if t.systemProxyFocusedField < 0 || t.systemProxyFocusedField >= len(t.systemProxyFields) {
		return nil
	}
	if t.systemProxyFields[t.systemProxyFocusedField].fieldType != FieldTypeInput || t.systemProxyFields[t.systemProxyFocusedField].disabled {
		return nil
	}
	return &t.systemProxyFields[t.systemProxyFocusedField]
}

func (t *SettingsTab) tunFocusNext() {
	if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeInput {
		t.tunFields[t.tunFocusedField].input.Blur()
	}
	for {
		t.tunFocusedField++
		if t.tunFocusedField >= len(t.tunFields) {
			t.tunFocusedField = 0
		}
		if t.tunFocusedField >= len(t.tunFields) || (t.tunFields[t.tunFocusedField].fieldType != FieldTypeHeader && !t.tunFields[t.tunFocusedField].disabled) {
			break
		}
	}
	if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeInput {
		t.tunFields[t.tunFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) tunFocusPrev() {
	if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeInput {
		t.tunFields[t.tunFocusedField].input.Blur()
	}
	for {
		t.tunFocusedField--
		if t.tunFocusedField < 0 {
			t.tunFocusedField = len(t.tunFields) - 1
		}
		if t.tunFocusedField >= len(t.tunFields) || (t.tunFields[t.tunFocusedField].fieldType != FieldTypeHeader && !t.tunFields[t.tunFocusedField].disabled) {
			break
		}
	}
	if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeInput {
		t.tunFields[t.tunFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) systemProxyFocusNext() {
	if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeInput {
		t.systemProxyFields[t.systemProxyFocusedField].input.Blur()
	}
	for {
		t.systemProxyFocusedField++
		if t.systemProxyFocusedField >= len(t.systemProxyFields) {
			t.systemProxyFocusedField = 0
		}
		if t.systemProxyFocusedField >= len(t.systemProxyFields) || (t.systemProxyFields[t.systemProxyFocusedField].fieldType != FieldTypeHeader && !t.systemProxyFields[t.systemProxyFocusedField].disabled) {
			break
		}
	}
	if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeInput {
		t.systemProxyFields[t.systemProxyFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) systemProxyFocusPrev() {
	if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeInput {
		t.systemProxyFields[t.systemProxyFocusedField].input.Blur()
	}
	for {
		t.systemProxyFocusedField--
		if t.systemProxyFocusedField < 0 {
			t.systemProxyFocusedField = len(t.systemProxyFields) - 1
		}
		if t.systemProxyFocusedField >= len(t.systemProxyFields) || (t.systemProxyFields[t.systemProxyFocusedField].fieldType != FieldTypeHeader && !t.systemProxyFields[t.systemProxyFocusedField].disabled) {
			break
		}
	}
	if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeInput {
		t.systemProxyFields[t.systemProxyFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) saveTunSettings() tea.Cmd {
	return func() tea.Msg {
		for _, field := range t.tunFields {
			switch field.key {
			case "tun_stack":
				t.cfg.Proxy.TunStack = field.options[field.selectedIdx].value
			case "tun_device":
				t.cfg.Proxy.TunDevice = strings.TrimSpace(field.input.Value())
			case "tun_auto_route":
				t.cfg.Proxy.TunAutoRoute = field.options[field.selectedIdx].value == "true"
			case "tun_strict_route":
				t.cfg.Proxy.TunStrictRoute = field.options[field.selectedIdx].value == "true"
			case "tun_auto_detect_interface":
				t.cfg.Proxy.TunAutoDetectInterface = field.options[field.selectedIdx].value == "true"
			case "tun_dns_hijack":
				t.cfg.Proxy.TunDNSHijack = parseCSVList(field.input.Value())
			case "tun_mtu":
				v := strings.TrimSpace(field.input.Value())
				if v == "" {
					t.cfg.Proxy.TunMTU = 0
				} else {
					n, err := strconv.Atoi(v)
					if err != nil || n <= 0 || n >= 65536 {
						return SettingsErrorMsg{Error: fmt.Errorf("invalid tun_mtu value: %q", field.input.Value())}
					}
					t.cfg.Proxy.TunMTU = n
				}
			case "tun_route_exclude_address":
				t.cfg.Proxy.TunRouteExcludeAddress = parseCSVList(field.input.Value())
			}
		}
		if err := t.cfg.Save(); err != nil {
			return SettingsErrorMsg{Error: err}
		}
		return tunChangedSaveMsg{notice: "Saved TUN settings"}
	}
}

func (t *SettingsTab) saveSystemProxySettings() tea.Cmd {
	return func() tea.Msg {
		for _, field := range t.systemProxyFields {
			switch field.key {
			case "system_proxy_host":
				t.cfg.Proxy.SystemProxy.Host = strings.TrimSpace(field.input.Value())
			case "system_proxy_pac_mode":
				t.cfg.Proxy.SystemProxy.PACMode = field.options[field.selectedIdx].value == "true"
			case "system_proxy_guard":
				t.cfg.Proxy.SystemProxy.ProxyGuard = field.options[field.selectedIdx].value == "true"
			case "system_proxy_guard_duration":
				v := strings.TrimSpace(field.input.Value())
				if v != "" {
					n, err := strconv.Atoi(v)
					if err != nil || n <= 0 {
						return SettingsErrorMsg{Error: fmt.Errorf("invalid guard duration: %q", field.input.Value())}
					}
					t.cfg.Proxy.SystemProxy.GuardDurationSeconds = n
				}
			case "system_proxy_default_bypass":
				t.cfg.Proxy.SystemProxy.AlwaysUseDefaultBypass = field.options[field.selectedIdx].value == "true"
			case "system_proxy_validate_bypass":
				t.cfg.Proxy.SystemProxy.ValidateBypassFormat = field.options[field.selectedIdx].value == "true"
			case "system_proxy_bypass":
				t.cfg.Proxy.SystemProxy.Bypass = parseCSVList(field.input.Value())
			}
		}
		if err := t.cfg.Save(); err != nil {
			return SettingsErrorMsg{Error: err}
		}
		status, err := netctl.ReadSystemProxyStatus()
		if err == nil {
			t.systemProxyStatus = status
		}
		return SettingsSavedMsg{Message: "Saved system proxy settings"}
	}
}

func (t *SettingsTab) updateTunSettings(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if field := t.focusedTunInputField(); field != nil {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				t.showTunSettings = false
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				cmd := t.saveTunSettings()
				t.tunFocusPrev()
				return t, cmd
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				cmd := t.saveTunSettings()
				t.tunFocusNext()
				return t, cmd
			default:
				return t, t.updateInputField(field, msg)
			}
		}
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.showTunSettings = false
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			cmd := t.saveTunSettings()
			t.tunFocusPrev()
			return t, cmd
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			cmd := t.saveTunSettings()
			t.tunFocusNext()
			return t, cmd
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeSelect && !t.tunFields[t.tunFocusedField].disabled {
				f := &t.tunFields[t.tunFocusedField]
				f.selectedIdx--
				if f.selectedIdx < 0 {
					f.selectedIdx = len(f.options) - 1
				}
				return t, t.saveTunSettings()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if t.tunFocusedField < len(t.tunFields) && t.tunFields[t.tunFocusedField].fieldType == FieldTypeSelect && !t.tunFields[t.tunFocusedField].disabled {
				f := &t.tunFields[t.tunFocusedField]
				f.selectedIdx++
				if f.selectedIdx >= len(f.options) {
					f.selectedIdx = 0
				}
				return t, t.saveTunSettings()
			}
		}
	case tunChangedSaveMsg:
		t.showTunSettings = false
		return t, tea.Batch(
			t.startFlash(3*time.Second),
			func() tea.Msg { return RestartAllSlotsMsg{} },
		)
	case SettingsSavedMsg, SettingsErrorMsg, settingsFlashTickMsg:
		return t, nil
	}
	return t, nil
}

func (t *SettingsTab) updateSystemProxySettings(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if field := t.focusedSystemProxyInputField(); field != nil {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				t.showSystemProxySettings = false
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				t.systemProxyFocusPrev()
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				t.systemProxyFocusNext()
				return t, nil
			default:
				return t, t.updateInputField(field, msg)
			}
		}
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.showSystemProxySettings = false
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.systemProxyFocusPrev()
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.systemProxyFocusNext()
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeButton && t.systemProxyFields[t.systemProxyFocusedField].key == "system_proxy_save" {
				return t, t.saveSystemProxySettings()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeSelect && !t.systemProxyFields[t.systemProxyFocusedField].disabled {
				f := &t.systemProxyFields[t.systemProxyFocusedField]
				f.selectedIdx--
				if f.selectedIdx < 0 {
					f.selectedIdx = len(f.options) - 1
				}
				return t, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if t.systemProxyFocusedField < len(t.systemProxyFields) && t.systemProxyFields[t.systemProxyFocusedField].fieldType == FieldTypeSelect && !t.systemProxyFields[t.systemProxyFocusedField].disabled {
				f := &t.systemProxyFields[t.systemProxyFocusedField]
				f.selectedIdx++
				if f.selectedIdx >= len(f.options) {
					f.selectedIdx = 0
				}
				return t, nil
			}
		}
	case SettingsSavedMsg, SettingsErrorMsg, settingsFlashTickMsg:
		return t, nil
	}
	return t, nil
}

func renderSettingsSubpage(title string, fields []settingField, focused int, scrollOffset *int, width, height int, message, messageType string, messageShow bool) string {
	var lines []string
	focusedLine := 0
	lines = append(lines, styles.Title.Render(title), "")

	appendBlock := func(block string) {
		lines = append(lines, strings.Split(block, "\n")...)
	}
	appendLine := func(line string) {
		lines = append(lines, line)
	}

	for i, field := range fields {
		if field.fieldType == FieldTypeHeader {
			if i == focused {
				focusedLine = len(lines)
			}
			appendBlock(styles.Title.Render(field.label))
			appendLine("")
			continue
		}
		if field.fieldType == FieldTypeButton {
			if i == focused {
				focusedLine = len(lines)
			}
			btnLabel := field.label
			if i == focused {
				btnLabel = styles.StatusConnected.Render("▶ [" + btnLabel + "]")
			} else {
				btnLabel = styles.Muted.Render("  [" + btnLabel + "]")
			}
			appendBlock(btnLabel)
			appendLine("")
			continue
		}
		if i == focused {
			focusedLine = len(lines)
		}
		label := field.label
		if i == focused {
			label = styles.Primary.Render("▶ " + label)
		} else {
			label = "  " + label
		}
		if field.disabled {
			label = styles.Muted.Render("  " + field.label)
		}
		appendBlock(label)
		if field.helpText != "" {
			helpText := "    " + field.helpText
			if field.disabled {
				helpText = styles.Muted.Render(helpText)
			} else {
				helpText = styles.HelpText.Render(helpText)
			}
			appendBlock(helpText)
		}
		switch field.fieldType {
		case FieldTypeInput:
			inputStyle := styles.InputField
			if i == focused {
				inputStyle = styles.InputFocused
			}
			appendBlock(inputStyle.Render(field.input.View()))
			appendLine("")
		case FieldTypeSelect:
			appendBlock(renderSelectStatic(field, i == focused))
			appendLine("")
		}
	}

	if message != "" && messageShow {
		appendLine("")
		if messageType == "success" {
			appendBlock(styles.SuccessMessage.Render(message))
		} else {
			appendBlock(styles.ErrorMessage.Render(message))
		}
	}

	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" navigate  ") +
		styles.HelpKey.Render("←/→") + styles.HelpText.Render(" select  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" save  ") +
		styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	if height <= 0 {
		return styles.ContentArea.Render(strings.Join(append(lines, "", help), "\n"))
	}

	needsScroll := len(lines) > height-1
	showScrollIndicator := needsScroll && height > 2
	reservedBottom := 1
	if showScrollIndicator {
		reservedBottom = 2
	}
	visibleHeight := height - reservedBottom
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if focusedLine < *scrollOffset {
		*scrollOffset = focusedLine
	}
	if focusedLine >= *scrollOffset+visibleHeight {
		*scrollOffset = focusedLine - visibleHeight + 1
	}
	if *scrollOffset < 0 {
		*scrollOffset = 0
	}
	maxOffset := len(lines) - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if *scrollOffset > maxOffset {
		*scrollOffset = maxOffset
	}
	start := *scrollOffset
	end := *scrollOffset + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}
	content := strings.Join(lines[start:end], "\n")
	if showScrollIndicator {
		scrollText := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(lines))
		scrollInfo := lipgloss.NewStyle().Foreground(styles.MutedColor).Width(width).Align(lipgloss.Right).Render(scrollText)
		content += "\n" + scrollInfo
	}
	content += "\n" + help
	return styles.ContentArea.Render(content)
}

func renderSelectStatic(field settingField, focused bool) string {
	if len(field.options) == 0 {
		return ""
	}
	idx := field.selectedIdx
	if idx < 0 || idx >= len(field.options) {
		idx = 0
	}
	opt := field.options[idx]
	label := " " + opt.label + " "
	if field.disabled {
		return styles.Muted.Render("◀") + " " + styles.Muted.Render(label) + " " + styles.Muted.Render("▶")
	}
	if focused {
		label = styles.SelectOptionActive.Render(label)
	} else {
		label = styles.SelectOptionSelected.Render(label)
	}
	return styles.Muted.Render("◀") + " " + label + " " + styles.Muted.Render("▶")
}

func (t *SettingsTab) viewTunSettings() string {
	return renderSettingsSubpage("Tun Mode", t.tunFields, t.tunFocusedField, &t.tunScrollOffset, t.width, t.height, t.message, t.messageType, t.messageShow)
}

func (t *SettingsTab) viewSystemProxySettings() string {
	title := "System Proxy Setting"
	if t.systemProxyStatus != nil {
		state := "Disabled"
		if t.systemProxyStatus.Enabled {
			state = "Enabled"
		}
		title = fmt.Sprintf("System Proxy Setting (%s)", state)
	}
	return renderSettingsSubpage(title, t.systemProxyFields, t.systemProxyFocusedField, &t.systemProxyScrollOffset, t.width, t.height, t.message, t.messageType, t.messageShow)
}

func (t *SettingsTab) saveSlotUpstream() tea.Cmd {
	return func() tea.Msg {
		slotID := t.selectedSlotDraft()
		if slotID < 0 || slotID >= proxy.MaxSlots {
			return SettingsErrorMsg{Error: fmt.Errorf("invalid slot id: %d", slotID)}
		}

		upstreamID := ""
		if field := t.slotFieldByKey("slot_upstream_select"); field != nil && len(field.options) > 0 {
			upstreamID = field.options[field.selectedIdx].value
		}

		t.cfg.SetSlotUpstreamRef(slotID, upstreamID)
		if err := t.cfg.Save(); err != nil {
			return SettingsErrorMsg{Error: err}
		}

		restart := false
		if t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil {
			restart = true
		}
		message := fmt.Sprintf("Saved slot %d upstream settings", slotID)
		if upstreamID == "" {
			message = fmt.Sprintf("Saved slot %d upstream settings (no upstream)", slotID)
		}
		if restart {
			message += " and restarting the slot"
		}
		return slotUpstreamSavedMsg{
			slotID:  slotID,
			restart: restart,
			message: message,
		}
	}
}

func (t *SettingsTab) updateSlotList(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.showSlotList = false
			t.showSlotDetail = false
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.slotListIndex--
			if t.slotListIndex < 0 {
				t.slotListIndex = proxy.MaxSlots - 1
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.slotListIndex++
			if t.slotListIndex >= proxy.MaxSlots {
				t.slotListIndex = 0
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			t.loadSlotUpstreamDraft(t.slotListIndex)
			t.showSlotDetail = true
			return t, nil
		}
	case settingsFlashTickMsg:
		if msg.id != t.flashID {
			return t, nil
		}
		t.flashTicks--
		if t.flashTicks <= 0 {
			t.message = ""
			t.messageType = ""
			t.messageShow = false
			return t, nil
		}
		t.messageShow = !t.messageShow
		return t, t.nextFlashTick()
	}

	return t, nil
}

func (t *SettingsTab) updateSlotDetail(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if field := t.focusedSlotInputField(); field != nil {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				t.showSlotDetail = false
				for i := range t.slotFields {
					if t.slotFields[i].fieldType == FieldTypeInput {
						t.slotFields[i].input.Blur()
					}
				}
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				t.slotFocusPrev()
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				t.slotFocusNext()
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				return t, nil
			default:
				return t, t.updateInputField(field, msg)
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.showSlotDetail = false
			for i := range t.slotFields {
				if t.slotFields[i].fieldType == FieldTypeInput {
					t.slotFields[i].input.Blur()
				}
			}
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.slotFocusPrev()
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.slotFocusNext()
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if t.slotFocusedField < len(t.slotFields) {
				field := &t.slotFields[t.slotFocusedField]
				if field.fieldType == FieldTypeButton && field.key == "slot_upstream_apply" {
					return t, t.saveSlotUpstream()
				}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeSelect && !t.slotFields[t.slotFocusedField].disabled {
				field := &t.slotFields[t.slotFocusedField]
				field.selectedIdx--
				if field.selectedIdx < 0 {
					field.selectedIdx = len(field.options) - 1
				}
				return t, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeSelect && !t.slotFields[t.slotFocusedField].disabled {
				field := &t.slotFields[t.slotFocusedField]
				field.selectedIdx++
				if field.selectedIdx >= len(field.options) {
					field.selectedIdx = 0
				}
				return t, nil
			}
		}
	case slotUpstreamSavedMsg:
		if msg.message != "" {
			t.message = msg.message
		} else {
			t.message = fmt.Sprintf("Saved upstream settings for slot %d", msg.slotID)
		}
		t.messageType = "success"
		t.messageShow = true
		if msg.restart {
			return t, tea.Batch(
				t.startFlash(3*time.Second),
				func() tea.Msg { return RestartSlotMsg{SlotID: msg.slotID} },
			)
		}
		return t, t.startFlash(3 * time.Second)
	case SettingsErrorMsg:
		if msg.Error != nil {
			t.message = msg.Error.Error()
		} else {
			t.message = "unknown error"
		}
		t.messageType = "error"
		t.messageShow = true
		return t, nil
	case settingsFlashTickMsg:
		if msg.id != t.flashID {
			return t, nil
		}
		t.flashTicks--
		if t.flashTicks <= 0 {
			t.message = ""
			t.messageType = ""
			t.messageShow = false
			return t, nil
		}
		t.messageShow = !t.messageShow
		return t, t.nextFlashTick()
	}

	return t, nil
}

// --- Upstream proxy registry UI ---

type upstreamSavedMsg struct {
	message string
}

func (t *SettingsTab) upstreamFieldByKey(key string) *settingField {
	for i := range t.upstreamFields {
		if t.upstreamFields[i].key == key {
			return &t.upstreamFields[i]
		}
	}
	return nil
}

func (t *SettingsTab) focusedUpstreamInputField() *settingField {
	if t.upstreamFocusedField < 0 || t.upstreamFocusedField >= len(t.upstreamFields) {
		return nil
	}
	if t.upstreamFields[t.upstreamFocusedField].fieldType != FieldTypeInput || t.upstreamFields[t.upstreamFocusedField].disabled {
		return nil
	}
	return &t.upstreamFields[t.upstreamFocusedField]
}

func (t *SettingsTab) upstreamFocusNext() {
	if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeInput {
		t.upstreamFields[t.upstreamFocusedField].input.Blur()
	}
	for {
		t.upstreamFocusedField++
		if t.upstreamFocusedField >= len(t.upstreamFields) {
			t.upstreamFocusedField = 0
		}
		if t.isUpstreamFieldFocusable(t.upstreamFocusedField) {
			break
		}
	}
	if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeInput {
		t.upstreamFields[t.upstreamFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) upstreamFocusPrev() {
	if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeInput {
		t.upstreamFields[t.upstreamFocusedField].input.Blur()
	}
	for {
		t.upstreamFocusedField--
		if t.upstreamFocusedField < 0 {
			t.upstreamFocusedField = len(t.upstreamFields) - 1
		}
		if t.isUpstreamFieldFocusable(t.upstreamFocusedField) {
			break
		}
	}
	if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeInput {
		t.upstreamFields[t.upstreamFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) isUpstreamFieldFocusable(index int) bool {
	if index < 0 || index >= len(t.upstreamFields) {
		return false
	}
	f := t.upstreamFields[index]
	if f.fieldType == FieldTypeHeader || f.disabled {
		return false
	}
	return true
}

func (t *SettingsTab) loadUpstreamDraft(id string) {
	t.upstreamDetailID = id
	t.upstreamFocusedField = 1
	t.upstreamScrollOffset = 0

	var entry config.UpstreamProxyEntry
	if id != "" {
		entry, _ = t.cfg.GetUpstreamProxy(id)
	}
	if entry.Protocol == "" {
		entry.Protocol = "http"
	}

	if field := t.upstreamFieldByKey("upstream_label"); field != nil {
		field.input.SetValue(entry.Label)
	}
	if field := t.upstreamFieldByKey("upstream_protocol"); field != nil {
		field.selectedIdx = 0
		for i, opt := range field.options {
			if opt.value == entry.Protocol {
				field.selectedIdx = i
				break
			}
		}
	}
	if field := t.upstreamFieldByKey("upstream_server"); field != nil {
		field.input.SetValue(entry.Server)
	}
	if field := t.upstreamFieldByKey("upstream_port"); field != nil {
		if entry.Port > 0 {
			field.input.SetValue(strconv.Itoa(entry.Port))
		} else {
			field.input.SetValue("")
		}
	}
	if field := t.upstreamFieldByKey("upstream_username"); field != nil {
		field.input.SetValue(entry.Username)
	}
	if field := t.upstreamFieldByKey("upstream_password"); field != nil {
		field.input.SetValue(entry.Password)
	}

	for i := range t.upstreamFields {
		if t.upstreamFields[i].fieldType == FieldTypeInput {
			t.upstreamFields[i].input.Blur()
		}
	}
	if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeInput {
		t.upstreamFields[t.upstreamFocusedField].input.Focus()
	}
}

func (t *SettingsTab) saveUpstreamProxy() tea.Cmd {
	return func() tea.Msg {
		label := ""
		if field := t.upstreamFieldByKey("upstream_label"); field != nil {
			label = strings.TrimSpace(field.input.Value())
		}
		if label == "" {
			return SettingsErrorMsg{Error: fmt.Errorf("label is required")}
		}

		protocol := "http"
		if field := t.upstreamFieldByKey("upstream_protocol"); field != nil && len(field.options) > 0 {
			protocol = field.options[field.selectedIdx].value
		}
		server := ""
		if field := t.upstreamFieldByKey("upstream_server"); field != nil {
			server = strings.TrimSpace(field.input.Value())
		}
		if server == "" {
			return SettingsErrorMsg{Error: fmt.Errorf("server is required")}
		}
		portText := ""
		if field := t.upstreamFieldByKey("upstream_port"); field != nil {
			portText = strings.TrimSpace(field.input.Value())
		}
		port := 0
		if portText != "" {
			var err error
			port, err = strconv.Atoi(portText)
			if err != nil || port <= 0 || port >= 65536 {
				return SettingsErrorMsg{Error: fmt.Errorf("invalid port: %q", portText)}
			}
		}
		if port <= 0 || port >= 65536 {
			return SettingsErrorMsg{Error: fmt.Errorf("port is required (1-65535)")}
		}

		username := ""
		if field := t.upstreamFieldByKey("upstream_username"); field != nil {
			username = field.input.Value()
		}
		password := ""
		if field := t.upstreamFieldByKey("upstream_password"); field != nil {
			password = field.input.Value()
		}

		entry := config.UpstreamProxyEntry{
			ID:       t.upstreamDetailID,
			Label:    label,
			Protocol: protocol,
			Server:   server,
			Port:     port,
			Username: username,
			Password: password,
		}

		if t.upstreamDetailID == "" {
			id := t.cfg.AddUpstreamProxy(entry)
			t.upstreamDetailID = id
		} else {
			if err := t.cfg.UpdateUpstreamProxy(entry); err != nil {
				return SettingsErrorMsg{Error: err}
			}
		}
		if err := t.cfg.Save(); err != nil {
			return SettingsErrorMsg{Error: err}
		}

		return upstreamSavedMsg{message: fmt.Sprintf("Saved upstream proxy %q", label)}
	}
}

func (t *SettingsTab) updateUpstreamList(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	entries := t.cfg.ListUpstreamProxies()
	// Total items: entries + 1 "Add new" button
	totalItems := len(entries) + 1

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.showUpstreamList = false
			t.showUpstreamDetail = false
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.upstreamListIndex--
			if t.upstreamListIndex < 0 {
				t.upstreamListIndex = totalItems - 1
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.upstreamListIndex++
			if t.upstreamListIndex >= totalItems {
				t.upstreamListIndex = 0
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if t.upstreamListIndex < len(entries) {
				// Edit existing entry.
				t.loadUpstreamDraft(entries[t.upstreamListIndex].ID)
			} else {
				// Add new entry.
				t.loadUpstreamDraft("")
			}
			t.showUpstreamDetail = true
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("d", "delete"))):
			if t.upstreamListIndex < len(entries) {
				entryID := entries[t.upstreamListIndex].ID
				if err := t.cfg.DeleteUpstreamProxy(entryID); err != nil {
					t.message = err.Error()
					t.messageType = "error"
					t.messageShow = true
					return t, t.startFlash(3 * time.Second)
				}
				if err := t.cfg.Save(); err != nil {
					t.message = err.Error()
					t.messageType = "error"
					t.messageShow = true
					return t, t.startFlash(3 * time.Second)
				}
				t.message = "Deleted upstream proxy"
				t.messageType = "success"
				t.messageShow = true
				// Adjust index if needed.
				newEntries := t.cfg.ListUpstreamProxies()
				if t.upstreamListIndex >= len(newEntries)+1 {
					t.upstreamListIndex = len(newEntries)
				}
				return t, t.startFlash(3 * time.Second)
			}
		}
	case settingsFlashTickMsg:
		if msg.id != t.flashID {
			return t, nil
		}
		t.flashTicks--
		if t.flashTicks <= 0 {
			t.message = ""
			t.messageType = ""
			t.messageShow = false
			return t, nil
		}
		t.messageShow = !t.messageShow
		return t, t.nextFlashTick()
	}

	return t, nil
}

func (t *SettingsTab) updateUpstreamDetail(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if field := t.focusedUpstreamInputField(); field != nil {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				t.showUpstreamDetail = false
				for i := range t.upstreamFields {
					if t.upstreamFields[i].fieldType == FieldTypeInput {
						t.upstreamFields[i].input.Blur()
					}
				}
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				t.upstreamFocusPrev()
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				t.upstreamFocusNext()
				return t, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				return t, nil
			default:
				return t, t.updateInputField(field, msg)
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.showUpstreamDetail = false
			for i := range t.upstreamFields {
				if t.upstreamFields[i].fieldType == FieldTypeInput {
					t.upstreamFields[i].input.Blur()
				}
			}
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.upstreamFocusPrev()
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.upstreamFocusNext()
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if t.upstreamFocusedField < len(t.upstreamFields) {
				field := &t.upstreamFields[t.upstreamFocusedField]
				if field.fieldType == FieldTypeButton && field.key == "upstream_save" {
					return t, t.saveUpstreamProxy()
				}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeSelect && !t.upstreamFields[t.upstreamFocusedField].disabled {
				field := &t.upstreamFields[t.upstreamFocusedField]
				field.selectedIdx--
				if field.selectedIdx < 0 {
					field.selectedIdx = len(field.options) - 1
				}
				return t, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if t.upstreamFocusedField < len(t.upstreamFields) && t.upstreamFields[t.upstreamFocusedField].fieldType == FieldTypeSelect && !t.upstreamFields[t.upstreamFocusedField].disabled {
				field := &t.upstreamFields[t.upstreamFocusedField]
				field.selectedIdx++
				if field.selectedIdx >= len(field.options) {
					field.selectedIdx = 0
				}
				return t, nil
			}
		}
	case upstreamSavedMsg:
		if msg.message != "" {
			t.message = msg.message
		} else {
			t.message = "Saved"
		}
		t.messageType = "success"
		t.messageShow = true
		return t, t.startFlash(3 * time.Second)
	case SettingsErrorMsg:
		if msg.Error != nil {
			t.message = msg.Error.Error()
		} else {
			t.message = "unknown error"
		}
		t.messageType = "error"
		t.messageShow = true
		return t, nil
	case settingsFlashTickMsg:
		if msg.id != t.flashID {
			return t, nil
		}
		t.flashTicks--
		if t.flashTicks <= 0 {
			t.message = ""
			t.messageType = ""
			t.messageShow = false
			return t, nil
		}
		t.messageShow = !t.messageShow
		return t, t.nextFlashTick()
	}

	return t, nil
}

func (t *SettingsTab) viewUpstreamList() string {
	entries := t.cfg.ListUpstreamProxies()

	lines := []string{
		styles.Title.Render("Edit Upstream Proxies"),
		styles.HelpText.Render("Manage reusable upstream proxy definitions."),
		"",
	}

	for i, entry := range entries {
		label := entry.Label
		if label == "" {
			label = entry.ID
		}
		detail := fmt.Sprintf("%s://%s:%d", entry.Protocol, entry.Server, entry.Port)
		line := fmt.Sprintf("  %s  %s", label, styles.Muted.Render(detail))
		if i == t.upstreamListIndex {
			line = styles.Primary.Render(fmt.Sprintf("▶ %s  %s", label, detail))
		}
		lines = append(lines, line)
	}

	addLine := "  [+ Add new upstream proxy]"
	if t.upstreamListIndex == len(entries) {
		addLine = styles.StatusConnected.Render("▶ [+ Add new upstream proxy]")
	} else {
		addLine = styles.Muted.Render(addLine)
	}
	lines = append(lines, addLine)

	if t.message != "" && t.messageShow {
		lines = append(lines, "")
		if t.messageType == "success" {
			lines = append(lines, styles.SuccessMessage.Render(t.message))
		} else {
			lines = append(lines, styles.ErrorMessage.Render(t.message))
		}
	}

	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" select  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" edit  ") +
		styles.HelpKey.Render("d") + styles.HelpText.Render(" delete  ") +
		styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	return styles.ContentArea.Render(strings.Join(append(lines, "", help), "\n"))
}

func (t *SettingsTab) viewUpstreamDetail() string {
	var lines []string
	focusedLine := 0

	titleText := "New Upstream Proxy"
	if t.upstreamDetailID != "" {
		titleText = "Edit Upstream Proxy"
	}
	lines = append(lines, styles.Title.Render(titleText))
	lines = append(lines, "")

	appendBlock := func(block string) {
		lines = append(lines, strings.Split(block, "\n")...)
	}
	appendLine := func(line string) {
		lines = append(lines, line)
	}

	for i, field := range t.upstreamFields {
		if field.fieldType == FieldTypeHeader {
			if i == t.upstreamFocusedField {
				focusedLine = len(lines)
			}
			appendBlock(styles.Title.Render(field.label))
			appendLine("")
			continue
		}

		if field.fieldType == FieldTypeButton {
			if i == t.upstreamFocusedField {
				focusedLine = len(lines)
			}
			btnLabel := field.label
			if i == t.upstreamFocusedField {
				btnLabel = styles.StatusConnected.Render("▶ [" + btnLabel + "]")
			} else {
				btnLabel = styles.Muted.Render("  [" + btnLabel + "]")
			}
			appendBlock(btnLabel)
			appendLine("")
			continue
		}

		if i == t.upstreamFocusedField {
			focusedLine = len(lines)
		}

		label := field.label
		if i == t.upstreamFocusedField {
			label = styles.Primary.Render("▶ " + label)
		} else {
			label = "  " + label
		}
		appendBlock(label)
		if field.helpText != "" {
			appendBlock(styles.HelpText.Render("    " + field.helpText))
		}

		switch field.fieldType {
		case FieldTypeInput:
			inputStyle := styles.InputField
			if i == t.upstreamFocusedField {
				inputStyle = styles.InputFocused
			}
			appendBlock(inputStyle.Render(field.input.View()))
			appendLine("")
		case FieldTypeSelect:
			appendBlock(t.renderSelect(field, i == t.upstreamFocusedField))
			appendLine("")
		}
	}

	if t.message != "" && t.messageShow {
		appendLine("")
		if t.messageType == "success" {
			appendBlock(styles.SuccessMessage.Render(t.message))
		} else {
			appendBlock(styles.ErrorMessage.Render(t.message))
		}
	}

	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" navigate  ") +
		styles.HelpKey.Render("←/→") + styles.HelpText.Render(" select  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" save  ") +
		styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	if t.height <= 0 {
		return styles.ContentArea.Render(strings.Join(append(lines, "", help), "\n"))
	}

	needsScroll := len(lines) > t.height-1
	showScrollIndicator := needsScroll && t.height > 2
	reservedBottom := 1
	if showScrollIndicator {
		reservedBottom = 2
	}
	visibleHeight := t.height - reservedBottom
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	if focusedLine < t.upstreamScrollOffset {
		t.upstreamScrollOffset = focusedLine
	}
	if focusedLine >= t.upstreamScrollOffset+visibleHeight {
		t.upstreamScrollOffset = focusedLine - visibleHeight + 1
	}
	if t.upstreamScrollOffset < 0 {
		t.upstreamScrollOffset = 0
	}
	maxOffset := len(lines) - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.upstreamScrollOffset > maxOffset {
		t.upstreamScrollOffset = maxOffset
	}

	start := t.upstreamScrollOffset
	end := t.upstreamScrollOffset + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}

	content := strings.Join(lines[start:end], "\n")
	if showScrollIndicator {
		scrollText := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(lines))
		scrollInfo := lipgloss.NewStyle().
			Foreground(styles.MutedColor).
			Width(t.width).
			Align(lipgloss.Right).
			Render(scrollText)
		content += "\n" + scrollInfo
	}
	content += "\n" + help

	return styles.ContentArea.Render(content)
}

func (t *SettingsTab) slotFocusNext() {
	if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeInput {
		t.slotFields[t.slotFocusedField].input.Blur()
	}
	for {
		t.slotFocusedField++
		if t.slotFocusedField >= len(t.slotFields) {
			t.slotFocusedField = 0
		}
		if t.isSlotFieldFocusable(t.slotFocusedField) {
			break
		}
	}
	if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeInput {
		t.slotFields[t.slotFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) slotFocusPrev() {
	if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeInput {
		t.slotFields[t.slotFocusedField].input.Blur()
	}
	for {
		t.slotFocusedField--
		if t.slotFocusedField < 0 {
			t.slotFocusedField = len(t.slotFields) - 1
		}
		if t.isSlotFieldFocusable(t.slotFocusedField) {
			break
		}
	}
	if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeInput {
		t.slotFields[t.slotFocusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
	}
}

func (t *SettingsTab) isSlotFieldFocusable(index int) bool {
	if index < 0 || index >= len(t.slotFields) {
		return false
	}
	f := t.slotFields[index]
	if f.fieldType == FieldTypeHeader || f.disabled {
		return false
	}
	return true
}

func (t *SettingsTab) startFlash(duration time.Duration) tea.Cmd {
	const interval = 500 * time.Millisecond
	t.flashID++
	t.flashTicks = int(duration / interval)
	if t.flashTicks <= 0 {
		t.flashTicks = 1
	}
	return t.nextFlashTick()
}

func (t *SettingsTab) nextFlashTick() tea.Cmd {
	const interval = 500 * time.Millisecond
	id := t.flashID
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return settingsFlashTickMsg{id: id}
	})
}

// View renders the tab
func (t *SettingsTab) View() string {
	if t.showSystemInfo {
		return t.viewSystemInfo()
	}
	if t.showUpstreamDetail {
		return t.viewUpstreamDetail()
	}
	if t.showUpstreamList {
		return t.viewUpstreamList()
	}
	if t.showSlotDetail {
		return t.viewSlotDetail()
	}
	if t.showSlotList {
		return t.viewSlotList()
	}
	if t.showTunSettings {
		return t.viewTunSettings()
	}
	if t.showSystemProxySettings {
		return t.viewSystemProxySettings()
	}
	var lines []string
	// Track the rendered line index where the focused item starts.
	focusedLine := 0

	appendBlock := func(block string) {
		lines = append(lines, strings.Split(block, "\n")...)
	}

	appendLine := func(line string) {
		lines = append(lines, line)
	}

	for i, field := range t.fields {
		if field.fieldType == FieldTypeHeader {
			if i == t.focusedField {
				focusedLine = len(lines)
			}
			appendBlock(styles.Title.Render(field.label))
			appendLine("")
			continue
		}

		if field.fieldType == FieldTypeButton {
			if i == t.focusedField {
				focusedLine = len(lines)
			}
			btnLabel := field.label
			if i == t.focusedField {
				btnLabel = styles.StatusConnected.Render("▶ [" + btnLabel + "]")
			} else {
				btnLabel = styles.Muted.Render("  [" + btnLabel + "]")
			}
			appendBlock(btnLabel)
			appendLine("")
			continue
		}

		if i == t.focusedField {
			focusedLine = len(lines)
		}

		label := field.label
		if i == t.focusedField {
			label = styles.Primary.Render("▶ " + label)
		} else {
			label = "  " + label
		}
		if field.disabled {
			label = styles.Muted.Render("  " + field.label)
		}
		appendBlock(label)
		if field.helpText != "" {
			helpText := "    " + field.helpText
			if field.disabled {
				helpText = styles.Muted.Render(helpText)
			} else {
				helpText = styles.HelpText.Render(helpText)
			}
			appendBlock(helpText)
		}

		switch field.fieldType {
		case FieldTypeInput:
			inputStyle := styles.InputField
			if i == t.focusedField {
				inputStyle = styles.InputFocused
			}
			appendBlock(inputStyle.Render(field.input.View()))
			appendLine("")

		case FieldTypeSelect:
			appendBlock(t.renderSelect(field, i == t.focusedField))
			appendLine("")

		}
	}

	// Restart core button (the only virtual button after all fields)
	restartLabel := i18n.M().Settings.RestartCore
	if restartLabel == "" {
		restartLabel = "Restart core"
	}
	if t.focusedField == len(t.fields) {
		focusedLine = len(lines)
		restartLabel = styles.StatusConnected.Render("▶ [" + restartLabel + "]")
	} else {
		restartLabel = styles.Muted.Render("  [" + restartLabel + "]")
	}
	appendBlock(restartLabel)

	// Message
	if t.message != "" && t.messageShow {
		appendLine("")
		if t.messageType == "success" {
			appendBlock(styles.SuccessMessage.Render(t.message))
		} else {
			appendBlock(styles.ErrorMessage.Render(t.message))
		}
	}

	// Help (always shown at bottom, outside scrollable area)
	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" navigate  ") +
		styles.HelpKey.Render("←/→") + styles.HelpText.Render(" select  ") +
		styles.HelpKey.Render("Ctrl+R") + styles.HelpText.Render(" restart")

	// If size has not been set yet (tests / early init), render full content without clipping.
	if t.height <= 0 {
		content := strings.Join(lines, "\n")
		content += "\n" + help
		return styles.ContentArea.Render(content)
	}

	// Reserve lines for fixed bottom area: help text (1 line).
	// When scrolling, also reserve 1 line for the scroll indicator (only if enough space).
	needsScroll := len(lines) > t.height-1
	showScrollIndicator := needsScroll && t.height > 2
	reservedBottom := 1 // help line
	if showScrollIndicator {
		reservedBottom = 2 // scroll indicator + help line
	}
	visibleHeight := t.height - reservedBottom
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Adjust scroll offset to keep focused item visible.
	if focusedLine < t.scrollOffset {
		t.scrollOffset = focusedLine
	}
	if focusedLine >= t.scrollOffset+visibleHeight {
		t.scrollOffset = focusedLine - visibleHeight + 1
	}
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
	maxOffset := len(lines) - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.scrollOffset > maxOffset {
		t.scrollOffset = maxOffset
	}

	start := t.scrollOffset
	end := t.scrollOffset + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]

	content := strings.Join(visible, "\n")
	if showScrollIndicator {
		scrollText := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(lines))
		scrollInfo := lipgloss.NewStyle().
			Foreground(styles.MutedColor).
			Width(t.width).
			Align(lipgloss.Right).
			Render(scrollText)
		content += "\n" + scrollInfo
	}
	content += "\n" + help

	return styles.ContentArea.Render(content)
}

func (t *SettingsTab) viewSlotList() string {
	lines := []string{
		styles.Title.Render("Edit Slots"),
		styles.HelpText.Render("Select a slot to edit its per-slot settings."),
		"",
	}

	for i := 0; i < proxy.MaxSlots; i++ {
		line := fmt.Sprintf("  Slot %d  %s", i, t.slotListSummary(i))
		if i == t.slotListIndex {
			line = styles.Primary.Render("▶ " + strings.TrimPrefix(line, "  "))
		}
		lines = append(lines, line)
	}

	if t.message != "" && t.messageShow {
		lines = append(lines, "")
		if t.messageType == "success" {
			lines = append(lines, styles.SuccessMessage.Render(t.message))
		} else {
			lines = append(lines, styles.ErrorMessage.Render(t.message))
		}
	}

	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" select slot  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" open  ") +
		styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	return styles.ContentArea.Render(strings.Join(append(lines, "", help), "\n"))
}

func (t *SettingsTab) viewSlotDetail() string {
	var lines []string
	focusedLine := 0

	lines = append(lines, styles.Title.Render(fmt.Sprintf("Slot %d Settings", t.slotUpstreamSlot)))
	lines = append(lines, styles.HelpText.Render(t.slotRuntimeSummary(t.slotUpstreamSlot)))
	lines = append(lines, "")

	appendBlock := func(block string) {
		lines = append(lines, strings.Split(block, "\n")...)
	}
	appendLine := func(line string) {
		lines = append(lines, line)
	}

	for i, field := range t.slotFields {
		if field.fieldType == FieldTypeHeader {
			if i == t.slotFocusedField {
				focusedLine = len(lines)
			}
			appendBlock(styles.Title.Render(field.label))
			appendLine("")
			continue
		}

		if field.fieldType == FieldTypeButton {
			if i == t.slotFocusedField {
				focusedLine = len(lines)
			}
			btnLabel := field.label
			if i == t.slotFocusedField {
				btnLabel = styles.StatusConnected.Render("▶ [" + btnLabel + "]")
			} else {
				btnLabel = styles.Muted.Render("  [" + btnLabel + "]")
			}
			appendBlock(btnLabel)
			appendLine("")
			continue
		}

		if i == t.slotFocusedField {
			focusedLine = len(lines)
		}

		label := field.label
		if i == t.slotFocusedField {
			label = styles.Primary.Render("▶ " + label)
		} else {
			label = "  " + label
		}
		appendBlock(label)
		if field.helpText != "" {
			appendBlock(styles.HelpText.Render("    " + field.helpText))
		}

		switch field.fieldType {
		case FieldTypeInput:
			inputStyle := styles.InputField
			if i == t.slotFocusedField {
				inputStyle = styles.InputFocused
			}
			appendBlock(inputStyle.Render(field.input.View()))
			appendLine("")
		case FieldTypeSelect:
			appendBlock(t.renderSelect(field, i == t.slotFocusedField))
			appendLine("")
		}
	}

	if t.message != "" && t.messageShow {
		appendLine("")
		if t.messageType == "success" {
			appendBlock(styles.SuccessMessage.Render(t.message))
		} else {
			appendBlock(styles.ErrorMessage.Render(t.message))
		}
	}

	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" navigate  ") +
		styles.HelpKey.Render("←/→") + styles.HelpText.Render(" select  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" apply  ") +
		styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	if t.height <= 0 {
		return styles.ContentArea.Render(strings.Join(append(lines, "", help), "\n"))
	}

	needsScroll := len(lines) > t.height-1
	showScrollIndicator := needsScroll && t.height > 2
	reservedBottom := 1
	if showScrollIndicator {
		reservedBottom = 2
	}
	visibleHeight := t.height - reservedBottom
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	if focusedLine < t.slotScrollOffset {
		t.slotScrollOffset = focusedLine
	}
	if focusedLine >= t.slotScrollOffset+visibleHeight {
		t.slotScrollOffset = focusedLine - visibleHeight + 1
	}
	if t.slotScrollOffset < 0 {
		t.slotScrollOffset = 0
	}
	maxOffset := len(lines) - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.slotScrollOffset > maxOffset {
		t.slotScrollOffset = maxOffset
	}

	start := t.slotScrollOffset
	end := t.slotScrollOffset + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}

	content := strings.Join(lines[start:end], "\n")
	if showScrollIndicator {
		scrollText := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(lines))
		scrollInfo := lipgloss.NewStyle().
			Foreground(styles.MutedColor).
			Width(t.width).
			Align(lipgloss.Right).
			Render(scrollText)
		content += "\n" + scrollInfo
	}
	content += "\n" + help

	return styles.ContentArea.Render(content)
}

func (t *SettingsTab) slotListSummary(slotID int) string {
	upstreamText := "no upstream"
	if ref, ok := t.cfg.GetSlotUpstreamRef(slotID); ok && ref != "" {
		if entry, found := t.cfg.GetUpstreamProxy(ref); found {
			label := entry.Label
			if label == "" {
				label = entry.ID
			}
			upstreamText = fmt.Sprintf("%s (%s://%s:%d)", label, entry.Protocol, entry.Server, entry.Port)
		} else {
			upstreamText = fmt.Sprintf("missing ref: %s", ref)
		}
	}

	stateText := "stopped"
	if t.slotMgr != nil {
		if state := t.slotMgr.GetSlotState(slotID); state != nil {
			stateText = fmt.Sprintf("running %s at :%d", state.Mode, state.Port)
		}
	}

	return fmt.Sprintf("%s | %s", stateText, upstreamText)
}

func (t *SettingsTab) slotRuntimeSummary(slotID int) string {
	if t.slotMgr == nil {
		return "Current runtime: stopped"
	}
	state := t.slotMgr.GetSlotState(slotID)
	if state == nil {
		return "Current runtime: stopped"
	}
	if state.UpstreamEnabled {
		return fmt.Sprintf("Current runtime: %s at :%d via %s://%s:%d", state.Mode, state.Port, state.UpstreamProtocol, state.UpstreamServer, state.UpstreamPort)
	}
	return fmt.Sprintf("Current runtime: %s at :%d without upstream proxy", state.Mode, state.Port)
}

func (t *SettingsTab) renderSelect(field settingField, focused bool) string {
	if len(field.options) == 0 {
		return ""
	}
	if field.selectedIdx < 0 || field.selectedIdx >= len(field.options) {
		field.selectedIdx = 0
	}
	opt := field.options[field.selectedIdx]
	label := " " + opt.label + " "
	if field.disabled {
		return styles.Muted.Render("◀") + " " + styles.Muted.Render(label) + " " + styles.Muted.Render("▶")
	}
	if focused {
		label = styles.SelectOptionActive.Render(label)
	} else {
		label = styles.SelectOptionSelected.Render(label)
	}
	return styles.Muted.Render("◀") + " " + label + " " + styles.Muted.Render("▶")
}

// SetSize sets the tab size
func (t *SettingsTab) SetSize(width, height int) {
	t.width = width
	t.height = height
	for i := range t.fields {
		if t.fields[i].fieldType == FieldTypeInput {
			t.fields[i].input.Width = width / 2
		}
	}
	for i := range t.slotFields {
		if t.slotFields[i].fieldType == FieldTypeInput {
			t.slotFields[i].input.Width = width / 2
		}
	}
	for i := range t.upstreamFields {
		if t.upstreamFields[i].fieldType == FieldTypeInput {
			t.upstreamFields[i].input.Width = width / 2
		}
	}
}

func (t *SettingsTab) saveSettings() tea.Cmd {
	return func() tea.Msg {
		prevPort := t.cfg.Proxy.MixedPort
		prevDocker := t.cfg.Proxy.DockerRedirectTCP
		prevTun := t.cfg.Proxy.TunEnabled

		// Parse and validate values
		for _, field := range t.fields {
			if field.manualApply {
				continue
			}
			switch field.key {
			case "mixed_port":
				port, err := strconv.Atoi(field.input.Value())
				if err != nil || port <= 0 || port >= 65536 {
					return SettingsErrorMsg{Error: fmt.Errorf("invalid mixed port: %q", field.input.Value())}
				}
				t.cfg.Proxy.MixedPort = port
				t.cfg.Proxy.BaseMixedPort = port
			case "allow_lan":
				v := field.options[field.selectedIdx].value
				switch v {
				case "true":
					t.cfg.Proxy.AllowLan = true
				case "false":
					t.cfg.Proxy.AllowLan = false
				default:
					return SettingsErrorMsg{Error: fmt.Errorf("invalid allow_lan value: %q", v)}
				}
			case "update_via_proxy":
				v := field.options[field.selectedIdx].value
				switch v {
				case "true":
					t.cfg.Subscription.UpdateViaProxy = true
				case "false":
					t.cfg.Subscription.UpdateViaProxy = false
				default:
					return SettingsErrorMsg{Error: fmt.Errorf("invalid update_via_proxy value: %q", v)}
				}
			case "system_proxy_enabled":
				v := field.options[field.selectedIdx].value
				desired := v == "true"
				if desired != t.cfg.Proxy.SystemProxy.Enabled && runtime.GOOS == "darwin" {
					if desired {
						snapshot, err := netctl.SnapshotSystemProxy()
						if err != nil {
							return SettingsErrorMsg{Error: err}
						}
						if err := netctl.SaveSystemProxySnapshot(snapshot); err != nil {
							return SettingsErrorMsg{Error: err}
						}
						if err := netctl.ApplySystemProxy(t.cfg.Proxy.SystemProxy); err != nil {
							_ = netctl.RestoreSystemProxy(snapshot)
							return SettingsErrorMsg{Error: err}
						}
					} else {
						snapshot, err := netctl.LoadSystemProxySnapshot()
						if err != nil {
							return SettingsErrorMsg{Error: err}
						}
						if snapshot != nil {
							if err := netctl.RestoreSystemProxy(snapshot); err != nil {
								return SettingsErrorMsg{Error: err}
							}
							_ = netctl.ClearSystemProxySnapshot()
						}
					}
				}
				t.cfg.Proxy.SystemProxy.Enabled = desired
			case "tun_enabled":
				v := field.options[field.selectedIdx].value
				switch v {
				case "true":
					t.cfg.Proxy.TunEnabled = true
				case "false":
					t.cfg.Proxy.TunEnabled = false
				default:
					return SettingsErrorMsg{Error: fmt.Errorf("invalid tun_enabled value: %q", v)}
				}
			case "tun_stack":
				t.cfg.Proxy.TunStack = field.options[field.selectedIdx].value
			case "tun_mtu":
				value := strings.TrimSpace(field.input.Value())
				if value == "" {
					t.cfg.Proxy.TunMTU = 0
					break
				}
				n, err := strconv.Atoi(value)
				if err != nil || n <= 0 || n >= 65536 {
					return SettingsErrorMsg{Error: fmt.Errorf("invalid tun_mtu value: %q", field.input.Value())}
				}
				t.cfg.Proxy.TunMTU = n
			case "tun_device":
				t.cfg.Proxy.TunDevice = strings.TrimSpace(field.input.Value())
			case "docker_redirect_tcp":
				v := field.options[field.selectedIdx].value
				switch v {
				case "true":
					t.cfg.Proxy.DockerRedirectTCP = true
				case "false":
					t.cfg.Proxy.DockerRedirectTCP = false
				default:
					return SettingsErrorMsg{Error: fmt.Errorf("invalid docker_redirect_tcp value: %q", v)}
				}
			case "language":
				lang := field.options[field.selectedIdx].value
				t.cfg.Language = lang
				_ = i18n.SetLanguage(lang)
			case "server_display":
				v := field.options[field.selectedIdx].value
				switch v {
				case "show", "blur", "off":
					t.cfg.UI.ServerDisplay = v
				default:
					return SettingsErrorMsg{Error: fmt.Errorf("invalid server display: %q", v)}
				}
			case "speed_unit":
				v := field.options[field.selectedIdx].value
				switch v {
				case "bytes", "bits":
					t.cfg.UI.SpeedUnit = v
				default:
					return SettingsErrorMsg{Error: fmt.Errorf("invalid speed unit: %q", v)}
				}
			case "concurrent":
				n, err := strconv.Atoi(field.input.Value())
				if err != nil || n <= 0 || n > 100 {
					return SettingsErrorMsg{Error: fmt.Errorf("invalid concurrent value: %q", field.input.Value())}
				}
				t.cfg.SpeedTest.Concurrent = n
			}
		}

		if err := t.cfg.Save(); err != nil {
			return SettingsErrorMsg{Error: err}
		}

		// Root-only side effects (Linux): apply/remove docker hijack iptables rules.
		notice := ""
		if runtime.GOOS == "linux" && prevTun != t.cfg.Proxy.TunEnabled && t.cfg.Proxy.TunEnabled && os.Geteuid() != 0 {
			notice = "Saved. TUN requires root/CAP_NET_ADMIN. Re-run c4c with sudo and restart core."
		}
		if runtime.GOOS == "linux" {
			desiredDocker := t.cfg.Proxy.DockerRedirectTCP

			if os.Geteuid() != 0 {
				if desiredDocker {
					if notice == "" {
						notice = "Saved. Docker hijack needs root to apply iptables. Re-run c4c with sudo and save again."
					}
				} else if prevDocker {
					if notice == "" {
						notice = "Saved. Docker hijack removal needs root. Re-run c4c with sudo and save again."
					}
				}
			} else {
				if desiredDocker {
					bind := strings.TrimSpace(t.cfg.Proxy.BindAddress)
					if !t.cfg.Proxy.AllowLan || bind == "127.0.0.1" || bind == "::1" {
						if notice == "" {
							notice = "Saved. WARN: Docker REDIRECT needs non-loopback bind; set allow_lan=true and bind_address=0.0.0.0."
						}
					}

					lock, err := proxy.ReadLockFile()
					if err != nil || lock == nil || lock.RedirPort <= 0 {
						if notice == "" {
							notice = "Saved. Restart core first to get redir-port, then save again to apply iptables."
						}
					} else if err := netctl.EnableDockerTCPRedirect(lock.RedirPort); err != nil {
						return SettingsErrorMsg{Error: err}
					} else if notice == "" {
						notice = fmt.Sprintf("Docker hijack enabled (TCP). iptables -> redir-port %d", lock.RedirPort)
					}
				} else if prevDocker {
					if err := netctl.DisableDockerTCPRedirect(); err != nil {
						return SettingsErrorMsg{Error: err}
					}
					notice = "Docker hijack disabled (iptables removed)."
				}
			}
		}

		if prevTun != t.cfg.Proxy.TunEnabled {
			return tunChangedSaveMsg{notice: notice}
		}
		if t.cfg.Proxy.MixedPort != prevPort {
			return portChangedSaveMsg{notice: notice}
		}
		return SettingsSavedMsg{Message: notice}
	}
}

func (t *SettingsTab) fetchSystemInfo() tea.Cmd {
	return func() tea.Msg {
		info := SystemInfoData{
			AppVersion: t.appVersion,
			OSInfo:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		}

		if t.slotMgr == nil {
			info.RunningMode = "stopped"
			return systemInfoFetchedMsg{info: info}
		}

		activeSlots := t.slotMgr.ActiveSlots()
		if len(activeSlots) == 0 {
			info.RunningMode = "stopped"
			return systemInfoFetchedMsg{info: info}
		}

		first := activeSlots[0]
		info.MixedPort = first.Port
		info.RunningMode = first.Mode

		if v, err := t.slotMgr.GetCoreVersion(first.SlotID); err == nil {
			info.CoreVersion = v
		}
		if n, err := t.slotMgr.GetRulesCount(first.SlotID); err == nil {
			info.RulesCount = n
		}
		if startedStr, err := t.slotMgr.GetStartedAt(first.SlotID); err == nil {
			if startedAt, err := time.Parse(time.RFC3339, startedStr); err == nil {
				d := time.Since(startedAt)
				h := int(d.Hours())
				m := int(d.Minutes()) % 60
				s := int(d.Seconds()) % 60
				info.Uptime = fmt.Sprintf("%dh %dm %ds", h, m, s)
			}
		}

		return systemInfoFetchedMsg{info: info}
	}
}

func (t *SettingsTab) viewSystemInfo() string {
	title := styles.Title.Render("System Info")
	help := styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	if t.systemInfo == nil {
		content := title + "\n\n  Loading..."
		content += "\n\n" + help
		return styles.ContentArea.Render(content)
	}

	info := t.systemInfo
	rows := []struct{ k, v string }{
		{"App Version", info.AppVersion},
		{"Core Version", info.CoreVersion},
		{"Mixed Port", strconv.Itoa(info.MixedPort)},
		{"Running Mode", info.RunningMode},
		{"Uptime", info.Uptime},
		{"Rules", strconv.Itoa(info.RulesCount)},
		{"OS", info.OSInfo},
	}

	var lines []string
	lines = append(lines, title, "")
	for _, r := range rows {
		val := r.v
		if val == "" || val == "0" {
			val = "-"
		}
		line := fmt.Sprintf("  %-16s %s", r.k, val)
		lines = append(lines, line)
	}
	lines = append(lines, "", help)

	return styles.ContentArea.Render(strings.Join(lines, "\n"))
}

// GetConfig returns the current config
func (t *SettingsTab) GetConfig() *config.Config {
	return t.cfg
}
