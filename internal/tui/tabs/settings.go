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
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/netctl"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/tui/styles"
)

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

type settingsFlashTickMsg struct {
	id int
}

// FieldType represents the type of setting field
type FieldType int

const (
	FieldTypeInput FieldType = iota
	FieldTypeSelect
	FieldTypeHeader
)

// SettingsTab represents the settings tab
type SettingsTab struct {
	cfg          *config.Config
	width        int
	height       int
	focusedField int
	fields       []settingField
	armedReplace bool
	message      string
	messageType  string
	messageShow  bool
	flashID      int
	flashTicks   int
}

type settingField struct {
	label     string
	fieldType FieldType
	key       string
	disabled  bool
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

// NewSettingsTab creates a new settings tab
func NewSettingsTab(cfg *config.Config) *SettingsTab {
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
			label:     "Mixed Port (SOCKS5 + HTTP)",
			fieldType: FieldTypeInput,
			input:     mixedPortInput,
			key:       "mixed_port",
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
			label:     i18n.M().Settings.Concurrent,
			fieldType: FieldTypeInput,
			input:     concurrentInput,
			key:       "concurrent",
		},
	)

	// Focus first field
	fields[0].input.Focus()

	return &SettingsTab{
		cfg:          cfg,
		fields:       fields,
		armedReplace: true,
	}
}

// Init initializes the tab
func (t *SettingsTab) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages
func (t *SettingsTab) Update(msg tea.Msg) (*SettingsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.focusPrev()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.focusNext()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			switch t.focusedField {
			case len(t.fields):
				// Save button
				return t, t.saveSettings()
			case len(t.fields) + 1:
				// Restart core button
				return t, func() tea.Msg { return RestartCoreMsg{} }
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+s"))):
			return t, t.saveSettings()
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
			return t, func() tea.Msg { return RestartCoreMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			// For select fields, move selection left
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeSelect && !t.fields[t.focusedField].disabled {
				field := &t.fields[t.focusedField]
				field.selectedIdx--
				if field.selectedIdx < 0 {
					field.selectedIdx = len(field.options) - 1
				}
				return t, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			// For select fields, move selection right
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeSelect && !t.fields[t.focusedField].disabled {
				field := &t.fields[t.focusedField]
				field.selectedIdx++
				if field.selectedIdx >= len(field.options) {
					field.selectedIdx = 0
				}
				return t, nil
			}
		default:
			// Pass to focused input
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput && !t.fields[t.focusedField].disabled {
				field := &t.fields[t.focusedField]
				// Numeric-only behavior: accept only digits. On first typing after focus,
				// clear the current value so the user can replace it easily.
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					digits := filterDigits(msg.Runes)
					if len(digits) == 0 {
						return t, nil
					}
					if t.armedReplace {
						field.input.SetValue("")
						field.input.SetCursor(0)
						t.armedReplace = false
					}
					msg.Runes = digits
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
	}
	for {
		t.focusedField++
		if t.focusedField > len(t.fields)+1 {
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
	}
	for {
		t.focusedField--
		if t.focusedField < 0 {
			t.focusedField = len(t.fields) + 1
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
	var content string

	for i, field := range t.fields {
		if field.fieldType == FieldTypeHeader {
			content += styles.Title.Render(field.label) + "\n\n"
			continue
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
		content += label + "\n"

		switch field.fieldType {
		case FieldTypeInput:
			inputStyle := styles.InputField
			if i == t.focusedField {
				inputStyle = styles.InputFocused
			}
			content += inputStyle.Render(field.input.View()) + "\n\n"

		case FieldTypeSelect:
			content += t.renderSelect(field, i == t.focusedField) + "\n\n"
		}
	}

	// Save button
	saveLabel := i18n.M().Settings.Save
	if t.focusedField == len(t.fields) {
		saveLabel = styles.StatusConnected.Render("▶ [" + saveLabel + "]")
	} else {
		saveLabel = styles.Muted.Render("  [" + saveLabel + "]")
	}
	content += saveLabel + "\n"

	// Restart core button
	restartLabel := i18n.M().Settings.RestartCore
	if restartLabel == "" {
		restartLabel = "Restart core"
	}
	if t.focusedField == len(t.fields)+1 {
		restartLabel = styles.StatusConnected.Render("▶ [" + restartLabel + "]")
	} else {
		restartLabel = styles.Muted.Render("  [" + restartLabel + "]")
	}
	content += restartLabel + "\n"

	// Message
	if t.message != "" && t.messageShow {
		if t.messageType == "success" {
			content += "\n" + styles.SuccessMessage.Render(t.message)
		} else {
			content += "\n" + styles.ErrorMessage.Render(t.message)
		}
	}

	// Help
	help := "\n\n" +
		styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" navigate  ") +
		styles.HelpKey.Render("←/→") + styles.HelpText.Render(" select  ") +
		styles.HelpKey.Render("Ctrl+S") + styles.HelpText.Render(" save  ") +
		styles.HelpKey.Render("Ctrl+R") + styles.HelpText.Render(" restart")

	return styles.ContentArea.Render(content + help)
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
}

func (t *SettingsTab) saveSettings() tea.Cmd {
	return func() tea.Msg {
		prevDocker := t.cfg.Proxy.DockerRedirectTCP
		prevTun := t.cfg.Proxy.TunEnabled

		// Parse and validate values
		for _, field := range t.fields {
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

		return SettingsSavedMsg{Message: notice}
	}
}

// GetConfig returns the current config
func (t *SettingsTab) GetConfig() *config.Config {
	return t.cfg
}
