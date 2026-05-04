package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	PrimaryColor    = lipgloss.Color("#EC4899") // Pink
	SecondaryColor  = lipgloss.Color("#10B981") // Green
	AccentColor     = lipgloss.Color("#F59E0B") // Amber
	ErrorColor      = lipgloss.Color("#EF4444") // Red
	MutedColor      = lipgloss.Color("#6B7280") // Gray
	BackgroundColor = lipgloss.Color("#1F2937") // Dark gray
	ForegroundColor = lipgloss.Color("#F9FAFB") // Light gray

	// Base styles for colors
	Primary   = lipgloss.NewStyle().Foreground(PrimaryColor)
	Secondary = lipgloss.NewStyle().Foreground(SecondaryColor)
	Accent    = lipgloss.NewStyle().Foreground(AccentColor)
	Muted     = lipgloss.NewStyle().Foreground(MutedColor)

	// Tab styles
	ActiveTab = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Background(lipgloss.Color("#374151")).
			Padding(0, 2)

	InactiveTab = lipgloss.NewStyle().
			Foreground(MutedColor).
			Padding(0, 2)

	TabBar = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(MutedColor)

	// Status bar styles
	StatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#374151")).
			Foreground(ForegroundColor).
			Padding(0, 1)

	StatusConnected = lipgloss.NewStyle().
			Foreground(SecondaryColor).
			Bold(true)

	StatusDisconnected = lipgloss.NewStyle().
				Foreground(MutedColor)

	StatusError = lipgloss.NewStyle().
			Foreground(ErrorColor).
			Bold(true)

	// Content area
	ContentArea = lipgloss.NewStyle().
			Padding(0, 0)

	// Table styles
	TableHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(MutedColor)

	TableRow = lipgloss.NewStyle().
			Foreground(ForegroundColor)

	TableRowSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("#374151")).
				Foreground(ForegroundColor).
				Bold(true)

	// Node status styles
	NodeOnline = lipgloss.NewStyle().
			Foreground(SecondaryColor)

	NodeOffline = lipgloss.NewStyle().
			Foreground(ErrorColor)

	NodeTesting = lipgloss.NewStyle().
			Foreground(AccentColor)

	NodeUnknown = lipgloss.NewStyle().
			Foreground(MutedColor)

	// Input styles
	InputLabel = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true)

	InputField = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(PrimaryColor).
			Padding(0, 1)

	InputFocused = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(SecondaryColor).
			Padding(0, 1)

	// Help text
	HelpKey = lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Bold(true)

	HelpText = lipgloss.NewStyle().
			Foreground(MutedColor)

	// Title
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(PrimaryColor).
		Background(lipgloss.Color("#374151")).
		Padding(0, 1)

	// TitleBar is a full-width title row with background and bottom border,
	// visually distinct from the content below.
	TitleBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Background(lipgloss.Color("#374151")).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(MutedColor)

	// Log level styles
	LogDebug = lipgloss.NewStyle().Foreground(MutedColor)
	LogInfo  = lipgloss.NewStyle().Foreground(SecondaryColor)
	LogWarn  = lipgloss.NewStyle().Foreground(AccentColor)
	LogError = lipgloss.NewStyle().Foreground(ErrorColor)

	// Message styles
	SuccessMessage = lipgloss.NewStyle().
			Foreground(SecondaryColor).
			Bold(true)

	ErrorMessage = lipgloss.NewStyle().
			Foreground(ErrorColor).
			Bold(true)

	// Box styles
	InfoBox = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(PrimaryColor).
		Padding(1, 2)

	// Latency colors
	LatencyGood    = lipgloss.NewStyle().Foreground(SecondaryColor) // < 100ms
	LatencyMedium  = lipgloss.NewStyle().Foreground(AccentColor)    // 100-300ms
	LatencyBad     = lipgloss.NewStyle().Foreground(ErrorColor)     // > 300ms
	LatencyTimeout = lipgloss.NewStyle().Foreground(MutedColor)

	// Select option styles
	SelectOption = lipgloss.NewStyle().
			Foreground(MutedColor).
			Padding(0, 1)

	SelectOptionSelected = lipgloss.NewStyle().
				Foreground(ForegroundColor).
				Background(lipgloss.Color("#374151")).
				Padding(0, 1)

	SelectOptionActive = lipgloss.NewStyle().
				Foreground(ForegroundColor).
				Background(PrimaryColor).
				Bold(true).
				Padding(0, 1)
)

// FormatLatency returns styled latency string
func FormatLatency(ms int64) string {
	if ms < 0 {
		return LatencyTimeout.Render("Timeout")
	}

	text := lipgloss.NewStyle()
	switch {
	case ms < 2000:
		text = LatencyGood
	case ms < 3000:
		text = LatencyMedium
	default:
		text = LatencyBad
	}

	return text.Render(fmt.Sprintf("%dms", ms))
}
