package proxy

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	PID        int    `json:"pid"`
	SubID      string `json:"sub_id,omitempty"`
	Node       string `json:"node,omitempty"`
	NodeServer string `json:"node_server,omitempty"`
	NodePort   int    `json:"node_port,omitempty"`
	NodeType   string `json:"node_type,omitempty"`
	ProxyHost  string `json:"proxy_host,omitempty"`
	APIHost    string `json:"api_host,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`
	WorkDir    string `json:"work_dir,omitempty"`
	LogPath    string `json:"log_path,omitempty"`
	Secret     string `json:"secret,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	Detached   bool   `json:"detached,omitempty"`
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

// LogEntry represents a log entry from mihomo
type LogEntry struct {
	Level   string
	Message string
}

// Controller controls the mihomo process
type Controller struct {
	cmd        *exec.Cmd
	status     Status
	mu         sync.RWMutex
	logCh      chan LogEntry
	cfg        *config.Config
	configPath string
	pid        int
	port       int
	apiPort    int
	apiHost    string
	proxyHost  string
	secret     string
	logPath    string
}

// NewController creates a new proxy controller
func NewController(cfg *config.Config) *Controller {
	c := &Controller{
		status:    StatusStopped,
		logCh:     make(chan LogEntry, 100),
		cfg:       cfg,
		apiHost:   "127.0.0.1",
		proxyHost: "127.0.0.1",
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
		c.logCh <- LogEntry{Level: "warn", Message: "proxy already running"}
		return fmt.Errorf("proxy already running")
	}
	if lock, err := ReadLockFile(); err == nil && lock != nil {
		if c.lockFileLooksAlive(lock) {
			c.applyLockFileLocked(lock)
			c.logCh <- LogEntry{Level: "warn", Message: "proxy already running (detected from proxy.lock)"}
			return fmt.Errorf("proxy already running")
		}
		// Stale lock: best-effort remove.
		c.removeLockFile()
	}

	c.status = StatusStarting
	c.logCh <- LogEntry{Level: "info", Message: "Starting proxy (detached)..."}

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

	c.port = mixedPort
	c.apiPort = apiPort
	c.proxyHost = proxyDialHost
	c.secret = secret

	// Generate mihomo config
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg, c.port, c.apiPort, apiHost)
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to generate config: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to generate config: %w", err)
	}
	mihomoConfig.Secret = secret

	// Save config
	configPath, err := SaveConfig(mihomoConfig)
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to save config: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to save config: %w", err)
	}
	c.configPath = configPath
	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Config saved to: %s", configPath)}

	// Find mihomo binary
	mihomoPath, err := findMihomoBinary()
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("mihomo binary not found: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return err
	}
	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Using mihomo: %s", mihomoPath)}

	// Get working directory
	workDir, err := config.GetMihomoWorkDir()
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to get work dir: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return err
	}

	logPath, err := prepareMihomoLogFile()
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to prepare mihomo log file: %w", err)
	}
	c.logPath = logPath

	// Start mihomo process in detached mode so closing c4c won't stop it.
	cmd := exec.Command(mihomoPath, "-d", workDir, "-f", c.configPath)
	cmd.Dir = workDir
	configureDetachedCommand(cmd)

	// Redirect stdio to files (no pipes) so the child won't die on SIGPIPE when
	// the UI exits.
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to open %s: %w", os.DevNull, err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		_ = devNull.Close()
		c.status = StatusError
		return fmt.Errorf("failed to open mihomo log file: %w", err)
	}
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Executing (detached): %s -d %s -f %s", mihomoPath, workDir, c.configPath)}

	if err := cmd.Start(); err != nil {
		_ = devNull.Close()
		_ = logFile.Close()
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to start mihomo: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to start mihomo: %w", err)
	}

	// Close parent copies of file descriptors; the child keeps its own.
	_ = devNull.Close()
	_ = logFile.Close()

	c.cmd = cmd
	c.pid = cmd.Process.Pid

	logger.Info("mihomo process started (detached) with PID: %d", c.pid)
	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("mihomo started (PID: %d)", c.pid)}
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
		Port:       c.port,
		APIPort:    c.apiPort,
		PID:        c.pid,
		SubID:      subID,
		Node:       nodeName,
		NodeServer: nodeServer,
		NodePort:   nodePort,
		NodeType:   nodeType,
		ProxyHost:  c.proxyHost,
		APIHost:    c.apiHost,
		ConfigPath: c.configPath,
		WorkDir:    workDir,
		LogPath:    c.logPath,
		Secret:     c.secret,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		Detached:   true,
	}
	if err := writeLockFile(lock); err != nil {
		logger.Warn("Failed to create lock file: %v", err)
	}

	startedPID := c.pid
	startedCmd := c.cmd
	selectedName := ""
	if selectedNode != nil {
		selectedName = selectedNode.Name
	}
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
				c.logCh <- LogEntry{Level: "warn", Message: "mihomo API not ready, skip switching node"}
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
				c.logCh <- LogEntry{Level: "warn", Message: fmt.Sprintf("Failed to switch node: %v", err)}
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
			c.logCh <- LogEntry{Level: "info", Message: "mihomo stopped normally"}
			c.removeLockFile()
			c.cmd = nil
			c.pid = 0
		} else if err != nil {
			c.status = StatusError
			errMsg := fmt.Sprintf("mihomo exited unexpectedly: %v", err)
			c.logCh <- LogEntry{Level: "error", Message: errMsg}
			logger.Error("%s", errMsg)
			c.removeLockFile()
			c.cmd = nil
			c.pid = 0
		} else {
			c.status = StatusStopped
			c.logCh <- LogEntry{Level: "warn", Message: "mihomo exited without error but unexpectedly"}
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

	lock, err := ReadLockFile()
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
	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Stopping mihomo (PID: %d)...", lock.PID)}

	killErr := terminateProcess(lock.PID, 2*time.Second)

	if killErr != nil && isProcessAlive(lock.PID) {
		// Still alive: keep lock + fields so c4cgo can continue to attach.
		c.status = StatusError
		c.pid = lock.PID
		c.port = lock.Port
		c.apiPort = lock.APIPort
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
		}
	})
}

// LogChannel returns the log channel
func (c *Controller) LogChannel() <-chan LogEntry {
	return c.logCh
}

// readLogs reads logs from a pipe
func (c *Controller) readLogs(pipe io.ReadCloser, source string) {
	defer pipe.Close() // Ensure pipe is closed when done
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		select {
		case c.logCh <- LogEntry{Level: "info", Message: line}:
		default:
			// Drop log if channel is full
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("Error reading %s: %v", source, err)
	}
}

// findMihomoBinary finds the mihomo binary
func findMihomoBinary() (string, error) {
	// Check in PATH first
	path, err := exec.LookPath("mihomo")
	if err == nil {
		return path, nil
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
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg, c.port, c.apiPort, c.apiHost)
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

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Switched %s to: %s", groupName, entryName)}
	logger.Info("Switched %s to: %s", groupName, entryName)

	_ = updateLockFile(func(l *LockFile) {
		l.Node = entryName
	})

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

	client := &http.Client{Timeout: 5 * time.Second}
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

		resp, err := client.Do(req)
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

	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Mode switched to: %s", mode)}
	logger.Info("Mode switched to: %s", mode)
	return nil
}

// removeLockFile removes the proxy.lock file
func (c *Controller) removeLockFile() {
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return
	}
	os.Remove(lockPath)
}

// ReadLockFile reads the proxy.lock file (exported for c4cgo)
func ReadLockFile() (*LockFile, error) {
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}

	var lockFile LockFile
	if err := json.Unmarshal(data, &lockFile); err != nil {
		return nil, err
	}

	return &lockFile, nil
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
	proxyHost := lock.ProxyHost
	if proxyHost == "" {
		proxyHost = "127.0.0.1"
	}
	if !isTCPReachable(proxyHost, lock.Port) {
		return false
	}
	if !isProcessAlive(lock.PID) {
		return false
	}
	return true
}

func (c *Controller) applyLockFileLocked(lock *LockFile) {
	c.status = StatusRunning
	c.pid = lock.PID
	c.port = lock.Port
	c.apiPort = lock.APIPort
	if lock.APIHost != "" {
		c.apiHost = lock.APIHost
	}
	if lock.ProxyHost != "" {
		c.proxyHost = lock.ProxyHost
	}
	c.configPath = lock.ConfigPath
	c.logPath = lock.LogPath
	c.secret = lock.Secret
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
		c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Detected running proxy core (PID: %d)", lock.PID)}
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

		c.logCh <- LogEntry{Level: "warn", Message: "Proxy core stopped (detected)"} // best-effort
		return
	}
}

func isTCPReachable(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
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
