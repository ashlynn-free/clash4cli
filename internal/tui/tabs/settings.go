package tabs

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/tui/styles"
)

// SettingsSavedMsg is sent when settings are saved
type SettingsSavedMsg struct{}

// SettingsErrorMsg is sent when settings could not be saved or validated
type SettingsErrorMsg struct {
	Error error
}

type settingsFlashTickMsg struct {
	id int
}

// FieldType represents the type of setting field
type FieldType int

const (
	FieldTypeInput FieldType = iota
	FieldTypeSelect
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
	{label: "English", value: "en_us"},
	{label: "简体中文", value: "zh_cn"},
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
			label:       i18n.M().Settings.Language,
			fieldType:   FieldTypeSelect,
			key:         "language",
			options:     languageOptions,
			selectedIdx: langIdx,
		},
		{
			label:       i18n.M().Settings.ServerDisplay,
			fieldType:   FieldTypeSelect,
			key:         "server_display",
			options:     serverDisplayOptions,
			selectedIdx: serverDisplayIdx,
		},
		{
			label:     i18n.M().Settings.Concurrent,
			fieldType: FieldTypeInput,
			input:     concurrentInput,
			key:       "concurrent",
		},
	}

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
			if t.focusedField == len(t.fields) {
				// Save button
				return t, t.saveSettings()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+s"))):
			return t, t.saveSettings()
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			// For select fields, move selection left
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeSelect {
				field := &t.fields[t.focusedField]
				field.selectedIdx--
				if field.selectedIdx < 0 {
					field.selectedIdx = len(field.options) - 1
				}
				return t, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			// For select fields, move selection right
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeSelect {
				field := &t.fields[t.focusedField]
				field.selectedIdx++
				if field.selectedIdx >= len(field.options) {
					field.selectedIdx = 0
				}
				return t, nil
			}
		default:
			// Pass to focused input
			if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput {
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
		t.message = i18n.M().Settings.Saved
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
	t.focusedField++
	if t.focusedField > len(t.fields) {
		t.focusedField = 0
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
	t.focusedField--
	if t.focusedField < 0 {
		t.focusedField = len(t.fields)
	}
	if t.focusedField < len(t.fields) && t.fields[t.focusedField].fieldType == FieldTypeInput {
		t.fields[t.focusedField].input.Focus()
		t.armedReplace = true
	} else {
		t.armedReplace = false
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
		label := field.label
		if i == t.focusedField {
			label = styles.Primary.Render("▶ " + label)
		} else {
			label = "  " + label
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
		styles.HelpKey.Render("Ctrl+S") + styles.HelpText.Render(" save")

	return styles.ContentArea.Render(content + help)
}

func (t *SettingsTab) renderSelect(field settingField, focused bool) string {
	var result string
	for i, opt := range field.options {
		if i == field.selectedIdx {
			if focused {
				result += styles.SelectOptionActive.Render(" " + opt.label + " ")
			} else {
				result += styles.SelectOptionSelected.Render(" " + opt.label + " ")
			}
		} else {
			result += styles.SelectOption.Render(" " + opt.label + " ")
		}
		if i < len(field.options)-1 {
			result += "  "
		}
	}
	return result
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
		// Parse and validate values
		for _, field := range t.fields {
			switch field.key {
			case "mixed_port":
				port, err := strconv.Atoi(field.input.Value())
				if err != nil || port <= 0 || port >= 65536 {
					return SettingsErrorMsg{Error: fmt.Errorf("invalid mixed port: %q", field.input.Value())}
				}
				t.cfg.Proxy.MixedPort = port
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

		return SettingsSavedMsg{}
	}
}

// GetConfig returns the current config
func (t *SettingsTab) GetConfig() *config.Config {
	return t.cfg
}
