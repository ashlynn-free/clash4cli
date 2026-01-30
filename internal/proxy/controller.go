package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	Port    int    `json:"port"`
	APIPort int    `json:"api_port"`
	PID     int    `json:"pid"`
	Node    string `json:"node,omitempty"`
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
	cancel     context.CancelFunc
	logCh      chan LogEntry
	cfg        *config.Config
	configPath string
}

// NewController creates a new proxy controller
func NewController(cfg *config.Config) *Controller {
	return &Controller{
		status: StatusStopped,
		logCh:  make(chan LogEntry, 100),
		cfg:    cfg,
	}
}

// Start starts the proxy with given nodes
func (c *Controller) Start(nodes []subscription.Node, selectedNode *subscription.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status == StatusRunning || c.status == StatusStarting {
		c.logCh <- LogEntry{Level: "warn", Message: "proxy already running"}
		return fmt.Errorf("proxy already running")
	}

	c.status = StatusStarting
	c.logCh <- LogEntry{Level: "info", Message: "Starting proxy..."}

	// Generate mihomo config
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg)
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to generate config: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to generate config: %w", err)
	}

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

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// Start mihomo process
	c.cmd = exec.CommandContext(ctx, mihomoPath, "-d", workDir, "-f", c.configPath)
	c.cmd.Dir = workDir

	// Capture stdout and stderr
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to get stdout pipe: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to get stderr pipe: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start reading logs
	go c.readLogs(stdout, "stdout")
	go c.readLogs(stderr, "stderr")

	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Executing: %s -d %s -f %s", mihomoPath, workDir, c.configPath)}

	if err := c.cmd.Start(); err != nil {
		c.status = StatusError
		errMsg := fmt.Sprintf("failed to start mihomo: %v", err)
		c.logCh <- LogEntry{Level: "error", Message: errMsg}
		logger.Error("%s", errMsg)
		return fmt.Errorf("failed to start mihomo: %w", err)
	}
	startedCmd := c.cmd

	logger.Info("mihomo process started with PID: %d", c.cmd.Process.Pid)
	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("mihomo started (PID: %d)", c.cmd.Process.Pid)}
	c.status = StatusRunning

	// Create lock file
	nodeName := ""
	if selectedNode != nil {
		nodeName = selectedNode.Name
	}
	if err := c.createLockFile(c.cfg.Proxy.MixedPort, 9090, c.cmd.Process.Pid, nodeName); err != nil {
		logger.Warn("Failed to create lock file: %v", err)
	}

	// Wait for mihomo API to be ready and switch to selected node
	if selectedNode != nil {
		selectedName := selectedNode.Name
		go func() {
			// Wait for mihomo to be ready (API available)
			apiURL := "http://127.0.0.1:9090/proxies/Proxy"
			ready := false
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				resp, err := http.Get(apiURL)
				if err == nil {
					resp.Body.Close()
					ready = true
					break
				}
			}
			if !ready {
				logger.Warn("mihomo API not ready, skip switching node")
				c.logCh <- LogEntry{Level: "warn", Message: "mihomo API not ready, skip switching node"}
				return
			}

			c.mu.RLock()
			isStale := c.cmd != startedCmd || c.status != StatusRunning
			c.mu.RUnlock()
			if isStale {
				return
			}

			// Switch our selector group to the selected node
			if err := c.SwitchNode(selectedName); err != nil {
				logger.Warn("Failed to switch to node %s: %v", selectedName, err)
				c.logCh <- LogEntry{Level: "warn", Message: fmt.Sprintf("Failed to switch node: %v", err)}
			}
		}()
	}

	// Monitor process
	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.cmd != cmd {
			return
		}

		if c.status == StatusStopping {
			c.status = StatusStopped
			c.logCh <- LogEntry{Level: "info", Message: "mihomo stopped normally"}
			c.removeLockFile()
			c.cmd = nil
			c.cancel = nil
		} else if err != nil {
			c.status = StatusError
			errMsg := fmt.Sprintf("mihomo exited unexpectedly: %v", err)
			c.logCh <- LogEntry{Level: "error", Message: errMsg}
			logger.Error("%s", errMsg)
			c.removeLockFile()
			c.cmd = nil
			c.cancel = nil
		} else {
			c.status = StatusStopped
			c.logCh <- LogEntry{Level: "warn", Message: "mihomo exited without error but unexpectedly"}
			c.removeLockFile()
			c.cmd = nil
			c.cancel = nil
		}
	}(startedCmd)

	return nil
}

// Stop stops the proxy
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status != StatusRunning && c.status != StatusStarting {
		return nil
	}

	logger.Info("stopping mihomo...")
	c.status = StatusStopping

	if c.cancel != nil {
		c.cancel()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	// Remove lock file
	c.removeLockFile()

	logger.Info("mihomo stopped")
	return nil
}

// GetStatus returns the current proxy status
func (c *Controller) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
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
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg)
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
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
	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s", groupName)
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

	// Update lock file with new node name
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.createLockFile(c.cfg.Proxy.MixedPort, 9090, c.cmd.Process.Pid, entryName)
	}

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

	apiURL := "http://127.0.0.1:9090/configs"
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

// createLockFile creates the proxy.lock file
func (c *Controller) createLockFile(port, apiPort, pid int, nodeName string) error {
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return err
	}

	lockFile := LockFile{
		Port:    port,
		APIPort: apiPort,
		PID:     pid,
		Node:    nodeName,
	}

	data, err := json.MarshalIndent(lockFile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lockPath, data, 0644)
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
