package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/components"
	"rocket4cli/internal/tui/styles"
	"rocket4cli/internal/tui/tabs"
)

// Tab represents a tab index
type Tab int

const (
	TabNodes Tab = iota
	TabSubscriptions
	TabSettings
	TabLogs
)

func sameNode(a, b *subscription.Node) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name && a.Server == b.Server && a.Port == b.Port
}

type overlayMsg struct {
	text string
}

type hideOverlayMsg struct {
	id int
}

type splashDoneMsg struct{}

const splashCooldown = 24 * time.Hour

// Model represents the main TUI model
type Model struct {
	// Managers
	cfg        *config.Config
	subManager *subscription.Manager
	nodeMgr    *node.Manager
	slotMgr    *proxy.SlotManager

	// UI State
	activeTab      Tab
	width          int
	height         int
	ready          bool
	quitting       bool
	version        string
	startupWarning string
	showSplash     bool
	activeSubID    string
	subTabIdx      int
	selectedNode   *subscription.Node
	overlayText    string
	overlayID      int

	// Per-slot traffic tracking (slot → last snapshot)
	slotTrafficLast map[int]trafficSnapshot

	// Components
	statusBar *components.StatusBar

	// Tabs
	nodesTab    *tabs.NodesTab
	subsTab     *tabs.SubscriptionsTab
	settingsTab *tabs.SettingsTab
	logsTab     *tabs.LogsTab
}

type trafficSnapshot struct {
	at            time.Time
	uploadTotal   int64
	downloadTotal int64
}

// New creates a new TUI model
func New(cfg *config.Config, subMgr *subscription.Manager, nodeMgr *node.Manager, slotMgr *proxy.SlotManager, version string, startupWarning string) *Model {
	m := &Model{
		cfg:             cfg,
		subManager:      subMgr,
		nodeMgr:         nodeMgr,
		slotMgr:         slotMgr,
		statusBar:       components.NewStatusBar(),
		nodesTab:        tabs.NewNodesTab(nodeMgr),
		subsTab:         tabs.NewSubscriptionsTab(subMgr),
		settingsTab:     tabs.NewSettingsTab(cfg),
		logsTab:         tabs.NewLogsTab(),
		version:         strings.TrimSpace(version),
		startupWarning:  strings.TrimSpace(startupWarning),
		slotTrafficLast: make(map[int]trafficSnapshot),
	}

	if m.nodesTab != nil {
		m.nodesTab.SetServerDisplay(cfg.UI.ServerDisplay)
	}

	if cfg != nil && cfg.UI.ActiveSubscriptionID != "" {
		m.activeSubID = cfg.UI.ActiveSubscriptionID
		if m.nodesTab != nil {
			m.nodesTab.SetSubscriptionID(m.activeSubID)
		}
		if m.subsTab != nil {
			m.subsTab.SetActiveSubscription(m.activeSubID)
		}
	}

	// Set the save history callback for slot manager
	if slotMgr != nil {
		slotMgr.SetSaveHistoryFunc(m.persistSlotHistory)
	}

	m.restoreSelectionFromConfig()
	m.restoreStateFromProxyLock()

	return m
}

func (m *Model) persistActiveSubscription(id string) tea.Cmd {
	if m == nil || m.cfg == nil {
		return nil
	}
	if m.cfg.UI.ActiveSubscriptionID == id {
		return nil
	}
	m.cfg.UI.ActiveSubscriptionID = id
	return func() tea.Msg {
		if err := m.cfg.Save(); err != nil {
			logger.Warn("failed to save config: %v", err)
		}
		return nil
	}
}

func (m *Model) persistSelectedNode(subID string, node *subscription.Node) tea.Cmd {
	if m == nil || m.cfg == nil || node == nil {
		return nil
	}
	if subID == "" {
		subID = m.activeSubID
	}
	if m.cfg.UI.SelectedNodeSubID == subID &&
		m.cfg.UI.SelectedNodeName == node.Name &&
		m.cfg.UI.SelectedNodeServer == node.Server &&
		m.cfg.UI.SelectedNodePort == node.Port &&
		m.cfg.UI.SelectedNodeType == node.Type {
		return nil
	}

	m.cfg.UI.SelectedNodeSubID = subID
	m.cfg.UI.SelectedNodeName = node.Name
	m.cfg.UI.SelectedNodeServer = node.Server
	m.cfg.UI.SelectedNodePort = node.Port
	m.cfg.UI.SelectedNodeType = node.Type

	return func() tea.Msg {
		if err := m.cfg.Save(); err != nil {
			logger.Warn("failed to save config: %v", err)
		}
		return nil
	}
}

func (m *Model) shouldShowSplash(now time.Time) bool {
	if m == nil || m.cfg == nil {
		return true
	}
	lastRaw := strings.TrimSpace(m.cfg.UI.SplashLastShownAt)
	if lastRaw == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, lastRaw)
	if err != nil {
		return true
	}
	if now.Before(last) {
		// Clock skew; don't spam the splash.
		return false
	}
	return now.Sub(last) >= splashCooldown
}

func (m *Model) persistSplashShown(now time.Time) tea.Cmd {
	if m == nil || m.cfg == nil {
		return nil
	}
	ts := now.UTC().Format(time.RFC3339)
	if m.cfg.UI.SplashLastShownAt == ts {
		return nil
	}
	m.cfg.UI.SplashLastShownAt = ts
	return func() tea.Msg {
		if err := m.cfg.Save(); err != nil {
			logger.Warn("failed to save config: %v", err)
		}
		return nil
	}
}

// persistSlotHistory saves the slot history to the config file.
func (m *Model) persistSlotHistory(history []config.SlotHistoryEntry) {
	if m == nil || m.cfg == nil {
		return
	}
	m.cfg.UI.SlotHistory = history
	if err := m.cfg.Save(); err != nil {
		logger.Warn("failed to save slot history config: %v", err)
	}
}

func (m *Model) showStartupWarningOverlay() tea.Cmd {
	if m == nil || strings.TrimSpace(m.startupWarning) == "" {
		return nil
	}
	m.overlayID++
	id := m.overlayID
	m.overlayText = m.startupWarning
	return tea.Tick(6*time.Second, func(time.Time) tea.Msg {
		return hideOverlayMsg{id: id}
	})
}

func (m *Model) restoreSelectionFromConfig() {
	if m == nil || m.cfg == nil || m.subManager == nil || m.nodeMgr == nil {
		return
	}
	ui := m.cfg.UI

	// Only restore selection for the active subscription (keeps UI self-consistent).
	if ui.SelectedNodeSubID == "" || m.activeSubID == "" || ui.SelectedNodeSubID != m.activeSubID {
		return
	}

	nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
	var selected *subscription.Node
	for i := range nodes {
		n := &nodes[i]
		if ui.SelectedNodeServer != "" && ui.SelectedNodePort > 0 {
			if n.Server == ui.SelectedNodeServer && n.Port == ui.SelectedNodePort {
				nodeCopy := *n
				selected = &nodeCopy
				break
			}
			continue
		}
		if ui.SelectedNodeName != "" && n.Name == ui.SelectedNodeName {
			nodeCopy := *n
			selected = &nodeCopy
			break
		}
	}
	if selected == nil {
		return
	}

	m.selectedNode = selected
	m.nodeMgr.Select(selected)
	if m.statusBar != nil {
		m.statusBar.SetSelectedNode(selected)
	}
	if m.nodesTab != nil {
		m.nodesTab.SetPreselect(selected)
	}
}

func (m *Model) restoreStateFromProxyLock() {
	if m == nil || m.slotMgr == nil || m.subManager == nil || m.nodeMgr == nil {
		return
	}

	// SyncFromLockFile restores all slots from the multi-lock file.
	m.slotMgr.SyncFromLockFile()

	// Update UI based on restored slots.
	activeSlots := m.slotMgr.ActiveSlots()
	if len(activeSlots) == 0 {
		return
	}

	// Use the first active slot to infer subscription context.
	first := activeSlots[0]
	if first.SubID != "" {
		m.activeSubID = first.SubID
		if m.nodesTab != nil {
			m.nodesTab.SetSubscriptionID(first.SubID)
		}
		if m.subsTab != nil {
			m.subsTab.SetActiveSubscription(first.SubID)
		}
	}

	// Set selected node from slot 0 (or first slot).
	if first.NodeName != "" {
		nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
		for i := range nodes {
			if nodes[i].Name == first.NodeName {
				m.selectedNode = &nodes[i]
				m.nodeMgr.Select(m.selectedNode)
				m.statusBar.SetSelectedNode(m.selectedNode)
				if m.nodesTab != nil {
					m.nodesTab.SetPreselect(m.selectedNode)
				}
				break
			}
		}
	}

	// Update slot indicators
	m.updateSlotIndicators()
}

func inferSubscriptionAndNode(subMgr *subscription.Manager, lock *proxy.LockFile) (string, *subscription.Node) {
	if subMgr == nil || lock == nil {
		return "", nil
	}

	// 1) Trust explicit sub_id if it exists and is present on disk.
	if lock.SubID != "" {
		if sub, ok := subMgr.Get(lock.SubID); ok && sub != nil {
			if n := findNodeInSubscription(sub, lock); n != nil {
				return lock.SubID, n
			}
			// Even if node match fails, returning the subscription still helps the UI.
			return lock.SubID, nil
		}
	}

	// 2) Infer by node match across subscriptions.
	subs := subMgr.List()
	var matchedSubID string
	var matchedNode *subscription.Node
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		n := findNodeInSubscription(sub, lock)
		if n == nil {
			continue
		}
		if matchedNode != nil {
			// Ambiguous across multiple subscriptions: keep the first match.
			continue
		}
		matchedSubID = sub.ID
		matchedNode = n
	}
	return matchedSubID, matchedNode
}

func sortSubscriptionsByNameAndID(subs []*subscription.Subscription) {
	sort.Slice(subs, func(i, j int) bool {
		if subs[i].Name == subs[j].Name {
			return subs[i].ID < subs[j].ID
		}
		return subs[i].Name < subs[j].Name
	})
}

func findNodeInSubscription(sub *subscription.Subscription, lock *proxy.LockFile) *subscription.Node {
	if sub == nil || lock == nil {
		return nil
	}
	for i := range sub.Nodes {
		n := &sub.Nodes[i]

		// Strong match (server:port) if present.
		if lock.NodeServer != "" && lock.NodePort > 0 {
			if n.Server == lock.NodeServer && n.Port == lock.NodePort {
				nodeCopy := *n
				return &nodeCopy
			}
			continue
		}

		// Fallback match by name.
		if lock.Node != "" && n.Name == lock.Node {
			nodeCopy := *n
			return &nodeCopy
		}
	}
	return nil
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.nodesTab.Init(),
		m.subsTab.Init(),
		m.settingsTab.Init(),
		m.logsTab.Init(),
		m.listenSlotLogs(),
		m.listenAppLogs(),
		m.pollTraffic(),
	)
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		first := !m.ready
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.updateSizes()
		if first {
			now := time.Now()
			if m.shouldShowSplash(now) {
				m.showSplash = true
				cmds = append(cmds, m.persistSplashShown(now))
				cmds = append(cmds, tea.Tick(1*time.Second, func(time.Time) tea.Msg {
					return splashDoneMsg{}
				}))
			} else {
				cmds = append(cmds, m.showStartupWarningOverlay())
			}
		}

	case tea.KeyMsg:
		if m.showSplash {
			// Allow quitting during splash, ignore other keys.
			if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

		// If a center prompt is visible, hide it on any keypress.
		if m.overlayText != "" {
			m.overlayText = ""
		}

		// Global keys
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			m.nextTab()
			return m, nil

		case m.activeTab == TabNodes && key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m, m.handleSlotAction(0)

		case m.activeTab == TabNodes && isDigitKey(msg):
			slotID := digitKeyToSlot(msg)
			return m, m.handleSlotAction(slotID)

		case m.activeTab == TabNodes && key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			return m, m.switchSubscription(-1)

		case m.activeTab == TabNodes && key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			return m, m.switchSubscription(1)

		case m.activeTab == TabNodes && key.Matches(msg, key.NewBinding(key.WithKeys("D"))):
			m.disconnectAllSlots()
			return m, tea.Batch(m.nodesTab.Init())
		}

	case overlayMsg:
		m.overlayID++
		id := m.overlayID
		m.overlayText = msg.text
		cmds = append(cmds, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return hideOverlayMsg{id: id}
		}))

	case hideOverlayMsg:
		if msg.id == m.overlayID {
			m.overlayText = ""
		}

	case splashDoneMsg:
		m.showSplash = false
		cmds = append(cmds, m.showStartupWarningOverlay())

	case tabs.NodeSelectedMsg:
		m.selectedNode = msg.Node
		m.statusBar.SetSelectedNode(msg.Node)
		if m.nodesTab != nil {
			m.nodesTab.SetPreselect(msg.Node)
		}
		m.logsTab.AddLog("info", fmt.Sprintf("Selected node: %s", msg.Node.Name))
		cmds = append(cmds, m.persistSelectedNode(m.activeSubID, msg.Node))

	case tabs.SubscriptionAddedMsg:
		// Refresh nodes when subscription is added
		m.logsTab.AddLog("info", fmt.Sprintf("Subscription added: %s", msg.Sub.Name))
		cmds = append(cmds, m.nodesTab.Init())
		// Also refresh subscriptions tab
		var cmd tea.Cmd
		m.subsTab, cmd = m.subsTab.Update(msg)
		cmds = append(cmds, cmd)

	case tabs.SubscriptionUpdatedMsg:
		// Refresh nodes when subscriptions are updated
		m.logsTab.AddLog("info", "Subscriptions updated")
		cmds = append(cmds, m.nodesTab.Init())

	case tabs.NodesRefreshedMsg:
		// Always forward NodesRefreshedMsg to nodes tab regardless of active tab
		m.nodesTab, _ = m.nodesTab.Update(msg)

	case tabs.SubsRefreshedMsg:
		prevActive := m.activeSubID

		// Ensure we have an active subscription (for Nodes filtering).
		newActive := m.activeSubID
		newSubTabIdx := m.subTabIdx
		if len(msg.Subs) == 0 {
			newActive = ""
			newSubTabIdx = 0
		} else {
			valid := false
			for i, sub := range msg.Subs {
				if sub != nil && sub.ID == m.activeSubID {
					valid = true
					newSubTabIdx = i
					break
				}
			}
			if newActive == "" || !valid {
				newActive = msg.Subs[0].ID
				newSubTabIdx = 0
			}
		}

		if newActive != prevActive && prevActive != "" {
			m.selectedNode = nil
			m.nodeMgr.Select(nil)
			m.statusBar.SetSelectedNode(nil)
		}
		m.activeSubID = newActive
		m.subTabIdx = newSubTabIdx
		cmds = append(cmds, m.persistActiveSubscription(m.activeSubID))

		if m.nodesTab != nil {
			m.nodesTab.SetSubscriptionID(m.activeSubID)
			cmds = append(cmds, m.nodesTab.Init())
		}

		// Always forward SubsRefreshedMsg to subscriptions tab regardless of active tab
		m.subsTab, _ = m.subsTab.Update(msg)
		if m.subsTab != nil {
			m.subsTab.SetActiveSubscription(m.activeSubID)
		}

	case tabs.SettingsSavedMsg:
		// Apply settings that affect other tabs immediately.
		if m.nodesTab != nil {
			m.nodesTab.SetServerDisplay(m.cfg.UI.ServerDisplay)
		}
		if m.subManager != nil {
			m.subManager.SetUpdateViaProxy(m.cfg.Subscription.UpdateViaProxy)
		}

	case tabs.RestartCoreMsg:
		cmds = append(cmds, m.restartSlot0())

	case proxy.LogEntry:
		// Forward to logs tab
		var cmd tea.Cmd
		m.logsTab, cmd = m.logsTab.Update(msg)
		cmds = append(cmds, cmd)
		// Continue listening for more logs
		cmds = append(cmds, m.listenSlotLogs())

	case logger.Entry:
		// Forward app logs to logs tab
		m.logsTab.AddLog(msg.Level.String(), msg.Message)
		// Continue listening for more logs
		cmds = append(cmds, m.listenAppLogs())

	case slotTrafficMsg:
		// Keep polling.
		cmds = append(cmds, m.pollTraffic())

		// Update per-slot traffic and compute rates for status bar (aggregate all slots).
		var totalUpRate, totalDownRate, totalUp, totalDown int64
		nodeTraffic := make(map[string]string)

		for slotID, snap := range msg.snapshots {
			prev, hasPrev := m.slotTrafficLast[slotID]
			upRate := int64(0)
			downRate := int64(0)

			if hasPrev && !prev.at.IsZero() && snap.at.After(prev.at) {
				dt := snap.at.Sub(prev.at).Seconds()
				if dt > 0 {
					if snap.uploadTotal >= prev.uploadTotal {
						upRate = int64(float64(snap.uploadTotal-prev.uploadTotal) / dt)
					}
					if snap.downloadTotal >= prev.downloadTotal {
						downRate = int64(float64(snap.downloadTotal-prev.downloadTotal) / dt)
					}
				}
			}
			m.slotTrafficLast[slotID] = snap
			totalUpRate += upRate
			totalDownRate += downRate
			totalUp += snap.uploadTotal
			totalDown += snap.downloadTotal

			// Build per-node traffic string
			state := m.slotMgr.GetSlotState(slotID)
			if state != nil && state.NodeName != "" {
				nodeTraffic[state.NodeName] = fmt.Sprintf(":%d ↑%s ↓%s",
					state.Port,
					components.FormatRate(upRate),
					components.FormatRate(downRate),
				)
			}
		}

		// Clean up stale traffic entries
		for slotID := range m.slotTrafficLast {
			if _, ok := msg.snapshots[slotID]; !ok {
				delete(m.slotTrafficLast, slotID)
			}
		}

		if m.statusBar != nil {
			m.statusBar.SetTraffic(totalUp, totalDown, totalUpRate, totalDownRate)
			m.statusBar.SetActiveSlots(m.slotMgr.ActiveSlotCount())
		}

		// Update node traffic display
		if m.nodesTab != nil {
			m.nodesTab.SetSlotTraffic(nodeTraffic)
		}

	case slotActionDoneMsg:
		if msg.err != nil {
			m.logsTab.AddLog("error", msg.err.Error())
		}
		// Refresh slot indicators
		m.updateSlotIndicators()
		m.updateStatusBar()
	}

	// Update active tab
	var cmd tea.Cmd
	switch m.activeTab {
	case TabNodes:
		m.nodesTab, cmd = m.nodesTab.Update(msg)
	case TabSubscriptions:
		m.subsTab, cmd = m.subsTab.Update(msg)
	case TabSettings:
		m.settingsTab, cmd = m.settingsTab.Update(msg)
	case TabLogs:
		m.logsTab, cmd = m.logsTab.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the TUI
func (m *Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if !m.ready {
		return "Initializing..."
	}

	if m.showSplash {
		title := "Clash 4 Cli"
		if m.version != "" && m.version != "dev" {
			title = fmt.Sprintf("%s v%s", title, m.version)
		} else if m.version != "" {
			title = fmt.Sprintf("%s %s", title, m.version)
		}
		box := styles.InfoBox.Render(styles.Title.Render(title))
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}

	// Build layout
	var content strings.Builder

	// Tab bar
	content.WriteString(m.renderTabBar())
	content.WriteString("\n")

	// Content area
	var contentView string
	if m.overlayText != "" {
		// Calculate content height for overlay centering: subtract tab bar (1) + status bar (1)
		contentHeight := m.height - 2
		if m.activeTab == TabNodes {
			// Nodes tab has sub-tabs, subtract 1 more line for them
			contentHeight--
		}
		maxWidth := 60
		if m.width-10 < maxWidth {
			maxWidth = m.width - 10
		}
		if maxWidth < 20 {
			maxWidth = m.width - 2
		}
		if maxWidth < 10 {
			maxWidth = 10
		}
		wrapped := wrapPlainText(m.overlayText, maxWidth)
		message := lipgloss.NewStyle().Width(maxWidth).Align(lipgloss.Center).Render(wrapped)
		box := styles.InfoBox.Render(styles.ErrorMessage.Render(message))
		contentView = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center, box)
	} else {
		contentView = m.renderActiveTab()
	}
	content.WriteString(contentView)
	content.WriteString("\n")

	// Status bar
	m.statusBar.SetWidth(m.width)
	m.updateStatusBar()
	content.WriteString(m.statusBar.View())

	return content.String()
}

func (m *Model) renderTabBar() string {
	msgs := i18n.M()
	tabNames := []string{
		msgs.Tabs.Nodes,
		msgs.Tabs.Subscription,
		msgs.Tabs.Settings,
		msgs.Tabs.Logs,
	}

	var tabs []string
	for i, name := range tabNames {
		if Tab(i) == m.activeTab {
			tabs = append(tabs, styles.ActiveTab.Render(name))
		} else {
			tabs = append(tabs, styles.InactiveTab.Render(name))
		}
	}

	prefix := styles.HelpText.Render(msgs.Actions.Tab + "： ")
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, append([]string{prefix}, tabs...)...)
	return styles.TabBar.Width(m.width).Render(tabBar)
}

// renderSubTabs renders subscription sub-tabs for the Nodes page.
// It performs horizontal scrolling so the tab bar always fits in one line.
func (m *Model) renderSubTabs() string {
	subs := m.subManager.List()
	sortSubscriptionsByNameAndID(subs)
	if len(subs) == 0 {
		return ""
	}

	// Find the index of the currently active subscription by ID
	activeIdx := 0
	if m.activeSubID != "" {
		for i, sub := range subs {
			if sub != nil && sub.ID == m.activeSubID {
				activeIdx = i
				break
			}
		}
	}

	// Pre-render every tab label so we can measure widths.
	type renderedTab struct {
		text  string
		width int
	}
	rendered := make([]renderedTab, 0, len(subs))
	for i, sub := range subs {
		if sub == nil {
			continue
		}
		label := sub.Name
		var text string
		if i == activeIdx {
			text = styles.ActiveTab.Render(label)
		} else {
			text = styles.InactiveTab.Render(label)
		}
		rendered = append(rendered, renderedTab{text: text, width: lipgloss.Width(text)})
	}

	if len(rendered) == 0 {
		return ""
	}

	// Available width for tabs (reserve space for possible ← / → indicators).
	const arrowWidth = 2 // "← " or " →"
	maxWidth := m.width
	if maxWidth < 1 {
		maxWidth = 80
	}

	// Determine the visible window of tabs that fits within maxWidth,
	// ensuring the active tab is always visible.
	// Map activeIdx to the rendered slice index (skip nil subs).
	activeRendered := 0
	{
		j := 0
		for i, sub := range subs {
			if sub == nil {
				continue
			}
			if i == activeIdx {
				activeRendered = j
				break
			}
			j++
		}
	}

	// Start with the active tab and expand outward.
	startIdx := activeRendered
	endIdx := activeRendered // inclusive
	usedWidth := rendered[activeRendered].width

	for {
		expanded := false
		// Try expanding left
		if startIdx > 0 {
			need := rendered[startIdx-1].width
			extra := 0
			if startIdx-1 > 0 {
				extra = arrowWidth // will need "← " indicator
			}
			if endIdx < len(rendered)-1 {
				extra = max(extra, arrowWidth) // already have " →"
			}
			if usedWidth+need+extra <= maxWidth {
				startIdx--
				usedWidth += need
				expanded = true
			}
		}
		// Try expanding right
		if endIdx < len(rendered)-1 {
			need := rendered[endIdx+1].width
			extra := 0
			if endIdx+1 < len(rendered)-1 {
				extra = arrowWidth
			}
			if startIdx > 0 {
				extra = max(extra, arrowWidth)
			}
			if usedWidth+need+extra <= maxWidth {
				endIdx++
				usedWidth += need
				expanded = true
			}
		}
		if !expanded {
			break
		}
	}

	// Build the visible tab bar.
	msgs := i18n.M()
	prefix := styles.HelpText.Render(msgs.Actions.LrSwitch + "： ")
	var parts []string
	parts = append(parts, prefix)
	if startIdx > 0 {
		parts = append(parts, styles.Muted.Render("◀ "))
	}
	for i := startIdx; i <= endIdx; i++ {
		parts = append(parts, rendered[i].text)
	}
	if endIdx < len(rendered)-1 {
		parts = append(parts, styles.Muted.Render(" ▶"))
	}

	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return styles.TabBar.Width(m.width).Render(tabBar)
}

func (m *Model) renderActiveTab() string {
	switch m.activeTab {
	case TabNodes:
		// Render subscription sub-tabs above nodes table
		subTabs := m.renderSubTabs()
		nodesView := m.nodesTab.View()
		if subTabs != "" {
			return subTabs + "\n" + nodesView
		}
		return nodesView
	case TabSubscriptions:
		return m.subsTab.View()
	case TabSettings:
		return m.settingsTab.View()
	case TabLogs:
		return m.logsTab.View()
	default:
		return ""
	}
}

func (m *Model) nextTab() {
	// Discard unsaved settings when leaving Settings tab.
	if m.activeTab == TabSettings && m.settingsTab != nil && m.cfg != nil {
		m.settingsTab = tabs.NewSettingsTab(m.cfg)
		m.settingsTab.SetSize(m.width, m.height-4)
	}
	m.activeTab = Tab((int(m.activeTab) + 1) % 4)
}

func (m *Model) prevTab() {
	// Discard unsaved settings when leaving Settings tab.
	if m.activeTab == TabSettings && m.settingsTab != nil && m.cfg != nil {
		m.settingsTab = tabs.NewSettingsTab(m.cfg)
		m.settingsTab.SetSize(m.width, m.height-4)
	}
	m.activeTab = Tab((int(m.activeTab) + 3) % 4)
}

func wrapPlainText(s string, width int) string {
	if width <= 0 || s == "" {
		return s
	}

	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		runes := []rune(paragraph)
		for len(runes) > width {
			cut := width
			for i := cut; i > 0; i-- {
				if runes[i-1] == ' ' || runes[i-1] == '\t' {
					cut = i
					break
				}
			}
			line := strings.TrimSpace(string(runes[:cut]))
			if line != "" {
				out = append(out, line)
			}
			runes = []rune(strings.TrimLeft(string(runes[cut:]), " \t"))
		}
		out = append(out, string(runes))
	}
	return strings.Join(out, "\n")
}

func (m *Model) updateSizes() {
	// Content height: subtract tab bar (1) + status bar (1)
	contentHeight := m.height - 2
	m.nodesTab.SetSize(m.width, contentHeight)
	m.subsTab.SetSize(m.width, contentHeight-1)
	m.settingsTab.SetSize(m.width, contentHeight-1)
	m.logsTab.SetSize(m.width, contentHeight-1)
}

// -- Message types --

type slotActionDoneMsg struct {
	slotID int
	err    error
}

type slotTrafficMsg struct {
	snapshots map[int]trafficSnapshot
}

// -- Slot action dispatch --

func isDigitKey(msg tea.KeyMsg) bool {
	if len(msg.Runes) == 1 {
		r := msg.Runes[0]
		return r >= '0' && r <= '9'
	}
	return false
}

func digitKeyToSlot(msg tea.KeyMsg) int {
	if len(msg.Runes) == 1 {
		return int(msg.Runes[0] - '0')
	}
	return 0
}

func (m *Model) handleSlotAction(slotID int) tea.Cmd {
	var focused *subscription.Node
	if m.nodesTab != nil {
		focused = m.nodesTab.GetSelectedNode()
	}
	if focused == nil {
		nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
		if len(nodes) > 0 {
			focused = &nodes[0]
		}
	}
	if focused == nil {
		return func() tea.Msg { return overlayMsg{text: i18n.M().Nodes.NoNodes} }
	}

	// Track selection
	if m.selectedNode == nil || !sameNode(m.selectedNode, focused) {
		m.selectedNode = focused
		m.nodeMgr.Select(focused)
		m.statusBar.SetSelectedNode(focused)
		if m.nodesTab != nil {
			m.nodesTab.SetPreselect(focused)
		}
	}

	persistCmd := m.persistSelectedNode(m.activeSubID, focused)

	existingSlot, nodeInUse := m.slotMgr.NodeSlot(focused.Name)
	state := m.slotMgr.GetSlotState(slotID)

	if state == nil {
		// Slot not running → start it
		if nodeInUse && existingSlot != slotID {
			// Node used in another slot → free it first
			return tea.Batch(persistCmd, m.switchSlotToNode(slotID, existingSlot, focused))
		}
		subID := m.activeSubID
		nodeCopy := *focused
		nodes := m.subManager.GetNodesForSubscription(subID)
		return tea.Batch(persistCmd, func() tea.Msg {
			err := m.slotMgr.StartSlot(slotID, subID, nodes, &nodeCopy)
			return slotActionDoneMsg{slotID: slotID, err: err}
		})
	}

	if state.NodeName == focused.Name {
		// Same node → cycle mode (rule → global → stop)
		return tea.Batch(persistCmd, func() tea.Msg {
			_, err := m.slotMgr.CycleSlotMode(slotID)
			return slotActionDoneMsg{slotID: slotID, err: err}
		})
	}

	// Slot running with different node → stop old, start with new node
	return tea.Batch(persistCmd, m.restartSlotWithNode(slotID, focused))
}

// restartSlotWithNode stops a running slot and restarts it with a new node.
func (m *Model) restartSlotWithNode(slotID int, node *subscription.Node) tea.Cmd {
	nodeCopy := *node
	subID := m.activeSubID
	return func() tea.Msg {
		_ = m.slotMgr.StopSlot(slotID)
		time.Sleep(300 * time.Millisecond)
		nodes := m.subManager.GetNodesForSubscription(subID)
		err := m.slotMgr.StartSlot(slotID, subID, nodes, &nodeCopy)
		return slotActionDoneMsg{slotID: slotID, err: err}
	}
}

// switchSlotToNode moves a node from existingSlot to targetSlot.
// If existingSlot had the node, stop it first; then start targetSlot with the node.
func (m *Model) switchSlotToNode(targetSlot, existingSlot int, node *subscription.Node) tea.Cmd {
	nodeCopy := *node
	subID := m.activeSubID
	return func() tea.Msg {
		_ = m.slotMgr.StopSlot(existingSlot)
		time.Sleep(300 * time.Millisecond)
		nodes := m.subManager.GetNodesForSubscription(subID)
		err := m.slotMgr.StartSlot(targetSlot, subID, nodes, &nodeCopy)
		return slotActionDoneMsg{slotID: targetSlot, err: err}
	}
}

// -- Traffic polling --

func (m *Model) pollTraffic() tea.Cmd {
	const interval = 1 * time.Second
	return tea.Tick(interval, func(now time.Time) tea.Msg {
		if m == nil || m.slotMgr == nil {
			return slotTrafficMsg{}
		}

		snapshots := make(map[int]trafficSnapshot)
		for _, s := range m.slotMgr.ActiveSlots() {
			up, down, err := m.slotMgr.GetTrafficForSlot(s.SlotID)
			if err != nil {
				continue
			}
			snapshots[s.SlotID] = trafficSnapshot{at: now, uploadTotal: up, downloadTotal: down}
		}
		return slotTrafficMsg{snapshots: snapshots}
	})
}

// -- Log listeners --

func (m *Model) listenSlotLogs() tea.Cmd {
	return func() tea.Msg {
		logCh := m.slotMgr.LogChannel()
		for log := range logCh {
			return log
		}
		return nil
	}
}

func (m *Model) listenAppLogs() tea.Cmd {
	return func() tea.Msg {
		logCh := logger.Channel()
		if logCh == nil {
			return nil
		}
		for log := range logCh {
			return log
		}
		return nil
	}
}

// -- Restart slot 0 (from settings) --

func (m *Model) restartSlot0() tea.Cmd {
	return func() tea.Msg {
		nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
		if len(nodes) == 0 {
			return overlayMsg{text: i18n.M().Nodes.NoNodes}
		}
		if m.selectedNode == nil {
			msg := i18n.M().Errors.NoNodeSelected
			if msg == "" {
				msg = "No node selected"
			}
			return overlayMsg{text: fmt.Sprintf(msg, i18n.M().Tabs.Nodes)}
		}

		_ = m.slotMgr.StopSlot(0)
		time.Sleep(500 * time.Millisecond)

		err := m.slotMgr.StartSlot(0, m.activeSubID, nodes, m.selectedNode)
		return slotActionDoneMsg{slotID: 0, err: err}
	}
}

// -- UI helpers --

func slotModeIcon(mode string) string {
	switch mode {
	case "rule":
		return "📜"
	case "global":
		return "🌐"
	default:
		return ""
	}
}

func (m *Model) updateSlotIndicators() {
	if m.nodesTab == nil {
		return
	}
	indicators := make(map[string]tabs.SlotIndicator)

	// Prioritize active slots (running proxies)
	activeSlots := m.slotMgr.ActiveSlots()
	for _, s := range activeSlots {
		if s.NodeName != "" && s.Mode != "" {
			indicators[s.NodeName] = tabs.SlotIndicator{
				SlotID: s.SlotID,
				Icon:   slotModeIcon(s.Mode),
			}
		}
	}

	// If no active slots, use historical state (for UI display only)
	if len(indicators) == 0 && m.cfg != nil && len(m.cfg.UI.SlotHistory) > 0 {
		for _, h := range m.cfg.UI.SlotHistory {
			if h.NodeName != "" && h.Mode != "" {
				indicators[h.NodeName] = tabs.SlotIndicator{
					SlotID: h.SlotID,
					Icon:   slotModeIcon(h.Mode),
				}
			}
		}
	}

	m.nodesTab.SetSlotIndicators(indicators)
}

func (m *Model) updateStatusBar() {
	if m.statusBar == nil {
		return
	}
	activeCount := m.slotMgr.ActiveSlotCount()
	m.statusBar.SetActiveSlots(activeCount)
	m.statusBar.SetMixedPort(m.slotMgr.Slot0Port())

	if activeCount > 0 {
		m.statusBar.SetProxyStatus(proxy.StatusRunning)
		// Show mode of slot 0 (or first active slot)
		state := m.slotMgr.GetSlotState(0)
		if state == nil {
			slots := m.slotMgr.ActiveSlots()
			if len(slots) > 0 {
				state = slots[0]
			}
		}
		if state != nil {
			switch state.Mode {
			case "rule":
				m.statusBar.SetProxyMode("Rule")
			case "global":
				m.statusBar.SetProxyMode("Global")
			default:
				m.statusBar.SetProxyMode("Disconnect")
			}
		}
	} else {
		m.statusBar.SetProxyStatus(proxy.StatusStopped)
		m.statusBar.SetProxyMode("Disconnect")
	}
}

func (m *Model) switchSubscription(delta int) tea.Cmd {
	subs := m.subManager.List()
	if len(subs) <= 1 {
		return nil
	}

	sortSubscriptionsByNameAndID(subs)

	currentIdx := m.subTabIdx
	if currentIdx < 0 || currentIdx >= len(subs) || subs[currentIdx] == nil || subs[currentIdx].ID != m.activeSubID {
		currentIdx = -1
		for i, sub := range subs {
			if sub != nil && sub.ID == m.activeSubID {
				currentIdx = i
				break
			}
		}
	}

	if currentIdx < 0 {
		currentIdx = 0
	}

	nextIdx := (currentIdx + delta + len(subs)) % len(subs)
	newSub := subs[nextIdx]
	if newSub == nil || newSub.ID == m.activeSubID {
		return nil
	}

	// NOTE: Do NOT disconnect running slots — slots from other subscriptions keep running.
	m.activeSubID = newSub.ID
	m.subTabIdx = nextIdx
	if m.nodesTab != nil {
		m.nodesTab.SetSubscriptionID(m.activeSubID)
	}
	if m.subsTab != nil {
		m.subsTab.SetActiveSubscription(m.activeSubID)
	}

	return tea.Batch(
		m.persistActiveSubscription(m.activeSubID),
		m.nodesTab.Init(),
	)
}

func (m *Model) disconnectAllSlots() {
	if m.slotMgr != nil {
		m.slotMgr.StopAll()
	}
	if m.nodesTab != nil {
		m.nodesTab.SetSlotIndicators(nil)
	}
	m.slotTrafficLast = make(map[int]trafficSnapshot)
	if m.statusBar != nil {
		m.statusBar.ClearTraffic()
		m.statusBar.SetActiveSlots(0)
	}
}
