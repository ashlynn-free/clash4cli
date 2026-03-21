package tabs

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/components"
	"rocket4cli/internal/tui/styles"
)

// SlotToggleRequestMsg asks the model to start/stop a slot from its persisted config.
type SlotToggleRequestMsg struct {
	SlotID int
}

// SlotRestartRequestMsg asks the model to restart a running slot from its persisted config.
type SlotRestartRequestMsg struct {
	SlotID int
}

// SlotActionResultMsg reports the result of a slot start/stop/restart action.
type SlotActionResultMsg struct {
	SlotID  int
	Action  string
	Running bool
	Err     error
}

type slotsView int

const (
	slotsViewList slotsView = iota
	_                       // reserved (was slotsViewDetail)
	_                       // reserved (was slotsViewModePicker)
	slotsViewProxyPicker
	slotsViewPortPicker
	slotsViewManualPortInput
	slotsViewUpstreamPicker
)

const (
	cardFocusPort = iota
	cardFocusMode
	cardFocusProxy
	cardFocusUpstream
	cardFocusItemCount
)

// SlotsTab renders the top-level slot workspace.
type SlotsTab struct {
	cfg        *config.Config
	subManager *subscription.Manager
	slotMgr    *proxy.SlotManager
	nodeMgr    *node.Manager

	width  int
	height int

	view         slotsView
	proxyList    *components.Table
	upstreamList *components.Table

	slotListIndex      int
	slotRowOffset      int
	activeSlot         int
	detailCursor       int
	portChoiceIndex    int
	proxySubID         string
	proxySubIndex      int
	manualPortInput    textinput.Model
	manualPortSelected bool
	message            string
	messageType        string
}

// NewSlotsTab creates the slots tab.
func NewSlotsTab(cfg *config.Config, subMgr *subscription.Manager, slotMgr *proxy.SlotManager, nodeMgr *node.Manager) *SlotsTab {
	manualPortInput := textinput.New()
	manualPortInput.CharLimit = 5
	manualPortInput.Placeholder = "17890"

	return &SlotsTab{
		cfg:             cfg,
		subManager:      subMgr,
		slotMgr:         slotMgr,
		nodeMgr:         nodeMgr,
		view:            slotsViewList,
		detailCursor:    cardFocusProxy,
		proxyList:       components.NewTable(nil),
		upstreamList:    components.NewTable(nil),
		manualPortInput: manualPortInput,
	}
}

// Init initializes the tab.
func (t *SlotsTab) Init() tea.Cmd {
	return textinput.Blink
}

// SlotTestDoneMsg signals that a slot node latency test has completed.
type SlotTestDoneMsg struct{ SlotID int }

// BlocksGlobalKeys reports whether a modal sub-view is active.
func (t *SlotsTab) BlocksGlobalKeys() bool {
	return t != nil && t.view != slotsViewList
}

// IsFullScreen reports whether the slots tab wants full-screen rendering (no tab bar / status bar).
func (t *SlotsTab) IsFullScreen() bool {
	return t != nil && t.view != slotsViewList
}

// SetSize updates layout dimensions.
func (t *SlotsTab) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.refreshTables()
}

// Update handles slot UI input.
func (t *SlotsTab) Update(msg tea.Msg) (*SlotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case SlotActionResultMsg:
		if msg.Err != nil {
			t.setMessage(msg.Err.Error(), "error")
		} else {
			t.message = ""
		}
		return t, nil
	case SlotTestDoneMsg:
		return t, nil // nodeMgr already cached the result; redraw picks it up
	}

	switch t.view {
	case slotsViewProxyPicker:
		return t.updateProxyPicker(msg)
	case slotsViewPortPicker:
		return t.updatePortPicker(msg)
	case slotsViewManualPortInput:
		return t.updateManualPortInput(msg)
	case slotsViewUpstreamPicker:
		return t.updateUpstreamPicker(msg)
	default:
		return t.updateList(msg)
	}
}

func (t *SlotsTab) updateList(msg tea.Msg) (*SlotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if t.slotListIndex > 0 {
				t.slotListIndex--
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if t.slotListIndex < proxy.MaxSlots-1 {
				t.slotListIndex++
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if t.detailCursor > cardFocusPort {
				t.detailCursor--
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if t.detailCursor < cardFocusUpstream {
				t.detailCursor++
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			t.activeSlot = t.selectedSlotID()
			switch t.detailCursor {
			case cardFocusMode:
				// Direct toggle Rule ↔ Global
				current := t.cfg.GetSlotStartMode(t.activeSlot)
				if strings.EqualFold(current, "global") {
					t.cfg.SetSlotStartMode(t.activeSlot, "rule")
				} else {
					t.cfg.SetSlotStartMode(t.activeSlot, "global")
				}
				return t, t.saveAndMaybeRestart(t.activeSlot)
			case cardFocusProxy:
				t.openProxyPicker()
			case cardFocusPort:
				if t.cfg.GetSlotPortMode(t.activeSlot) == "manual" {
					t.portChoiceIndex = 1
				} else {
					t.portChoiceIndex = 0
				}
				t.view = slotsViewPortPicker
			case cardFocusUpstream:
				t.view = slotsViewUpstreamPicker
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			return t, t.testSelectedSlotNode()
		case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
			return t, func() tea.Msg {
				return SlotToggleRequestMsg{SlotID: t.selectedSlotID()}
			}
		}
	}
	return t, nil
}

func (t *SlotsTab) updateProxyPicker(msg tea.Msg) (*SlotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.view = slotsViewList
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			t.switchProxySubscription(-1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			t.switchProxySubscription(1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.proxyList.MoveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.proxyList.MoveDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			nodes := t.nodesForProxyPicker()
			idx := t.proxyList.Selected()
			if idx >= 0 && idx < len(nodes) {
				node := nodes[idx]
				t.cfg.SetSlotNodeRef(t.activeSlot, config.SlotNodeRef{
					SubID:      t.proxySubID,
					NodeName:   node.Name,
					NodeServer: node.Server,
					NodePort:   node.Port,
					NodeType:   node.Type,
				})
				t.view = slotsViewList
				return t, t.saveAndMaybeRestart(t.activeSlot)
			}
		}
	}
	return t, nil
}

func (t *SlotsTab) updatePortPicker(msg tea.Msg) (*SlotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.view = slotsViewList
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.portChoiceIndex = (t.portChoiceIndex + 1) % 2
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.portChoiceIndex = (t.portChoiceIndex + 1) % 2
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if t.portChoiceIndex == 0 {
				t.cfg.SetSlotPortMode(t.activeSlot, "auto")
				t.view = slotsViewList
				return t, t.saveAndMaybeRestart(t.activeSlot)
			}
			t.prepareManualPortInput()
			t.view = slotsViewManualPortInput
		}
	}
	return t, nil
}

func (t *SlotsTab) updateManualPortInput(msg tea.Msg) (*SlotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			t.manualPortInput.Blur()
			t.view = slotsViewPortPicker
			return t, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			value := strings.TrimSpace(t.manualPortInput.Value())
			port, err := strconv.Atoi(value)
			if err != nil || port <= 0 || port >= 65536 {
				t.setMessage(invalidManualPortMessage(value), "error")
				return t, nil
			}
			t.cfg.SetSlotManualMixedPort(t.activeSlot, port)
			t.manualPortInput.Blur()
			t.view = slotsViewList
			return t, t.saveAndMaybeRestart(t.activeSlot)
		default:
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
				msg.Runes = filterDigits(msg.Runes)
				if len(msg.Runes) == 0 {
					return t, nil
				}
				if t.manualPortSelected {
					t.manualPortInput.SetValue("")
					t.manualPortInput.SetCursor(0)
					t.manualPortSelected = false
				}
			} else {
				t.manualPortSelected = false
			}
			var cmd tea.Cmd
			t.manualPortInput, cmd = t.manualPortInput.Update(msg)
			return t, cmd
		}
	}
	return t, nil
}

func (t *SlotsTab) updateUpstreamPicker(msg tea.Msg) (*SlotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			t.view = slotsViewList
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.upstreamList.MoveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.upstreamList.MoveDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			entries := t.cfg.ListUpstreamProxies()
			idx := t.upstreamList.Selected()
			upstreamID := ""
			if idx > 0 && idx-1 < len(entries) {
				upstreamID = entries[idx-1].ID
			}
			t.cfg.SetSlotUpstreamRef(t.activeSlot, upstreamID)
			t.view = slotsViewList
			return t, t.saveAndMaybeRestart(t.activeSlot)
		}
	}
	return t, nil
}

// View renders the slots tab.
func (t *SlotsTab) View() string {
	t.refreshTables()

	switch t.view {
	case slotsViewProxyPicker:
		return t.viewProxyPicker()
	case slotsViewPortPicker:
		return t.viewPortPicker()
	case slotsViewManualPortInput:
		return t.viewManualPortInput()
	case slotsViewUpstreamPicker:
		return t.viewUpstreamPicker()
	default:
		return t.viewList()
	}
}

func (t *SlotsTab) viewList() string {
	title := i18n.M().Slots.Title
	if title == "" {
		title = "Slots"
	}
	lines := []string{styles.Title.Render(title)}
	lines = append(lines, styles.HelpText.Render("Enter to configure, t to power on/off."))
	lines = append(lines, "")

	// Calculate exact height budget for card area.
	overhead := 3 // title + subtitle + blank above cards
	overhead += 2 // blank + help bar below cards
	if t.message != "" {
		overhead += 2 // blank + message
	}
	cardAreaLines := t.height - overhead
	if cardAreaLines < 1 {
		cardAreaLines = 1
	}

	cardRows, rowHeight := t.renderSlotCardRows()
	lines = append(lines, t.renderVisibleCardRows(cardRows, rowHeight, cardAreaLines)...)
	if t.message != "" {
		lines = append(lines, "")
		lines = append(lines, t.renderMessage())
	}

	help := styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" switch  ") +
		styles.HelpKey.Render("←→") + styles.HelpText.Render(" focus  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" edit  ") +
		styles.HelpKey.Render("t") + styles.HelpText.Render(" test  ") +
		styles.HelpKey.Render("Space") + styles.HelpText.Render(" on/off")
	lines = append(lines, "", help)
	return styles.ContentArea.Render(strings.Join(lines, "\n"))
}

func (t *SlotsTab) viewPortPicker() string {
	lines := []string{
		styles.Title.Render(slotLabel(i18n.M().Slots.SelectPort, "Select Port")),
		"",
	}
	options := []string{
		slotLabel(i18n.M().Slots.AutoPort, "Auto allocate"),
		slotLabel(i18n.M().Slots.ManualPort, "Manual"),
	}
	for i, option := range options {
		line := "  " + option
		if i == t.portChoiceIndex {
			line = styles.Primary.Render("▶ " + option)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, styles.HelpKey.Render("↑↓")+styles.HelpText.Render(" choose  ")+styles.HelpKey.Render("Enter")+styles.HelpText.Render(" select  ")+styles.HelpKey.Render("Esc")+styles.HelpText.Render(" back"))
	return styles.ContentArea.Render(strings.Join(lines, "\n"))
}

func (t *SlotsTab) viewManualPortInput() string {
	title := slotLabel(i18n.M().Slots.ManualPort, "Manual")
	prompt := slotLabel(i18n.M().Slots.ManualPortPrompt, "Enter manual mixed port:")
	lines := []string{
		styles.Title.Render(title),
		"",
		prompt,
		styles.InputFocused.Render(t.manualPortInput.View()),
	}
	if t.message != "" {
		lines = append(lines, "", t.renderMessage())
	}
	lines = append(lines, "", styles.HelpKey.Render("Enter")+styles.HelpText.Render(" save  ")+styles.HelpKey.Render("Esc")+styles.HelpText.Render(" back"))
	return styles.ContentArea.Render(strings.Join(lines, "\n"))
}

func (t *SlotsTab) viewProxyPicker() string {
	title := slotLabel(i18n.M().Slots.SelectProxy, "Select Proxy")
	lines := []string{styles.Title.Render(title)}
	subTabs := t.renderProxySubTabs()
	if subTabs != "" {
		lines = append(lines, subTabs)
	}
	if hint := i18n.M().Slots.ProxyPickerHint; hint != "" {
		lines = append(lines, styles.HelpText.Render(hint))
	}
	lines = append(lines, "", t.proxyList.View())
	if t.message != "" {
		lines = append(lines, "", t.renderMessage())
	}
	lines = append(lines, "", styles.HelpKey.Render("↑↓")+styles.HelpText.Render(" navigate  ")+styles.HelpKey.Render("←→")+styles.HelpText.Render(" sub  ")+styles.HelpKey.Render("Enter")+styles.HelpText.Render(" select  ")+styles.HelpKey.Render("Esc")+styles.HelpText.Render(" back"))
	return styles.ContentArea.Render(strings.Join(lines, "\n"))
}

func (t *SlotsTab) viewUpstreamPicker() string {
	title := slotLabel(i18n.M().Slots.SelectUpstream, "Select Upstream")
	lines := []string{styles.Title.Render(title)}
	if hint := i18n.M().Slots.UpstreamPickerHint; hint != "" {
		lines = append(lines, styles.HelpText.Render(hint))
	}
	lines = append(lines, "", t.upstreamList.View())
	if t.message != "" {
		lines = append(lines, "", t.renderMessage())
	}
	lines = append(lines, "", styles.HelpKey.Render("↑↓")+styles.HelpText.Render(" navigate  ")+styles.HelpKey.Render("Enter")+styles.HelpText.Render(" select  ")+styles.HelpKey.Render("Esc")+styles.HelpText.Render(" back"))
	return styles.ContentArea.Render(strings.Join(lines, "\n"))
}

func (t *SlotsTab) refreshTables() {
	t.refreshProxyRows()
	t.refreshUpstreamRows()
}

func (t *SlotsTab) refreshProxyRows() {
	nodes := t.nodesForProxyPicker()
	rows := make([][]string, 0, len(nodes))
	ref, _ := t.cfg.GetSlotNodeRef(t.activeSlot)
	for _, n := range nodes {
		indicator := ""
		if slotNodeMatchesRef(ref, &n) {
			indicator = "●"
		}
		rows = append(rows, []string{
			indicator,
			n.Name,
		})
	}
	t.proxyList.SetColumns(proxyTableColumns(t.width))
	height := t.height - 8
	if height < 3 {
		height = 3
	}
	t.proxyList.SetHeight(height)
	t.proxyList.SetRows(rows)
}

func (t *SlotsTab) refreshUpstreamRows() {
	rows := [][]string{{
		"",
		slotLabel(i18n.M().Slots.NoUpstream, "Direct"),
		"",
	}}
	selectedID, _ := t.cfg.GetSlotUpstreamRef(t.activeSlot)
	for _, entry := range t.cfg.ListUpstreamProxies() {
		indicator := ""
		if entry.ID == selectedID {
			indicator = "●"
		}
		label := entry.Label
		if label == "" {
			label = entry.ID
		}
		rows = append(rows, []string{
			indicator,
			label,
			fmt.Sprintf("%s://%s:%d", entry.Protocol, entry.Server, entry.Port),
		})
	}
	t.upstreamList.SetColumns(upstreamTableColumns(t.width))
	height := t.height - 7
	if height < 3 {
		height = 3
	}
	t.upstreamList.SetHeight(height)
	t.upstreamList.SetRows(rows)
}

func (t *SlotsTab) openProxyPicker() {
	ref, ok := t.cfg.GetSlotNodeRef(t.activeSlot)
	subs := t.sortedSubscriptions()
	if ok && ref.SubID != "" {
		t.proxySubID = ref.SubID
	} else if len(subs) > 0 {
		t.proxySubID = subs[0].ID
	}
	t.proxySubIndex = t.subscriptionIndex(t.proxySubID)
	t.view = slotsViewProxyPicker
	t.refreshProxyRows()

	selected := 0
	nodes := t.nodesForProxyPicker()
	for i := range nodes {
		if slotNodeMatchesRef(ref, &nodes[i]) {
			selected = i
			break
		}
	}
	t.proxyList.SetSelected(selected)
}

func (t *SlotsTab) prepareManualPortInput() {
	t.manualPortInput.Focus()
	t.manualPortSelected = true
	if port, ok := t.cfg.GetSlotManualMixedPort(t.activeSlot); ok {
		t.manualPortInput.SetValue(strconv.Itoa(port))
	} else {
		t.manualPortInput.SetValue("")
	}
	t.manualPortInput.SetCursor(len(t.manualPortInput.Value()))
}

func (t *SlotsTab) testSelectedSlotNode() tea.Cmd {
	slotID := t.selectedSlotID()
	ref, ok := t.cfg.GetSlotNodeRef(slotID)
	if !ok || t.nodeMgr == nil || t.subManager == nil {
		return nil
	}
	// Find the subscription.Node matching the slot's ref
	nodes := t.subManager.GetNodesForSubscription(ref.SubID)
	var target *subscription.Node
	for i := range nodes {
		if slotNodeMatchesRef(ref, &nodes[i]) {
			target = &nodes[i]
			break
		}
	}
	if target == nil {
		return nil
	}
	nodeCopy := *target
	return func() tea.Msg {
		t.nodeMgr.TestNode(context.Background(), &nodeCopy)
		return SlotTestDoneMsg{SlotID: slotID}
	}
}

// findSlotNode looks up the subscription.Node bound to a slot.
func (t *SlotsTab) findSlotNode(slotID int) *subscription.Node {
	ref, ok := t.cfg.GetSlotNodeRef(slotID)
	if !ok || t.subManager == nil {
		return nil
	}
	nodes := t.subManager.GetNodesForSubscription(ref.SubID)
	for i := range nodes {
		if slotNodeMatchesRef(ref, &nodes[i]) {
			return &nodes[i]
		}
	}
	return nil
}

func (t *SlotsTab) saveAndMaybeRestart(slotID int) tea.Cmd {
	if err := t.cfg.Save(); err != nil {
		t.setMessage(err.Error(), "error")
		return nil
	}
	if t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil {
		t.setMessage(formatSlotMessage(i18n.M().Slots.Restarting, slotID, fmt.Sprintf("Slot %d saved. Restarting...", slotID)), "success")
		return func() tea.Msg {
			return SlotRestartRequestMsg{SlotID: slotID}
		}
	}
	t.setMessage(formatSlotMessage(i18n.M().Slots.Saved, slotID, fmt.Sprintf("Slot %d saved.", slotID)), "success")
	return nil
}

func (t *SlotsTab) renderProxySubTabs() string {
	subs := t.sortedSubscriptions()
	if len(subs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(subs))
	for _, sub := range subs {
		label := sub.Name
		if subscription.IsCustomSubscription(sub) {
			label = "📌 " + label
		}
		if sub.ID == t.proxySubID {
			parts = append(parts, styles.ActiveTab.Render(label))
		} else {
			parts = append(parts, styles.InactiveTab.Render(label))
		}
	}
	return styles.TabBar.Width(t.width).Render(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
}

func (t *SlotsTab) sortedSubscriptions() []*subscription.Subscription {
	if t.subManager == nil {
		return nil
	}
	subs := t.subManager.List()
	sort.Slice(subs, func(i, j int) bool {
		if subs[i] == nil {
			return false
		}
		if subs[j] == nil {
			return true
		}
		if subs[i].Name == subs[j].Name {
			return subs[i].ID < subs[j].ID
		}
		return subs[i].Name < subs[j].Name
	})
	out := make([]*subscription.Subscription, 0, len(subs))
	for _, sub := range subs {
		if sub != nil {
			out = append(out, sub)
		}
	}
	return out
}

func (t *SlotsTab) subscriptionIndex(subID string) int {
	subs := t.sortedSubscriptions()
	for i, sub := range subs {
		if sub != nil && sub.ID == subID {
			return i
		}
	}
	return 0
}

func (t *SlotsTab) switchProxySubscription(delta int) {
	subs := t.sortedSubscriptions()
	if len(subs) == 0 {
		t.proxySubID = ""
		return
	}
	idx := t.subscriptionIndex(t.proxySubID)
	idx = (idx + delta + len(subs)) % len(subs)
	t.proxySubIndex = idx
	t.proxySubID = subs[idx].ID
	t.refreshProxyRows()
	t.proxyList.SetSelected(0)
}

func (t *SlotsTab) nodesForProxyPicker() []subscription.Node {
	if t.subManager == nil || strings.TrimSpace(t.proxySubID) == "" {
		return nil
	}
	return t.subManager.GetNodesForSubscription(t.proxySubID)
}

func (t *SlotsTab) selectedSlotID() int {
	if t.slotListIndex < 0 {
		return 0
	}
	if t.slotListIndex >= proxy.MaxSlots {
		t.slotListIndex = proxy.MaxSlots - 1
	}
	return t.slotListIndex
}

func (t *SlotsTab) slotRuntimeSummary(slotID int) string {
	if t.slotMgr != nil {
		if state := t.slotMgr.GetSlotState(slotID); state != nil {
			base := fmt.Sprintf("Running %s at :%d", slotModeTitle(state.Mode), state.Port)
			if state.UpstreamEnabled {
				return fmt.Sprintf("%s via %s://%s:%d", base, state.UpstreamProtocol, state.UpstreamServer, state.UpstreamPort)
			}
			return base
		}
	}
	ready, reason := t.slotReady(slotID)
	if ready {
		return reason
	}
	return reason
}

func (t *SlotsTab) slotPowerSummary(slotID int) string {
	if t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil {
		return slotLabel(i18n.M().Slots.Running, "Running")
	}
	if ready, _ := t.slotReady(slotID); ready {
		return slotLabel(i18n.M().Slots.Ready, "Ready")
	}
	_, reason := t.slotReady(slotID)
	return reason
}

func (t *SlotsTab) slotProxySummary(slotID int) string {
	ref, ok := t.cfg.GetSlotNodeRef(slotID)
	if !ok {
		return slotLabel(i18n.M().Slots.NoProxy, "Not set")
	}
	subName := ref.SubID
	if t.subManager != nil {
		if sub, ok := t.subManager.Get(ref.SubID); ok && sub != nil && strings.TrimSpace(sub.Name) != "" {
			subName = sub.Name
		}
	}
	if ref.NodeName != "" {
		return fmt.Sprintf("%s @ %s", ref.NodeName, subName)
	}
	if ref.NodeServer != "" && ref.NodePort > 0 {
		return fmt.Sprintf("%s:%d @ %s", ref.NodeServer, ref.NodePort, subName)
	}
	return slotLabel(i18n.M().Slots.NoProxy, "Not set")
}

func (t *SlotsTab) slotPortSummary(slotID int) string {
	port, manual, ok := t.cfg.SlotMixedPort(slotID)
	if manual {
		if !ok {
			return slotLabel(i18n.M().Slots.InvalidPort, "Invalid port")
		}
		return fmt.Sprintf("%s :%d", slotLabel(i18n.M().Slots.ManualPort, "Manual"), port)
	}
	return fmt.Sprintf("%s :%d", slotLabel(i18n.M().Slots.AutoPort, "Auto allocate"), port)
}

func (t *SlotsTab) slotUpstreamSummary(slotID int) string {
	ref, ok := t.cfg.GetSlotUpstreamRef(slotID)
	if !ok || ref == "" {
		return slotLabel(i18n.M().Slots.NoUpstream, "Direct")
	}
	entry, found := t.cfg.GetUpstreamProxy(ref)
	if !found {
		return fmt.Sprintf("Missing %s", ref)
	}
	if entry.Label != "" {
		return entry.Label
	}
	return entry.ID
}

func (t *SlotsTab) slotReady(slotID int) (bool, string) {
	ref, ok := t.cfg.GetSlotNodeRef(slotID)
	if !ok || strings.TrimSpace(ref.SubID) == "" {
		return false, slotLabel(i18n.M().Slots.MissingProxy, "Missing proxy")
	}
	if t.subManager == nil {
		return false, slotLabel(i18n.M().Slots.MissingProxy, "Missing proxy")
	}
	nodes := t.subManager.GetNodesForSubscription(ref.SubID)
	found := false
	for i := range nodes {
		if slotNodeMatchesRef(ref, &nodes[i]) {
			found = true
			break
		}
	}
	if !found {
		return false, slotLabel(i18n.M().Slots.MissingProxy, "Missing proxy")
	}
	if _, _, ok := t.cfg.SlotMixedPort(slotID); !ok {
		return false, slotLabel(i18n.M().Slots.InvalidPort, "Invalid port")
	}
	return true, "Configured and ready to start."
}

func (t *SlotsTab) renderMessage() string {
	if t.messageType == "error" {
		return styles.ErrorMessage.Render(t.message)
	}
	return styles.SuccessMessage.Render(t.message)
}

func (t *SlotsTab) setMessage(message string, messageType string) {
	t.message = message
	t.messageType = messageType
}

func slotNodeMatchesRef(ref config.SlotNodeRef, node *subscription.Node) bool {
	if node == nil {
		return false
	}
	if ref.NodeServer != "" && ref.NodePort > 0 {
		return ref.NodeServer == node.Server && ref.NodePort == node.Port
	}
	if ref.NodeName != "" {
		return ref.NodeName == node.Name
	}
	return false
}

func slotModeTitle(mode string) string {
	if strings.EqualFold(mode, "global") {
		return "Global"
	}
	return "Rule"
}

func slotLabel(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func invalidManualPortMessage(value string) string {
	if value == "" {
		return "invalid manual port"
	}
	return fmt.Sprintf("invalid manual port: %q", value)
}

func formatSlotMessage(format string, slotID int, fallback string) string {
	if strings.TrimSpace(format) == "" {
		return fallback
	}
	return fmt.Sprintf(format, slotID)
}

func proxyTableColumns(width int) []components.Column {
	if width < 40 {
		width = 40
	}
	nameW := width - 4
	if nameW < 16 {
		nameW = 16
	}
	return []components.Column{
		{Title: "", Width: 4},
		{Title: "Name", Width: nameW},
	}
}

func upstreamTableColumns(width int) []components.Column {
	if width < 60 {
		width = 60
	}
	detailW := 28
	labelW := width - 4 - detailW
	if labelW < 16 {
		labelW = 16
	}
	return []components.Column{
		{Title: "", Width: 4},
		{Title: "Label", Width: labelW},
		{Title: "Detail", Width: detailW},
	}
}

func (t *SlotsTab) moveSlotSelection(dx, dy int) {
	cols := t.slotGridColumns()
	if cols <= 0 {
		cols = 1
	}

	current := t.selectedSlotID()
	row := current / cols
	col := current % cols
	row += dy
	col += dx
	if col < 0 || col >= cols || row < 0 {
		return
	}

	target := row*cols + col
	if target < 0 || target >= proxy.MaxSlots {
		return
	}
	t.slotListIndex = target
}

func (t *SlotsTab) slotGridColumns() int {
	switch {
	case t.width >= 186:
		return 3
	case t.width >= 112:
		return 2
	default:
		return 1
	}
}

func (t *SlotsTab) renderSlotCardRows() ([]string, int) {
	cols := t.slotGridColumns()
	if cols <= 0 {
		cols = 1
	}

	const indicatorWidth = 2 // "▶ " or "  "
	gap := 2
	availableWidth := t.width - cols*indicatorWidth - (cols-1)*gap
	if availableWidth < cols*36 {
		availableWidth = cols * 36
	}
	cardWidth := availableWidth / cols
	if cardWidth < 36 {
		cardWidth = 36
	}

	type cardWithIndicator struct {
		indicator string
		card      string
	}
	items := make([]cardWithIndicator, 0, proxy.MaxSlots)
	for slotID := 0; slotID < proxy.MaxSlots; slotID++ {
		ind := "  "
		if slotID == t.selectedSlotID() {
			ind = styles.Primary.Render("▶ ")
		}
		items = append(items, cardWithIndicator{
			indicator: ind,
			card:      t.renderSlotCard(slotID, cardWidth),
		})
	}

	rows := make([]string, 0, (len(items)+cols-1)/cols)
	for i := 0; i < len(items); i += cols {
		end := i + cols
		if end > len(items) {
			end = len(items)
		}
		var rowParts []string
		for j := i; j < end; j++ {
			// Vertically center the indicator with the card (pad top line for border)
			indBlock := lipgloss.NewStyle().Width(indicatorWidth).Height(4).PaddingTop(1).Render(items[j].indicator)
			rowParts = append(rowParts, indBlock, items[j].card)
		}
		// Fill remaining columns with blanks
		for len(rowParts) < cols*2 {
			rowParts = append(rowParts,
				lipgloss.NewStyle().Width(indicatorWidth).Render(""),
				lipgloss.NewStyle().Width(cardWidth).Render(""))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, rowParts...))
	}

	rowHeight := 0
	if len(rows) > 0 {
		rowHeight = len(strings.Split(rows[0], "\n"))
	}
	return rows, rowHeight
}

func (t *SlotsTab) renderSlotCard(slotID int, cardWidth int) string {
	isSelected := slotID == t.selectedSlotID()
	running := t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil
	ready, _ := t.slotReady(slotID)
	inFocus := slotID == t.selectedSlotID()

	statusLabel := t.slotPowerSummary(slotID)
	statusStyle := styles.Muted
	switch {
	case running:
		statusStyle = styles.StatusConnected
	case ready:
		statusStyle = styles.Accent
	default:
		statusStyle = styles.StatusError
	}

	innerWidth := cardWidth - 4 // minus border + padding
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Line 1 (header): Slot N              ● STATUS
	titleLeft := styles.Title.UnsetPadding().Render(fmt.Sprintf("Slot %d", slotID))
	titleRight := statusStyle.Render(strings.ToUpper(statusLabel))
	leftWidth := innerWidth - lipgloss.Width(titleRight)
	if leftWidth < 6 {
		leftWidth = 6
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(titleLeft),
		titleRight,
	)

	// Line 2 (flow): :PORT ─▸ Rule ─▸ ProxyNode ─▸ Upstream
	port := t.slotPortSummaryShort(slotID)
	mode := slotModeTitle(t.cfg.GetSlotStartMode(slotID))
	proxyName := t.slotProxySummaryShort(slotID)
	upstream := t.slotUpstreamSummary(slotID)
	flowLine := t.renderFlowLine(port, mode, proxyName, upstream, innerWidth, inFocus)

	cardBody := lipgloss.JoinVertical(lipgloss.Left, header, flowLine)

	cardStyle := lipgloss.NewStyle().
		Width(cardWidth).
		Height(2).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.MutedColor)

	_ = isSelected // selection indicated by ▶ prefix
	if running {
		cardStyle = cardStyle.Background(lipgloss.Color("#24343A"))
	} else if ready {
		cardStyle = cardStyle.Background(lipgloss.Color("#302A18"))
	}

	return cardStyle.Render(cardBody)
}

func (t *SlotsTab) slotProxySummaryShort(slotID int) string {
	ref, ok := t.cfg.GetSlotNodeRef(slotID)
	if !ok {
		return "---"
	}
	if ref.NodeName != "" {
		return ref.NodeName
	}
	if ref.NodeServer != "" && ref.NodePort > 0 {
		return fmt.Sprintf("%s:%d", ref.NodeServer, ref.NodePort)
	}
	return "---"
}

func (t *SlotsTab) slotPortSummaryShort(slotID int) string {
	port, _, ok := t.cfg.SlotMixedPort(slotID)
	if !ok {
		return "---"
	}
	return fmt.Sprintf(":%d", port)
}

func (t *SlotsTab) renderFlowLine(port, mode, proxyName, upstream string, width int, inFocus bool) string {
	arrow := styles.Muted.Render(" ─▸ ")
	arrowWidth := 4 // " ─▸ " is 4 chars
	numArrows := 3  // port ─▸ mode ─▸ proxy ─▸ upstream

	// Calculate available width for proxy name (the flexible part)
	fixedWidth := len([]rune(port)) + len([]rune(mode)) + arrowWidth*numArrows + len([]rune(upstream))
	proxyMaxWidth := width - fixedWidth
	if proxyMaxWidth < 4 {
		proxyMaxWidth = 4
	}
	clippedProxy := clipText(proxyName, proxyMaxWidth)

	// Style each segment: focused = Primary (bold pink), else default
	highlight := func(text string, focus int) string {
		if inFocus && t.detailCursor == focus {
			return styles.Primary.Render(text)
		}
		return text
	}
	portText := highlight(port, cardFocusPort)
	modeText := highlight(mode, cardFocusMode)
	if !(inFocus && t.detailCursor == cardFocusMode) {
		modeText = styles.Accent.Render(mode)
	}
	proxyText := highlight(clippedProxy, cardFocusProxy)
	upstreamText := highlight(upstream, cardFocusUpstream)

	return portText + arrow + modeText + arrow + proxyText + arrow + upstreamText
}

func clipText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func (t *SlotsTab) renderVisibleCardRows(rows []string, rowHeight int, cardAreaLines int) []string {
	if len(rows) == 0 {
		return []string{styles.Muted.Render("No slots")}
	}

	if cardAreaLines <= 0 || rowHeight <= 0 {
		return []string{strings.Join(rows, "\n")}
	}

	// Check if we need a scroll indicator
	totalVisibleWithoutScroll := cardAreaLines / rowHeight
	needsScroll := len(rows) > totalVisibleWithoutScroll

	// Reserve 1 line for scroll indicator if needed
	actualCardLines := cardAreaLines
	if needsScroll {
		actualCardLines = cardAreaLines - 1
	}
	visibleRows := actualCardLines / rowHeight
	if visibleRows < 1 {
		visibleRows = 1
	}

	selectedRow := t.selectedSlotID() / t.slotGridColumns()
	if selectedRow < t.slotRowOffset {
		t.slotRowOffset = selectedRow
	}
	if selectedRow >= t.slotRowOffset+visibleRows {
		t.slotRowOffset = selectedRow - visibleRows + 1
	}
	maxOffset := len(rows) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.slotRowOffset > maxOffset {
		t.slotRowOffset = maxOffset
	}
	if t.slotRowOffset < 0 {
		t.slotRowOffset = 0
	}

	startLine := t.slotRowOffset * rowHeight
	endLine := startLine + visibleRows*rowHeight
	bodyLines := strings.Split(strings.Join(rows, "\n"), "\n")
	if startLine > len(bodyLines) {
		startLine = len(bodyLines)
	}
	if endLine > len(bodyLines) {
		endLine = len(bodyLines)
	}

	visible := []string{strings.Join(bodyLines[startLine:endLine], "\n")}
	if needsScroll {
		scrollText := fmt.Sprintf("[rows %d-%d/%d]", t.slotRowOffset+1, minInt(t.slotRowOffset+visibleRows, len(rows)), len(rows))
		visible = append(visible, lipgloss.NewStyle().
			Foreground(styles.MutedColor).
			Width(t.width).
			Align(lipgloss.Right).
			Render(scrollText))
	}
	return visible
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
