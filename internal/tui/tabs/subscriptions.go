package tabs

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
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

// CustomNodesChangedMsg is sent when the reserved Cus subscription changes.
type CustomNodesChangedMsg struct {
	Action       string
	AddedCount   int
	SelectedNode *subscription.Node
	StoppedSlots []int
	Error        error
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

type customInputMode int

const (
	customInputNone customInputMode = iota
	customInputAdd
	customInputEdit
)

// SubscriptionsTab represents the subscriptions tab
type SubscriptionsTab struct {
	table           *components.Table
	customTable     *components.Table
	subManager      *subscription.Manager
	slotMgr         *proxy.SlotManager
	subs            []*subscription.Subscription
	customNodes     []subscription.Node
	nodeMgr         *node.Manager
	nodeBrowser     *components.NodeBrowser
	showNodeBrowser bool
	width           int
	height          int
	activeSubID     string

	inputMode    InputMode
	urlInput     textinput.Model
	nameInput    textinput.Model
	contentInput textarea.Model
	pendingURL   string
	pendingName  string

	showCustomManager bool
	customInputMode   customInputMode
	customURIInput    textarea.Model
	customEditTarget  *subscription.Node

	updating    bool
	message     string
	messageType string // "success" or "error"
}

// NewSubscriptionsTab creates a new subscriptions tab
func NewSubscriptionsTab(subMgr *subscription.Manager, slotMgr *proxy.SlotManager, nodeMgr *node.Manager) *SubscriptionsTab {
	columns := []components.Column{
		{Title: "Name", Width: 25},
		{Title: "Nodes", Width: 8},
		{Title: "Last", Width: 14},
		{Title: "URL", Width: 36},
	}
	customColumns := []components.Column{
		{Title: "Name", Width: 24},
		{Title: "Type", Width: 10},
		{Title: "Server", Width: 42},
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

	customURIInput := textarea.New()
	customURIInput.Placeholder = "Paste raw URI links here (for example: vless://...)"
	customURIInput.CharLimit = 200000

	return &SubscriptionsTab{
		table:          components.NewTable(columns),
		customTable:    components.NewTable(customColumns),
		subManager:     subMgr,
		slotMgr:        slotMgr,
		nodeMgr:        nodeMgr,
		nodeBrowser:    components.NewNodeBrowser(nil, nodeMgr, subMgr),
		urlInput:       urlInput,
		nameInput:      nameInput,
		contentInput:   contentInput,
		customURIInput: customURIInput,
	}
}

// Init initializes the tab
func (t *SubscriptionsTab) Init() tea.Cmd {
	return t.refreshSubs()
}

// BlocksGlobalKeys returns true when this tab has an active overlay/input state.
func (t *SubscriptionsTab) BlocksGlobalKeys() bool {
	return t.inputMode != InputModeNone || t.showCustomManager || t.showNodeBrowser
}

// Update handles messages
func (t *SubscriptionsTab) Update(msg tea.Msg) (*SubscriptionsTab, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if t.showNodeBrowser {
			event, _, cmd := t.nodeBrowser.Update(msg)
			if event == components.EventClosed {
				t.nodeBrowser.Close()
				t.showNodeBrowser = false
				return t, nil
			}
			return t, cmd
		}
		if t.showCustomManager {
			if t.customInputMode != customInputNone {
				return t.handleCustomInputMode(msg)
			}
			return t.handleCustomManagerMode(msg)
		}
		if t.inputMode != InputModeNone {
			return t.handleInputMode(msg)
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.table.MoveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.table.MoveDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			sub := t.selectedSubscription()
			if sub != nil {
				t.nodeBrowser.SetSize(t.width, t.height)
				t.nodeBrowser.SetTitle(sub.Name)
				t.nodeBrowser.SetAllowNoProxy(false)
				t.nodeBrowser.Open(sub.ID, nil, components.BrowseMode)
				t.showNodeBrowser = true
				return t, nil
			}
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
		case key.Matches(msg, key.NewBinding(key.WithKeys("E"))):
			return t, t.openSelectedCustomManager()
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			return t, t.refreshSubs()
		}

	case SubsRefreshedMsg:
		t.subs = msg.Subs
		t.updateTableRows()
		if t.showCustomManager {
			t.refreshCustomNodes()
		}

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

	case CustomNodesChangedMsg:
		t.updating = false
		if msg.Error != nil {
			t.message = msg.Error.Error()
			t.messageType = "error"
		} else {
			switch msg.Action {
			case "add":
				t.message = fmt.Sprintf("Cus added %d node(s)", msg.AddedCount)
			case "edit":
				t.message = "Cus node updated"
			case "delete":
				t.message = "Cus node deleted"
			default:
				t.message = "Cus updated"
			}
			if len(msg.StoppedSlots) > 0 {
				t.message += fmt.Sprintf(" and stopped slot(s) %s", joinSlotIDs(msg.StoppedSlots))
			}
			t.messageType = "success"
		}
		t.refreshCustomNodes()
		cmds = append(cmds, t.refreshSubs())
		if msg.SelectedNode != nil {
			t.preselectCustomNode(msg.SelectedNode)
		}
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

func (t *SubscriptionsTab) handleCustomManagerMode(msg tea.KeyMsg) (*SubscriptionsTab, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		t.showCustomManager = false
		t.resetCustomInput()
		return t, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		t.customTable.MoveUp()
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		t.customTable.MoveDown()
	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		t.startCustomInput(customInputAdd, nil)
		return t, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
		target := t.selectedCustomNode()
		if target == nil {
			return t, nil
		}
		t.startCustomInput(customInputEdit, target)
		return t, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		return t, t.deleteSelectedCustomNode()
	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		t.refreshCustomNodes()
		return t, nil
	}
	return t, nil
}

func (t *SubscriptionsTab) handleCustomInputMode(msg tea.KeyMsg) (*SubscriptionsTab, tea.Cmd) {
	switch msg.String() {
	case "esc":
		t.resetCustomInput()
		return t, nil
	case "ctrl+s":
		raw := t.customURIInput.Value()
		mode := t.customInputMode
		target := t.customEditTarget
		t.resetCustomInput()
		switch mode {
		case customInputAdd:
			return t, t.addCustomNodes(raw)
		case customInputEdit:
			if target == nil {
				t.message = "No Cus node selected for editing"
				t.messageType = "error"
				return t, nil
			}
			return t, t.editCustomNode(*target, raw)
		}
	}

	var cmd tea.Cmd
	t.customURIInput, cmd = t.customURIInput.Update(msg)
	return t, cmd
}

// View renders the tab
func (t *SubscriptionsTab) View() string {
	m := i18n.M()

	if t.showNodeBrowser {
		return t.nodeBrowser.View()
	}

	if t.showCustomManager {
		if t.customInputMode != customInputNone {
			return t.renderCustomInputMode()
		}
		return t.renderCustomManager()
	}

	if t.inputMode != InputModeNone {
		return t.renderInputMode()
	}

	if len(t.subs) == 0 {
		return styles.ContentArea.Render(
			styles.Muted.Render(m.Subscription.NoSubs),
		)
	}

	t.table.SetHeight(t.height - 8)
	tableView := t.table.View()

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

	help := "\n" +
		styles.HelpKey.Render("a") + styles.HelpText.Render(" add  ") +
		styles.HelpKey.Render("z") + styles.HelpText.Render(" paste  ") +
		styles.HelpKey.Render("d") + styles.HelpText.Render(" delete  ") +
		styles.HelpKey.Render("u") + styles.HelpText.Render(" update  ") +
		styles.HelpKey.Render("U") + styles.HelpText.Render(" update all  ") +
		styles.HelpKey.Render("E") + styles.HelpText.Render(" manage Cus")

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

func (t *SubscriptionsTab) renderCustomManager() string {
	title := styles.Title.Render("Cus")
	subtitle := styles.HelpText.Render("Local subscription. Manage nodes here with raw URI links.")

	body := styles.Muted.Render("Cus has no custom nodes yet.")
	if len(t.customNodes) > 0 {
		t.customTable.SetHeight(t.height - 10)
		body = t.customTable.View()
	}

	var messageView string
	if t.message != "" {
		if t.messageType == "success" {
			messageView = "\n" + styles.SuccessMessage.Render(t.message)
		} else {
			messageView = "\n" + styles.ErrorMessage.Render(t.message)
		}
	}

	help := "\n" +
		styles.HelpKey.Render("a") + styles.HelpText.Render(" add URI  ") +
		styles.HelpKey.Render("e") + styles.HelpText.Render(" replace selected  ") +
		styles.HelpKey.Render("d") + styles.HelpText.Render(" delete selected  ") +
		styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")

	content := title + "\n" + subtitle + "\n\n" + body + messageView + help
	return styles.ContentArea.Render(content)
}

func (t *SubscriptionsTab) renderCustomInputMode() string {
	title := "Paste Cus node URI(s)"
	if t.customInputMode == customInputEdit && t.customEditTarget != nil {
		title = fmt.Sprintf("Replace Cus node: %s", t.customEditTarget.Name)
	}

	content := styles.InputLabel.Render(title) + "\n\n" +
		t.customURIInput.View() + "\n\n" +
		styles.HelpText.Render("Ctrl+S to save, Esc to cancel")

	return styles.ContentArea.Render(
		styles.InfoBox.Render(content),
	)
}

// SetSize sets the tab size
func (t *SubscriptionsTab) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.nodeBrowser.SetSize(width, height)
	t.urlInput.Width = width - 10
	t.nameInput.Width = width - 10

	t.contentInput.SetWidth(width - 10)
	contentHeight := height - 14
	if contentHeight < 5 {
		contentHeight = 5
	}
	t.contentInput.SetHeight(contentHeight)

	t.customURIInput.SetWidth(width - 10)
	customHeight := height - 14
	if customHeight < 5 {
		customHeight = 5
	}
	t.customURIInput.SetHeight(customHeight)
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

func (t *SubscriptionsTab) resetCustomInput() {
	t.customInputMode = customInputNone
	t.customEditTarget = nil
	t.customURIInput.Blur()
	t.customURIInput.SetValue("")
}

func (t *SubscriptionsTab) startCustomInput(mode customInputMode, target *subscription.Node) {
	t.customInputMode = mode
	t.customEditTarget = nil
	t.customURIInput.SetValue("")
	if target != nil {
		nodeCopy := *target
		t.customEditTarget = &nodeCopy
	}
	t.customURIInput.Focus()
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

func (t *SubscriptionsTab) refreshCustomNodes() {
	if t.subManager == nil {
		t.customNodes = nil
		t.customTable.SetRows(nil)
		return
	}
	t.customNodes = t.subManager.GetNodesForSubscription(subscription.CustomSubscriptionID)
	t.updateCustomTableRows()
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
		switch sub.Source {
		case subscription.SourceManual:
			if url == "" {
				url = "<manual>"
			}
		case subscription.SourceCustom:
			url = "<local Cus>"
		}
		if len(url) > 37 {
			url = url[:37] + "..."
		}

		name := sub.Name
		if subscription.IsCustomSubscription(sub) {
			name = "📌 " + name
		}
		rows[i] = []string{
			name,
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

func (t *SubscriptionsTab) updateCustomTableRows() {
	prevIdx := t.customTable.Selected()
	rows := make([][]string, len(t.customNodes))
	for i, n := range t.customNodes {
		rows[i] = []string{
			n.Name,
			n.Type,
			fmt.Sprintf("%s:%d", n.Server, n.Port),
		}
	}
	t.customTable.SetRows(rows)
	if prevIdx >= 0 && prevIdx < len(rows) {
		t.customTable.SetSelected(prevIdx)
		return
	}
	if len(rows) > 0 {
		t.customTable.SetSelected(0)
	}
}

func (t *SubscriptionsTab) preselectCustomNode(node *subscription.Node) {
	if node == nil {
		return
	}
	for i := range t.customNodes {
		if t.customNodes[i].Name == node.Name &&
			t.customNodes[i].Type == node.Type &&
			t.customNodes[i].Server == node.Server &&
			t.customNodes[i].Port == node.Port {
			t.customTable.SetSelected(i)
			return
		}
	}
}

// SetNodeBrowserIndicators forwards slot indicators to the embedded browser.
func (t *SubscriptionsTab) SetNodeBrowserIndicators(indicators map[string]components.SlotIndicator) {
	if t.nodeBrowser != nil {
		t.nodeBrowser.SetSlotIndicators(indicators)
	}
}

// SetNodeBrowserTraffic forwards per-node traffic strings to the embedded browser.
func (t *SubscriptionsTab) SetNodeBrowserTraffic(traffic map[string]string) {
	if t.nodeBrowser != nil {
		t.nodeBrowser.SetSlotTraffic(traffic)
	}
}

// SetActiveSubscription sets the highlighted subscription row.
func (t *SubscriptionsTab) SetActiveSubscription(id string) {
	t.activeSubID = id
	t.updateTableRows()
}

func (t *SubscriptionsTab) selectedSubscription() *subscription.Subscription {
	idx := t.table.Selected()
	if idx < 0 || idx >= len(t.subs) {
		return nil
	}
	return t.subs[idx]
}

func (t *SubscriptionsTab) selectedCustomNode() *subscription.Node {
	idx := t.customTable.Selected()
	if idx < 0 || idx >= len(t.customNodes) {
		return nil
	}
	nodeCopy := t.customNodes[idx]
	return &nodeCopy
}

func (t *SubscriptionsTab) openSelectedCustomManager() tea.Cmd {
	sub := t.selectedSubscription()
	if sub == nil {
		return nil
	}
	if !subscription.IsCustomSubscription(sub) {
		t.message = "Only Cus can be managed with E"
		t.messageType = "error"
		return nil
	}
	t.showCustomManager = true
	t.resetCustomInput()
	t.refreshCustomNodes()
	return nil
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
	sub := t.selectedSubscription()
	if sub == nil {
		return nil
	}
	if subscription.IsCustomSubscription(sub) {
		t.message = "Cus cannot be deleted. Press E to manage its nodes."
		t.messageType = "error"
		return nil
	}
	return func() tea.Msg {
		err := t.subManager.Delete(sub.ID)
		if err != nil {
			return SubscriptionErrorMsg{Error: err}
		}
		return SubscriptionUpdatedMsg{}
	}
}

func (t *SubscriptionsTab) updateSelected() tea.Cmd {
	sub := t.selectedSubscription()
	if sub == nil {
		return nil
	}
	if subscription.IsCustomSubscription(sub) {
		t.message = "Cus is local. Press E to manage its nodes."
		t.messageType = "error"
		return nil
	}
	t.updating = true
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

func (t *SubscriptionsTab) updateAll() tea.Cmd {
	t.updating = true
	return func() tea.Msg {
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

func (t *SubscriptionsTab) addCustomNodes(raw string) tea.Cmd {
	t.updating = true
	return func() tea.Msg {
		nodes, err := t.subManager.AddCustomNodes(raw)
		var selected *subscription.Node
		if len(nodes) > 0 {
			nodeCopy := nodes[len(nodes)-1]
			selected = &nodeCopy
		}
		return CustomNodesChangedMsg{
			Action:       "add",
			AddedCount:   len(nodes),
			SelectedNode: selected,
			Error:        err,
		}
	}
}

func (t *SubscriptionsTab) editCustomNode(target subscription.Node, raw string) tea.Cmd {
	t.updating = true
	return func() tea.Msg {
		updated, err := t.subManager.UpdateCustomNode(target, raw)
		stoppedSlots, stopErr := t.stopCustomSlotsForNode(target)
		if err == nil && stopErr != nil {
			err = stopErr
		}
		return CustomNodesChangedMsg{
			Action:       "edit",
			SelectedNode: updated,
			StoppedSlots: stoppedSlots,
			Error:        err,
		}
	}
}

func (t *SubscriptionsTab) deleteSelectedCustomNode() tea.Cmd {
	target := t.selectedCustomNode()
	if target == nil {
		return nil
	}

	var nextSelected *subscription.Node
	if len(t.customNodes) > 1 {
		idx := t.customTable.Selected()
		nextIdx := idx
		if nextIdx >= len(t.customNodes)-1 {
			nextIdx = len(t.customNodes) - 2
		}
		if nextIdx >= 0 && nextIdx < len(t.customNodes) {
			nodeCopy := t.customNodes[nextIdx]
			nextSelected = &nodeCopy
		}
	}

	t.updating = true
	return func() tea.Msg {
		err := t.subManager.DeleteCustomNode(*target)
		stoppedSlots, stopErr := t.stopCustomSlotsForNode(*target)
		if err == nil && stopErr != nil {
			err = stopErr
		}
		return CustomNodesChangedMsg{
			Action:       "delete",
			SelectedNode: nextSelected,
			StoppedSlots: stoppedSlots,
			Error:        err,
		}
	}
}

func (t *SubscriptionsTab) stopCustomSlotsForNode(target subscription.Node) ([]int, error) {
	if t.slotMgr == nil {
		return nil, nil
	}

	var stopped []int
	var stopErrors []string
	for _, state := range t.slotMgr.ActiveSlots() {
		if state == nil {
			continue
		}
		if state.SubID != subscription.CustomSubscriptionID || state.NodeName != target.Name {
			continue
		}
		if err := t.slotMgr.StopSlot(state.SlotID); err != nil {
			stopErrors = append(stopErrors, fmt.Sprintf("slot %d: %v", state.SlotID, err))
			continue
		}
		stopped = append(stopped, state.SlotID)
	}
	sort.Ints(stopped)
	if len(stopErrors) > 0 {
		return stopped, fmt.Errorf("Cus changed, but failed to stop %s", strings.Join(stopErrors, ", "))
	}
	return stopped, nil
}

func joinSlotIDs(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ",")
}

// SubsRefreshedMsg is sent when subscriptions are refreshed
type SubsRefreshedMsg struct {
	Subs []*subscription.Subscription
}
