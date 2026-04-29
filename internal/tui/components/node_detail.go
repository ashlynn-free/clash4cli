package components

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui/styles"
)

// NodeDetailCloseMsg signals that the detail view should close.
type NodeDetailCloseMsg struct{}

// SiteTestResultMsg carries the result of a single site test.
type SiteTestResultMsg struct {
	Index   int
	Latency int64 // ms; -1 on error
	Error   string
}

// SiteTestAllDoneMsg signals all site tests finished.
type SiteTestAllDoneMsg struct{}

// IPInfoResultMsg carries the result of an IP info lookup.
type IPInfoResultMsg struct {
	Result proxy.IPInfoResult
}

type siteTestEntry struct {
	Name    string
	URL     string
	Latency int64 // ms; 0 = not tested, -1 = error
	Error   string
	Testing bool
}

// NodeDetailView shows full node info and website tests.
type NodeDetailView struct {
	node          subscription.Node
	width         int
	height        int
	scrollY       int
	sites         []siteTestEntry
	testing       bool
	resultCh      <-chan SiteTestResultMsg
	probeLock     *proxy.LockFile
	ipInfo        *proxy.IPInfoResult
	ipInfoTesting bool
}

var defaultTestSites = []siteTestEntry{
	{Name: "Apple", URL: "http://www.apple.com/library/test/success.html"},
	{Name: "GitHub", URL: "http://github.com"},
	{Name: "Google", URL: "http://www.gstatic.com/generate_204"},
	{Name: "YouTube", URL: "http://www.youtube.com"},
	{Name: "Claude", URL: "http://claude.ai"},
}

// NewNodeDetailView creates a new detail view for the given node.
func NewNodeDetailView(n subscription.Node, width, height int, probeLock *proxy.LockFile) *NodeDetailView {
	sites := make([]siteTestEntry, len(defaultTestSites))
	copy(sites, defaultTestSites)
	var lockCopy *proxy.LockFile
	if probeLock != nil {
		v := *probeLock
		lockCopy = &v
	}
	return &NodeDetailView{
		node:      n,
		width:     width,
		height:    height,
		sites:     sites,
		probeLock: lockCopy,
	}
}

// SetSize updates the available dimensions.
func (v *NodeDetailView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// Update handles messages for the detail view.
func (v *NodeDetailView) Update(msg tea.Msg) (*NodeDetailView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			return v, func() tea.Msg { return NodeDetailCloseMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			return v, v.testAllSites()
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.scrollY > 0 {
				v.scrollY--
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			v.scrollY++
		}

	case SiteTestResultMsg:
		if msg.Index >= 0 && msg.Index < len(v.sites) {
			v.sites[msg.Index].Latency = msg.Latency
			v.sites[msg.Index].Error = msg.Error
			v.sites[msg.Index].Testing = false
		}
		return v, waitForSiteResult(v.resultCh)

	case SiteTestAllDoneMsg:
		v.testing = false
		v.resultCh = nil

	case IPInfoResultMsg:
		v.ipInfoTesting = false
		result := msg.Result
		v.ipInfo = &result
	}

	return v, nil
}

// View renders the detail view.
func (v *NodeDetailView) View() string {
	m := i18n.M()
	var lines []string

	// Title
	lines = append(lines, styles.Title.Render(m.NodeDetail.Title))
	lines = append(lines, "")

	// Node info
	lines = append(lines, styles.HelpKey.Render("Name: ")+v.node.Name)
	lines = append(lines, styles.HelpKey.Render("Type: ")+v.node.Type)
	lines = append(lines, styles.HelpKey.Render(m.NodeDetail.Server+": ")+fmt.Sprintf("%s:%d", v.node.Server, v.node.Port))
	lines = append(lines, "")

	// Settings
	if len(v.node.Settings) > 0 {
		lines = append(lines, styles.Primary.Render("── "+m.NodeDetail.Settings+" ──"))
		keys := make([]string, 0, len(v.node.Settings))
		for k := range v.node.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := fmt.Sprintf("%v", v.node.Settings[k])
			lines = append(lines, styles.HelpKey.Render("  "+k+": ")+styles.HelpText.Render(val))
		}
		lines = append(lines, "")
	}

	// Website Tests
	lines = append(lines, styles.Primary.Render("── "+m.NodeDetail.WebsiteTests+" ──"))
	for _, site := range v.sites {
		var status string
		switch {
		case site.Testing:
			status = styles.NodeTesting.Render(m.NodeDetail.Testing)
		case site.Latency > 0:
			status = formatSiteLatency(site.Latency)
		case site.Latency == -1:
			status = styles.NodeOffline.Render(site.Error)
		default:
			status = styles.Muted.Render(m.NodeDetail.NotTested)
		}
		lines = append(lines, fmt.Sprintf("  %-10s %s", site.Name, status))
	}
	lines = append(lines, "")

	// IP Info
	lines = append(lines, styles.Primary.Render("── "+m.NodeDetail.IPInfo+" ──"))
	if v.ipInfoTesting {
		lines = append(lines, "  "+styles.NodeTesting.Render(m.NodeDetail.Testing))
	} else if v.ipInfo != nil {
		if v.ipInfo.Error != "" {
			lines = append(lines, "  "+styles.NodeOffline.Render(v.ipInfo.Error))
		} else {
			lines = append(lines, styles.HelpKey.Render("  IP: ")+v.ipInfo.IP)
			location := joinNonEmpty(", ", v.ipInfo.City, v.ipInfo.Region, v.ipInfo.Country)
			if location != "" {
				lines = append(lines, styles.HelpKey.Render("  "+m.NodeDetail.Location+": ")+location)
			}
			if v.ipInfo.Org != "" {
				lines = append(lines, styles.HelpKey.Render("  "+m.NodeDetail.Org+": ")+v.ipInfo.Org)
			}
		}
	} else {
		lines = append(lines, "  "+styles.Muted.Render(m.NodeDetail.NotTested))
	}
	lines = append(lines, "")

	// Help bar
	help := styles.HelpKey.Render("Esc") + styles.HelpText.Render(" "+m.Actions.Back+"  ") +
		styles.HelpKey.Render("t") + styles.HelpText.Render(" test  ") +
		styles.HelpKey.Render("↑/↓") + styles.HelpText.Render(" scroll")

	if v.testing {
		help += "  " + styles.NodeTesting.Render(m.NodeDetail.Testing)
	}

	// Apply scrolling
	contentLines := lines
	visibleHeight := v.height - 2 // reserve for help bar
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	maxScroll := len(contentLines) - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if v.scrollY > maxScroll {
		v.scrollY = maxScroll
	}

	end := v.scrollY + visibleHeight
	if end > len(contentLines) {
		end = len(contentLines)
	}

	visible := contentLines[v.scrollY:end]
	return strings.Join(visible, "\n") + "\n" + help
}

func formatSiteLatency(ms int64) string {
	text := fmt.Sprintf("%dms", ms)
	switch {
	case ms < 200:
		return styles.LatencyGood.Render(text)
	case ms < 500:
		return styles.LatencyMedium.Render(text)
	default:
		return styles.LatencyBad.Render(text)
	}
}

// testAllSites starts website tests and IP info lookup for all sites.
func (v *NodeDetailView) testAllSites() tea.Cmd {
	if v.testing {
		return nil
	}

	v.testing = true
	v.ipInfoTesting = true
	v.ipInfo = nil
	for i := range v.sites {
		v.sites[i].Latency = 0
		v.sites[i].Error = ""
		v.sites[i].Testing = true
	}

	ch := make(chan SiteTestResultMsg, len(v.sites))
	v.resultCh = ch

	nodeName := v.node.Name
	sites := make([]siteTestEntry, len(v.sites))
	copy(sites, v.sites)

	if v.probeLock == nil {
		// No running slot: use a single temporary core for both site tests and IP info.
		ipCh := make(chan IPInfoResultMsg, 1)

		go func() {
			defer close(ch)
			defer close(ipCh)

			urls := make([]string, 0, len(sites))
			for _, site := range sites {
				urls = append(urls, site.URL)
			}
			siteResults, ipInfo, err := proxy.ProbeSitesAndIPInfoWithTemporaryCore(v.node, 5*time.Second, urls)
			if err != nil && len(siteResults) == 0 {
				for i := range sites {
					ch <- SiteTestResultMsg{Index: i, Latency: -1, Error: err.Error()}
				}
				ipCh <- IPInfoResultMsg{Result: proxy.IPInfoResult{Error: err.Error()}}
				return
			}
			for i, result := range siteResults {
				ch <- SiteTestResultMsg{Index: i, Latency: result.Delay, Error: result.Error}
			}
			ipCh <- IPInfoResultMsg{Result: ipInfo}
		}()

		return tea.Batch(waitForSiteResult(ch), waitForIPInfo(ipCh))
	}

	// Running slot: site tests via API, IP info via proxy port concurrently.
	go func() {
		defer close(ch)

		apiHost := "127.0.0.1"
		if v.probeLock.APIHost != "" {
			apiHost = v.probeLock.APIHost
		}
		apiBase := fmt.Sprintf("http://%s:%d", apiHost, v.probeLock.APIPort)
		targetName := nodeName
		if v.probeLock.EffectiveProxy != "" {
			targetName = v.probeLock.EffectiveProxy
		}
		encodedName := url.PathEscape(targetName)
		client := &http.Client{Timeout: 6 * time.Second}

		for i, site := range sites {
			testURL := url.QueryEscape(site.URL)
			apiURL := fmt.Sprintf("%s/proxies/%s/delay?timeout=5000&url=%s",
				apiBase, encodedName, testURL)

			req, err := http.NewRequest("GET", apiURL, nil)
			if err != nil {
				ch <- SiteTestResultMsg{Index: i, Latency: -1, Error: err.Error()}
				continue
			}
			if v.probeLock.Secret != "" {
				req.Header.Set("Authorization", "Bearer "+v.probeLock.Secret)
			}

			resp, err := client.Do(req)
			if err != nil {
				ch <- SiteTestResultMsg{Index: i, Latency: -1, Error: "timeout"}
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				ch <- SiteTestResultMsg{Index: i, Latency: -1, Error: err.Error()}
				continue
			}

			var delayResp struct {
				Delay   int    `json:"delay"`
				Message string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(body, &delayResp); err != nil {
				ch <- SiteTestResultMsg{Index: i, Latency: -1, Error: err.Error()}
				continue
			}

			if delayResp.Delay <= 0 {
				errMsg := delayResp.Message
				if errMsg == "" {
					errMsg = "timeout"
				}
				ch <- SiteTestResultMsg{Index: i, Latency: -1, Error: errMsg}
				continue
			}

			ch <- SiteTestResultMsg{Index: i, Latency: int64(delayResp.Delay), Error: ""}
		}
	}()

	return tea.Batch(waitForSiteResult(ch), v.fetchIPInfoViaProxy())
}

// fetchIPInfoViaProxy returns a tea.Cmd that fetches IP info through the running proxy.
func (v *NodeDetailView) fetchIPInfoViaProxy() tea.Cmd {
	proxyHost := v.probeLock.ProxyHost
	if proxyHost == "" {
		proxyHost = "127.0.0.1"
	}
	port := v.probeLock.Port
	return func() tea.Msg {
		result := proxy.ProbeIPInfoViaProxy(proxyHost, port, 10*time.Second)
		return IPInfoResultMsg{Result: result}
	}
}

func waitForSiteResult(ch <-chan SiteTestResultMsg) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return SiteTestAllDoneMsg{}
		}
		return result
	}
}

func waitForIPInfo(ch <-chan IPInfoResultMsg) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return IPInfoResultMsg{Result: proxy.IPInfoResult{Error: "channel closed"}}
		}
		return result
	}
}

func joinNonEmpty(sep string, parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, sep)
}
