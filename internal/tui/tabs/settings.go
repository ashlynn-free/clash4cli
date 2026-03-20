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
	slotMgr          *proxy.SlotManager
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

var upstreamProtocolOptions = []selectOption{
	{label: "HTTP", value: "http"},
	{label: "HTTPS", value: "https"},
	{label: "SOCKS5", value: "socks5"},
}

// NewSettingsTab creates a new settings tab
func NewSettingsTab(cfg *config.Config, slotMgr *proxy.SlotManager, version string) *SettingsTab {
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

	// Linux-only features: show on all platforms, but disable on non-Linux.
	linuxSection := i18n.M().Settings.LinuxFeatures
	if linuxSection == "" {
		linuxSection = "Linux Exclusive Features"
	}
	fields = append(fields, settingField{
		label:     "Edit Slots",
		fieldType: FieldTypeHeader,
	})

	fields = append(fields, settingField{
		label:     "Edit Slots >",
		helpText:  "Open the slot list and configure per-slot settings.",
		fieldType: FieldTypeButton,
		key:       "edit_slots",
	})

	fields = append(fields, settingField{
		label:     linuxSection,
		fieldType: FieldTypeHeader,
	})

	linuxOnlyDisabled := runtime.GOOS != "linux"

	tunLabel := i18n.M().Settings.Tun
	if tunLabel == "" {
		tunLabel = "TUN mode (Linux)"
	}
	fields = append(fields, settingField{
		label:       tunLabel,
		fieldType:   FieldTypeSelect,
		key:         "tun_enabled",
		options:     tunOptions,
		selectedIdx: tunIdx,
		disabled:    linuxOnlyDisabled,
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
		disabled:    linuxOnlyDisabled,
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
	tab.slotFields = newSlotSettingsFields()
	tab.loadSlotUpstreamDraft(0)
	return tab
}

func newSlotSettingsFields() []settingField {
	upstreamServerInput := textinput.New()
	upstreamServerInput.Placeholder = "gateway.example.com"
	upstreamServerInput.CharLimit = 255

	upstreamPortInput := textinput.New()
	upstreamPortInput.CharLimit = 5

	upstreamUsernameInput := textinput.New()
	upstreamUsernameInput.CharLimit = 255

	upstreamPasswordInput := textinput.New()
	upstreamPasswordInput.CharLimit = 255

	onOffOptions := []selectOption{
		{label: i18n.M().Settings.On, value: "true"},
		{label: i18n.M().Settings.Off, value: "false"},
	}

	return []settingField{
		{
			label:     "Upstream Proxy",
			fieldType: FieldTypeHeader,
		},
		{
			label:       "Enable upstream proxy",
			helpText:    "When enabled, this slot exits through the configured HTTP/HTTPS/SOCKS5 upstream proxy.",
			fieldType:   FieldTypeSelect,
			key:         "slot_upstream_enabled",
			options:     onOffOptions,
			selectedIdx: 1,
			manualApply: true,
		},
		{
			label:       "Protocol",
			fieldType:   FieldTypeSelect,
			key:         "slot_upstream_protocol",
			options:     upstreamProtocolOptions,
			selectedIdx: 0,
			manualApply: true,
		},
		{
			label:       "Server",
			fieldType:   FieldTypeInput,
			input:       upstreamServerInput,
			key:         "slot_upstream_server",
			manualApply: true,
		},
		{
			label:       "Port",
			fieldType:   FieldTypeInput,
			input:       upstreamPortInput,
			key:         "slot_upstream_port",
			numericOnly: true,
			manualApply: true,
		},
		{
			label:       "Username",
			fieldType:   FieldTypeInput,
			input:       upstreamUsernameInput,
			key:         "slot_upstream_username",
			manualApply: true,
		},
		{
			label:       "Password",
			fieldType:   FieldTypeInput,
			input:       upstreamPasswordInput,
			key:         "slot_upstream_password",
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

// Init initializes the tab
func (t *SettingsTab) Init() tea.Cmd {
	return textinput.Blink
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

	if t.showSlotDetail {
		return t.updateSlotDetail(msg)
	}

	if t.showSlotList {
		return t.updateSlotList(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
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
				if field.fieldType == FieldTypeButton && field.key == "edit_slots" {
					t.showSlotList = true
					t.showSlotDetail = false
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
				return t, t.saveSettings()
			}
		default:
			// Pass to focused input
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput && !t.fields[t.focusedField].disabled {
				field := &t.fields[t.focusedField]
				// Numeric-only behavior: accept only digits. On first typing after focus,
				// clear the current value so the user can replace it easily.
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					if field.numericOnly {
						digits := filterDigits(msg.Runes)
						if len(digits) == 0 {
							return t, nil
						}
						msg.Runes = digits
					}
					if t.armedReplace {
						field.input.SetValue("")
						field.input.SetCursor(0)
						t.armedReplace = false
					}
				} else {
					// Any non-rune key indicates the user is editing the existing value.
					t.armedReplace = false
				}

				var cmd tea.Cmd
				field.input, cmd = field.input.Update(msg)
				return t, cmd
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

func (t *SettingsTab) selectedSlotDraft() int {
	return t.slotUpstreamSlot
}

func (t *SettingsTab) loadSlotUpstreamDraft(slotID int) {
	t.slotUpstreamSlot = slotID

	upstream, ok := t.cfg.GetSlotUpstream(slotID)
	if !ok {
		upstream = config.UpstreamProxyConfig{
			Protocol: "http",
		}
	}

	if field := t.slotFieldByKey("slot_upstream_enabled"); field != nil {
		field.selectedIdx = 1
		if upstream.Enabled {
			field.selectedIdx = 0
		}
	}
	if field := t.slotFieldByKey("slot_upstream_protocol"); field != nil {
		field.selectedIdx = 0
		for i, opt := range field.options {
			if opt.value == upstream.Protocol {
				field.selectedIdx = i
				break
			}
		}
	}
	if field := t.slotFieldByKey("slot_upstream_server"); field != nil {
		field.input.SetValue(upstream.Server)
	}
	if field := t.slotFieldByKey("slot_upstream_port"); field != nil {
		if upstream.Port > 0 {
			field.input.SetValue(strconv.Itoa(upstream.Port))
		} else {
			field.input.SetValue("")
		}
	}
	if field := t.slotFieldByKey("slot_upstream_username"); field != nil {
		field.input.SetValue(upstream.Username)
	}
	if field := t.slotFieldByKey("slot_upstream_password"); field != nil {
		field.input.SetValue(upstream.Password)
	}
	t.slotFocusedField = 1
	t.slotScrollOffset = 0
	for i := range t.slotFields {
		if t.slotFields[i].fieldType == FieldTypeInput {
			t.slotFields[i].input.Blur()
		}
	}
	if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeInput {
		t.slotFields[t.slotFocusedField].input.Focus()
	}
}

func (t *SettingsTab) saveSlotUpstream() tea.Cmd {
	return func() tea.Msg {
		slotID := t.selectedSlotDraft()
		if slotID < 0 || slotID >= proxy.MaxSlots {
			return SettingsErrorMsg{Error: fmt.Errorf("invalid slot id: %d", slotID)}
		}

		enabled := false
		if field := t.slotFieldByKey("slot_upstream_enabled"); field != nil {
			enabled = field.options[field.selectedIdx].value == "true"
		}

		protocol := "http"
		if field := t.slotFieldByKey("slot_upstream_protocol"); field != nil && len(field.options) > 0 {
			protocol = field.options[field.selectedIdx].value
		}
		server := ""
		if field := t.slotFieldByKey("slot_upstream_server"); field != nil {
			server = strings.TrimSpace(field.input.Value())
		}
		portText := ""
		if field := t.slotFieldByKey("slot_upstream_port"); field != nil {
			portText = strings.TrimSpace(field.input.Value())
		}
		username := ""
		if field := t.slotFieldByKey("slot_upstream_username"); field != nil {
			username = field.input.Value()
		}
		password := ""
		if field := t.slotFieldByKey("slot_upstream_password"); field != nil {
			password = field.input.Value()
		}

		port := 0
		if portText != "" {
			var err error
			port, err = strconv.Atoi(portText)
			if err != nil || port <= 0 || port >= 65536 {
				return SettingsErrorMsg{Error: fmt.Errorf("invalid upstream port: %q", portText)}
			}
		}

		upstream := config.UpstreamProxyConfig{
			Enabled:  enabled,
			Protocol: protocol,
			Server:   server,
			Port:     port,
			Username: username,
			Password: password,
		}
		if enabled {
			if upstream.Protocol == "" {
				return SettingsErrorMsg{Error: fmt.Errorf("upstream protocol is required")}
			}
			if upstream.Server == "" {
				return SettingsErrorMsg{Error: fmt.Errorf("upstream server is required")}
			}
			if upstream.Port <= 0 || upstream.Port >= 65536 {
				return SettingsErrorMsg{Error: fmt.Errorf("upstream port is required")}
			}
		}

		t.cfg.SetSlotUpstream(slotID, upstream)
		if err := t.cfg.Save(); err != nil {
			return SettingsErrorMsg{Error: err}
		}

		restart := false
		if t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil {
			restart = true
		}
		message := fmt.Sprintf("Saved slot %d upstream settings", slotID)
		if !enabled {
			message = fmt.Sprintf("Saved slot %d upstream settings (disabled)", slotID)
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
		default:
			if t.slotFocusedField < len(t.slotFields) && t.slotFields[t.slotFocusedField].fieldType == FieldTypeInput && !t.slotFields[t.slotFocusedField].disabled {
				field := &t.slotFields[t.slotFocusedField]
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					if field.numericOnly {
						digits := filterDigits(msg.Runes)
						if len(digits) == 0 {
							return t, nil
						}
						msg.Runes = digits
					}
					if t.armedReplace {
						field.input.SetValue("")
						field.input.SetCursor(0)
						t.armedReplace = false
					}
				} else {
					t.armedReplace = false
				}

				var cmd tea.Cmd
				field.input, cmd = field.input.Update(msg)
				return t, cmd
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
	if t.showSlotDetail {
		return t.viewSlotDetail()
	}
	if t.showSlotList {
		return t.viewSlotList()
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
	upstream, ok := t.cfg.GetSlotUpstream(slotID)
	upstreamText := "no upstream"
	if ok && upstream.Enabled && upstream.Protocol != "" && upstream.Server != "" && upstream.Port > 0 {
		upstreamText = fmt.Sprintf("%s://%s:%d", upstream.Protocol, upstream.Server, upstream.Port)
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
