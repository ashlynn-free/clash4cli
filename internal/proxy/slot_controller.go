package proxy

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"rocket4cli/internal/config"
	"rocket4cli/internal/subscription"
)

// SlotController manages a single mihomo instance for a specific slot ID.
// It wraps the core logic from Controller but uses per-slot config/log paths and ports.
type SlotController struct {
	slotID int
	ctrl   *Controller
}

// NewSlotController creates a SlotController for the given slot.
func NewSlotController(slotID int, cfg *config.Config) *SlotController {
	c := &Controller{
		status:    StatusStopped,
		logCh:     make(chan LogEntry, 100),
		apiClient: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}},
		cfg:       cfg,
		apiHost:   "127.0.0.1",
		proxyHost: "127.0.0.1",
	}
	return &SlotController{
		slotID: slotID,
		ctrl:   c,
	}
}

// SlotID returns this controller's slot number.
func (sc *SlotController) SlotID() int {
	return sc.slotID
}

// Start starts the proxy instance for this slot.
// forbidden is a set of ports already used by other slots.
func (sc *SlotController) Start(subID string, nodes []subscription.Node, selectedNode *subscription.Node, forbidden map[int]struct{}) error {
	return sc.ctrl.StartSlot(sc.slotID, subID, nodes, selectedNode, forbidden)
}

// Stop stops this slot's mihomo process.
func (sc *SlotController) Stop() error {
	return sc.ctrl.Stop()
}

// SetMode sets the proxy mode (rule/global/direct).
func (sc *SlotController) SetMode(mode string) error {
	return sc.ctrl.SetMode(mode)
}

// SwitchNode switches the Proxy group to the specified node.
func (sc *SlotController) SwitchNode(nodeName string) error {
	return sc.ctrl.SwitchNode(nodeName)
}

// SwitchGroup switches a named group to the specified entry.
func (sc *SlotController) SwitchGroup(groupName, entryName string) error {
	return sc.ctrl.SwitchGroup(groupName, entryName)
}

// GetTrafficTotals returns cumulative upload/download bytes.
func (sc *SlotController) GetTrafficTotals() (int64, int64, error) {
	return sc.ctrl.GetTrafficTotals()
}

// GetStatus returns the current proxy status.
func (sc *SlotController) GetStatus() Status {
	return sc.ctrl.GetStatus()
}

// CurrentPort returns the active mixed port.
func (sc *SlotController) CurrentPort() int {
	return sc.ctrl.CurrentPort()
}

// APIPort returns the active API port.
func (sc *SlotController) APIPort() int {
	sc.ctrl.mu.RLock()
	defer sc.ctrl.mu.RUnlock()
	return sc.ctrl.apiPort
}

// PID returns the current process PID.
func (sc *SlotController) PID() int {
	sc.ctrl.mu.RLock()
	defer sc.ctrl.mu.RUnlock()
	return sc.ctrl.pid
}

// IsAttached returns true if this slot was restored from lockfile (not started by this process).
func (sc *SlotController) IsAttached() bool {
	sc.ctrl.mu.RLock()
	defer sc.ctrl.mu.RUnlock()
	return sc.ctrl.attached
}

// LogChannel returns the log channel.
func (sc *SlotController) LogChannel() <-chan LogEntry {
	return sc.ctrl.LogChannel()
}

// AttachFromLock restores a slot controller's state from a lock file entry.
func (sc *SlotController) AttachFromLock(lock *LockFile) {
	sc.ctrl.mu.Lock()
	defer sc.ctrl.mu.Unlock()

	if lock == nil {
		return
	}

	if sc.ctrl.lockFileLooksAlive(lock) {
		sc.ctrl.applyLockFileLocked(lock)
		sc.ctrl.attached = true // Mark as attached from lockfile
		go sc.ctrl.monitorExternalProxy(lock.PID, lock.Port, lock.ProxyHost)
	}
}

// slotConfigPath returns the config file path for a given slot.
func slotConfigPath(slotID int) (string, error) {
	workDir, err := config.GetMihomoWorkDir()
	if err != nil {
		return "", err
	}
	if slotID == 0 {
		return filepath.Join(workDir, "config.yaml"), nil
	}
	return filepath.Join(workDir, fmt.Sprintf("config-slot-%d.yaml", slotID)), nil
}

// slotLogPath returns the log file path for a given slot.
func slotLogPath(slotID int) (string, error) {
	logDir, err := config.GetLogDir()
	if err != nil {
		return "", err
	}
	if slotID == 0 {
		return filepath.Join(logDir, "mihomo.log"), nil
	}
	return filepath.Join(logDir, fmt.Sprintf("mihomo-slot-%d.log", slotID)), nil
}

// StartSlot is an extended Start that uses per-slot paths and port offsets.
func (c *Controller) StartSlot(slotID int, subID string, nodes []subscription.Node, selectedNode *subscription.Node, forbidden map[int]struct{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status == StatusRunning || c.status == StatusStarting {
		c.logCh <- LogEntry{Level: "warn", Message: "proxy already running"}
		return fmt.Errorf("proxy already running")
	}

	c.status = StatusStarting
	c.attached = false // Mark as started by this process (not from lockfile)
	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Starting proxy slot %d (detached)...", slotID)}

	secret, err := generateSecret()
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to generate api secret: %w", err)
	}

	// Determine listen host
	proxyListenHost := "127.0.0.1"
	proxyDialHost := "127.0.0.1"
	if c.cfg != nil && c.cfg.Proxy.AllowLan {
		bind := c.cfg.Proxy.BindAddress
		if bind == "" {
			bind = "0.0.0.0"
		}
		proxyListenHost = bind
		if bind != "0.0.0.0" && bind != "127.0.0.1" && bind != "::" && bind != "" {
			proxyDialHost = bind
		}
	}

	// Port selection using base + slot offset
	baseMixed := 17890
	baseAPI := 9090
	if c.cfg != nil {
		if c.cfg.Proxy.BaseMixedPort > 0 {
			baseMixed = c.cfg.Proxy.BaseMixedPort
		}
		if c.cfg.Proxy.BaseAPIPort > 0 {
			baseAPI = c.cfg.Proxy.BaseAPIPort
		}
	}

	if forbidden == nil {
		forbidden = map[int]struct{}{}
	}

	mixedPort := baseMixed + slotID
	if _, blocked := forbidden[mixedPort]; blocked {
		c.status = StatusError
		return fmt.Errorf("mixed-port %d (slot %d) is already used by another slot", mixedPort, slotID)
	}
	if !canListenTCP(proxyListenHost, mixedPort) {
		// Attempt to kill stale mihomo process on this port
		if killStaleProcessOnPort(mixedPort) {
			c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Killed stale mihomo process on port %d", mixedPort)}
		}
		if !canListenTCP(proxyListenHost, mixedPort) {
			c.status = StatusError
			return fmt.Errorf("mixed-port %d (slot %d) is already in use by another program; free the port or change base-mixed-port in config", mixedPort, slotID)
		}
	}
	forbidden[mixedPort] = struct{}{}

	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort := baseAPI + slotID
	if _, blocked := forbidden[apiPort]; blocked {
		c.status = StatusError
		return fmt.Errorf("api-port %d (slot %d) is already used by another slot", apiPort, slotID)
	}
	if !canListenTCP(apiHost, apiPort) {
		if killStaleProcessOnPort(apiPort) {
			c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Killed stale mihomo process on port %d", apiPort)}
		}
		if !canListenTCP(apiHost, apiPort) {
			c.status = StatusError
			return fmt.Errorf("api-port %d (slot %d) is already in use by another program; free the port or change base-api-port in config", apiPort, slotID)
		}
	}

	c.port = mixedPort
	c.apiPort = apiPort
	c.proxyHost = proxyDialHost
	c.secret = secret

	// Generate mihomo config
	mihomoConfig, err := GenerateConfig(nodes, selectedNode, c.cfg, c.port, c.apiPort, apiHost, 0)
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to generate config: %w", err)
	}
	mihomoConfig.Secret = secret

	// Save to per-slot config path
	configPath, err := slotConfigPath(slotID)
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to determine config path: %w", err)
	}
	if err := saveConfigToPath(mihomoConfig, configPath); err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to save config: %w", err)
	}
	c.configPath = configPath

	// Find mihomo binary
	mihomoPath, err := findMihomoBinary()
	if err != nil {
		c.status = StatusError
		return err
	}

	// Get working directory
	workDir, err := config.GetMihomoWorkDir()
	if err != nil {
		c.status = StatusError
		return err
	}

	logPath, err := slotLogPath(slotID)
	if err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to determine log path: %w", err)
	}
	if err := config.EnsureDir(filepath.Dir(logPath)); err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to create log dir: %w", err)
	}
	c.logPath = logPath

	// Start mihomo process
	cmd := exec.Command(mihomoPath, "-d", workDir, "-f", c.configPath)
	cmd.Dir = workDir
	configureDetachedCommand(cmd)

	devNull, logFile, err := openProcessFiles(logPath)
	if err != nil {
		c.status = StatusError
		return err
	}
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = devNull.Close()
		_ = logFile.Close()
		c.status = StatusError
		return fmt.Errorf("failed to start mihomo: %w", err)
	}

	_ = devNull.Close()
	_ = logFile.Close()

	c.cmd = cmd
	c.pid = cmd.Process.Pid
	c.status = StatusRunning

	c.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("mihomo slot %d started (PID: %d, mixed: %d, api: %d)", slotID, c.pid, c.port, c.apiPort)}

	startedPID := c.pid
	startedCmd := c.cmd
	selectedName := ""
	if selectedNode != nil {
		selectedName = selectedNode.Name
	}
	apiSecret := c.secret

	// Wait for API and switch node (best-effort)
	if selectedName != "" {
		go func() {
			apiURL := fmt.Sprintf("http://%s:%d/proxies/Proxy", apiHost, apiPort)
			ready := false
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
				if req == nil {
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
				return
			}

			c.mu.RLock()
			isStale := c.pid != startedPID || c.cmd != startedCmd || c.status != StatusRunning
			c.mu.RUnlock()
			if isStale {
				return
			}

			_ = c.SwitchNode(selectedName)
		}()
	}

	// Monitor process
	go func(pid int, cmd *exec.Cmd) {
		_ = cmd.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.pid != pid || c.cmd != cmd {
			return
		}
		c.status = StatusStopped
		c.cmd = nil
		c.pid = 0
		c.logCh <- LogEntry{Level: "warn", Message: fmt.Sprintf("mihomo slot %d stopped", slotID)}
	}(startedPID, startedCmd)

	return nil
}

// openProcessFiles opens /dev/null for stdin and the log file for stdout/stderr.
func openProcessFiles(logPath string) (devNull, logFile *os.File, err error) {
	devNull, err = os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open %s: %w", os.DevNull, err)
	}
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		_ = devNull.Close()
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}
	return devNull, logFile, nil
}

// saveConfigToPath saves a mihomo config to a specific file path.
func saveConfigToPath(cfg *MihomoConfig, path string) error {
	if err := config.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
