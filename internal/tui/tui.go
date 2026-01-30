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
	activeSubID       string
	selectedNode      *subscription.Node
	connectedNodeName string
	proxyMode         ProxyMode
	overlayText       string
	overlayID         int

	// Components
	statusBar *components.StatusBar

	// Tabs
	nodesTab    *tabs.NodesTab
	subsTab     *tabs.SubscriptionsTab
	settingsTab *tabs.SettingsTab
	logsTab     *tabs.LogsTab
}

// New creates a new TUI model
func New(cfg *config.Config, subMgr *subscription.Manager, nodeMgr *node.Manager, proxyCtrl *proxy.Controller) *Model {
	m := &Model{
		cfg:         cfg,
		subManager:  subMgr,
		nodeMgr:     nodeMgr,
		proxyCtrl:   proxyCtrl,
		proxyMode:   ProxyModeDisconnect,
		statusBar:   components.NewStatusBar(),
		nodesTab:    tabs.NewNodesTab(nodeMgr),
		subsTab:     tabs.NewSubscriptionsTab(subMgr),
		settingsTab: tabs.NewSettingsTab(cfg),
		logsTab:     tabs.NewLogsTab(),
	}

	if m.nodesTab != nil {
		m.nodesTab.SetServerDisplay(cfg.UI.ServerDisplay)
	}

	return m
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
	)
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.updateSizes()

	case tea.KeyMsg:
		// If a center prompt is visible, hide it on any keypress.
		if m.overlayText != "" {
			m.overlayText = ""
		}

		// Global keys
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			m.quitting = true
			if m.proxyCtrl != nil {
				_ = m.proxyCtrl.Stop()
			}
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
				m.logsTab.AddLog("info", fmt.Sprintf("Selected node: %s", focused.Name))
			}

			if m.proxyCtrl.GetStatus() == proxy.StatusRunning {
				// If the user chose a different node, keep the current mode and just switch node.
				// Only cycle mode when pressing Enter again on the same node.
				if focused != nil && !sameAsPrev {
					return m, m.switchNode(focused.Name)
				}
				return m, m.cycleProxyMode()
			}

			// When disconnected, pressing Enter always connects (Rule) for the focused node.
			return m, m.startProxy()
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

	case tabs.NodeSelectedMsg:
		m.selectedNode = msg.Node
		m.statusBar.SetSelectedNode(msg.Node)
		m.logsTab.AddLog("info", fmt.Sprintf("Selected node: %s", msg.Node.Name))
		// If proxy is running, just switch node via API; otherwise start it
		if m.proxyCtrl.GetStatus() == proxy.StatusRunning {
			cmds = append(cmds, m.switchNode(msg.Node.Name))
		} else {
			cmds = append(cmds, m.startProxy())
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

		if newActive != prevActive {
			// If the active subscription changes (including deletion), disconnect old nodes/proxy.
			m.disconnectProxy()
			m.selectedNode = nil
			m.nodeMgr.Select(nil)
			m.statusBar.SetSelectedNode(nil)
		}
		m.activeSubID = newActive

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
	m.statusBar.SetMixedPort(m.cfg.Proxy.MixedPort)
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
	m.activeTab = Tab((int(m.activeTab) + 1) % 4)
}

func (m *Model) prevTab() {
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

		err := m.proxyCtrl.Start(nodes, m.selectedNode)
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
				if err := m.proxyCtrl.Start(nodes, selectedNode); err != nil {
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

		err := m.proxyCtrl.Start(nodes, m.selectedNode)
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

func (m *Model) switchNode(nodeName string) tea.Cmd {
	return func() tea.Msg {
		err := m.proxyCtrl.SwitchNode(nodeName)
		if err != nil {
			return proxyStatusMsg{status: proxy.StatusError, mode: m.proxyMode, err: err}
		}
		if m.proxyMode == ProxyModeGlobal {
			if err := m.proxyCtrl.SwitchGroup("GLOBAL", nodeName); err != nil {
				return proxyStatusMsg{status: proxy.StatusRunning, mode: m.proxyMode, err: err}
			}
		}
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
