package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/subscription"
)

const defaultLatencyTestURL = "http://ping0.cc"

// SiteDelayProbe is a per-site delay test result.
type SiteDelayProbe struct {
	Delay int64
	Error string
}

// ProbeSitesWithTemporaryCore starts a short-lived direct core for an unbound node.
func ProbeSitesWithTemporaryCore(node subscription.Node, timeout time.Duration, siteURLs []string) ([]SiteDelayProbe, error) {
	if node.Name == "" {
		return nil, fmt.Errorf("node name cannot be empty")
	}
	if len(siteURLs) == 0 {
		return nil, nil
	}

	tempDir, err := os.MkdirTemp("", "rocket4cli-probe-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp probe dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	mixedPort, err := pickTCPPort("127.0.0.1", 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate probe mixed port: %w", err)
	}
	apiPort, err := pickTCPPort("127.0.0.1", 0, 0, map[int]struct{}{mixedPort: {}})
	if err != nil {
		return nil, fmt.Errorf("failed to allocate probe api port: %w", err)
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate probe api secret: %w", err)
	}

	nodes := []subscription.Node{node}
	selected := node
	cfg, err := GenerateConfig(nodes, &selected, nil, 0, mixedPort, apiPort, "127.0.0.1", 0)
	if err != nil {
		return nil, fmt.Errorf("failed to generate probe config: %w", err)
	}
	cfg.Secret = secret

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := saveConfigToPath(cfg, configPath); err != nil {
		return nil, fmt.Errorf("failed to write probe config: %w", err)
	}
	logPath := filepath.Join(tempDir, "mihomo.log")

	mihomoPath, err := findMihomoBinaryFunc()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(mihomoPath, "-d", tempDir, "-f", configPath)
	cmd.Dir = tempDir

	devNull, logFile, err := openProcessFiles(logPath)
	if err != nil {
		return nil, err
	}
	defer devNull.Close()
	defer logFile.Close()
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start temporary core: %w", err)
	}

	cleanupDone := false
	defer func() {
		if cleanupDone {
			return
		}
		_ = terminateProcess(cmd.Process.Pid, 2*time.Second)
		_, _ = cmd.Process.Wait()
	}()

	if err := waitForProbeAPI(apiPort, secret, timeout+time.Second); err != nil {
		return nil, err
	}

	if err := switchProbeGroup(apiPort, secret, "Proxy", node.Name); err != nil {
		return nil, err
	}

	results := make([]SiteDelayProbe, 0, len(siteURLs))
	for _, siteURL := range siteURLs {
		result := SiteDelayProbe{}
		delay, err := probeDelay(apiPort, secret, node.Name, siteURL, timeout)
		if err != nil {
			result.Delay = -1
			result.Error = err.Error()
		} else {
			result.Delay = delay
		}
		results = append(results, result)
	}

	cleanupDone = true
	if err := terminateProcess(cmd.Process.Pid, 2*time.Second); err != nil {
		return nil, err
	}
	_, _ = cmd.Process.Wait()
	return results, nil
}

// ProbeSitesAndIPInfoWithTemporaryCore starts a temporary mihomo core, runs site
// delay probes AND fetches IP info via ipinfo.io through the same proxy instance.
func ProbeSitesAndIPInfoWithTemporaryCore(node subscription.Node, timeout time.Duration, siteURLs []string) ([]SiteDelayProbe, IPInfoResult, error) {
	if node.Name == "" {
		return nil, IPInfoResult{Error: "node name empty"}, fmt.Errorf("node name cannot be empty")
	}

	tempDir, err := os.MkdirTemp("", "rocket4cli-probe-*")
	if err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to create temp probe dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	mixedPort, err := pickTCPPort("127.0.0.1", 0, 0, nil)
	if err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to allocate probe mixed port: %w", err)
	}
	apiPort, err := pickTCPPort("127.0.0.1", 0, 0, map[int]struct{}{mixedPort: {}})
	if err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to allocate probe api port: %w", err)
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to generate probe api secret: %w", err)
	}

	nodes := []subscription.Node{node}
	selected := node
	cfg, err := GenerateConfig(nodes, &selected, nil, 0, mixedPort, apiPort, "127.0.0.1", 0)
	if err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to generate probe config: %w", err)
	}
	cfg.Secret = secret

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := saveConfigToPath(cfg, configPath); err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to write probe config: %w", err)
	}
	logPath := filepath.Join(tempDir, "mihomo.log")

	mihomoPath, err := findMihomoBinaryFunc()
	if err != nil {
		return nil, IPInfoResult{}, err
	}

	cmd := exec.Command(mihomoPath, "-d", tempDir, "-f", configPath)
	cmd.Dir = tempDir

	devNull, logFile, err := openProcessFiles(logPath)
	if err != nil {
		return nil, IPInfoResult{}, err
	}
	defer devNull.Close()
	defer logFile.Close()
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return nil, IPInfoResult{}, fmt.Errorf("failed to start temporary core: %w", err)
	}

	cleanupDone := false
	defer func() {
		if cleanupDone {
			return
		}
		_ = terminateProcess(cmd.Process.Pid, 2*time.Second)
		_, _ = cmd.Process.Wait()
	}()

	if err := waitForProbeAPI(apiPort, secret, timeout+time.Second); err != nil {
		return nil, IPInfoResult{}, err
	}

	if err := switchProbeGroup(apiPort, secret, "Proxy", node.Name); err != nil {
		return nil, IPInfoResult{}, err
	}

	// Probe site delays
	siteResults := make([]SiteDelayProbe, 0, len(siteURLs))
	for _, siteURL := range siteURLs {
		result := SiteDelayProbe{}
		delay, err := probeDelay(apiPort, secret, node.Name, siteURL, timeout)
		if err != nil {
			result.Delay = -1
			result.Error = err.Error()
		} else {
			result.Delay = delay
		}
		siteResults = append(siteResults, result)
	}

	// Probe IP info through the same proxy
	ipInfo := ProbeIPInfoViaProxy("127.0.0.1", mixedPort, timeout)

	cleanupDone = true
	if err := terminateProcess(cmd.Process.Pid, 2*time.Second); err != nil {
		return siteResults, ipInfo, err
	}
	_, _ = cmd.Process.Wait()
	return siteResults, ipInfo, nil
}

func waitForProbeAPI(apiPort int, secret string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/proxies/Proxy", apiPort)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err == nil {
			if secret != "" {
				req.Header.Set("Authorization", "Bearer "+secret)
			}
			resp, err := (&http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}).Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 500 {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("temporary probe core API not ready")
}

func switchProbeGroup(apiPort int, secret string, groupName string, entryName string) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/proxies/%s", apiPort, groupName)
	payload := map[string]string{"name": entryName}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("probe API returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func probeDelay(apiPort int, secret string, nodeName string, siteURL string, timeout time.Duration) (int64, error) {
	encodedName := url.PathEscape(nodeName)
	testURL := url.QueryEscape(siteURL)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/proxies/%s/delay?timeout=%d&url=%s",
		apiPort, encodedName, int(timeout.Milliseconds()), testURL)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return -1, err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := (&http.Client{Timeout: timeout + time.Second, Transport: &http.Transport{DisableKeepAlives: true}}).Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	var body struct {
		Delay   int    `json:"delay"`
		Message string `json:"message,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return -1, err
	}
	if body.Message != "" {
		return -1, fmt.Errorf("%s", body.Message)
	}
	if body.Delay <= 0 {
		return -1, fmt.Errorf("timeout")
	}
	return int64(body.Delay), nil
}

// probeLatencyViaProxy measures latency by sending an HTTP GET through the
// proxy to the test URL and timing how long until the response arrives.
func probeLatencyViaProxy(proxyHost string, proxyPort int, testURL string, timeout time.Duration) (int64, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%d", proxyHost, proxyPort))
	if err != nil {
		return -1, err
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURL),
			DisableKeepAlives: true,
		},
	}

	start := time.Now()
	resp, err := client.Get(testURL)
	if err != nil {
		return -1, err
	}
	resp.Body.Close()

	return time.Since(start).Milliseconds(), nil
}

// SlotTestResult holds the combined latency and region result for a slot test.
type SlotTestResult struct {
	Latency int64  // ms, -1 = timeout/error
	Region  string // "Country, City"
	Err     error
}

// SlotTest tests a slot through its full proxy chain and returns latency + region.
// For RUNNING slots it uses the existing proxy port; for READY slots it starts
// a temporary mihomo instance, tests, and shuts it down.
func SlotTest(cfg *config.Config, subMgr *subscription.Manager, slotMgr SlotService, slotID int, timeout time.Duration) SlotTestResult {
	// Running slot — SOCKS5 through existing proxy
	if slotMgr != nil {
		if state := slotMgr.GetSlotState(slotID); state != nil && state.Port > 0 {
			return slotTestViaPort(state.Port, state.APIPort, state.NodeName, timeout)
		}
	}

	// Check if this slot has a node configured
	ref, hasNode := cfg.GetSlotNodeRef(slotID)

	// Not running — try temporary instance first
	result := slotTestViaTemporary(cfg, subMgr, slotID, timeout)
	if result.Err == nil {
		return result
	}

	// Fallback 1: direct TCP connect using persisted server info
	if hasNode && ref.NodeServer != "" && ref.NodePort > 0 {
		return slotTestDirect(ref.NodeServer, ref.NodePort, timeout)
	}

	// Fallback 2: resolve node from subscription (ref may only have SubID)
	if hasNode && subMgr != nil && ref.SubID != "" {
		for _, n := range subMgr.GetNodesForSubscription(ref.SubID) {
			if n.Name == ref.NodeName && n.Server != "" && n.Port > 0 {
				return slotTestDirect(n.Server, n.Port, timeout)
			}
		}
	}

	// Slot has node but all tests failed — return timeout (not error)
	// so the UI still shows the result instead of silently discarding it
	if hasNode {
		return SlotTestResult{Latency: -1}
	}

	return result
}

func slotTestDirect(server string, port int, timeout time.Duration) SlotTestResult {
	addr := fmt.Sprintf("%s:%d", server, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return SlotTestResult{Latency: -1}
	}
	conn.Close()
	return SlotTestResult{Latency: time.Since(start).Milliseconds()}
}

func slotTestViaPort(mixedPort, apiPort int, nodeName string, timeout time.Duration) SlotTestResult {
	var result SlotTestResult

	// Test latency via HTTP GET to ping0.cc through proxy
	delay, err := probeLatencyFunc("127.0.0.1", mixedPort, defaultLatencyTestURL, timeout)
	if err != nil {
		result.Latency = -1
	} else {
		result.Latency = delay
	}

	// Query region via proxy
	ipInfo := ProbeIPInfoViaProxy("127.0.0.1", mixedPort, timeout)
	if ipInfo.Error == "" {
		result.Region = formatRegion(ipInfo)
	}

	return result
}

func slotTestViaTemporary(cfg *config.Config, subMgr *subscription.Manager, slotID int, timeout time.Duration) SlotTestResult {
	// Resolve node for this slot
	ref, hasNode := cfg.GetSlotNodeRef(slotID)
	var nodes []subscription.Node
	var selected *subscription.Node
	if hasNode && subMgr != nil {
		subNodes := subMgr.GetNodesForSubscription(ref.SubID)
		for i := range subNodes {
			if subNodes[i].Name == ref.NodeName && subNodes[i].Server == ref.NodeServer && subNodes[i].Port == ref.NodePort {
				nodes = subNodes
				selected = &subNodes[i]
				break
			}
		}
	}

	// Create temp dir
	tempDir, err := os.MkdirTemp("", "rocket4cli-slottest-*")
	if err != nil {
		return SlotTestResult{Err: err}
	}
	defer os.RemoveAll(tempDir)

	// Allocate ports
	mixedPort, err := pickTCPPort("127.0.0.1", 0, 0, nil)
	if err != nil {
		return SlotTestResult{Err: err}
	}
	apiPort, err := pickTCPPort("127.0.0.1", 0, 0, map[int]struct{}{mixedPort: {}})
	if err != nil {
		return SlotTestResult{Err: err}
	}

	secret, err := generateSecret()
	if err != nil {
		return SlotTestResult{Err: err}
	}

	// Generate config — pass cfg to support upstream proxy
	if nodes == nil {
		nodes = []subscription.Node{}
	}
	mode := cfg.GetSlotStartMode(slotID)
	mihomoCfg, err := GenerateConfigWithMode(nodes, selected, cfg, slotID, mixedPort, apiPort, "127.0.0.1", 0, mode)
	if err != nil {
		return SlotTestResult{Err: err}
	}
	mihomoCfg.Secret = secret

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := saveConfigToPath(mihomoCfg, configPath); err != nil {
		return SlotTestResult{Err: err}
	}
	logPath := filepath.Join(tempDir, "mihomo.log")

	mihomoPath, err := findMihomoBinaryFunc()
	if err != nil {
		return SlotTestResult{Err: err}
	}

	cmd := exec.Command(mihomoPath, "-d", tempDir, "-f", configPath)
	cmd.Dir = tempDir

	devNull, logFile, err := openProcessFiles(logPath)
	if err != nil {
		return SlotTestResult{Err: err}
	}
	defer devNull.Close()
	defer logFile.Close()
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return SlotTestResult{Err: fmt.Errorf("failed to start temporary core: %w", err)}
	}

	defer func() {
		_ = terminateProcess(cmd.Process.Pid, 2*time.Second)
		_, _ = cmd.Process.Wait()
	}()

	if err := waitForProbeAPI(apiPort, secret, timeout+time.Second); err != nil {
		return SlotTestResult{Err: err}
	}

	// Determine which proxy entry to test
	proxyTarget := effectiveProxyName(slotID, selected, cfg)
	if proxyTarget == "" {
		proxyTarget = "DIRECT"
	}

	// Switch the Proxy group to the target
	_ = switchProbeGroup(apiPort, secret, "Proxy", proxyTarget)

	var result SlotTestResult

	// Test latency via HTTP GET to ping0.cc through proxy
	delay, err := probeLatencyFunc("127.0.0.1", mixedPort, defaultLatencyTestURL, timeout)
	if err != nil {
		result.Latency = -1
	} else {
		result.Latency = delay
	}

	// Query region
	ipInfo := ProbeIPInfoViaProxy("127.0.0.1", mixedPort, timeout)
	if ipInfo.Error == "" {
		result.Region = formatRegion(ipInfo)
	}

	return result
}

func formatRegion(info IPInfoResult) string {
	var parts []string
	if info.Country != "" {
		parts = append(parts, info.Country)
	}
	if info.City != "" {
		parts = append(parts, info.City)
	}
	return strings.Join(parts, ", ")
}
