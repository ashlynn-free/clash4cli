package tabs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// SlotTrafficInfo holds computed per-slot traffic data for display.
type SlotTrafficInfo struct {
	UpRate    int64
	DownRate  int64
	ConnCount int
}

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
	slotRegions        [proxy.MaxSlots]string
	slotLatencies      [proxy.MaxSlots]int64
	slotTraffic        [proxy.MaxSlots]*SlotTrafficInfo
	lastTestTime       time.Time
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

// SlotTestResultMsg reports the combined latency + region result for a slot test.
type SlotTestResultMsg struct {
	SlotID  int
	Latency int64
	Region  string
	Err     error
}

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

// SetSlotTraffic updates the live traffic info for a slot.
func (t *SlotsTab) SetSlotTraffic(slotID int, info *SlotTrafficInfo) {
	if slotID >= 0 && slotID < proxy.MaxSlots {
		t.slotTraffic[slotID] = info
	}
}

// ClearSlotTraffic removes traffic info for a slot.
func (t *SlotsTab) ClearSlotTraffic(slotID int) {
	if slotID >= 0 && slotID < proxy.MaxSlots {
		t.slotTraffic[slotID] = nil
	}
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
	case SlotTestResultMsg:
		if msg.SlotID >= 0 && msg.SlotID < proxy.MaxSlots && msg.Err == nil {
			t.slotRegions[msg.SlotID] = msg.Region
			t.slotLatencies[msg.SlotID] = msg.Latency
			t.cfg.SetSlotLastLatency(msg.SlotID, msg.Latency)
			t.cfg.SetSlotLastRegion(msg.SlotID, msg.Region)
			_ = t.cfg.Save()
		}
		return t, nil
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
			} else {
				t.slotListIndex = proxy.MaxSlots - 1
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if t.slotListIndex < proxy.MaxSlots-1 {
				t.slotListIndex++
			} else {
				t.slotListIndex = 0
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
			if time.Since(t.lastTestTime) < 2*time.Second {
				return t, nil
			}
			t.lastTestTime = time.Now()
			return t, t.testAllSlotNodes()
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
	desc := i18n.M().Slots.Description
	if desc == "" {
		desc = "Each Slot is an independent proxy instance with its own port, mode, and node. This software is Slot-driven."
	}
	lines = append(lines, styles.HelpText.Render(desc))
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
		styles.HelpKey.Render("t") + styles.HelpText.Render(" test all  ") +
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
			fmt.Sprintf("%s://%s:%d", n.Type, n.Server, n.Port),
		})
	}
	t.proxyList.SetColumns(proxyTableColumns(t.width))
	// Reserve lines for viewProxyPicker non-table elements:
	// title(1) + hint(1) + empty(1) + table header(1) + separator(1)
	// + scroll indicator(1) + empty(1) + help(1) = 8
	reserved := 8
	if len(t.sortedSubscriptions()) > 0 {
		reserved += 2 // sub-tabs line + TabBar border
	}
	if t.message != "" {
		reserved += 2 // empty line + message
	}
	height := t.height - reserved
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


func (t *SlotsTab) testAllSlotNodes() tea.Cmd {
	var cmds []tea.Cmd
	for slotID := 0; slotID < proxy.MaxSlots; slotID++ {
		running := t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil
		ready, _ := t.slotReady(slotID)
		if !running && !ready {
			continue
		}
		sid := slotID
		cfg := t.cfg
		subMgr := t.subManager
		slotMgr := t.slotMgr
		cmds = append(cmds, func() tea.Msg {
			result := proxy.SlotTest(cfg, subMgr, slotMgr, sid, 5*time.Second)
			return SlotTestResultMsg{
				SlotID:  sid,
				Latency: result.Latency,
				Region:  result.Region,
				Err:     result.Err,
			}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (t *SlotsTab) saveAndMaybeRestart(slotID int) tea.Cmd {
	if err := t.cfg.Save(); err != nil {
		t.setMessage(err.Error(), "error")
		return nil
	}
	if t.slotMgr != nil && t.slotMgr.GetSlotState(slotID) != nil {
		return func() tea.Msg {
			return SlotRestartRequestMsg{SlotID: slotID}
		}
	}
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
	if width < 60 {
		width = 60
	}
	detailW := 28
	nameW := width - 4 - detailW
	if nameW < 16 {
		nameW = 16
	}
	return []components.Column{
		{Title: "", Width: 4},
		{Title: "Name", Width: nameW},
		{Title: "Detail", Width: detailW},
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
	return 1
}

func (t *SlotsTab) renderSlotCardRows() ([]string, int) {
	cols := t.slotGridColumns()
	if cols <= 0 {
		cols = 1
	}

	gap := 2
	availableWidth := t.width - (cols-1)*gap
	if availableWidth < cols*36 {
		availableWidth = cols * 36
	}
	cardWidth := availableWidth / cols
	if cardWidth < 36 {
		cardWidth = 36
	}

	items := make([]string, 0, proxy.MaxSlots)
	for slotID := 0; slotID < proxy.MaxSlots; slotID++ {
		selected := slotID == t.selectedSlotID()
		items = append(items, t.renderSlotCard(slotID, cardWidth, selected))
	}

	rows := make([]string, 0, (len(items)+cols-1)/cols)
	for i := 0; i < len(items); i += cols {
		end := i + cols
		if end > len(items) {
			end = len(items)
		}
		var rowParts []string
		for j := i; j < end; j++ {
			rowParts = append(rowParts, items[j])
		}
		// Fill remaining columns with blanks
		for len(rowParts) < cols {
			rowParts = append(rowParts,
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

func (t *SlotsTab) renderSlotCard(slotID int, cardWidth int, selected bool) string {
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

	const indicatorPad = 3 // "▶  " or "   " — 3 chars
	indicator := "   "
	if selected {
		indicator = styles.Primary.Render("▶") + "  "
	}
	indentPad := strings.Repeat(" ", indicatorPad)

	innerWidth := cardWidth - 4 - indicatorPad // border(2) + padding(2) + indicator(3)
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Line 1 (header): Slot N  [traffic info]  STATUS
	slotTitle := fmt.Sprintf("Slot %d", slotID)
	if slotID == 0 {
		slotTitle += " (" + slotLabel(i18n.M().Slots.MainSlotSuffix, "Main") + ")"
	}
	titleLeft := styles.Title.UnsetPadding().Render(slotTitle)
	statusText := statusStyle.Render(strings.ToUpper(statusLabel))

	// Build traffic info for running slots
	var trafficStr string
	if running {
		if info := t.slotTraffic[slotID]; info != nil {
			unit := ""
			if t.cfg != nil {
				unit = t.cfg.UI.SpeedUnit
			}
			trafficStr = fmt.Sprintf("↑%s ↓%s ⇌%d",
				components.FormatRateWithUnit(info.UpRate, unit),
				components.FormatRateWithUnit(info.DownRate, unit),
				info.ConnCount,
			)
		}
	}

	var titleRight string
	if trafficStr != "" {
		rendered := styles.Muted.Render(trafficStr)
		// Only show traffic if there's enough room
		if lipgloss.Width(rendered)+lipgloss.Width(statusText)+8 <= innerWidth {
			titleRight = rendered + " " + statusText
		} else {
			titleRight = statusText
		}
	} else {
		titleRight = statusText
	}

	leftWidth := innerWidth - lipgloss.Width(titleRight)
	if leftWidth < 6 {
		leftWidth = 6
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(titleLeft),
		titleRight,
	)

	// Line 2: flow chain on the left, region + latency on the right
	// Build right-side info: "region latency"
	regionText := t.slotRegions[slotID]
	if regionText == "" {
		regionText = t.cfg.GetSlotLastRegion(slotID)
	}
	latencyText := t.slotLatencyText(slotID)
	var rightInfo string
	switch {
	case regionText != "" && latencyText != "":
		rightInfo = styles.Muted.Render(regionText) + " " + latencyText
	case latencyText != "":
		rightInfo = latencyText
	case regionText != "":
		rightInfo = styles.Muted.Render(regionText)
	}
	rightWidth := lipgloss.Width(rightInfo)

	// Flow line gets the remaining width
	flowWidth := innerWidth
	if rightWidth > 0 {
		flowWidth = innerWidth - rightWidth - 2 // 2 for gap
	}
	if flowWidth < 20 {
		flowWidth = 20
	}

	port := t.slotPortSummaryShort(slotID)
	mode := slotModeTitle(t.cfg.GetSlotStartMode(slotID))
	proxyName := t.slotProxySummaryShort(slotID)
	upstream := t.slotUpstreamSummary(slotID)
	flowLine := t.renderFlowLine(port, mode, proxyName, upstream, flowWidth, inFocus)

	var infoLine string
	if rightWidth > 0 {
		// Truncate by display width first so lipgloss Width() only pads, never wraps.
		clipped := ansi.Truncate(flowLine, innerWidth-rightWidth, "")
		flowPart := lipgloss.NewStyle().Width(innerWidth - rightWidth).Render(clipped)
		infoLine = flowPart + rightInfo
	} else {
		infoLine = ansi.Truncate(flowLine, innerWidth, "")
	}

	cardBody := lipgloss.JoinVertical(lipgloss.Left,
		indicator+header,
		indentPad+infoLine,
	)

	cardStyle := lipgloss.NewStyle().
		Width(cardWidth).
		Height(2).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.MutedColor)

	return cardStyle.Render(cardBody)
}

func (t *SlotsTab) slotLatencyText(slotID int) string {
	// Use in-memory result from SlotTest
	if slotID >= 0 && slotID < proxy.MaxSlots && t.slotLatencies[slotID] != 0 {
		return styles.FormatLatency(t.slotLatencies[slotID])
	}
	// Fallback to persisted cache
	if cached := t.cfg.GetSlotLastLatency(slotID); cached != 0 {
		return styles.FormatLatency(cached)
	}
	return ""
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

	const minProxyRW = 4
	portRW := len([]rune(port))
	modeRW := len([]rune(mode))

	// baseFixed: space used by port + mode + all arrows + minimum proxy, without upstream
	baseFixed := portRW + modeRW + arrowWidth*numArrows + minProxyRW

	// Clip upstream if the total exceeds width
	clippedUpstream := upstream
	if baseFixed+len([]rune(upstream)) > width {
		budget := width - baseFixed
		if budget <= 0 {
			clippedUpstream = ""
		} else {
			clippedUpstream = clipText(upstream, budget)
		}
	}

	// Recalculate proxy max width with the (possibly clipped) upstream
	fixedWidth := portRW + modeRW + arrowWidth*numArrows + len([]rune(clippedUpstream))
	proxyMaxWidth := width - fixedWidth
	if proxyMaxWidth < minProxyRW {
		proxyMaxWidth = minProxyRW
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

	if clippedUpstream == "" {
		return ansi.Truncate(portText+arrow+modeText+arrow+proxyText, width, "")
	}
	upstreamText := highlight(clippedUpstream, cardFocusUpstream)
	return ansi.Truncate(portText+arrow+modeText+arrow+proxyText+arrow+upstreamText, width, "")
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
