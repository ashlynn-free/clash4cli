package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/tui/styles"
)

// StatusBar represents the status bar component
type StatusBar struct {
	width       int
	activeSlots int

	// Fields kept for setter compatibility (not displayed).
	proxyStatus proxy.Status
	mixedPort   int
	proxyMode   string
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

// SetSelectedNode is a no-op kept for API compatibility.
func (s *StatusBar) SetSelectedNode(node interface{}) {
}

// SetMixedPort sets the mixed port
func (s *StatusBar) SetMixedPort(port int) {
	s.mixedPort = port
}

// SetProxyMode sets the current proxy mode label (e.g. Rule/Global/Disconnect).
func (s *StatusBar) SetProxyMode(mode string) {
	s.proxyMode = mode
}

// SetActiveSlots sets the number of active proxy slots.
func (s *StatusBar) SetActiveSlots(count int) {
	s.activeSlots = count
}

// SetTraffic is a no-op; traffic is shown per-node in the nodes table.
func (s *StatusBar) SetTraffic(uploadTotal, downloadTotal, uploadRate, downloadRate int64) {}

// ClearTraffic is a no-op.
func (s *StatusBar) ClearTraffic() {}

// View renders the status bar
func (s *StatusBar) View() string {
	// Left: instance count
	slotsText := fmt.Sprintf("%d/10 instances", s.activeSlots)
	if s.activeSlots > 0 {
		slotsText = styles.StatusConnected.Render(slotsText)
	} else {
		slotsText = styles.StatusDisconnected.Render(slotsText)
	}

	left := slotsText

	// Right: shortcuts
	right := styles.HelpKey.Render("0-9") + styles.HelpText.Render(" slot  ") +
		styles.HelpKey.Render("←/→") + styles.HelpText.Render(" sub  ") +
		styles.HelpKey.Render("q") + styles.HelpText.Render("uit")

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

// FormatRate formats bytes per second into a human-readable string.
func FormatRate(bytesPerSec int64) string {
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
