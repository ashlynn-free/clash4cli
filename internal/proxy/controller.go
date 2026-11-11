package proxy

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/subscription"
)

// LockFile represents the proxy.lock file content
type LockFile struct {
	Port       int    `json:"port"`
	APIPort    int    `json:"api_port"`
	RedirPort  int    `json:"redir_port,omitempty"`
	PID        int    `json:"pid"`
	SubID      string `json:"sub_id,omitempty"`
	Node       string `json:"node,omitempty"`
	NodeServer string `json:"node_server,omitempty"`
	NodePort   int    `json:"node_port,omitempty"`
	NodeType   string `json:"node_type,omitempty"`
	// DockerRedirectTCP indicates that the generated mihomo config includes a redir-port
	// suitable for iptables REDIRECT from Docker containers (TCP only).
	DockerRedirectTCP bool   `json:"docker_redirect_tcp,omitempty"`
	ProxyHost         string `json:"proxy_host,omitempty"`
	APIHost           string `json:"api_host,omitempty"`
	ConfigPath        string `json:"config_path,omitempty"`
	WorkDir           string `json:"work_dir,omitempty"`
	LogPath           string `json:"log_path,omitempty"`
	Secret            string `json:"secret,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	Detached          bool   `json:"detached,omitempty"`
	EffectiveProxy    string `json:"effective_proxy,omitempty"`
	UpstreamEnabled   bool   `json:"upstream_enabled,omitempty"`
	UpstreamProtocol  string `json:"upstream_protocol,omitempty"`
	UpstreamServer    string `json:"upstream_server,omitempty"`
	UpstreamPort      int    `json:"upstream_port,omitempty"`
}

// Status represents the proxy status
type Status int

const (
	StatusStopped Status = iota
	StatusStarting
	StatusRunning
	StatusStopping
	StatusError
)

// String returns the string representation of Status
func (s Status) String() string {
	switch s {
	case StatusStopped:
		return "Stopped"
	case StatusStarting:
		return "Starting"
	case StatusRunning:
		return "Running"
	case StatusStopping:
		return "Stopping"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Controller controls the mihomo process
type Controller struct {
	cmd               *exec.Cmd
	status            Status
	mu                sync.RWMutex
	apiClient         *http.Client
	cfg               *config.Config
	configPath        string
	pid               int
	port              int
	apiPort           int
	redirPort         int
	apiHost           string
	proxyHost         string
	secret            string
	logPath           string
	lockSlotID        int
	multiInstanceLock bool
	attached          bool // true if restored from lockfile (not started by this process)
	effectiveProxy    string
	upstreamEnabled   bool
	upstreamProtocol  string
	upstreamServer    string
	upstreamPort      int
}

// NewController creates a new proxy controller
func NewController(cfg *config.Config) *Controller {
	c := &Controller{
		status:            StatusStopped,
		apiClient:         &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}},
		cfg:               cfg,
		apiHost:           "127.0.0.1",
		proxyHost:         "127.0.0.1",
		lockSlotID:        0,
		multiInstanceLock: false,
	}
	c.syncFromLockFile()
	return c
}

// Start starts the proxy with given nodes, and records the active subscription + node state into proxy.lock.
func (c *Controller) Start(subID string, nodes []subscription.Node, selectedNode *subscription.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If a previous c4c session left mihomo running, treat it as already running.
	if c.status == StatusRunning || c.status == StatusStarting {
		logger.Warn("[slot0] proxy already running")
		return fmt.Errorf("proxy already running")
	}
	if lock, err := ReadLockFile(); err == nil && lock != nil {
		if c.lockFileLooksAlive(lock) {
			c.applyLockFileLocked(lock)
			logger.Warn("[slot0] proxy already running (detected from proxy.lock)")
			return fmt.Errorf("proxy already running")
		}
		// Stale lock: best-effort remove.
		c.removeLockFile()
	}

	c.status = StatusStarting
	logger.Info("[slot0] Starting proxy (detached)...")

	secret, err := generateSecret()
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to generate api secret: %w", err)
	}

	// Select ports (multi-user safe):
	// - mixed-port uses configured port, but falls back to the next free one if occupied.
	// - external-controller (API) must be unique per core; default range starts at 9090.
	proxyListenHost := "127.0.0.1"
	proxyDialHost := "127.0.0.1"
	preferredMixedPort := 17890
	if c.cfg != nil {
		if c.cfg.Proxy.MixedPort > 0 {
			preferredMixedPort = c.cfg.Proxy.MixedPort
		}
		if c.cfg.Proxy.AllowLan {
			bind := strings.TrimSpace(c.cfg.Proxy.BindAddress)
			if bind == "" {
				bind = "0.0.0.0"
			}
			proxyListenHost = bind
			// If the user binds to a specific interface, dial that address too.
			if bind != "0.0.0.0" && bind != "127.0.0.1" && bind != "::" && bind != "" {
				proxyDialHost = bind
			}
		}
	}

	mixedPort, err := pickTCPPort(proxyListenHost, preferredMixedPort, 200, nil)
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to pick a free mixed-port: %w", err)
	}

	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort, err := pickTCPPort(apiHost, 9090, 200, map[int]struct{}{mixedPort: {}})
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to pick a free api port: %w", err)
	}

	redirPort := 0
	dockerRedirectTCP := false
	if c.cfg != nil && c.cfg.Proxy.DockerRedirectTCP && runtime.GOOS == "linux" {
		dockerRedirectTCP = true
		preferredRedirPort := 0
		if preferredMixedPort > 0 && preferredMixedPort < 65535 {
			preferredRedirPort = preferredMixedPort + 1
		}
		redirPort, err = pickTCPPort(proxyListenHost, preferredRedirPort, 200, map[int]struct{}{
			mixedPort: {},
			apiPort:   {},
		})
		if err != nil {
			c.status = StatusError
			return fmt.Errorf("failed to pick a free redir-port: %w", err)
		}
	}

	c.port = mixedPort
	c.apiPort = apiPort
	c.redirPort = redirPort
	c.proxyHost = proxyDialHost
	c.secret = secret
	c.setChainRuntimeLocked(0, selectedNode)

	// Generate mihomo config
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg, 0, c.port, c.apiPort, apiHost, c.redirPort)
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to generate config: %v", err)
		logger.Error("[slot0] %s", errMsg)
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to generate config: %w", err)
	}
	mihomoConfig.Secret = secret

	// Save config
	configPath, err := SaveConfig(mihomoConfig)
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to save config: %v", err)
		logger.Error("[slot0] %s", errMsg)
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to save config: %w", err)
	}
	c.configPath = configPath
	logger.Info("[slot0] Config saved to: %s", configPath)

	// Find mihomo binary
	mihomoPath, err := findMihomoBinaryFunc()
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("mihomo binary not found: %v", err)
		logger.Error("[slot0] %s", errMsg)
		logger.Error("%s", errMsg)
		return err
	}
	logger.Info("[slot0] Using mihomo: %s", mihomoPath)

	// Get working directory
	workDir, err := config.GetMihomoWorkDir()
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to get work dir: %v", err)
		logger.Error("[slot0] %s", errMsg)
		logger.Error("%s", errMsg)
		return err
	}

	logPath, err := prepareMihomoLogFile()
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to prepare mihomo log file: %w", err)
	}
	c.logPath = logPath

	logger.Info("[slot0] Executing (detached): %s -d %s -f %s", mihomoPath, workDir, c.configPath)

	cmd, err := startMihomoProcess(mihomoPath, workDir, c.configPath, logPath)
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to start mihomo: %v", err)
		logger.Error("[slot0] %s", errMsg)
		logger.Error("%s", errMsg)
		return err
	}

	c.cmd = cmd
	c.pid = cmd.Process.Pid

	logger.Info("mihomo process started (detached) with PID: %d", c.pid)
	logger.Info("[slot0] mihomo started (PID: %d)", c.pid)
	c.status = StatusRunning

	// Create lock file (used by c4cgo and by the next c4c session to re-attach).
	nodeName := ""
	nodeServer := ""
	nodePort := 0
	nodeType := ""
	if selectedNode != nil {
		nodeName = selectedNode.Name
		nodeServer = selectedNode.Server
		nodePort = selectedNode.Port
		nodeType = selectedNode.Type
	}
	lock := LockFile{
		Port:              c.port,
		APIPort:           c.apiPort,
		RedirPort:         c.redirPort,
		PID:               c.pid,
		SubID:             subID,
		Node:              nodeName,
		NodeServer:        nodeServer,
		NodePort:          nodePort,
		NodeType:          nodeType,
		DockerRedirectTCP: dockerRedirectTCP,
		ProxyHost:         c.proxyHost,
		APIHost:           c.apiHost,
		ConfigPath:        c.configPath,
		WorkDir:           workDir,
		LogPath:           c.logPath,
		Secret:            c.secret,
		StartedAt:         time.Now().UTC().Format(time.RFC3339),
		Detached:          true,
		EffectiveProxy:    c.effectiveProxy,
		UpstreamEnabled:   c.upstreamEnabled,
		UpstreamProtocol:  c.upstreamProtocol,
		UpstreamServer:    c.upstreamServer,
		UpstreamPort:      c.upstreamPort,
	}
	// Write lock file in v2 (MultiLockFile) format so it won't be misinterpreted
	// on TUI restart. Slot 0 is used as the default for the legacy Start() path.
	if err := updateMultiLockFile(func(mlf *MultiLockFile) {
		mlf.Instances[0] = lock
	}); err != nil {
		logger.Warn("Failed to create lock file: %v", err)
	}

	startedPID := c.pid
	startedCmd := c.cmd
	selectedName := c.effectiveProxy
	apiSecret := c.secret

	// Wait for mihomo API to be ready and switch to selected node (best-effort).
	if selectedName != "" {
		go func() {
			apiURL := fmt.Sprintf("http://%s:%d/proxies/Proxy", apiHost, apiPort)
			ready := false
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				req, err := http.NewRequest(http.MethodGet, apiURL, nil)
				if err != nil {
					continue
				}
				if apiSecret != "" {
					req.Header.Set("Authorization", "Bearer "+apiSecret)
				}
				resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
				if err == nil {
					_ = resp.Body.Close()
					if resp.StatusCode >= 200 && resp.StatusCode < 500 {
						ready = true
						break
					}
				}
			}
			if !ready {
				logger.Warn("mihomo API not ready, skip switching node")
				logger.Warn("[slot0] mihomo API not ready, skip switching node")
				return
			}

			c.mu.RLock()
			isStale := c.pid != startedPID || c.cmd != startedCmd || c.status != StatusRunning
			c.mu.RUnlock()
			if isStale {
				return
			}

			if err := c.SwitchNode(selectedName); err != nil {
				logger.Warn("Failed to switch to node %s: %v", selectedName, err)
				logger.Warn("[slot0] Failed to switch node: %v", err)
			}
		}()
	}

	// Monitor process while this controller is alive (prevents zombies).
	go func(pid int, cmd *exec.Cmd) {
		err := cmd.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.pid != pid || c.cmd != cmd {
			return
		}

		if c.status == StatusStopping {
			c.status = StatusStopped
			logger.Info("[slot0] mihomo stopped normally")
			c.removeLockFile()
			c.cmd = nil
			c.pid = 0
		} else if err != nil {
			c.status = StatusError
			errMsg := fmt.Sprintf("mihomo exited unexpectedly: %v", err)
			logger.Error("[slot0] %s", errMsg)
			logger.Error("%s", errMsg)
			c.removeLockFile()
			c.cmd = nil
			c.pid = 0
		} else {
			c.status = StatusStopped
			logger.Warn("[slot0] mihomo exited without error but unexpectedly")
			c.removeLockFile()
			c.cmd = nil
			c.pid = 0
		}
	}(startedPID, startedCmd)

	return nil
}

// Stop stops the proxy
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	lock, err := c.readLockFile()
	if err != nil || lock == nil {
		c.status = StatusStopped
		c.cmd = nil
		c.pid = 0
		return nil
	}

	if lock.PID <= 0 {
		c.removeLockFile()
		c.status = StatusStopped
		return nil
	}
	if !isProcessAlive(lock.PID) {
		c.removeLockFile()
		c.status = StatusStopped
		c.cmd = nil
		c.pid = 0
		return nil
	}

	if !processLooksLikeMihomo(lock.PID, lock.ConfigPath) {
		return fmt.Errorf("refusing to kill pid %d: not recognized as mihomo (config=%q)", lock.PID, lock.ConfigPath)
	}

	logger.Info("stopping mihomo (pid=%d)...", lock.PID)
	c.status = StatusStopping
	logger.Info("[slot0] Stopping mihomo (PID: %d)...", lock.PID)

	killErr := terminateProcess(lock.PID, 2*time.Second)

	if killErr != nil && isProcessAlive(lock.PID) {
		// Still alive: keep lock + fields so c4cgo can continue to attach.
		c.status = StatusError
		c.pid = lock.PID
		c.port = lock.Port
		c.apiPort = lock.APIPort
		c.redirPort = lock.RedirPort
		if lock.APIHost != "" {
			c.apiHost = lock.APIHost
		}
		if lock.ProxyHost != "" {
			c.proxyHost = lock.ProxyHost
		}
		c.secret = lock.Secret
		return killErr
	}

	// Stopped successfully (or already dead).
	c.removeLockFile()
	c.cmd = nil
	c.pid = 0
	c.status = StatusStopped

	logger.Info("mihomo stopped")
	return nil
}

// GetStatus returns the current proxy status
func (c *Controller) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// CurrentPort returns the currently active mixed port if the proxy core is running.
// If it is not running (or unknown), it falls back to the configured port.
func (c *Controller) CurrentPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.status == StatusRunning && c.port > 0 {
		return c.port
	}
	if c.cfg != nil && c.cfg.Proxy.MixedPort > 0 {
		return c.cfg.Proxy.MixedPort
	}
	return 0
}

func (c *Controller) setChainRuntimeLocked(slotID int, selectedNode *subscription.Node) {
	c.effectiveProxy = effectiveProxyName(slotID, selectedNode, c.cfg)
	c.upstreamEnabled = false
	c.upstreamProtocol = ""
	c.upstreamServer = ""
	c.upstreamPort = 0

	if upstream, ok := c.cfg.EffectiveUpstreamForSlot(slotID); ok {
		c.upstreamEnabled = true
		c.upstreamProtocol = upstream.Protocol
		c.upstreamServer = upstream.Server
		c.upstreamPort = upstream.Port
	}
}

// UpdateLockState best-effort updates the proxy.lock file with extra metadata for re-attach.
// It does not start/stop the proxy core.
func (c *Controller) UpdateLockState(subID string, node *subscription.Node) {
	_ = updateLockFile(func(l *LockFile) {
		if subID != "" {
			l.SubID = subID
		}
		if node != nil {
			l.Node = node.Name
			l.NodeServer = node.Server
			l.NodePort = node.Port
			l.NodeType = node.Type
			if !l.UpstreamEnabled {
				l.EffectiveProxy = node.Name
			}
		}
	})
}

// readLogs reads logs from a pipe
func (c *Controller) readLogs(pipe io.ReadCloser, source string) {
	defer pipe.Close()
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		logger.Info("[slot0] %s", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("Error reading %s: %v", source, err)
	}
}

// findMihomoBinaryDefault finds the mihomo binary
func findMihomoBinaryDefault() (string, error) {
	// Check in PATH first
	path, err := exec.LookPath("mihomo")
	if err == nil {
		return path, nil
	}

	// Check in common packaged locations (deb/rpm/Homebrew-style layouts)
	for _, candidate := range []string{
		"/usr/lib/clash4cli/mihomo",
		"/usr/lib64/clash4cli/mihomo",
		"/usr/local/lib/clash4cli/mihomo",
		"/opt/homebrew/lib/clash4cli/mihomo",
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Check in third_party directory relative to executable
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		candidates := []string{
			filepath.Join(execDir, "mihomo"),
			filepath.Join(execDir, "third_party", "mihomo", "mihomo"),
			filepath.Join(execDir, "third_party", "mihomo", getMihomoBinaryName()),
			filepath.Join(execDir, "..", "third_party", "mihomo", "mihomo"),
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	// Check in current working directory
	cwd, err := os.Getwd()
	if err == nil {
		candidates := []string{
			filepath.Join(cwd, "mihomo"),
			filepath.Join(cwd, "third_party", "mihomo", "mihomo"),
			filepath.Join(cwd, "third_party", "mihomo", getMihomoBinaryName()),
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("mihomo binary not found. Please install mihomo or place it in third_party/mihomo/")
}

// getMihomoBinaryName returns the platform-specific binary name
func getMihomoBinaryName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	name := fmt.Sprintf("mihomo-%s-%s", os, arch)
	if os == "windows" {
		name += ".exe"
	}
	return name
}

// Reload reloads the configuration
func (c *Controller) Reload(nodes []subscription.Node, selectedNode *subscription.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Generate new config
	c.setChainRuntimeLocked(0, selectedNode)
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg, 0, c.port, c.apiPort, c.apiHost, c.redirPort)
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}
	if c.secret != "" {
		mihomoConfig.Secret = c.secret
	}

	// Save config
	_, err = SaveConfig(mihomoConfig)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// TODO: Signal mihomo to reload via API
	// For now, we'd need to restart

	return nil
}

// SwitchNode switches the GLOBAL proxy to the specified node via API
func (c *Controller) SwitchNode(nodeName string) error {
	return c.SwitchGroup("Proxy", nodeName)
}

// SwitchGroup switches a select-type group to the specified entry via API.
// Example groups: "Proxy", "GLOBAL".
func (c *Controller) SwitchGroup(groupName, entryName string) error {
	if c.status != StatusRunning {
		return fmt.Errorf("proxy is not running")
	}

	if groupName == "" {
		return fmt.Errorf("group name cannot be empty")
	}

	// Call mihomo API to switch selector group
	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort := c.apiPort
	if apiPort == 0 {
		apiPort = 9090
	}
	apiURL := fmt.Sprintf("http://%s:%d/proxies/%s", apiHost, apiPort, groupName)
	payload := map[string]string{"name": entryName}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.doAPI(req)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info("[slot0] Switched %s to: %s", groupName, entryName)
	logger.Info("Switched %s to: %s", groupName, entryName)

	// NOTE: Do NOT update the lock file here with v1 updateLockFile().
	// The SlotManager already tracks node names via slotNodes and writes the
	// v2 MultiLockFile. A v1 write here would overwrite the v2 data with
	// zero-valued fields (PID=0, Port=0), breaking lock file restoration on
	// TUI restart.

	return nil
}

// SetMode switches mihomo mode via API. Supported modes: rule, global, direct.
func (c *Controller) SetMode(mode string) error {
	if c.status != StatusRunning {
		return fmt.Errorf("proxy is not running")
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "rule", "global", "direct":
	default:
		return fmt.Errorf("invalid mode: %s (expected rule|global|direct)", mode)
	}

	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort := c.apiPort
	if apiPort == 0 {
		apiPort = 9090
	}
	apiURL := fmt.Sprintf("http://%s:%d/configs", apiHost, apiPort)
	payload := map[string]string{"mode": mode}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	var lastStatus int
	for _, method := range []string{http.MethodPatch, http.MethodPut} {
		// mihomo/clash external controller typically supports PATCH /configs for live updates.
		// Some builds may use PUT, so we try both.
		req, err := http.NewRequest(method, apiURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.secret != "" {
			req.Header.Set("Authorization", "Bearer "+c.secret)
		}

		resp, err := c.doAPI(req)
		if err != nil {
			return fmt.Errorf("failed to call API: %w", err)
		}
		lastStatus = resp.StatusCode
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			lastStatus = 0
			break
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		// Only fall back to the next method on "not found" or "method not allowed".
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
			return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}
	}
	if lastStatus != 0 {
		return fmt.Errorf("API returned status %d", lastStatus)
	}

	logger.Info("[slot0] Mode switched to: %s", mode)
	logger.Info("Mode switched to: %s", mode)
	return nil
}

type versionResponse struct {
	Version string `json:"version"`
}

type rulesResponse struct {
	Rules []json.RawMessage `json:"rules"`
}

func (c *Controller) doAPI(req *http.Request) (*http.Response, error) {
	client := c.apiClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	}
	return client.Do(req)
}

// GetTrafficTotals returns the total uploaded/downloaded bytes reported by the core.
func (c *Controller) GetTrafficTotals() (uploadTotal int64, downloadTotal int64, err error) {
	up, down, _, err := c.GetTrafficAndConnCount()
	return up, down, err
}

// GetCoreVersion returns the mihomo core version string.
func (c *Controller) GetCoreVersion() (string, error) {
	c.mu.RLock()
	if c.status != StatusRunning {
		c.mu.RUnlock()
		return "", fmt.Errorf("proxy is not running")
	}

	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort := c.apiPort
	if apiPort == 0 {
		apiPort = 9090
	}
	secret := c.secret
	c.mu.RUnlock()

	apiURL := fmt.Sprintf("http://%s:%d/version", apiHost, apiPort)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := c.doAPI(req)
	if err != nil {
		return "", fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var out versionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return out.Version, nil
}

// GetRulesCount returns the number of rules loaded in the core.
func (c *Controller) GetRulesCount() (int, error) {
	c.mu.RLock()
	if c.status != StatusRunning {
		c.mu.RUnlock()
		return 0, fmt.Errorf("proxy is not running")
	}

	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort := c.apiPort
	if apiPort == 0 {
		apiPort = 9090
	}
	secret := c.secret
	c.mu.RUnlock()

	apiURL := fmt.Sprintf("http://%s:%d/rules", apiHost, apiPort)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := c.doAPI(req)
	if err != nil {
		return 0, fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var out rulesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}
	return len(out.Rules), nil
}

func (c *Controller) readLockFile() (*LockFile, error) {
	if c != nil && c.multiInstanceLock {
		return readSlotLockFile(c.lockSlotID)
	}
	return ReadLockFile()
}

// removeLockFile removes the controller's lock entry.
func (c *Controller) removeLockFile() {
	if c != nil && c.multiInstanceLock {
		_ = removeSlotLockFile(c.lockSlotID)
		return
	}
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return
	}
	os.Remove(lockPath)
}

// ReadLockFile reads the proxy.lock file (exported for c4cgo)
func ReadLockFile() (*LockFile, error) {
	mlf, err := ReadMultiLockFile()
	if err != nil {
		return nil, err
	}
	if mlf == nil || len(mlf.Instances) == 0 {
		return nil, os.ErrNotExist
	}
	if lock, ok := mlf.Instances[0]; ok {
		lockCopy := lock
		return &lockCopy, nil
	}

	ids := make([]int, 0, len(mlf.Instances))
	for id := range mlf.Instances {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	lock := mlf.Instances[ids[0]]
	lockCopy := lock
	return &lockCopy, nil
}

func writeLockFile(lockFile LockFile) error {
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(lockFile, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := lockPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, lockPath); err != nil {
		// Windows cannot rename over an existing file.
		_ = os.Remove(lockPath)
		return os.Rename(tmpPath, lockPath)
	}
	return nil
}

func updateLockFile(mut func(*LockFile)) error {
	lockFile, err := ReadLockFile()
	if err != nil {
		return err
	}
	mut(lockFile)
	return writeLockFile(*lockFile)
}

func (c *Controller) lockFileLooksAlive(lock *LockFile) bool {
	if lock == nil {
		return false
	}
	if lock.Port <= 0 || lock.Port >= 65536 || lock.PID <= 0 {
		return false
	}
	if !isProcessAlive(lock.PID) {
		return false
	}
	proxyHost := lock.ProxyHost
	if proxyHost == "" {
		proxyHost = "127.0.0.1"
	}
	// Retry TCP check: mihomo may be slow to accept connections on cold start
	for attempt := 0; attempt < 3; attempt++ {
		if isTCPReachable(proxyHost, lock.Port) {
			return true
		}
		if attempt < 2 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return false
}

func (c *Controller) applyLockFileLocked(lock *LockFile) {
	c.status = StatusRunning
	c.pid = lock.PID
	c.port = lock.Port
	c.apiPort = lock.APIPort
	c.redirPort = lock.RedirPort
	if lock.APIHost != "" {
		c.apiHost = lock.APIHost
	}
	if lock.ProxyHost != "" {
		c.proxyHost = lock.ProxyHost
	}
	c.configPath = lock.ConfigPath
	c.logPath = lock.LogPath
	c.secret = lock.Secret
	c.effectiveProxy = lock.EffectiveProxy
	c.upstreamEnabled = lock.UpstreamEnabled
	c.upstreamProtocol = lock.UpstreamProtocol
	c.upstreamServer = lock.UpstreamServer
	c.upstreamPort = lock.UpstreamPort
	if c.effectiveProxy == "" {
		c.effectiveProxy = lock.Node
	}
}

func (c *Controller) syncFromLockFile() {
	lock, err := ReadLockFile()
	if err != nil || lock == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lockFileLooksAlive(lock) {
		c.applyLockFileLocked(lock)
		logger.Info("Detected running mihomo from proxy.lock (pid=%d, port=%d)", lock.PID, lock.Port)
		logger.Info("[slot0] Detected running proxy core (PID: %d)", lock.PID)
		go c.monitorExternalProxy(lock.PID, lock.Port, lock.ProxyHost)
		return
	}

	// Stale lock.
	c.removeLockFile()
}

func (c *Controller) monitorExternalProxy(pid int, port int, host string) {
	if pid <= 0 || port <= 0 {
		return
	}
	if host == "" {
		host = "127.0.0.1"
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		alive := isProcessAlive(pid) && isTCPReachable(host, port)
		if alive {
			continue
		}

		c.mu.Lock()
		if c.pid != pid || c.status != StatusRunning {
			c.mu.Unlock()
			return
		}
		c.status = StatusStopped
		c.cmd = nil
		c.pid = 0
		c.removeLockFile()
		c.mu.Unlock()

		logger.Warn("[slot0] Proxy core stopped (detected)") // best-effort
		return
	}
}

func isTCPReachable(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := dialTimeout("tcp", addr, 800*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func prepareMihomoLogFile() (string, error) {
	logDir, err := config.GetLogDir()
	if err != nil {
		return "", err
	}
	if err := config.EnsureDir(logDir); err != nil {
		return "", err
	}

	logPath := filepath.Join(logDir, "mihomo.log")
	if info, err := os.Stat(logPath); err == nil && info.Mode().IsRegular() && info.Size() > 5*1024*1024 {
		backup := logPath + ".1"
		_ = os.Remove(backup)
		_ = os.Rename(logPath, backup)
	}
	return logPath, nil
}
