package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/styles"
)

// StatusBar represents the status bar component
type StatusBar struct {
	width        int
	proxyStatus  proxy.Status
	selectedNode *subscription.Node
	mixedPort    int
	proxyMode    string
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	return &StatusBar{
		proxyStatus: proxy.StatusStopped,
		mixedPort:   17890,
		proxyMode:   "Disconnect",
	}
}

// SetWidth sets the status bar width
func (s *StatusBar) SetWidth(width int) {
	s.width = width
}

// SetProxyStatus sets the proxy status
func (s *StatusBar) SetProxyStatus(status proxy.Status) {
	s.proxyStatus = status
}

// SetSelectedNode sets the selected node
func (s *StatusBar) SetSelectedNode(node *subscription.Node) {
	s.selectedNode = node
}

// SetMixedPort sets the mixed port
func (s *StatusBar) SetMixedPort(port int) {
	s.mixedPort = port
}

// SetProxyMode sets the current proxy mode label (e.g. Rule/Global/Disconnect).
func (s *StatusBar) SetProxyMode(mode string) {
	s.proxyMode = mode
}

// View renders the status bar
func (s *StatusBar) View() string {
	m := i18n.M()

	// Status indicator
	var statusText string
	var statusStyle lipgloss.Style

	switch s.proxyStatus {
	case proxy.StatusRunning:
		switch s.proxyMode {
		case "Rule":
			if m.Status.ConnectedRule != "" {
				statusText = m.Status.ConnectedRule
			} else {
				statusText = m.Status.Connected + " (Rule)"
			}
		case "Global":
			if m.Status.ConnectedGlobal != "" {
				statusText = m.Status.ConnectedGlobal
			} else {
				statusText = m.Status.Connected + " (Global)"
			}
		default:
			statusText = m.Status.Connected
		}
		statusStyle = styles.StatusConnected
	case proxy.StatusStarting:
		statusText = m.Status.Connecting
		statusStyle = styles.StatusConnected
	case proxy.StatusError:
		statusText = m.Status.Error
		statusStyle = styles.StatusError
	default:
		statusText = m.Status.Disconnected
		statusStyle = styles.StatusDisconnected
	}

	status := statusStyle.Render("● " + statusText)

	// Node info
	var nodeText string
	if s.selectedNode != nil {
		nodeText = runewidth.Truncate(s.selectedNode.Name, 30, "...")
	} else {
		nodeText = "-"
	}

	// Port info - mixed port for both SOCKS5 and HTTP
	portInfo := fmt.Sprintf("Mixed: %d", s.mixedPort)

	modeInfo := styles.HelpKey.Render("Enter") +
		styles.HelpText.Render(" to switch")

	// Build status bar
	left := status + "  " + modeInfo + "  " + styles.Muted.Render("Node: ") + nodeText
	right := portInfo + "  " + styles.HelpKey.Render("q") + styles.HelpText.Render(" quit")

	// Calculate spacing
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	spacing := s.width - leftWidth - rightWidth - 4

	if spacing < 0 {
		spacing = 1
	}

	spacer := lipgloss.NewStyle().Width(spacing).Render("")

	return styles.StatusBar.Width(s.width).Render(
		left + spacer + right,
	)
}
