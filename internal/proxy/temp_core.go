package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"rocket4cli/internal/subscription"
)

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

	mihomoPath, err := findMihomoBinary()
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

func waitForProbeAPI(apiPort int, secret string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/proxies/Proxy", apiPort)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err == nil {
			if secret != "" {
				req.Header.Set("Authorization", "Bearer "+secret)
			}
			resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
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
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
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

	resp, err := (&http.Client{Timeout: timeout + time.Second}).Do(req)
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
