package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

// SpeedTester handles node speed testing
type SpeedTester struct {
	options   SpeedTestOptions
	timeout   time.Duration
	proxyPort int // Local proxy port (mihomo mixed port)
}

// NewSpeedTester creates a new speed tester
func NewSpeedTester(opts SpeedTestOptions, timeout time.Duration) *SpeedTester {
	return &SpeedTester{
		options:   opts,
		timeout:   timeout,
		proxyPort: 17890, // Default mihomo mixed port
	}
}

// SetProxyPort sets the local proxy port for testing
func (st *SpeedTester) SetProxyPort(port int) {
	st.proxyPort = port
}

// TestAll tests all nodes concurrently and returns results through channel
func (st *SpeedTester) TestAll(ctx context.Context, nodes []subscription.Node, resultCh chan<- TestResult) {
	sem := make(chan struct{}, st.options.Concurrent)
	var wg sync.WaitGroup

	for i := range nodes {
		node := &nodes[i]

		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(n *subscription.Node) {
			defer wg.Done()
			defer func() { <-sem }()

			result := st.TestNode(ctx, n)
			select {
			case resultCh <- result:
			case <-ctx.Done():
			}
		}(node)
	}

	wg.Wait()
}

// TestNode tests a single node
func (st *SpeedTester) TestNode(ctx context.Context, node *subscription.Node) TestResult {
	result := TestResult{
		Node:   node,
		Status: StatusTesting,
	}

	// Test latency (TCP connect)
	if st.options.TestLatency {
		latency, err := st.testLatency(ctx, node)
		if err != nil {
			result.Latency = -1
			result.Status = StatusTimeout
			result.Error = err
			return result
		}
		result.Latency = latency
	}

	// Test packet loss
	if st.options.TestLoss {
		loss := st.testPacketLoss(ctx, node)
		result.Loss = loss
	}

	// Test download speed (optional, more expensive)
	if st.options.TestSpeed {
		speed, err := st.testSpeed(ctx, node)
		if err == nil {
			result.Speed = speed
		}
	}

	result.Status = StatusOnline
	return result
}

// testLatency measures latency by using mihomo API to test the specific node
// This tests the actual usable latency through the proxy node to google.com
func (st *SpeedTester) testLatency(ctx context.Context, node *subscription.Node) (int64, error) {
	// Use mihomo's built-in delay test API
	// This tests each node individually through the proxy to google.com
	latency, err := st.testLatencyViaMihomoAPI(ctx, node)
	if err == nil {
		return latency, nil
	}

	// Fallback: Test direct TCP connection to the node server
	return st.testLatencyDirect(ctx, node)
}

// MihomoDelayResponse represents the response from mihomo delay API
type MihomoDelayResponse struct {
	Delay   int    `json:"delay"`
	Message string `json:"message,omitempty"`
}

// testLatencyViaMihomoAPI tests latency using mihomo's RESTful API
// This calls mihomo's /proxies/:name/delay endpoint which tests the specific node
func (st *SpeedTester) testLatencyViaMihomoAPI(ctx context.Context, node *subscription.Node) (int64, error) {
	// Prefer the running clash4cli core endpoint (dynamic + secret-protected).
	apiHost := "127.0.0.1"
	apiPort := 9090
	apiSecret := ""

	if lock, err := proxy.ReadLockFile(); err == nil && lock != nil && lock.APIPort > 0 {
		if lock.APIHost != "" {
			apiHost = lock.APIHost
		}
		apiPort = lock.APIPort
		apiSecret = lock.Secret
	}
	apiBase := fmt.Sprintf("http://%s:%d", apiHost, apiPort)

	// URL encode the node name for the API call
	encodedName := url.PathEscape(node.Name)

	// Test URL - use Google's generate_204 endpoint
	testURL := url.QueryEscape("http://www.gstatic.com/generate_204")

	// Build the API URL
	apiURL := fmt.Sprintf("%s/proxies/%s/delay?timeout=%d&url=%s",
		apiBase, encodedName, int(st.timeout.Milliseconds()), testURL)

	ctx, cancel := context.WithTimeout(ctx, st.timeout+time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return -1, err
	}
	if apiSecret != "" {
		req.Header.Set("Authorization", "Bearer "+apiSecret)
	}

	client := &http.Client{Timeout: st.timeout + time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, err
	}

	// Parse response
	var delayResp MihomoDelayResponse
	if err := json.Unmarshal(body, &delayResp); err != nil {
		return -1, err
	}

	// Check for error message
	if delayResp.Message != "" {
		return -1, fmt.Errorf("%s", delayResp.Message)
	}

	if delayResp.Delay <= 0 {
		return -1, fmt.Errorf("timeout or unreachable")
	}

	return int64(delayResp.Delay), nil
}

// testLatencyDirect measures direct TCP connection latency to node server
func (st *SpeedTester) testLatencyDirect(ctx context.Context, node *subscription.Node) (int64, error) {
	addr := fmt.Sprintf("%s:%d", node.Server, node.Port)

	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	latency := time.Since(start).Milliseconds()
	return latency, nil
}

// testPacketLoss measures packet loss by multiple TCP connects
func (st *SpeedTester) testPacketLoss(ctx context.Context, node *subscription.Node) float64 {
	const attempts = 5
	var failures int

	addr := fmt.Sprintf("%s:%d", node.Server, node.Port)
	dialer := &net.Dialer{Timeout: st.timeout}

	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return 100.0
		default:
		}

		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			failures++
		} else {
			conn.Close()
		}

		// Small delay between attempts
		time.Sleep(100 * time.Millisecond)
	}

	return float64(failures) / float64(attempts) * 100.0
}

// testSpeed measures download speed using a speed test URL
func (st *SpeedTester) testSpeed(ctx context.Context, node *subscription.Node) (int64, error) {
	// For now, we use a simple approach: measure time to download small data through the proxy
	// In production, this would connect through the actual proxy

	// This is a placeholder - in real implementation, we'd connect through the proxy
	// and download from a speed test server

	// For demonstration, we'll use a simple HTTP GET through direct connection
	// This will be replaced with proxy-aware speed testing

	testURL := "http://speedtest.tele2.net/1MB.zip"

	client := &http.Client{
		Timeout: st.timeout * 2,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// In production, route through the proxy node
				dialer := &net.Dialer{Timeout: st.timeout}
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}

	ctx, cancel := context.WithTimeout(ctx, st.timeout*2)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Read up to 1MB
	limited := io.LimitReader(resp.Body, 1024*1024)
	n, err := io.Copy(io.Discard, limited)
	if err != nil {
		return 0, err
	}

	duration := time.Since(start).Seconds()
	if duration == 0 {
		duration = 0.001 // avoid division by zero
	}

	speed := int64(float64(n) / duration)
	return speed, nil
}

// FormatLatency formats latency for display
func FormatLatency(ms int64) string {
	if ms < 0 {
		return "Timeout"
	}
	return fmt.Sprintf("%dms", ms)
}

// FormatSpeed formats speed for display
func FormatSpeed(bytesPerSec int64) string {
	if bytesPerSec <= 0 {
		return "-"
	}

	const (
		KB = 1024
		MB = 1024 * KB
	)

	switch {
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.1fMB/s", float64(bytesPerSec)/float64(MB))
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.1fKB/s", float64(bytesPerSec)/float64(KB))
	default:
		return fmt.Sprintf("%dB/s", bytesPerSec)
	}
}

// FormatLoss formats packet loss for display
func FormatLoss(loss float64) string {
	return fmt.Sprintf("%.0f%%", loss)
}
