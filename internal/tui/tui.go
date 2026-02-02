package tui

import (
	"fmt"
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

type ProxyMode int

const (
	ProxyModeRule ProxyMode = iota
	ProxyModeGlobal
	ProxyModeDisconnect
)

func (pm ProxyMode) String() string {
	switch pm {
	case ProxyModeRule:
		return "Rule"
	case ProxyModeGlobal:
		return "Global"
	case ProxyModeDisconnect:
		return "Disconnect"
	default:
		return "Unknown"
	}
}

func (pm ProxyMode) next() ProxyMode {
	return ProxyMode((int(pm) + 1) % 3)
}

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
	proxyCtrl  *proxy.Controller

	// UI State
	activeTab         Tab
	width             int
	height            int
	ready             bool
	quitting          bool
	version           string
	startupWarning    string
	showSplash        bool
	activeSubID       string
	selectedNode      *subscription.Node
	connectedNodeName string
	proxyMode         ProxyMode
	overlayText       string
	overlayID         int

	trafficLastAt            time.Time
	trafficLastUploadTotal   int64
	trafficLastDownloadTotal int64

	// Components
	statusBar *components.StatusBar

	// Tabs
	nodesTab    *tabs.NodesTab
	subsTab     *tabs.SubscriptionsTab
	settingsTab *tabs.SettingsTab
	logsTab     *tabs.LogsTab
}

// New creates a new TUI model
func New(cfg *config.Config, subMgr *subscription.Manager, nodeMgr *node.Manager, proxyCtrl *proxy.Controller, version string, startupWarning string) *Model {
	m := &Model{
		cfg:            cfg,
		subManager:     subMgr,
		nodeMgr:        nodeMgr,
		proxyCtrl:      proxyCtrl,
		proxyMode:      ProxyModeDisconnect,
		statusBar:      components.NewStatusBar(),
		nodesTab:       tabs.NewNodesTab(nodeMgr),
		subsTab:        tabs.NewSubscriptionsTab(subMgr),
		settingsTab:    tabs.NewSettingsTab(cfg),
		logsTab:        tabs.NewLogsTab(),
		version:        strings.TrimSpace(version),
		startupWarning: strings.TrimSpace(startupWarning),
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
	if m == nil || m.proxyCtrl == nil || m.subManager == nil || m.nodeMgr == nil {
		return
	}
	if m.proxyCtrl.GetStatus() != proxy.StatusRunning {
		return
	}

	lock, err := proxy.ReadLockFile()
	if err != nil || lock == nil {
		return
	}

	subID, node := inferSubscriptionAndNode(m.subManager, lock)
	if subID != "" {
		m.activeSubID = subID
		if m.nodesTab != nil {
			m.nodesTab.SetSubscriptionID(subID)
		}
		if m.subsTab != nil {
			m.subsTab.SetActiveSubscription(subID)
		}
	}

	if node == nil && lock.Node != "" {
		// Best-effort: show the current node in UI even if we cannot find it in subscriptions.
		node = &subscription.Node{
			Name:   lock.Node,
			Type:   lock.NodeType,
			Server: lock.NodeServer,
			Port:   lock.NodePort,
		}
	}

	if node != nil {
		m.selectedNode = node
		m.nodeMgr.Select(node)
		m.statusBar.SetSelectedNode(node)
		if m.nodesTab != nil {
			m.nodesTab.SetPreselect(node)
		}
	}

	if lock.Node != "" {
		m.connectedNodeName = lock.Node
		m.proxyMode = ProxyModeRule
		if m.nodesTab != nil {
			m.nodesTab.SetConnectionIndicator(lock.Node, proxyModeIcon(m.proxyMode))
		}
	}

	// Backfill lock metadata for future restarts (best-effort).
	if lock.SubID == "" && subID != "" {
		m.proxyCtrl.UpdateLockState(subID, node)
	}
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
		m.listenProxyLogs(),
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
			// Use the highlighted node as "selected" and cycle proxy mode:
			// Disconnect -> Rule -> Global -> Disconnect.
			nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
			if len(nodes) == 0 {
				return m, func() tea.Msg { return overlayMsg{text: i18n.M().Nodes.NoNodes} }
			}

			var focused *subscription.Node
			if m.nodesTab != nil {
				focused = m.nodesTab.GetSelectedNode()
			}
			if focused == nil {
				focused = &nodes[0]
			}

			sameAsPrev := sameNode(m.selectedNode, focused)
			if focused != nil && !sameAsPrev {
				m.selectedNode = focused
				m.nodeMgr.Select(focused)
				m.statusBar.SetSelectedNode(focused)
				if m.nodesTab != nil {
					m.nodesTab.SetPreselect(focused)
				}
				m.logsTab.AddLog("info", fmt.Sprintf("Selected node: %s", focused.Name))
			}
			persistCmd := m.persistSelectedNode(m.activeSubID, focused)

			if m.proxyCtrl.GetStatus() == proxy.StatusRunning {
				// If the user chose a different node, keep the current mode and just switch node.
				// Only cycle mode when pressing Enter again on the same node.
				if focused != nil && !sameAsPrev {
					return m, tea.Batch(persistCmd, m.switchNode(focused))
				}
				return m, tea.Batch(persistCmd, m.cycleProxyMode())
			}

			// When disconnected, pressing Enter always connects (Rule) for the focused node.
			return m, tea.Batch(persistCmd, m.startProxy())
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
		// If proxy is running, just switch node via API; otherwise start it
		if m.proxyCtrl.GetStatus() == proxy.StatusRunning {
			cmds = append(cmds, tea.Batch(m.persistSelectedNode(m.activeSubID, msg.Node), m.switchNode(msg.Node)))
		} else {
			cmds = append(cmds, tea.Batch(m.persistSelectedNode(m.activeSubID, msg.Node), m.startProxy()))
		}

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

	case tabs.SubscriptionSelectedMsg:
		if msg.ID != "" && msg.ID != m.activeSubID {
			// Switching subscription should disconnect the old subscription's nodes/proxy.
			m.disconnectProxy()
			m.selectedNode = nil
			m.nodeMgr.Select(nil)
			m.statusBar.SetSelectedNode(nil)
		}

		m.activeSubID = msg.ID
		cmds = append(cmds, m.persistActiveSubscription(m.activeSubID))
		if m.subsTab != nil {
			m.subsTab.SetActiveSubscription(m.activeSubID)
		}
		if m.nodesTab != nil {
			m.nodesTab.SetSubscriptionID(m.activeSubID)
			cmds = append(cmds, m.nodesTab.Init())
		}

	case tabs.NodesRefreshedMsg:
		// Always forward NodesRefreshedMsg to nodes tab regardless of active tab
		m.nodesTab, _ = m.nodesTab.Update(msg)

	case tabs.SubsRefreshedMsg:
		prevActive := m.activeSubID

		// Ensure we have an active subscription (for Nodes filtering).
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
				newActive = msg.Subs[0].ID
			}
		}

		if newActive != prevActive && prevActive != "" {
			// If the active subscription changes (including deletion), disconnect old nodes/proxy.
			// NOTE: On startup, prevActive may be empty while a background proxy core is running;
			// we should not kill it just because we picked a default subscription.
			m.disconnectProxy()
			m.selectedNode = nil
			m.nodeMgr.Select(nil)
			m.statusBar.SetSelectedNode(nil)
		}
		m.activeSubID = newActive
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
		cmds = append(cmds, m.restartProxyCore())

	case proxy.LogEntry:
		// Forward to logs tab
		var cmd tea.Cmd
		m.logsTab, cmd = m.logsTab.Update(msg)
		cmds = append(cmds, cmd)
		// Continue listening for more logs
		cmds = append(cmds, m.listenProxyLogs())

	case logger.Entry:
		// Forward app logs to logs tab
		m.logsTab.AddLog(msg.Level.String(), msg.Message)
		// Continue listening for more logs
		cmds = append(cmds, m.listenAppLogs())

	case trafficMsg:
		// Keep polling.
		cmds = append(cmds, m.pollTraffic())

		if !msg.running {
			m.resetTraffic()
			break
		}
		if msg.err != nil {
			// Best-effort: keep the last known values; don't spam logs.
			break
		}

		uploadRate := int64(0)
		downloadRate := int64(0)
		if !m.trafficLastAt.IsZero() && msg.at.After(m.trafficLastAt) {
			dt := msg.at.Sub(m.trafficLastAt).Seconds()
			if dt > 0 && msg.uploadTotal >= m.trafficLastUploadTotal && msg.downloadTotal >= m.trafficLastDownloadTotal {
				uploadRate = int64(float64(msg.uploadTotal-m.trafficLastUploadTotal) / dt)
				downloadRate = int64(float64(msg.downloadTotal-m.trafficLastDownloadTotal) / dt)
			}
		}

		// If the core restarted and counters reset, treat it as a baseline reset.
		if msg.uploadTotal < m.trafficLastUploadTotal || msg.downloadTotal < m.trafficLastDownloadTotal {
			uploadRate = 0
			downloadRate = 0
		}

		m.trafficLastAt = msg.at
		m.trafficLastUploadTotal = msg.uploadTotal
		m.trafficLastDownloadTotal = msg.downloadTotal
		if m.statusBar != nil {
			m.statusBar.SetTraffic(msg.uploadTotal, msg.downloadTotal, uploadRate, downloadRate)
		}

	case proxyStatusMsg:
		m.statusBar.SetProxyStatus(msg.status)
		m.proxyMode = msg.mode
		if msg.err != nil {
			m.logsTab.AddLog("error", msg.err.Error())
		}

		// Track the currently connected node (best-effort).
		if msg.status == proxy.StatusStopped || msg.mode == ProxyModeDisconnect {
			m.connectedNodeName = ""
		}
		if msg.connectedNodeName != "" {
			m.connectedNodeName = msg.connectedNodeName
		}
		if m.proxyCtrl.GetStatus() != proxy.StatusRunning || m.proxyMode == ProxyModeDisconnect {
			m.connectedNodeName = ""
		}

		if msg.status != proxy.StatusRunning || msg.mode == ProxyModeDisconnect {
			m.resetTraffic()
		}

		// Update node list indicator.
		if m.nodesTab != nil {
			if m.connectedNodeName == "" {
				m.nodesTab.SetConnectionIndicator("", "")
			} else {
				m.nodesTab.SetConnectionIndicator(m.connectedNodeName, proxyModeIcon(m.proxyMode))
			}
		}
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
	contentHeight := m.height - 4 // Tab bar + status bar
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
	content.WriteString(lipgloss.NewStyle().Height(contentHeight).Render(contentView))
	content.WriteString("\n")

	// Status bar
	m.statusBar.SetWidth(m.width)
	m.statusBar.SetProxyStatus(m.proxyCtrl.GetStatus())
	if m.proxyCtrl != nil {
		m.statusBar.SetMixedPort(m.proxyCtrl.CurrentPort())
	} else {
		m.statusBar.SetMixedPort(m.cfg.Proxy.MixedPort)
	}
	m.statusBar.SetProxyMode(m.proxyMode.String())
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

func (m *Model) renderActiveTab() string {
	switch m.activeTab {
	case TabNodes:
		return m.nodesTab.View()
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
	contentHeight := m.height - 4
	m.nodesTab.SetSize(m.width, contentHeight)
	m.subsTab.SetSize(m.width, contentHeight)
	m.settingsTab.SetSize(m.width, contentHeight)
	m.logsTab.SetSize(m.width, contentHeight)
}

type proxyStatusMsg struct {
	status            proxy.Status
	mode              ProxyMode
	err               error
	connectedNodeName string
}

type trafficMsg struct {
	at            time.Time
	running       bool
	uploadTotal   int64
	downloadTotal int64
	err           error
}

func (m *Model) pollTraffic() tea.Cmd {
	const interval = 1 * time.Second
	return tea.Tick(interval, func(now time.Time) tea.Msg {
		if m == nil || m.proxyCtrl == nil {
			return trafficMsg{at: now, running: false}
		}
		if m.proxyCtrl.GetStatus() != proxy.StatusRunning {
			return trafficMsg{at: now, running: false}
		}
		up, down, err := m.proxyCtrl.GetTrafficTotals()
		if err != nil {
			return trafficMsg{at: now, running: true, err: err}
		}
		return trafficMsg{at: now, running: true, uploadTotal: up, downloadTotal: down}
	})
}

func (m *Model) resetTraffic() {
	m.trafficLastAt = time.Time{}
	m.trafficLastUploadTotal = 0
	m.trafficLastDownloadTotal = 0
	if m.statusBar != nil {
		m.statusBar.ClearTraffic()
	}
}

func (m *Model) toggleProxy() tea.Cmd {
	return func() tea.Msg {
		status := m.proxyCtrl.GetStatus()

		if status == proxy.StatusRunning {
			err := m.proxyCtrl.Stop()
			return proxyStatusMsg{status: proxy.StatusStopped, mode: ProxyModeDisconnect, err: err}
		}

		// Start proxy
		nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
		if len(nodes) == 0 {
			return proxyStatusMsg{
				status: proxy.StatusError,
				mode:   m.proxyMode,
				err:    fmt.Errorf("no nodes available"),
			}
		}
		if m.selectedNode == nil {
			return proxyStatusMsg{
				status: proxy.StatusError,
				mode:   m.proxyMode,
				err:    fmt.Errorf("no node selected"),
			}
		}

		err := m.proxyCtrl.Start(m.activeSubID, nodes, m.selectedNode)
		if err != nil {
			return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: err}
		}

		nodeName := ""
		if m.selectedNode != nil {
			nodeName = m.selectedNode.Name
		}
		return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeRule, connectedNodeName: nodeName}
	}
}

func (m *Model) cycleProxyMode() tea.Cmd {
	nextMode := m.proxyMode.next()

	return func() tea.Msg {
		var connectedNodeName string

		switch nextMode {
		case ProxyModeDisconnect:
			err := m.proxyCtrl.Stop()
			return proxyStatusMsg{status: proxy.StatusStopped, mode: ProxyModeDisconnect, err: err}

		case ProxyModeRule, ProxyModeGlobal:
			// Ensure we have something to start with if proxy isn't running.
			nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
			if len(nodes) == 0 {
				return overlayMsg{text: i18n.M().Nodes.NoNodes}
			}
			selectedNode := m.selectedNode
			if selectedNode == nil {
				selectedNode = &nodes[0]
			}

			wasRunning := m.proxyCtrl.GetStatus() == proxy.StatusRunning
			if !wasRunning {
				if err := m.proxyCtrl.Start(m.activeSubID, nodes, selectedNode); err != nil {
					return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: err}
				}
				connectedNodeName = selectedNode.Name
			}

			// Switch mihomo mode (rule/global). If we just started, the default is rule.
			var target string
			switch nextMode {
			case ProxyModeRule:
				target = "rule"
			case ProxyModeGlobal:
				target = "global"
			}

			if target != "" {
				if err := m.proxyCtrl.SetMode(target); err != nil {
					// If we just started and target is rule, we're already in rule mode.
					if !wasRunning && nextMode == ProxyModeRule {
						return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeRule, connectedNodeName: connectedNodeName}
					}
					return proxyStatusMsg{status: proxy.StatusRunning, mode: m.proxyMode, err: err, connectedNodeName: connectedNodeName}
				}
			}

			// In global mode, also try to align the GLOBAL group to the selected node.
			// (Best-effort: even if this fails, the mode change itself may still succeed.)
			if nextMode == ProxyModeGlobal {
				if err := m.proxyCtrl.SwitchGroup("GLOBAL", selectedNode.Name); err != nil {
					return proxyStatusMsg{status: proxy.StatusRunning, mode: nextMode, err: err, connectedNodeName: connectedNodeName}
				}
				connectedNodeName = selectedNode.Name
			}

			return proxyStatusMsg{status: proxy.StatusRunning, mode: nextMode, connectedNodeName: connectedNodeName}
		default:
			return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: fmt.Errorf("unknown mode")}
		}
	}
}

func (m *Model) listenProxyLogs() tea.Cmd {
	return func() tea.Msg {
		logCh := m.proxyCtrl.LogChannel()
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

// startProxy starts or restarts the proxy with current nodes and selected node
func (m *Model) startProxy() tea.Cmd {
	return func() tea.Msg {
		// Stop existing proxy if running
		status := m.proxyCtrl.GetStatus()
		if status == proxy.StatusRunning || status == proxy.StatusStarting {
			_ = m.proxyCtrl.Stop()
			// Wait a bit for the process to exit
			time.Sleep(500 * time.Millisecond)
		}

		nodes := m.subManager.GetNodesForSubscription(m.activeSubID)
		if len(nodes) == 0 {
			return proxyStatusMsg{
				status: proxy.StatusError,
				mode:   m.proxyMode,
				err:    fmt.Errorf("no nodes available"),
			}
		}
		if m.selectedNode == nil {
			return proxyStatusMsg{
				status: proxy.StatusError,
				mode:   m.proxyMode,
				err:    fmt.Errorf("no node selected"),
			}
		}

		err := m.proxyCtrl.Start(m.activeSubID, nodes, m.selectedNode)
		if err != nil {
			return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: err}
		}

		nodeName := ""
		if m.selectedNode != nil {
			nodeName = m.selectedNode.Name
		}
		return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeRule, connectedNodeName: nodeName}
	}
}

func (m *Model) restartProxyCore() tea.Cmd {
	desiredMode := m.proxyMode
	if desiredMode == ProxyModeDisconnect {
		desiredMode = ProxyModeRule
	}

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

		// Best-effort stop, then start with the same selected node.
		_ = m.proxyCtrl.Stop()
		time.Sleep(500 * time.Millisecond)

		if err := m.proxyCtrl.Start(m.activeSubID, nodes, m.selectedNode); err != nil {
			return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: err}
		}

		nodeName := m.selectedNode.Name

		// Restore mode if needed (Start defaults to rule).
		if desiredMode == ProxyModeGlobal {
			var lastErr error
			for i := 0; i < 50; i++ {
				if err := m.proxyCtrl.SetMode("global"); err == nil {
					lastErr = nil
					break
				} else {
					lastErr = err
				}
				time.Sleep(100 * time.Millisecond)
			}
			if lastErr != nil {
				return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeRule, err: lastErr, connectedNodeName: nodeName}
			}
			if err := m.proxyCtrl.SwitchGroup("GLOBAL", nodeName); err != nil {
				return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeGlobal, err: err, connectedNodeName: nodeName}
			}
			return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeGlobal, connectedNodeName: nodeName}
		}

		return proxyStatusMsg{status: proxy.StatusRunning, mode: ProxyModeRule, connectedNodeName: nodeName}
	}
}

func (m *Model) switchNode(node *subscription.Node) tea.Cmd {
	if node == nil {
		return nil
	}
	nodeSnapshot := *node
	subID := m.activeSubID
	return func() tea.Msg {
		nodeName := nodeSnapshot.Name
		err := m.proxyCtrl.SwitchNode(nodeName)
		if err != nil {
			return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: err}
		}
		if m.proxyMode == ProxyModeGlobal {
			if err := m.proxyCtrl.SwitchGroup("GLOBAL", nodeName); err != nil {
				return proxyStatusMsg{status: proxy.StatusRunning, mode: m.proxyMode, err: err}
			}
		}
		m.proxyCtrl.UpdateLockState(subID, &nodeSnapshot)
		return proxyStatusMsg{status: proxy.StatusRunning, mode: m.proxyMode, connectedNodeName: nodeName}
	}
}

func proxyModeIcon(mode ProxyMode) string {
	switch mode {
	case ProxyModeRule:
		return "📜"
	case ProxyModeGlobal:
		return "🌐"
	default:
		return ""
	}
}

func (m *Model) disconnectProxy() {
	if m.proxyCtrl != nil {
		_ = m.proxyCtrl.Stop()
	}
	m.proxyMode = ProxyModeDisconnect
	m.connectedNodeName = ""
	if m.nodesTab != nil {
		m.nodesTab.SetConnectionIndicator("", "")
	}
}
