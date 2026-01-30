package tabs

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/node"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/components"
	"rocket4cli/internal/tui/styles"
)

// NodeTestResultMsg is sent when a node test completes
type NodeTestResultMsg struct {
	Result node.TestResult
}

// NodesTab represents the nodes tab
type NodesTab struct {
	table             *components.Table
	nodeManager       *node.Manager
	nodes             []subscription.Node
	width             int
	height            int
	testing           bool
	cancelTest        context.CancelFunc
	subscriptionID    string
	connectedNodeName string
	connectedIcon     string
	serverDisplay     string // show|blur|off
}

// NewNodesTab creates a new nodes tab
func NewNodesTab(nodeMgr *node.Manager) *NodesTab {
	return &NodesTab{
		table:         components.NewTable(nil),
		nodeManager:   nodeMgr,
		serverDisplay: "show",
	}
}

// Init initializes the tab
func (t *NodesTab) Init() tea.Cmd {
	return t.refreshNodes()
}

// Update handles messages
func (t *NodesTab) Update(msg tea.Msg) (*NodesTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			t.table.MoveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			t.table.MoveDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return t, t.selectNode()
		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			return t, t.testAllNodes()
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			return t, t.refreshNodes()
		}

	case NodeTestResultMsg:
		// Update node with test result
		t.updateNodeResult(msg.Result)

	case TestAllResultsMsg:
		// Update all nodes with test results
		for _, result := range msg.Results {
			t.updateNodeResult(result)
		}
		t.testing = false

	case NodesRefreshedMsg:
		t.nodes = msg.Nodes
		t.updateTableRows()
	}

	return t, nil
}

// View renders the tab
func (t *NodesTab) View() string {
	m := i18n.M()

	if len(t.nodes) == 0 {
		return styles.Muted.Render(m.Nodes.NoNodes)
	}

	// Table
	rowsHeight := t.height - 4
	if rowsHeight < 1 {
		rowsHeight = 1
	}
	t.table.SetHeight(rowsHeight)
	tableView := t.table.View()

	// Help
	help := styles.HelpKey.Render("↑/↓") + styles.HelpText.Render(" navigate  ") +
		styles.HelpKey.Render("Enter") + styles.HelpText.Render(" mode  ") +
		styles.HelpKey.Render("t") + styles.HelpText.Render(" test all  ") +
		styles.HelpKey.Render("r") + styles.HelpText.Render(" refresh")

	if t.testing {
		help += "  " + styles.NodeTesting.Render(m.Nodes.Testing)
	}

	return tableView + "\n" + help
}

// SetSize sets the tab size
func (t *NodesTab) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.table.SetColumns(calcNodeTableColumns(width, t.serverDisplay))
}

func (t *NodesTab) SetSubscriptionID(subscriptionID string) {
	t.subscriptionID = subscriptionID
}

func calcNodeTableColumns(width int, serverDisplay string) []components.Column {
	if width < 0 {
		width = 0
	}

	iconW := 3
	typeW := 10
	latencyW := 10
	statusW := 10
	showServer := serverDisplay != "off"

	minNameW := 16
	baseNameW := 27

	minServerW := 18
	fixed := iconW + typeW + latencyW + statusW
	if showServer {
		// serverW will be computed from remaining; the fixed part stays the same.
	} else {
		// Without the Server column, we can allocate all remaining width to Name.
		minServerW = 0
	}
	remaining := width - fixed
	if remaining < 0 {
		remaining = 0
	}

	nameW := 0
	serverW := 0

	if showServer && remaining >= minNameW+minServerW {
		nameW = baseNameW
		if nameW > remaining-minServerW {
			nameW = remaining - minServerW
		}
		if nameW < minNameW {
			nameW = minNameW
		}
		serverW = remaining - nameW
	} else if remaining > 0 {
		// Very narrow terminals: split what we have.
		if showServer {
			nameW = remaining / 2
			serverW = remaining - nameW
		} else {
			nameW = remaining
			serverW = 0
		}
	}

	// Ensure we never exceed width.
	if fixed+nameW+serverW > width {
		excess := fixed + nameW + serverW - width
		if serverW >= excess {
			serverW -= excess
		} else if nameW >= excess {
			nameW -= excess
		} else {
			nameW = 0
			serverW = 0
		}
	}

	columns := []components.Column{
		{Title: "", Width: iconW},
		{Title: "Name", Width: nameW},
		{Title: "Type", Width: typeW},
	}
	if showServer {
		columns = append(columns, components.Column{Title: "Server", Width: serverW})
	}
	columns = append(columns,
		components.Column{Title: "Latency", Width: latencyW},
		components.Column{Title: "Status", Width: statusW},
	)
	return columns
}

func (t *NodesTab) refreshNodes() tea.Cmd {
	return func() tea.Msg {
		nodes := t.nodeManager.GetNodesForSubscription(t.subscriptionID)
		return NodesRefreshedMsg{Nodes: nodes}
	}
}

func (t *NodesTab) updateTableRows() {
	showServer := t.serverDisplay != "off"
	rows := make([][]string, len(t.nodes))
	for i, n := range t.nodes {
		result := t.nodeManager.GetNodeWithResult(&t.nodes[i])

		icon := ""
		if t.connectedIcon != "" && t.connectedNodeName != "" && n.Name == t.connectedNodeName {
			icon = t.connectedIcon
		}

		latency := "-"
		status := "Unknown"

		if result != nil {
			switch result.Status {
			case node.StatusOnline:
				latency = node.FormatLatency(result.Latency)
				status = "Online"
			case node.StatusTimeout:
				latency = "Timeout"
				status = "Timeout"
			case node.StatusTesting:
				latency = "..."
				status = "Testing"
			case node.StatusError:
				status = "Error"
			}
		}

		base := []string{icon, n.Name, n.Type}
		if showServer {
			server := n.Server
			if t.serverDisplay == "blur" {
				server = maskMiddle(server)
			}
			base = append(base, fmt.Sprintf("%s:%d", server, n.Port))
		}
		base = append(base, latency, status)
		rows[i] = base
	}
	t.table.SetRows(rows)
}

func (t *NodesTab) SetServerDisplay(mode string) {
	switch mode {
	case "show", "blur", "off":
		t.serverDisplay = mode
	default:
		t.serverDisplay = "show"
	}
	if t.width > 0 {
		t.table.SetColumns(calcNodeTableColumns(t.width, t.serverDisplay))
	}
	t.updateTableRows()
}

func maskMiddle(s string) string {
	runes := []rune(s)
	if len(runes) <= 4 {
		return s
	}
	keep := 2
	return string(runes[:keep]) + strings.Repeat("*", len(runes)-(keep*2)) + string(runes[len(runes)-keep:])
}

func (t *NodesTab) selectNode() tea.Cmd {
	if t.table.Selected() >= 0 && t.table.Selected() < len(t.nodes) {
		selectedNode := &t.nodes[t.table.Selected()]
		t.nodeManager.Select(selectedNode)
		return func() tea.Msg {
			return NodeSelectedMsg{Node: selectedNode}
		}
	}
	return nil
}

func (t *NodesTab) SetConnectionIndicator(nodeName, icon string) {
	t.connectedNodeName = nodeName
	t.connectedIcon = icon
	t.updateTableRows()
}

// TestCompleteMsg is sent when all tests are done
type TestCompleteMsg struct{}

func (t *NodesTab) testAllNodes() tea.Cmd {
	if t.testing {
		return nil
	}

	t.testing = true
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelTest = cancel

	// Start testing in background and return commands for each result
	return func() tea.Msg {
		resultCh := make(chan node.TestResult, len(t.nodes))

		go func() {
			t.nodeManager.TestNodes(ctx, t.nodes, resultCh)
			close(resultCh)
		}()

		// Collect all results and return them
		var results []node.TestResult
		for result := range resultCh {
			results = append(results, result)
		}

		return TestAllResultsMsg{Results: results}
	}
}

// TestAllResultsMsg contains all test results
type TestAllResultsMsg struct {
	Results []node.TestResult
}

func (t *NodesTab) updateNodeResult(result node.TestResult) {
	// Update node in the list
	for i := range t.nodes {
		if t.nodes[i].Server == result.Node.Server && t.nodes[i].Port == result.Node.Port {
			t.nodes[i].Latency = result.Latency
			break
		}
	}
	t.updateTableRows()
}

// NodesRefreshedMsg is sent when nodes are refreshed
type NodesRefreshedMsg struct {
	Nodes []subscription.Node
}

// NodeSelectedMsg is sent when a node is selected
type NodeSelectedMsg struct {
	Node *subscription.Node
}

// GetSelectedNode returns the currently selected node in the table
func (t *NodesTab) GetSelectedNode() *subscription.Node {
	idx := t.table.Selected()
	if idx >= 0 && idx < len(t.nodes) {
		return &t.nodes[idx]
	}
	return nil
}
