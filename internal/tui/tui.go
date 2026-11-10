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
	TabSlots Tab = iota
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

// DaemonLogMsg wraps a logger.Entry from the daemon SSE stream,
// allowing the Update handler to distinguish daemon-originated logs
// from local app logs and re-arm the correct listener.
type DaemonLogMsg struct {
	Entry logger.Entry
}

// Model represents the main TUI model
type Model struct {
	// Managers
	cfg         *config.Config
	subManager  *subscription.Manager
	nodeMgr     *node.Manager
	slotMgr     proxy.SlotService
	daemonLogCh <-chan logger.Entry

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
	selectedNode   *subscription.Node
	overlayText    string
	overlayID      int
	// Per-slot traffic tracking (slot → last snapshot)
	slotTrafficLast map[int]trafficSnapshot

	// Components
	statusBar *components.StatusBar

	// Tabs
	slotsTab    *tabs.SlotsTab
	subsTab     *tabs.SubscriptionsTab
	settingsTab *tabs.SettingsTab
	logsTab     *tabs.LogsTab
}

type trafficSnapshot struct {
	at            time.Time
	uploadTotal   int64
	downloadTotal int64
	connCount     int
}

// New creates a new TUI model
func New(cfg *config.Config, subMgr *subscription.Manager, nodeMgr *node.Manager, slotMgr proxy.SlotService, daemonLogCh <-chan logger.Entry, version string, startupWarning string) *Model {
	logsTab := tabs.NewLogsTab()
	settingsTab := tabs.NewSettingsTab(cfg, slotMgr, version)
	settingsTab.SetLogsTab(logsTab)

	m := &Model{
		cfg:             cfg,
		subManager:      subMgr,
		nodeMgr:         nodeMgr,
		slotMgr:         slotMgr,
		daemonLogCh:     daemonLogCh,
		statusBar:       components.NewStatusBar(),
		slotsTab:        tabs.NewSlotsTab(cfg, subMgr, slotMgr, nodeMgr),
		subsTab:         tabs.NewSubscriptionsTab(subMgr, slotMgr, nodeMgr),
		settingsTab:     settingsTab,
		logsTab:         logsTab,
		version:         strings.TrimSpace(version),
		startupWarning:  strings.TrimSpace(startupWarning),
		slotTrafficLast: make(map[int]trafficSnapshot),
	}

	if cfg != nil && cfg.UI.ActiveSubscriptionID != "" {
		m.activeSubID = cfg.UI.ActiveSubscriptionID
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

func preferredSubscriptionIndex(subs []*subscription.Subscription) int {
	if len(subs) == 0 {
		return 0
	}
	for i, sub := range subs {
		if sub != nil && !subscription.IsCustomSubscription(sub) {
			return i
		}
	}
	return 0
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
		m.slotsTab.Init(),
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

		// Let tabs with active overlays/forms consume keys before global shortcuts.
		slotsOverlay := m.activeTab == TabSlots && m.slotsTab != nil && m.slotsTab.BlocksGlobalKeys()
		subOverlay := m.activeTab == TabSubscriptions && m.subsTab != nil && m.subsTab.BlocksGlobalKeys()
		blockGlobal := slotsOverlay || subOverlay
		settingsInput := m.activeTab == TabSettings && m.settingsTab != nil && m.settingsTab.HasFocusedInput()

		// Global keys
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			if !blockGlobal {
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			if !blockGlobal {
				m.nextTab()
				return m, nil
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("1"))):
			if !blockGlobal && !settingsInput {
				m.setTab(TabSlots)
				return m, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("2"))):
			if !blockGlobal && !settingsInput {
				m.setTab(TabSubscriptions)
				return m, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("3"))):
			if !blockGlobal && !settingsInput {
				m.setTab(TabSettings)
				return m, nil
			}
		}

	case tea.MouseMsg:
		if m.showSplash {
			break
		}
		slotsFullScreen := (m.activeTab == TabSlots && m.slotsTab != nil && m.slotsTab.IsFullScreen()) || (m.activeTab == TabSubscriptions && m.subsTab != nil && m.subsTab.IsFullScreen())
		if slotsFullScreen {
			break
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if msg.Y == 0 {
				if tab, ok := m.tabHitTest(msg.X); ok {
					m.setTab(tab)
					return m, nil
				}
			}
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

	case tabs.SubscriptionAddedMsg:
		m.logsTab.AddLog("info", fmt.Sprintf("Subscription added: %s", msg.Sub.Name))
		var cmd tea.Cmd
		m.subsTab, cmd = m.subsTab.Update(msg)
		cmds = append(cmds, cmd)

	case tabs.SubscriptionUpdatedMsg:
		m.logsTab.AddLog("info", "Subscriptions updated")

	case tabs.CustomNodesChangedMsg:
		m.updateSlotIndicators()
		m.updateStatusBar()
		if m.activeSubID == subscription.CustomSubscriptionID {
			m.selectedNode = msg.SelectedNode
			m.nodeMgr.Select(msg.SelectedNode)
			m.statusBar.SetSelectedNode(msg.SelectedNode)
			if msg.SelectedNode != nil {
				cmds = append(cmds, m.persistSelectedNode(m.activeSubID, msg.SelectedNode))
			}
		}

	case tabs.SubsRefreshedMsg:
		prevActive := m.activeSubID
		newActive := m.activeSubID
		if len(msg.Subs) == 0 {
			newActive = ""
		} else {
			valid := false
			for _, sub := range msg.Subs {
				if sub != nil && sub.ID == m.activeSubID {
					valid = true
					break
				}
			}
			if newActive == "" || !valid {
				idx := preferredSubscriptionIndex(msg.Subs)
				if idx >= 0 && idx < len(msg.Subs) && msg.Subs[idx] != nil {
					newActive = msg.Subs[idx].ID
				} else {
					newActive = ""
				}
			}
		}

		if newActive != prevActive && prevActive != "" {
			m.selectedNode = nil
			m.nodeMgr.Select(nil)
			m.statusBar.SetSelectedNode(nil)
		}
		m.activeSubID = newActive
		cmds = append(cmds, m.persistActiveSubscription(m.activeSubID))

		m.subsTab, _ = m.subsTab.Update(msg)
		if m.subsTab != nil {
			m.subsTab.SetActiveSubscription(m.activeSubID)
		}

	case tabs.SettingsSavedMsg:
		if m.subManager != nil {
			m.subManager.SetUpdateViaProxy(m.cfg.Subscription.UpdateViaProxy)
		}

	case tabs.RestartCoreMsg:
		cmds = append(cmds, m.restartSlot0())

	case tabs.RestartAllSlotsMsg:
		cmds = append(cmds, m.restartAllSlots())

	case tabs.RestartSlotMsg:
		cmds = append(cmds, m.restartSlot(msg.SlotID))

	case tabs.SlotToggleRequestMsg:
		cmds = append(cmds, m.toggleConfiguredSlot(msg.SlotID))

	case tabs.SlotDisconnectAllMsg:
		m.disconnectAllSlots()

	case tabs.SlotRestartRequestMsg:
		cmds = append(cmds, m.restartConfiguredSlot(msg.SlotID))

	case tabs.SlotActionResultMsg:
		m.updateSlotIndicators()
		m.updateStatusBar()
		if msg.Err != nil {
			m.logsTab.AddLog("error", msg.Err.Error())
		}

	case DaemonLogMsg:
		m.logsTab.AddLog(msg.Entry.Level.String(), msg.Entry.Message)
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

			// Forward per-slot traffic to Slots tab.
			if m.slotsTab != nil {
				m.slotsTab.SetSlotTraffic(slotID, &tabs.SlotTrafficInfo{
					UpRate:    upRate,
					DownRate:  downRate,
					ConnCount: snap.connCount,
				})
			}

			// Nodes tab remains a simple Slot 0 panel, so only expose Slot 0 there.
			state := m.slotMgr.GetSlotState(slotID)
			if slotID == 0 && state != nil && state.NodeName != "" {
				upstreamLabel := ""
				if state.UpstreamEnabled {
					if ref, ok := m.cfg.GetSlotUpstreamRef(slotID); ok && ref != "" {
						if entry, found := m.cfg.GetUpstreamProxy(ref); found && entry.Label != "" {
							upstreamLabel = entry.Label
						}
					}
				}
				trafficStr := fmt.Sprintf(":%d ↑%s ↓%s",
					state.Port,
					components.FormatRate(upRate),
					components.FormatRate(downRate),
				)
				if upstreamLabel != "" {
					trafficStr += " " + upstreamLabel
				}
				nodeTraffic[state.NodeName] = trafficStr
			}
		}

		// Clean up stale traffic entries
		for slotID := range m.slotTrafficLast {
			if _, ok := msg.snapshots[slotID]; !ok {
				delete(m.slotTrafficLast, slotID)
				if m.slotsTab != nil {
					m.slotsTab.ClearSlotTraffic(slotID)
				}
			}
		}

		if m.statusBar != nil {
			m.statusBar.SetTraffic(totalUp, totalDown, totalUpRate, totalDownRate)
			m.statusBar.SetActiveSlots(m.slotMgr.ActiveSlotCount())
		}

		// Update node traffic display in embedded browsers.
		if m.slotsTab != nil {
			m.slotsTab.SetNodeBrowserTraffic(nodeTraffic)
		}
		if m.subsTab != nil {
			m.subsTab.SetNodeBrowserTraffic(nodeTraffic)
		}

	case slotActionDoneMsg:
		if msg.err != nil {
			m.logsTab.AddLog("error", msg.err.Error())
		}
		// Clear traffic for stopped slots
		if m.slotsTab != nil {
			for i := 0; i < proxy.MaxSlots; i++ {
				if m.slotMgr.GetSlotState(i) == nil {
					m.slotsTab.ClearSlotTraffic(i)
				}
			}
		}
		m.updateSlotIndicators()
		m.updateStatusBar()
	}

	// Update active tab
	var cmd tea.Cmd
	switch m.activeTab {
	case TabSlots:
		wasFullScreen := m.slotsTab.IsFullScreen()
		m.slotsTab, cmd = m.slotsTab.Update(msg)
		if m.slotsTab.IsFullScreen() != wasFullScreen {
			m.updateSizes()
		}
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

	// Hamburger layout: tab bar (top) + content (middle) + status bar (bottom).
	// Full-screen tabs hide both bars and own the entire terminal area.
	//
	// styles.TabBar has BorderBottom(true), so renderTabBar() renders 2 lines:
	//   line 1: tab name text
	//   line 2: bottom border ────
	// Therefore contentHeight = m.height - 2 (tab bar) - 1 (status bar) - 1
	// (safety row) = m.height - 4.
	//
	// The extra safety row matters on terminals that count one more row than
	// they display (or scroll when the bottom-right cell is written): without
	// it the whole frame scrolls up by one line and the tab bar disappears.
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}

	if m.isActiveTabFullScreen() {
		content := m.renderActiveTab()
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			MaxHeight(m.height).
			Render(content)
	}

	// Content area
	var contentView string
	if m.overlayText != "" {
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

	// Pin content to exactly contentHeight lines: pad if short, clip if tall.
	contentView = lipgloss.NewStyle().
		Width(m.width).
		Height(contentHeight).
		MaxHeight(contentHeight).
		Render(contentView)

	m.statusBar.SetWidth(m.width)
	m.updateStatusBar()

	return m.renderTabBar() + "\n" + contentView + "\n" + m.statusBar.View()
}

func (m *Model) renderTabBar() string {
	msgs := i18n.M()
	tabNames := []string{
		"1 " + msgs.Tabs.Slots,
		"2 " + msgs.Tabs.Subscription,
		"3 " + msgs.Tabs.Settings,
	}
	if strings.TrimSpace(tabNames[0]) == "" {
		tabNames[0] = "Slots"
	}

	type renderedTab struct {
		text  string
		width int
	}
	rendered := make([]renderedTab, len(tabNames))
	for i, name := range tabNames {
		var text string
		if Tab(i) == m.activeTab {
			text = styles.ActiveTab.Render(name)
		} else {
			text = styles.InactiveTab.Render(name)
		}
		rendered[i] = renderedTab{text: text, width: lipgloss.Width(text)}
	}

	prefix := styles.HelpText.Render(msgs.Actions.Tab + "： ")
	prefixWidth := lipgloss.Width(prefix)

	maxWidth := m.width
	if maxWidth < 1 {
		maxWidth = 80
	}

	// Check if all tabs fit without scrolling.
	totalWidth := prefixWidth
	for _, r := range rendered {
		totalWidth += r.width
	}
	if totalWidth <= maxWidth {
		tabBar := lipgloss.JoinHorizontal(lipgloss.Top, append([]string{prefix}, func() []string {
			s := make([]string, len(rendered))
			for i, r := range rendered {
				s[i] = r.text
			}
			return s
		}()...)...)
		return styles.TabBar.Width(m.width).Render(tabBar)
	}

	// Tabs don't all fit: scroll to keep active tab visible.
	activeIdx := int(m.activeTab)
	const arrowWidth = 2 // "◀ " or " ▶"

	startIdx := activeIdx
	endIdx := activeIdx
	usedWidth := prefixWidth + rendered[activeIdx].width

	for {
		expanded := false
		if startIdx > 0 {
			need := rendered[startIdx-1].width
			extra := 0
			if startIdx-1 > 0 {
				extra = arrowWidth
			}
			if endIdx < len(rendered)-1 {
				extra = max(extra, arrowWidth)
			}
			if usedWidth+need+extra <= maxWidth {
				startIdx--
				usedWidth += need
				expanded = true
			}
		}
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

func (m *Model) tabHitTest(x int) (Tab, bool) {
	msgs := i18n.M()
	tabNames := []string{
		"1 " + msgs.Tabs.Slots,
		"2 " + msgs.Tabs.Subscription,
		"3 " + msgs.Tabs.Settings,
	}
	if strings.TrimSpace(tabNames[0]) == "" {
		tabNames[0] = "Slots"
	}

	prefix := styles.HelpText.Render(msgs.Actions.Tab + "： ")
	cursor := lipgloss.Width(prefix)

	for i, name := range tabNames {
		var rendered string
		if Tab(i) == m.activeTab {
			rendered = styles.ActiveTab.Render(name)
		} else {
			rendered = styles.InactiveTab.Render(name)
		}
		w := lipgloss.Width(rendered)
		if x >= cursor && x < cursor+w {
			return Tab(i), true
		}
		cursor += w
	}
	return 0, false
}
func (m *Model) renderActiveTab() string {
	switch m.activeTab {
	case TabSlots:
		return m.slotsTab.View()
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
	m.activeTab = Tab((int(m.activeTab) + 1) % 3)
}

func (m *Model) prevTab() {
	m.activeTab = Tab((int(m.activeTab) + 2) % 3)
}

func (m *Model) setTab(t Tab) {
	if t < TabSlots || t > TabSettings {
		return
	}
	m.activeTab = t
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
	// Content height: tab bar is 2 lines (text + BorderBottom) + status bar is
	// 1 line + 1 safety row (see Model.View).
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}
	if m.slotsTab != nil && m.slotsTab.IsFullScreen() {
		m.slotsTab.SetSize(m.width, m.height)
	} else {
		m.slotsTab.SetSize(m.width, contentHeight)
	}
	if m.subsTab != nil && m.subsTab.IsFullScreen() {
		m.subsTab.SetSize(m.width, m.height)
	} else {
		m.subsTab.SetSize(m.width, contentHeight)
	}
	m.settingsTab.SetSize(m.width, contentHeight)
	m.logsTab.SetSize(m.width, contentHeight)
}

// isActiveTabFullScreen reports whether the current tab has requested full-screen
// mode, which hides the tab bar and status bar. Add new tabs here as needed.
func (m *Model) isActiveTabFullScreen() bool {
	return (m.activeTab == TabSlots && m.slotsTab != nil && m.slotsTab.IsFullScreen()) || (m.activeTab == TabSubscriptions && m.subsTab != nil && m.subsTab.IsFullScreen())
}

// -- Message types --

type slotActionDoneMsg struct {
	slotID int
	err    error
}

type slotTrafficMsg struct {
	snapshots map[int]trafficSnapshot
}

func sameSlotSelection(state *proxy.SlotState, subID string, n *subscription.Node) bool {
	if state == nil || n == nil {
		return false
	}
	return state.SubID == subID && state.NodeName == n.Name
}

func (m *Model) startSlotWithMode(slotID int, subID string, node *subscription.Node, mode string) tea.Cmd {

	if node == nil {
		return func() tea.Msg {
			return slotActionDoneMsg{slotID: slotID, err: fmt.Errorf("no node selected")}
		}
	}

	nodeCopy := *node
	return func() tea.Msg {
		nodes := m.subManager.GetNodesForSubscription(subID)
		var targetNode *subscription.Node
		for i := range nodes {
			if sameNode(&nodes[i], &nodeCopy) {
				targetNode = &nodes[i]
				break
			}
		}
		if targetNode == nil {
			return slotActionDoneMsg{slotID: slotID, err: fmt.Errorf("node %q not found for slot %d", nodeCopy.Name, slotID)}
		}

		currentState := m.slotMgr.GetSlotState(slotID)
		if sameSlotSelection(currentState, subID, targetNode) {
			if currentState.Mode == mode {
				return slotActionDoneMsg{slotID: slotID, err: nil}
			}
			err := m.slotMgr.SetSlotMode(slotID, mode)
			return slotActionDoneMsg{slotID: slotID, err: err}
		}
		if currentState != nil {
			_ = m.slotMgr.StopSlot(slotID)
			time.Sleep(300 * time.Millisecond)
		}

		err := m.slotMgr.StartSlotWithMode(slotID, subID, nodes, targetNode, mode)
		return slotActionDoneMsg{slotID: slotID, err: err}
	}
}

func slotNodeRefMatches(ref config.SlotNodeRef, node *subscription.Node) bool {
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

func (m *Model) startConfiguredSlot(slotID int) error {
	if m == nil || m.cfg == nil || m.slotMgr == nil || m.subManager == nil {
		return fmt.Errorf("slot runtime is not ready")
	}
	if _, _, ok := m.cfg.SlotMixedPort(slotID); !ok {
		return fmt.Errorf("slot %d has an invalid manual mixed-port", slotID)
	}

	ref, hasNodeRef := m.cfg.GetSlotNodeRef(slotID)
	_, hasUpstream := m.cfg.EffectiveUpstreamForSlot(slotID)

	var nodes []subscription.Node
	var targetNode *subscription.Node
	subID := ""

	if hasNodeRef && strings.TrimSpace(ref.SubID) != "" {
		subID = ref.SubID
		nodes = m.subManager.GetNodesForSubscription(ref.SubID)
		for i := range nodes {
			if slotNodeRefMatches(ref, &nodes[i]) {
				targetNode = &nodes[i]
				break
			}
		}
		if targetNode == nil && !hasUpstream {
			return fmt.Errorf("slot %d proxy node no longer exists", slotID)
		}
	}

	return m.slotMgr.StartSlotWithMode(slotID, subID, nodes, targetNode, m.cfg.GetSlotStartMode(slotID))
}

func (m *Model) toggleConfiguredSlot(slotID int) tea.Cmd {
	return func() tea.Msg {
		if state := m.slotMgr.GetSlotState(slotID); state != nil {
			err := m.slotMgr.StopSlot(slotID)
			return tabs.SlotActionResultMsg{SlotID: slotID, Action: "toggle", Running: false, Err: err}
		}
		err := m.startConfiguredSlot(slotID)
		return tabs.SlotActionResultMsg{SlotID: slotID, Action: "toggle", Running: err == nil, Err: err}
	}
}

func (m *Model) restartConfiguredSlot(slotID int) tea.Cmd {
	return func() tea.Msg {
		if state := m.slotMgr.GetSlotState(slotID); state != nil {
			_ = m.slotMgr.StopSlot(slotID)
			time.Sleep(500 * time.Millisecond)
		}
		err := m.startConfiguredSlot(slotID)
		return tabs.SlotActionResultMsg{SlotID: slotID, Action: "restart", Running: err == nil, Err: err}
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
			up, down, conns, err := m.slotMgr.GetTrafficAndConnCountForSlot(s.SlotID)
			if err != nil {
				continue
			}
			snapshots[s.SlotID] = trafficSnapshot{at: now, uploadTotal: up, downloadTotal: down, connCount: conns}
		}
		return slotTrafficMsg{snapshots: snapshots}
	})
}

// -- Log listeners --

func (m *Model) listenSlotLogs() tea.Cmd {
	return func() tea.Msg {
		for entry := range m.daemonLogCh {
			return DaemonLogMsg{Entry: entry}
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
		if m.slotMgr.GetSlotState(0) != nil {
			return m.restartSlot(0)()
		}

		nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
		if len(nodes) == 0 {
			return overlayMsg{text: i18n.M().Nodes.NoNodes}
		}
		if m.selectedNode == nil {
			msg := i18n.M().Errors.NoNodeSelected
			if msg == "" {
				msg = "No node selected"
			}
			return overlayMsg{text: fmt.Sprintf(msg, i18n.M().Tabs.Subscription)}
		}

		_ = m.slotMgr.StopSlot(0)
		time.Sleep(500 * time.Millisecond)

		err := m.slotMgr.StartSlotWithMode(0, m.activeSubID, nodes, m.selectedNode, "rule")
		return slotActionDoneMsg{slotID: 0, err: err}
	}
}

// -- Restart all slots (after port change) --

func (m *Model) restartSlot(slotID int) tea.Cmd {
	return func() tea.Msg {
		state := m.slotMgr.GetSlotState(slotID)
		if state == nil {
			return slotActionDoneMsg{slotID: slotID, err: nil}
		}

		nodes := m.subManager.GetNodesForSubscription(state.SubID)
		if len(nodes) == 0 {
			return slotActionDoneMsg{slotID: slotID, err: fmt.Errorf("no nodes found for slot %d subscription %q", slotID, state.SubID)}
		}

		var targetNode *subscription.Node
		for i := range nodes {
			if nodes[i].Name == state.NodeName {
				targetNode = &nodes[i]
				break
			}
		}
		if targetNode == nil {
			return slotActionDoneMsg{slotID: slotID, err: fmt.Errorf("node %q not found for slot %d restart", state.NodeName, slotID)}
		}

		_ = m.slotMgr.StopSlot(slotID)
		time.Sleep(500 * time.Millisecond)
		err := m.slotMgr.StartSlotWithMode(slotID, state.SubID, nodes, targetNode, state.Mode)
		return slotActionDoneMsg{slotID: slotID, err: err}
	}
}

func (m *Model) restartAllSlots() tea.Cmd {
	return func() tea.Msg {
		active := m.slotMgr.ActiveSlots()
		if len(active) == 0 {
			return slotActionDoneMsg{slotID: -1, err: nil}
		}
		// Record what each slot was running
		type slotInfo struct {
			slotID   int
			nodeName string
			subID    string
			mode     string
		}
		var infos []slotInfo
		for _, s := range active {
			infos = append(infos, slotInfo{s.SlotID, s.NodeName, s.SubID, s.Mode})
			_ = m.slotMgr.StopSlot(s.SlotID)
		}
		time.Sleep(500 * time.Millisecond)
		for _, info := range infos {
			nodes := m.subManager.GetNodesForSubscription(info.subID)
			var targetNode *subscription.Node
			for i := range nodes {
				if nodes[i].Name == info.nodeName {
					targetNode = &nodes[i]
					break
				}
			}
			if targetNode != nil {
				_ = m.slotMgr.StartSlotWithMode(info.slotID, info.subID, nodes, targetNode, info.mode)
			}
		}
		return slotActionDoneMsg{slotID: -1, err: nil}
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
	indicators := make(map[string]components.SlotIndicator)

	if s := m.slotMgr.GetSlotState(0); s != nil && s.NodeName != "" && s.Mode != "" {
		indicators[s.NodeName] = components.SlotIndicator{
			SlotID: 0,
			Icon:   slotModeIcon(s.Mode),
		}
	}

	if m.slotsTab != nil {
		m.slotsTab.SetNodeBrowserIndicators(indicators)
	}
	if m.subsTab != nil {
		m.subsTab.SetNodeBrowserIndicators(indicators)
	}
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

func (m *Model) disconnectAllSlots() {
	if m.slotMgr != nil {
		m.slotMgr.StopAll()
	}
	m.slotTrafficLast = make(map[int]trafficSnapshot)
	if m.statusBar != nil {
		m.statusBar.ClearTraffic()
		m.statusBar.SetActiveSlots(0)
	}
}
