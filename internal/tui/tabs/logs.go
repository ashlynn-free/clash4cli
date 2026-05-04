package tabs

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/tui/styles"
)

// LogEntry represents a log entry in the UI
type LogEntry struct {
	Time    time.Time
	Level   string
	Message string
}

// LogReceivedMsg is sent when a new log is received
type LogReceivedMsg struct {
	Entry LogEntry
}

// LogsTab represents the logs tab
type LogsTab struct {
	logs       []LogEntry
	width      int
	height     int
	offset     int
	autoScroll bool
	maxLogs    int
}

// NewLogsTab creates a new logs tab
func NewLogsTab() *LogsTab {
	return &LogsTab{
		logs:       make([]LogEntry, 0),
		autoScroll: true,
		maxLogs:    1000,
	}
}

// Init initializes the tab
func (t *LogsTab) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (t *LogsTab) Update(msg tea.Msg) (*LogsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.scrollUp()
			t.autoScroll = false
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.scrollDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			t.scrollToTop()
			t.autoScroll = false
		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			t.scrollToBottom()
			t.autoScroll = true
		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			t.clear()
		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			t.autoScroll = !t.autoScroll
		}

	case LogReceivedMsg:
		t.addLog(msg.Entry)

	}

	return t, nil
}

// View renders the tab
func (t *LogsTab) View() string {
	m := i18n.M()

	if len(t.logs) == 0 {
		return styles.ContentArea.Render(
			styles.Muted.Render(m.Logs.NoLogs),
		)
	}

	visibleHeight := t.height - 6
	if visibleHeight < 1 {
		visibleHeight = 10
	}

	// Calculate visible range
	start := t.offset
	end := start + visibleHeight
	if end > len(t.logs) {
		end = len(t.logs)
	}

	var logLines []string
	for i := start; i < end; i++ {
		entry := t.logs[i]
		logLines = append(logLines, t.formatLogEntry(entry))
	}

	logsView := strings.Join(logLines, "\n")

	// Scroll indicator
	scrollInfo := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(t.logs))
	if t.autoScroll {
		scrollInfo += " [AUTO]"
	}
	scrollIndicator := styles.Muted.Render(scrollInfo)

	// Help
	help := "\n" +
		styles.HelpKey.Render("↑↓") + styles.HelpText.Render(" scroll  ") +
		styles.HelpKey.Render("g/G") + styles.HelpText.Render(" top/bottom  ") +
		styles.HelpKey.Render("a") + styles.HelpText.Render(" auto-scroll  ") +
		styles.HelpKey.Render("c") + styles.HelpText.Render(" clear")

	return styles.ContentArea.Render(logsView + "\n" + scrollIndicator + help)
}

// SetSize sets the tab size
func (t *LogsTab) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// AddLog adds a log entry
func (t *LogsTab) AddLog(level, message string) {
	t.addLog(LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	})
}

func (t *LogsTab) addLog(entry LogEntry) {
	t.logs = append(t.logs, entry)

	// Trim if too many logs
	if len(t.logs) > t.maxLogs {
		t.logs = t.logs[len(t.logs)-t.maxLogs:]
		if t.offset > 0 {
			t.offset--
		}
	}

	// Auto-scroll
	if t.autoScroll {
		t.scrollToBottom()
	}
}

func (t *LogsTab) formatLogEntry(entry LogEntry) string {
	timeStr := entry.Time.Format("15:04:05")

	var levelStyle lipgloss.Style
	switch strings.ToLower(entry.Level) {
	case "debug":
		levelStyle = styles.LogDebug
	case "info":
		levelStyle = styles.LogInfo
	case "warn", "warning":
		levelStyle = styles.LogWarn
	case "error":
		levelStyle = styles.LogError
	default:
		levelStyle = styles.Muted
	}

	level := levelStyle.Render(fmt.Sprintf("%-5s", strings.ToUpper(entry.Level)))

	message := entry.Message
	maxMsgLen := t.width - 20
	if maxMsgLen > 0 && len(message) > maxMsgLen {
		message = message[:maxMsgLen-3] + "..."
	}

	return fmt.Sprintf("%s %s %s", styles.Muted.Render(timeStr), level, message)
}

func (t *LogsTab) scrollUp() {
	if t.offset > 0 {
		t.offset--
	}
}

func (t *LogsTab) scrollDown() {
	visibleHeight := t.height - 6
	if t.offset < len(t.logs)-visibleHeight {
		t.offset++
	}
	if t.offset >= len(t.logs)-visibleHeight {
		t.autoScroll = true
	}
}

func (t *LogsTab) scrollToTop() {
	t.offset = 0
}

func (t *LogsTab) scrollToBottom() {
	visibleHeight := t.height - 6
	if visibleHeight < 1 {
		visibleHeight = 10
	}
	t.offset = len(t.logs) - visibleHeight
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *LogsTab) clear() {
	t.logs = make([]LogEntry, 0)
	t.offset = 0
}
