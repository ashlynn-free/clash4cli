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

	trafficUploadTotal   int64
	trafficDownloadTotal int64
	trafficUploadRate    int64
	trafficDownloadRate  int64
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

// SetTraffic sets traffic totals and rates (bytes per second).
func (s *StatusBar) SetTraffic(uploadTotal, downloadTotal, uploadRate, downloadRate int64) {
	s.trafficUploadTotal = uploadTotal
	s.trafficDownloadTotal = downloadTotal
	s.trafficUploadRate = uploadRate
	s.trafficDownloadRate = downloadRate
}

func (s *StatusBar) ClearTraffic() {
	s.trafficUploadTotal = 0
	s.trafficDownloadTotal = 0
	s.trafficUploadRate = 0
	s.trafficDownloadRate = 0
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

	// Port info - mixed port for both SOCKS5 and HTTP
	portInfo := fmt.Sprintf("Mixed: %d", s.mixedPort)

	modeInfo := styles.HelpKey.Render("Enter") +
		styles.HelpText.Render(" to switch")

	// Traffic info (only when running)
	traffic := ""
	if s.proxyStatus == proxy.StatusRunning {
		traffic = fmt.Sprintf("↑%s ↓%s", formatRate(s.trafficUploadRate), formatRate(s.trafficDownloadRate))
		if s.width >= 120 {
			traffic += fmt.Sprintf(" · Σ↑%s Σ↓%s", formatBytes(s.trafficUploadTotal), formatBytes(s.trafficDownloadTotal))
		}
	}

	rightParts := []string{portInfo, styles.HelpKey.Render("q") + styles.HelpText.Render("uit")}
	if traffic != "" {
		rightParts = append([]string{traffic}, rightParts...)
	}
	right := ""
	for _, part := range rightParts {
		if part == "" {
			continue
		}
		if right != "" {
			right += "  "
		}
		right += part
	}

	// Node info (truncate to fit)
	nodeText := "-"
	if s.selectedNode != nil {
		nodeText = s.selectedNode.Name
	}
	fixedLeft := status + "  " + modeInfo + "  " + styles.Muted.Render("Node: ")
	availableNodeW := s.width - lipgloss.Width(fixedLeft) - lipgloss.Width(right) - 4
	if availableNodeW < 1 {
		availableNodeW = 1
	}
	nodeText = runewidth.Truncate(nodeText, availableNodeW, "…")

	left := fixedLeft + nodeText

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

func formatRate(bytesPerSec int64) string {
	if bytesPerSec < 0 {
		return "-"
	}
	if bytesPerSec == 0 {
		return "0B/s"
	}

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.1fGB/s", float64(bytesPerSec)/float64(GB))
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.1fMB/s", float64(bytesPerSec)/float64(MB))
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.1fKB/s", float64(bytesPerSec)/float64(KB))
	default:
		return fmt.Sprintf("%dB/s", bytesPerSec)
	}
}

func formatBytes(b int64) string {
	if b < 0 {
		return "-"
	}

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case b >= TB:
		return fmt.Sprintf("%.2fTB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.2fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
