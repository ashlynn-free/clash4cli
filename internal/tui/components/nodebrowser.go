package components

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/styles"
)

// BrowserMode controls whether the browser is for browsing or picking a node.
type BrowserMode int

const (
	BrowseMode BrowserMode = iota
	PickMode
)

// NodeBrowserEvent is returned from Update to signal state changes to the caller.
type NodeBrowserEvent int

const (
	EventNone     NodeBrowserEvent = iota
	EventSelected                  // a node was picked (pick mode only)
	EventClosed                    // user pressed Esc/Backspace
)

// NodeBrowserSelectedResult carries the selected node when EventSelected is returned.
type NodeBrowserSelectedResult struct {
	Node  subscription.Node
	SubID string
}

// SlotIndicator describes a proxy slot bound to a node.
type SlotIndicator struct {
	SlotID int
	Icon   string
}

type scrollState struct {
	selected int
	offset   int
}

// NodeBrowser is a reusable component that displays nodes from all subscriptions
// with full features: testing, detail view, sub-tab switching. It operates in two
// modes — browse (viewing/testing) and pick (selecting a node for a caller).
type NodeBrowser struct {
	cfg     *config.Config
	nodeMgr *node.Manager
	subMgr  *subscription.Manager

	table  *Table
	nodes  []subscription.Node
	width  int
	height int

	mode           BrowserMode
	subscriptionID string
	subTabIndex    int
	active         bool
	title          string
	allowNoProxy   bool

	testing    bool
	cancelTest context.CancelFunc
	resultCh   <-chan node.TestResult

	scrollStates    map[string]scrollState
	slotIndicators  map[string]SlotIndicator
	slotTraffic     map[string]string
	serverDisplay   string
	preselectName   string
	preselectServer string
	preselectPort   int

	showDetail bool
	detailView *NodeDetailView
}

// NewNodeBrowser creates a new NodeBrowser component.
func NewNodeBrowser(cfg *config.Config, nodeMgr *node.Manager, subMgr *subscription.Manager) *NodeBrowser {
	return &NodeBrowser{
		cfg:            cfg,
		nodeMgr:        nodeMgr,
		subMgr:         subMgr,
		table:          NewTable(nil),
		serverDisplay:  "show",
		scrollStates:   make(map[string]scrollState),
		slotIndicators: make(map[string]SlotIndicator),
		slotTraffic:    make(map[string]string),
	}
}

// Open activates the browser for the given subscription and mode.
// If preselect is non-nil, the cursor starts on that node.
func (b *NodeBrowser) Open(subID string, preselect *subscription.Node, mode BrowserMode) {
	b.mode = mode
	b.active = true
	b.showDetail = false
	b.detailView = nil
	b.testing = false

	if b.cfg != nil {
		b.serverDisplay = b.cfg.UI.ServerDisplay
	}

	// Save scroll state of previous subscription.
	if b.subscriptionID != "" {
		b.scrollStates[b.subscriptionID] = scrollState{
			selected: b.table.Selected(),
			offset:   b.table.Offset(),
		}
	}

	b.subscriptionID = subID
	if preselect != nil {
		b.preselectName = preselect.Name
		b.preselectServer = preselect.Server
		b.preselectPort = preselect.Port
	} else {
		b.preselectName = ""
		b.preselectServer = ""
		b.preselectPort = 0
	}

	// Set sub-tab index for the given subscription.
	if subID != "" {
		subs := b.sortedSubscriptions()
		for i, sub := range subs {
			if sub != nil && sub.ID == subID {
				b.subTabIndex = i
				break
			}
		}
	}

	// Restore scroll state.
	if ss, ok := b.scrollStates[subID]; ok {
		b.table.SetSelected(ss.selected)
		b.table.SetOffset(ss.offset)
	} else {
		b.table.SetSelected(0)
		b.table.SetOffset(0)
	}

	b.refreshNodes()
}

// Close deactivates the browser.
func (b *NodeBrowser) Close() {
	b.active = false
	b.showDetail = false
	b.detailView = nil
}

// IsActive reports whether the browser is currently showing.
func (b *NodeBrowser) IsActive() bool {
	return b.active
}

// BlocksGlobalKeys reports whether the browser should consume global keys.
func (b *NodeBrowser) BlocksGlobalKeys() bool {
	return b.active
}

// SetSize sets the available dimensions.
func (b *NodeBrowser) SetSize(width, height int) {
	b.width = width
	b.height = height
	if b.detailView != nil {
		b.detailView.SetSize(width, height)
	}
	if b.width > 0 {
		b.updateTableSize()
	}
}

// SetSlotIndicators replaces all slot indicators.
func (b *NodeBrowser) SetSlotIndicators(indicators map[string]SlotIndicator) {
	b.slotIndicators = indicators
	b.updateTableRows()
}

// SetSlotTraffic sets per-node traffic strings for active slots.
func (b *NodeBrowser) SetSlotTraffic(traffic map[string]string) {
	b.slotTraffic = traffic
	b.updateTableRows()
}

// SetTitle sets an optional title rendered above the browser.
func (b *NodeBrowser) SetTitle(title string) {
	b.title = title
}

// SetAllowNoProxy controls whether pick mode exposes a synthetic DIRECT row.
func (b *NodeBrowser) SetAllowNoProxy(enabled bool) {
	b.allowNoProxy = enabled
}

// SetSubscriptionID changes the active subscription and refreshes nodes.
func (b *NodeBrowser) SetSubscriptionID(subID string) {
	if b.subscriptionID == subID {
		return
	}
	if b.subscriptionID != "" {
		b.scrollStates[b.subscriptionID] = scrollState{
			selected: b.table.Selected(),
			offset:   b.table.Offset(),
		}
	}
	b.subscriptionID = subID
	if ss, ok := b.scrollStates[subID]; ok {
		b.table.SetSelected(ss.selected)
		b.table.SetOffset(ss.offset)
	} else {
		b.table.SetSelected(0)
		b.table.SetOffset(0)
	}
}

// SetServerDisplay updates how server addresses are shown.
func (b *NodeBrowser) SetServerDisplay(mode string) {
	switch mode {
	case "show", "blur", "off":
		b.serverDisplay = mode
	default:
		b.serverDisplay = "show"
	}
	if b.width > 0 {
		b.table.SetColumns(calcNodeTableColumns(b.width, b.serverDisplay))
	}
	b.updateTableRows()
}

// RestoreCursor re-applies the preselect position.
func (b *NodeBrowser) RestoreCursor() {
	b.applyPreselect()
}

// Update handles a Tea message. Returns the event and any command.
func (b *NodeBrowser) Update(msg tea.Msg) (NodeBrowserEvent, *NodeBrowserSelectedResult, tea.Cmd) {
	if !b.active {
		return EventNone, nil, nil
	}

	// Forward to detail view when active.
	if b.showDetail && b.detailView != nil {
		switch msg.(type) {
		case NodeDetailCloseMsg:
			b.showDetail = false
			b.detailView = nil
			return EventNone, nil, nil
		}
		var cmd tea.Cmd
		b.detailView, cmd = b.detailView.Update(msg)
		return EventNone, nil, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			b.table.MoveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			b.table.MoveDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			b.switchSubscription(-1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			b.switchSubscription(1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if b.mode == PickMode {
				if b.allowNoProxy && b.table.Selected() == 0 {
					return EventSelected, &NodeBrowserSelectedResult{Node: subscription.Node{}, SubID: b.subscriptionID}, nil
				}
				if sel := b.getSelectedNode(); sel != nil {
					return EventSelected, &NodeBrowserSelectedResult{Node: *sel, SubID: b.subscriptionID}, nil
				}
			}
			// Browse mode: enter does nothing (use i for detail).
		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			return EventNone, nil, b.testAllNodes()
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			return EventNone, nil, b.refreshNodesCmd()
		case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
			return EventNone, nil, b.openDetailView()
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			return EventClosed, nil, nil
		}

	case nodeTestResultMsg:
		b.updateNodeResult(msg.Result)
		return EventNone, nil, waitNodeResult(b.resultCh)

	case nodeTestCompleteMsg:
		b.testing = false
		b.resultCh = nil
		return EventNone, nil, nil

	case nodeRefreshedMsg:
		b.nodes = msg.Nodes
		b.updateTableRows()
		b.applyPreselect()
	}

	return EventNone, nil, nil
}

// View renders the browser.
func (b *NodeBrowser) View() string {
	if !b.active {
		return ""
	}

	if b.showDetail && b.detailView != nil {
		return b.detailView.View()
	}

	m := i18n.M()

	// Sub-tabs.
	subTabBar := b.renderSubTabs()

	if len(b.nodes) == 0 {
		parts := []string{}
		if b.title != "" {
			parts = append(parts, styles.TitleBar.Render(b.title))
		}
		if subTabBar != "" {
			parts = append(parts, subTabBar)
		}
		parts = append(parts, styles.Muted.Render(m.Nodes.NoNodes))
		return strings.Join(parts, "\n")
	}

	tableView := b.table.View()

	// Help bar depends on mode.
	var help string
	if b.mode == PickMode {
		help = styles.HelpKey.Render("↑/↓") + styles.HelpText.Render(" navigate  ") +
			styles.HelpKey.Render("←/→") + styles.HelpText.Render(" switch sub  ") +
			styles.HelpKey.Render("Enter") + styles.HelpText.Render(" select  ") +
			styles.HelpKey.Render("t") + styles.HelpText.Render(" test  ") +
			styles.HelpKey.Render("r") + styles.HelpText.Render(" refresh  ") +
			styles.HelpKey.Render("i") + styles.HelpText.Render(" detail  ") +
			styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")
	} else {
		help = styles.HelpKey.Render("↑/↓") + styles.HelpText.Render(" navigate  ") +
			styles.HelpKey.Render("←/→") + styles.HelpText.Render(" switch sub  ") +
			styles.HelpKey.Render("t") + styles.HelpText.Render(" test  ") +
			styles.HelpKey.Render("r") + styles.HelpText.Render(" refresh  ") +
			styles.HelpKey.Render("i") + styles.HelpText.Render(" detail  ") +
			styles.HelpKey.Render("Esc") + styles.HelpText.Render(" back")
	}

	if b.testing {
		help += "  " + styles.NodeTesting.Render(m.Nodes.Testing)
	}

	parts := []string{}
	if b.title != "" {
		parts = append(parts, styles.TitleBar.Render(b.title))
	}
	if subTabBar != "" {
		parts = append(parts, subTabBar)
	}
	parts = append(parts, tableView, help)
	return strings.Join(parts, "\n")
}

// Init returns initial commands (refreshes nodes for the active subscription).
func (b *NodeBrowser) Init() tea.Cmd {
	if !b.active {
		return nil
	}
	return b.refreshNodesCmd()
}

// -- internal types --

type nodeTestResultMsg struct {
	Result node.TestResult
}

type nodeTestCompleteMsg struct{}

type nodeRefreshedMsg struct {
	Nodes []subscription.Node
}

// -- internal methods --

func (b *NodeBrowser) getSelectedNode() *subscription.Node {
	idx := b.table.Selected()
	if b.allowNoProxy && b.mode == PickMode {
		if idx == 0 {
			return nil
		}
		idx--
	}
	if idx >= 0 && idx < len(b.nodes) {
		return &b.nodes[idx]
	}
	return nil
}

func (b *NodeBrowser) refreshNodes() {
	if b.nodeMgr != nil {
		b.nodes = b.nodeMgr.GetNodesForSubscription(b.subscriptionID)
	} else if b.subMgr != nil {
		b.nodes = b.subMgr.GetNodesForSubscription(b.subscriptionID)
	} else {
		b.nodes = nil
	}
	b.updateTableRows()
	b.applyPreselect()
}

func (b *NodeBrowser) refreshNodesCmd() tea.Cmd {
	return func() tea.Msg {
		if b.subMgr != nil && b.subscriptionID != "" {
			_ = b.subMgr.Update(b.subscriptionID)
		}
		if b.nodeMgr != nil {
			return nodeRefreshedMsg{Nodes: b.nodeMgr.GetNodesForSubscription(b.subscriptionID)}
		}
		if b.subMgr != nil {
			return nodeRefreshedMsg{Nodes: b.subMgr.GetNodesForSubscription(b.subscriptionID)}
		}
		return nodeRefreshedMsg{}
	}
}

func (b *NodeBrowser) switchSubscription(delta int) {
	subs := b.sortedSubscriptions()
	if len(subs) == 0 {
		return
	}

	// Find current index.
	curIdx := -1
	for i, sub := range subs {
		if sub != nil && sub.ID == b.subscriptionID {
			curIdx = i
			break
		}
	}
	if curIdx < 0 {
		curIdx = 0
	}

	nextIdx := (curIdx + delta + len(subs)) % len(subs)
	nextSub := subs[nextIdx]
	if nextSub == nil || nextSub.ID == b.subscriptionID {
		return
	}

	// Save scroll state.
	b.scrollStates[b.subscriptionID] = scrollState{
		selected: b.table.Selected(),
		offset:   b.table.Offset(),
	}

	b.subscriptionID = nextSub.ID
	b.subTabIndex = nextIdx

	// Restore scroll state.
	if ss, ok := b.scrollStates[b.subscriptionID]; ok {
		b.table.SetSelected(ss.selected)
		b.table.SetOffset(ss.offset)
	} else {
		b.table.SetSelected(0)
		b.table.SetOffset(0)
	}

	b.refreshNodes()
}

func (b *NodeBrowser) sortedSubscriptions() []*subscription.Subscription {
	if b.subMgr == nil {
		return nil
	}
	subs := b.subMgr.List()
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

func (b *NodeBrowser) renderSubTabs() string {
	subs := b.sortedSubscriptions()
	if len(subs) <= 1 {
		return ""
	}

	msgs := i18n.M()
	prefix := styles.HelpText.Render(msgs.Actions.LrSwitch + "： ")

	parts := []string{prefix}
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		label := sub.Name
		if subscription.IsCustomSubscription(sub) {
			label = "📌 " + label
		}
		if sub.ID == b.subscriptionID {
			parts = append(parts, styles.ActiveTab.Render(label))
		} else {
			parts = append(parts, styles.InactiveTab.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return styles.TabBar.Width(b.width).Render(tabBar)
}

func (b *NodeBrowser) updateTableSize() {
	// Reserve: title(2) + sub-tabs(2) + table header(1) + separator(1) + help(1) + scroll indicator(1) = 8
	const reservedLines = 8
	rowsHeight := b.height - reservedLines
	if rowsHeight < 1 {
		rowsHeight = 1
	}
	b.table.SetHeight(rowsHeight)
	b.table.SetColumns(calcNodeTableColumns(b.width, b.serverDisplay))
}

func (b *NodeBrowser) updateTableRows() {
	showServer := b.serverDisplay != "off"
	rows := make([][]string, 0, len(b.nodes)+1)

	if b.allowNoProxy && b.mode == PickMode {
		status := "DIRECT"
		if traffic, ok := b.slotTraffic[""]; ok && traffic != "" {
			status = traffic
		}
		base := []string{"", i18n.M().Slots.NoProxyOption}
		if base[1] == "" {
			base[1] = "No Proxy (DIRECT)"
		}
		if showServer {
			base = append(base, "DIRECT")
		}
		base = append(base, "-", status)
		rows = append(rows, base)
	}

	for i, n := range b.nodes {
		var result *node.TestResult
		if b.nodeMgr != nil {
			result = b.nodeMgr.GetNodeWithResult(&b.nodes[i])
		}

		icon := ""
		if ind, ok := b.slotIndicators[n.Name]; ok {
			icon = ind.Icon
		}

		latency := "-"
		status := ""

		if result != nil {
			switch result.Status {
			case node.StatusOnline:
				latency = node.FormatLatency(result.Latency)
				status = "Online"
			case node.StatusTimeout:
				latency = "Timeout"
				status = "Timeout"
			case node.StatusTesting:
				latency = "..."
				status = "Testing"
			case node.StatusError:
				status = "Error"
			default:
				status = ""
			}
		}

		if traffic, ok := b.slotTraffic[n.Name]; ok && traffic != "" {
			status = traffic
		}

		base := []string{icon, n.Name}
		if showServer {
			server := n.Server
			if b.serverDisplay == "blur" {
				server = maskMiddle(server)
			}
			base = append(base, fmt.Sprintf("%s:%d", server, n.Port))
		}
		base = append(base, latency, status)
		rows = append(rows, base)
	}
	prevIdx := b.table.Selected()
	b.table.SetRows(rows)
	if prevIdx >= 0 && prevIdx < len(rows) {
		b.table.SetSelected(prevIdx)
	}
}

func (b *NodeBrowser) applyPreselect() {
	if len(b.nodes) == 0 {
		return
	}
	if b.preselectServer != "" && b.preselectPort > 0 {
		for i := range b.nodes {
			if b.nodes[i].Server == b.preselectServer && b.nodes[i].Port == b.preselectPort {
				b.table.SetSelected(i)
				return
			}
		}
	}
	if b.preselectName != "" {
		for i := range b.nodes {
			if b.nodes[i].Name == b.preselectName {
				b.table.SetSelected(i)
				return
			}
		}
	}
}

func (b *NodeBrowser) openDetailView() tea.Cmd {
	idx := b.table.Selected()
	if idx < 0 || idx >= len(b.nodes) {
		return nil
	}
	n := b.nodes[idx]

	var probeLock *proxy.LockFile
	if b.nodeMgr != nil {
		probeLock = b.nodeMgr.GetProbeLock(&n)
	}

	b.detailView = NewNodeDetailView(n, b.width, b.height, probeLock)
	b.showDetail = true
	return nil
}

func (b *NodeBrowser) testAllNodes() tea.Cmd {
	if b.testing {
		return nil
	}

	b.testing = true
	ctx, cancel := context.WithCancel(context.Background())
	b.cancelTest = cancel

	ch := make(chan node.TestResult, len(b.nodes))
	b.resultCh = ch

	nodes := b.nodes
	go func() {
		b.nodeMgr.TestNodes(ctx, nodes, ch)
		close(ch)
	}()

	return waitNodeResult(ch)
}

func (b *NodeBrowser) updateNodeResult(result node.TestResult) {
	for i := range b.nodes {
		if b.nodes[i].Server == result.Node.Server && b.nodes[i].Port == result.Node.Port {
			b.nodes[i].Latency = result.Latency
			break
		}
	}
	b.updateTableRows()
}

func waitNodeResult(ch <-chan node.TestResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return nodeTestCompleteMsg{}
		}
		return nodeTestResultMsg{Result: result}
	}
}

func calcNodeTableColumns(width int, serverDisplay string) []Column {
	if width < 0 {
		width = 0
	}

	iconW := 4
	latencyW := 10
	statusW := 25
	showServer := serverDisplay != "off"

	minNameW := 16
	baseNameW := 27

	minServerW := 18
	fixed := iconW + latencyW + statusW
	if !showServer {
		minServerW = 0
	}
	remaining := width - fixed
	if remaining < 0 {
		remaining = 0
	}

	nameW := 0
	serverW := 0

	if showServer && remaining >= minNameW+minServerW {
		nameW = baseNameW
		if nameW > remaining-minServerW {
			nameW = remaining - minServerW
		}
		if nameW < minNameW {
			nameW = minNameW
		}
		serverW = remaining - nameW
	} else if remaining > 0 {
		if showServer {
			nameW = remaining / 2
			serverW = remaining - nameW
		} else {
			nameW = remaining
			if nameW > baseNameW {
				extra := nameW - baseNameW
				nameW = baseNameW
				statusW += extra
			}
			serverW = 0
		}
	}

	if fixed+nameW+serverW > width {
		excess := fixed + nameW + serverW - width
		if serverW >= excess {
			serverW -= excess
		} else if nameW >= excess {
			nameW -= excess
		} else {
			nameW = 0
			serverW = 0
		}
	}

	columns := []Column{
		{Title: "", Width: iconW},
		{Title: "Name", Width: nameW},
	}
	if showServer {
		columns = append(columns, Column{Title: "Server", Width: serverW})
	}
	columns = append(columns,
		Column{Title: "Latency", Width: latencyW},
		Column{Title: "Status", Width: statusW},
	)
	return columns
}

func maskMiddle(s string) string {
	runes := []rune(s)
	if len(runes) <= 4 {
		return s
	}
	keep := 2
	return string(runes[:keep]) + strings.Repeat("*", len(runes)-(keep*2)) + string(runes[len(runes)-keep:])
}
