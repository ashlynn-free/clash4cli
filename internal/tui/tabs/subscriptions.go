package tabs

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/components"
	"rocket4cli/internal/tui/styles"
)

// SubscriptionAddedMsg is sent when a subscription is added
type SubscriptionAddedMsg struct {
	Sub *subscription.Subscription
}

// SubscriptionUpdatedMsg is sent when subscriptions are updated
type SubscriptionUpdatedMsg struct {
	IDs      []string
	Name     string
	ViaProxy bool
}

// SubscriptionErrorMsg is sent when an error occurs
type SubscriptionErrorMsg struct {
	Error error
}

// InputMode represents the current input mode
type InputMode int

const (
	InputModeNone InputMode = iota
	InputModeURL
	InputModeName
	InputModeManualName
	InputModeManualContent
)

// SubscriptionsTab represents the subscriptions tab
type SubscriptionsTab struct {
	table       *components.Table
	subManager  *subscription.Manager
	subs        []*subscription.Subscription
	width       int
	height      int
	activeSubID string

	inputMode    InputMode
	urlInput     textinput.Model
	nameInput    textinput.Model
	contentInput textarea.Model
	pendingURL   string
	pendingName  string
	updating     bool
	message      string
	messageType  string // "success" or "error"
}

// NewSubscriptionsTab creates a new subscriptions tab
func NewSubscriptionsTab(subMgr *subscription.Manager) *SubscriptionsTab {
	columns := []components.Column{
		{Title: "Name", Width: 25},
		{Title: "Nodes", Width: 8},
		{Title: "Last", Width: 14},
		{Title: "URL", Width: 36},
	}

	urlInput := textinput.New()
	urlInput.Placeholder = "https://..."
	urlInput.CharLimit = 500

	nameInput := textinput.New()
	nameInput.Placeholder = "My Subscription"
	nameInput.CharLimit = 50

	contentInput := textarea.New()
	contentInput.Placeholder = "Paste subscription content here (base64 / Clash YAML / SIP008 JSON)..."
	contentInput.CharLimit = 200000

	return &SubscriptionsTab{
		table:        components.NewTable(columns),
		subManager:   subMgr,
		urlInput:     urlInput,
		nameInput:    nameInput,
		contentInput: contentInput,
	}
}

// Init initializes the tab
func (t *SubscriptionsTab) Init() tea.Cmd {
	return t.refreshSubs()
}

// Update handles messages
func (t *SubscriptionsTab) Update(msg tea.Msg) (*SubscriptionsTab, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if t.inputMode != InputModeNone {
			return t.handleInputMode(msg)
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.table.MoveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.table.MoveDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			t.inputMode = InputModeURL
			t.urlInput.Focus()
			return t, textinput.Blink
		case key.Matches(msg, key.NewBinding(key.WithKeys("z"))):
			t.inputMode = InputModeManualName
			t.nameInput.Focus()
			return t, textinput.Blink
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			return t, t.deleteSelected()
		case key.Matches(msg, key.NewBinding(key.WithKeys("u"))):
			return t, t.updateSelected()
		case key.Matches(msg, key.NewBinding(key.WithKeys("U"))):
			return t, t.updateAll()
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			return t, t.refreshSubs()
		}

	case SubsRefreshedMsg:
		t.subs = msg.Subs
		t.updateTableRows()

	case SubscriptionAddedMsg:
		t.message = fmt.Sprintf("Added: %s (%d nodes)", msg.Sub.Name, len(msg.Sub.Nodes))
		t.messageType = "success"
		cmds = append(cmds, t.refreshSubs())

	case SubscriptionUpdatedMsg:
		suffix := ""
		if msg.ViaProxy {
			suffix = " via proxy"
		}
		if msg.Name != "" {
			t.message = fmt.Sprintf("%s: %s%s", i18n.M().Subscription.Update, msg.Name, suffix)
		} else {
			t.message = i18n.M().Subscription.Update + " complete" + suffix
		}
		t.messageType = "success"
		t.updating = false
		cmds = append(cmds, t.refreshSubs())

	case SubscriptionErrorMsg:
		t.message = msg.Error.Error()
		t.messageType = "error"
		t.updating = false
	}

	return t, tea.Batch(cmds...)
}

func (t *SubscriptionsTab) handleInputMode(msg tea.KeyMsg) (*SubscriptionsTab, tea.Cmd) {
	switch msg.String() {
	case "esc":
		t.resetInputs()
		return t, nil

	case "enter":
		if t.inputMode == InputModeURL {
			url := t.urlInput.Value()
			if url != "" {
				t.pendingURL = url
				t.urlInput.Reset()
				t.inputMode = InputModeName
				t.nameInput.Focus()
				return t, textinput.Blink
			}
		} else if t.inputMode == InputModeName {
			name := t.nameInput.Value()
			if name == "" {
				name = "Subscription"
			}
			url := t.pendingURL
			t.resetInputs()
			return t, t.addSubscription(name, url)
		} else if t.inputMode == InputModeManualName {
			name := t.nameInput.Value()
			if name == "" {
				name = "Subscription"
			}
			t.pendingName = name
			t.nameInput.Reset()
			t.inputMode = InputModeManualContent
			t.contentInput.Focus()
			return t, nil
		}
		return t, nil

	case "ctrl+s":
		if t.inputMode == InputModeManualContent {
			name := t.pendingName
			if name == "" {
				name = "Subscription"
			}
			content := t.contentInput.Value()
			t.resetInputs()
			return t, t.addManualSubscription(name, content)
		}
	}

	var cmd tea.Cmd
	if t.inputMode == InputModeURL {
		t.urlInput, cmd = t.urlInput.Update(msg)
	} else if t.inputMode == InputModeName || t.inputMode == InputModeManualName {
		t.nameInput, cmd = t.nameInput.Update(msg)
	} else if t.inputMode == InputModeManualContent {
		t.contentInput, cmd = t.contentInput.Update(msg)
	}
	return t, cmd
}

// View renders the tab
func (t *SubscriptionsTab) View() string {
	m := i18n.M()

	// Input mode
	if t.inputMode != InputModeNone {
		return t.renderInputMode()
	}

	if len(t.subs) == 0 {
		return styles.ContentArea.Render(
			styles.Muted.Render(m.Subscription.NoSubs),
		)
	}

	// Table
	t.table.SetHeight(t.height - 8)
	tableView := t.table.View()

	// Message
	var messageView string
	if t.message != "" {
		if t.messageType == "success" {
			messageView = "\n" + styles.SuccessMessage.Render(t.message)
		} else {
			messageView = "\n" + styles.ErrorMessage.Render(t.message)
		}
	}

	if t.updating {
		messageView = "\n" + styles.NodeTesting.Render(m.Subscription.Updating)
	}

	// Help
	help := "\n" +
		styles.HelpKey.Render("a") + styles.HelpText.Render(" add  ") +
		styles.HelpKey.Render("z") + styles.HelpText.Render(" paste  ") +
		styles.HelpKey.Render("d") + styles.HelpText.Render(" delete  ") +
		styles.HelpKey.Render("u") + styles.HelpText.Render(" update  ") +
		styles.HelpKey.Render("U") + styles.HelpText.Render(" update all")

	return styles.ContentArea.Render(tableView + messageView + help)
}

func (t *SubscriptionsTab) renderInputMode() string {
	m := i18n.M()

	var content string
	if t.inputMode == InputModeURL {
		content = styles.InputLabel.Render(m.Subscription.InputURL) + "\n\n" +
			t.urlInput.View() + "\n\n" +
			styles.HelpText.Render("Enter to confirm, Esc to cancel")
	} else if t.inputMode == InputModeName {
		content = styles.InputLabel.Render(m.Subscription.InputName) + "\n\n" +
			t.nameInput.View() + "\n\n" +
			styles.HelpText.Render("Enter to confirm, Esc to cancel")
	} else if t.inputMode == InputModeManualName {
		content = styles.InputLabel.Render(m.Subscription.InputName) + "\n\n" +
			t.nameInput.View() + "\n\n" +
			styles.HelpText.Render("Enter to continue, Esc to cancel")
	} else if t.inputMode == InputModeManualContent {
		content = styles.InputLabel.Render(m.Subscription.InputContent) + "\n\n" +
			t.contentInput.View() + "\n\n" +
			styles.HelpText.Render("Ctrl+S to save, Esc to cancel")
	}

	return styles.ContentArea.Render(
		styles.InfoBox.Render(content),
	)
}

// SetSize sets the tab size
func (t *SubscriptionsTab) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.urlInput.Width = width - 10
	t.nameInput.Width = width - 10

	t.contentInput.SetWidth(width - 10)
	contentHeight := height - 14
	if contentHeight < 5 {
		contentHeight = 5
	}
	t.contentInput.SetHeight(contentHeight)
}

func (t *SubscriptionsTab) resetInputs() {
	t.urlInput.Blur()
	t.urlInput.Reset()
	t.nameInput.Blur()
	t.nameInput.Reset()
	t.contentInput.Blur()
	t.contentInput.SetValue("")
	t.pendingURL = ""
	t.pendingName = ""
	t.inputMode = InputModeNone
}

func (t *SubscriptionsTab) refreshSubs() tea.Cmd {
	return func() tea.Msg {
		subs := t.subManager.List()
		sortSubscriptionsByNameAndID(subs)
		return SubsRefreshedMsg{Subs: subs}
	}
}

func sortSubscriptionsByNameAndID(subs []*subscription.Subscription) {
	sort.Slice(subs, func(i, j int) bool {
		if subs[i].Name == subs[j].Name {
			return subs[i].ID < subs[j].ID
		}
		return subs[i].Name < subs[j].Name
	})
}

func (t *SubscriptionsTab) updateTableRows() {
	prevIdx := t.table.Selected()
	rows := make([][]string, len(t.subs))
	activeIdx := -1

	for i, sub := range t.subs {
		if sub != nil && sub.ID == t.activeSubID {
			activeIdx = i
		}

		last := "-"
		if !sub.LastUpdateAttemptAt.IsZero() {
			status := "🟢"
			if sub.LastUpdateError != "" {
				status = "🔴"
			} else if sub.LastUpdateViaProxy {
				status = "🟢🧦"
			}
			last = sub.LastUpdateAttemptAt.Format("01-02 15:04") + " " + status
		} else if !sub.UpdatedAt.IsZero() {
			last = sub.UpdatedAt.Format("01-02 15:04")
		}

		url := sub.URL
		if url == "" && sub.Source == "manual" {
			url = "<manual>"
		}
		if len(url) > 37 {
			url = url[:37] + "..."
		}

		rows[i] = []string{
			sub.Name,
			fmt.Sprintf("%d", len(sub.Nodes)),
			last,
			url,
		}
	}

	t.table.SetRows(rows)
	if activeIdx >= 0 {
		t.table.SetSelected(activeIdx)
		return
	}
	if prevIdx >= 0 && prevIdx < len(rows) {
		t.table.SetSelected(prevIdx)
		return
	}
	if len(rows) > 0 {
		t.table.SetSelected(0)
	}
}

func (t *SubscriptionsTab) SetActiveSubscription(id string) {
	t.activeSubID = id
	t.updateTableRows()
}

func (t *SubscriptionsTab) addSubscription(name, url string) tea.Cmd {
	return func() tea.Msg {
		sub, err := t.subManager.Add(name, url)
		if err != nil {
			return SubscriptionErrorMsg{Error: err}
		}
		return SubscriptionAddedMsg{Sub: sub}
	}
}

func (t *SubscriptionsTab) addManualSubscription(name, content string) tea.Cmd {
	return func() tea.Msg {
		sub, err := t.subManager.AddManual(name, content)
		if err != nil {
			return SubscriptionErrorMsg{Error: err}
		}
		return SubscriptionAddedMsg{Sub: sub}
	}
}

func (t *SubscriptionsTab) deleteSelected() tea.Cmd {
	idx := t.table.Selected()
	if idx >= 0 && idx < len(t.subs) {
		sub := t.subs[idx]
		return func() tea.Msg {
			err := t.subManager.Delete(sub.ID)
			if err != nil {
				return SubscriptionErrorMsg{Error: err}
			}
			return SubscriptionUpdatedMsg{}
		}
	}
	return nil
}

func (t *SubscriptionsTab) updateSelected() tea.Cmd {
	idx := t.table.Selected()
	if idx >= 0 && idx < len(t.subs) {
		t.updating = true
		sub := t.subs[idx]
		return func() tea.Msg {
			err := t.subManager.Update(sub.ID)
			if err != nil {
				return SubscriptionErrorMsg{Error: err}
			}
			updated, _ := t.subManager.Get(sub.ID)
			viaProxy := false
			if updated != nil {
				viaProxy = updated.LastUpdateViaProxy
			}
			return SubscriptionUpdatedMsg{
				IDs:      []string{sub.ID},
				Name:     sub.Name,
				ViaProxy: viaProxy,
			}
		}
	}
	return nil
}

func (t *SubscriptionsTab) updateAll() tea.Cmd {
	t.updating = true
	return func() tea.Msg {
		// Add small delay to show updating status
		time.Sleep(100 * time.Millisecond)
		subs := t.subManager.List()
		ids := make([]string, 0, len(subs))
		for _, sub := range subs {
			if sub != nil {
				ids = append(ids, sub.ID)
			}
		}
		err := t.subManager.UpdateAll()
		if err != nil {
			return SubscriptionErrorMsg{Error: err}
		}
		return SubscriptionUpdatedMsg{IDs: ids}
	}
}

// SubsRefreshedMsg is sent when subscriptions are refreshed
type SubsRefreshedMsg struct {
	Subs []*subscription.Subscription
}
