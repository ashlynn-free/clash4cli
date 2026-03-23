package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/tui/styles"
)

// StatusBar represents the status bar component
type StatusBar struct {
	width        int
	activeSlots  int
	uploadRate   int64
	downloadRate int64

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

// SetTraffic stores the aggregate upload/download rates for display.
func (s *StatusBar) SetTraffic(uploadTotal, downloadTotal, uploadRate, downloadRate int64) {
	s.uploadRate = uploadRate
	s.downloadRate = downloadRate
}

// ClearTraffic resets displayed traffic rates to zero.
func (s *StatusBar) ClearTraffic() {
	s.uploadRate = 0
	s.downloadRate = 0
}

// View renders the status bar
func (s *StatusBar) View() string {
	// Left: slot count
	slotsText := fmt.Sprintf("%d/10 Slots running", s.activeSlots)
	if s.activeSlots > 0 {
		slotsText = styles.StatusConnected.Render(slotsText)
	} else {
		slotsText = styles.StatusDisconnected.Render(slotsText)
	}
	left := slotsText

	// Center: aggregate upload/download speed
	center := styles.HelpText.Render(fmt.Sprintf("↑ %s  ↓ %s",
		FormatRate(s.uploadRate),
		FormatRate(s.downloadRate),
	))

	// Right: quit shortcut only
	right := styles.HelpKey.Render("q") + styles.HelpText.Render(" quit")

	// Calculate spacing to fill the bar width (StatusBar style has Padding(0,1) = 2 chars)
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)
	available := s.width - leftWidth - centerWidth - rightWidth - 4
	if available < 2 {
		available = 2
	}
	spacing1 := available / 2
	spacing2 := available - spacing1

	spacer1 := lipgloss.NewStyle().Width(spacing1).Render("")
	spacer2 := lipgloss.NewStyle().Width(spacing2).Render("")

	return styles.StatusBar.Width(s.width).Render(
		left + spacer1 + center + spacer2 + right,
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

// FormatRateMbps formats bytes per second as bits per second (Kbps/Mbps/Gbps).
// Uses decimal (1000-based) units following networking conventions.
func FormatRateMbps(bytesPerSec int64) string {
	if bytesPerSec < 0 {
		return "-"
	}
	if bytesPerSec == 0 {
		return "0bps"
	}
	bitsPerSec := float64(bytesPerSec) * 8
	const (
		Kbps = 1000.0
		Mbps = 1000.0 * Kbps
		Gbps = 1000.0 * Mbps
	)
	switch {
	case bitsPerSec >= Gbps:
		return fmt.Sprintf("%.1fGbps", bitsPerSec/Gbps)
	case bitsPerSec >= Mbps:
		return fmt.Sprintf("%.1fMbps", bitsPerSec/Mbps)
	case bitsPerSec >= Kbps:
		return fmt.Sprintf("%.1fKbps", bitsPerSec/Kbps)
	default:
		return fmt.Sprintf("%.0fbps", bitsPerSec)
	}
}

// FormatRateWithUnit formats rate using the configured speed unit.
// unit should be "bits" for Mbps or "bytes" (default) for MB/s.
func FormatRateWithUnit(bytesPerSec int64, unit string) string {
	if unit == "bits" {
		return FormatRateMbps(bytesPerSec)
	}
	return FormatRate(bytesPerSec)
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
