package proxy

import (
	"fmt"
	"sync"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/subscription"
)

// MaxSlots is the maximum number of concurrent proxy instances.
const MaxSlots = 10

// SlotState describes the runtime state of a single proxy slot.
type SlotState struct {
	SlotID           int
	Mode             string // "rule", "global", "" (stopped)
	NodeName         string
	SubID            string
	Port             int
	APIPort          int
	PID              int
	UpstreamEnabled  bool
	UpstreamProtocol string
	UpstreamServer   string
	UpstreamPort     int
}

// SlotManager manages up to 10 concurrent proxy instances.
type SlotManager struct {
	mu         sync.RWMutex
	cfg        *config.Config
	slots      [MaxSlots]*SlotController
	slotModes  [MaxSlots]string // "rule", "global", "" (stopped)
	slotNodes  [MaxSlots]string // node name per slot
	slotSubIDs [MaxSlots]string // subscription ID per slot
	nodeToSlot map[string]int   // node name → slot index
	logCh      chan LogEntry
	// Callback to save slot history to config
	saveHistoryFunc func(h []config.SlotHistoryEntry)
	// Persistent slot history: maps slotID to historical state (preserved when slots stop)
	slotHistory [MaxSlots]*config.SlotHistoryEntry
}

// NewSlotManager creates a new SlotManager.
func NewSlotManager(cfg *config.Config) *SlotManager {
	sm := &SlotManager{
		cfg:        cfg,
		nodeToSlot: make(map[string]int),
		logCh:      make(chan LogEntry, 100),
	}
	return sm
}

// SetSaveHistoryFunc sets a callback to save slot history to config.
func (sm *SlotManager) SetSaveHistoryFunc(fn func([]config.SlotHistoryEntry)) {
	sm.mu.Lock()
	sm.saveHistoryFunc = fn
	sm.mu.Unlock()
}

// LogChannel returns a channel that receives log entries from all slots.
func (sm *SlotManager) LogChannel() <-chan LogEntry {
	return sm.logCh
}

// StartSlot starts a proxy instance in the given slot.
func (sm *SlotManager) StartSlot(slotID int, subID string, nodes []subscription.Node, selected *subscription.Node) error {
	return sm.StartSlotWithMode(slotID, subID, nodes, selected, "rule")
}

// StartSlotWithMode starts a proxy instance in the given slot with an explicit initial mode.
func (sm *SlotManager) StartSlotWithMode(slotID int, subID string, nodes []subscription.Node, selected *subscription.Node, mode string) error {
	if slotID < 0 || slotID >= MaxSlots {
		return fmt.Errorf("invalid slot ID: %d", slotID)
	}
	if selected == nil {
		return fmt.Errorf("no node selected")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if this slot is already running
	if sc := sm.slots[slotID]; sc != nil && sc.GetStatus() == StatusRunning {
		return fmt.Errorf("slot %d is already running", slotID)
	}

	// Check node exclusivity
	if existingSlot, ok := sm.nodeToSlot[selected.Name]; ok && existingSlot != slotID {
		return fmt.Errorf("node %q already bound to slot %d", selected.Name, existingSlot)
	}

	// Collect forbidden ports from other running slots
	forbidden := sm.collectForbiddenPortsLocked()

	// Create and start the slot controller
	sc := NewSlotController(slotID, sm.cfg)
	initialMode := normalizeProxyMode(mode)
	if err := sc.StartWithMode(subID, nodes, selected, forbidden, initialMode); err != nil {
		return err
	}

	// Clean up old slot if any
	sm.cleanSlotLocked(slotID)

	sm.slots[slotID] = sc
	sm.slotModes[slotID] = initialMode
	sm.slotNodes[slotID] = selected.Name
	sm.slotSubIDs[slotID] = subID
	sm.nodeToSlot[selected.Name] = slotID

	// Update lock file
	sm.writeLockFileLocked()

	// Forward logs from this slot
	go sm.forwardLogs(slotID, sc)

	// Monitor slot for unexpected death
	go sm.monitorSlot(slotID, sc)

	return nil
}

// StopSlot stops a proxy instance.
func (sm *SlotManager) StopSlot(slotID int) error {
	if slotID < 0 || slotID >= MaxSlots {
		return fmt.Errorf("invalid slot ID: %d", slotID)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sc := sm.slots[slotID]
	if sc == nil {
		return nil
	}

	err := sc.Stop()
	sm.cleanSlotLocked(slotID)
	sm.writeLockFileLocked()
	return err
}

// SetSlotMode sets the proxy mode for a slot.
func (sm *SlotManager) SetSlotMode(slotID int, mode string) error {
	if slotID < 0 || slotID >= MaxSlots {
		return fmt.Errorf("invalid slot ID: %d", slotID)
	}

	sm.mu.Lock()
	sc := sm.slots[slotID]
	if sc == nil {
		sm.mu.Unlock()
		return fmt.Errorf("slot %d is not running", slotID)
	}
	sc.ctrl.mu.RLock()
	effectiveName := sm.slotNodes[slotID]
	if sc.ctrl.upstreamEnabled {
		effectiveName = sc.ctrl.effectiveProxy
	}
	sc.ctrl.mu.RUnlock()
	sm.mu.Unlock()

	if err := sc.SetMode(mode); err != nil {
		return err
	}

	// In global mode, also align the GLOBAL group to the selected node.
	if mode == "global" && effectiveName != "" {
		_ = sc.SwitchGroup("GLOBAL", effectiveName)
	}

	sm.mu.Lock()
	sm.slotModes[slotID] = mode
	sm.saveSlotHistoryLocked()
	sm.mu.Unlock()

	return nil
}

// CycleSlotMode cycles: rule → global → stop. Returns ("", nil) when stopped.
func (sm *SlotManager) CycleSlotMode(slotID int) (string, error) {
	sm.mu.RLock()
	sc := sm.slots[slotID]
	currentMode := sm.slotModes[slotID]
	sm.mu.RUnlock()

	if sc == nil {
		return "", fmt.Errorf("slot %d is not running", slotID)
	}

	switch currentMode {
	case "rule":
		if err := sm.SetSlotMode(slotID, "global"); err != nil {
			return currentMode, err
		}
		return "global", nil
	case "global":
		// Stop the slot
		if err := sm.StopSlot(slotID); err != nil {
			return currentMode, err
		}
		return "", nil
	default:
		// Unknown state → try rule
		if err := sm.SetSlotMode(slotID, "rule"); err != nil {
			return currentMode, err
		}
		return "rule", nil
	}
}

// GetSlotState returns the state of a slot, or nil if not running.
func (sm *SlotManager) GetSlotState(slotID int) *SlotState {
	if slotID < 0 || slotID >= MaxSlots {
		return nil
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sc := sm.slots[slotID]
	if sc == nil {
		return nil
	}
	sc.ctrl.mu.RLock()
	defer sc.ctrl.mu.RUnlock()

	return &SlotState{
		SlotID:           slotID,
		Mode:             sm.slotModes[slotID],
		NodeName:         sm.slotNodes[slotID],
		SubID:            sm.slotSubIDs[slotID],
		Port:             sc.ctrl.port,
		APIPort:          sc.ctrl.apiPort,
		PID:              sc.ctrl.pid,
		UpstreamEnabled:  sc.ctrl.upstreamEnabled,
		UpstreamProtocol: sc.ctrl.upstreamProtocol,
		UpstreamServer:   sc.ctrl.upstreamServer,
		UpstreamPort:     sc.ctrl.upstreamPort,
	}
}

// NodeSlot returns the slot ID for a node, and whether it's bound.
func (sm *SlotManager) NodeSlot(nodeName string) (int, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	id, ok := sm.nodeToSlot[nodeName]
	return id, ok
}

// GetTrafficForSlot returns traffic totals for a slot.
func (sm *SlotManager) GetTrafficForSlot(slotID int) (int64, int64, error) {
	if slotID < 0 || slotID >= MaxSlots {
		return 0, 0, fmt.Errorf("invalid slot ID: %d", slotID)
	}

	sm.mu.RLock()
	sc := sm.slots[slotID]
	sm.mu.RUnlock()

	if sc == nil {
		return 0, 0, fmt.Errorf("slot %d is not running", slotID)
	}

	return sc.GetTrafficTotals()
}

// GetCoreVersion returns the mihomo core version for a slot.
func (sm *SlotManager) GetCoreVersion(slotID int) (string, error) {
	if slotID < 0 || slotID >= MaxSlots {
		return "", fmt.Errorf("invalid slot ID: %d", slotID)
	}

	sm.mu.RLock()
	sc := sm.slots[slotID]
	sm.mu.RUnlock()

	if sc == nil {
		return "", fmt.Errorf("slot %d is not running", slotID)
	}

	return sc.GetCoreVersion()
}

// GetRulesCount returns the number of rules loaded for a slot.
func (sm *SlotManager) GetRulesCount(slotID int) (int, error) {
	if slotID < 0 || slotID >= MaxSlots {
		return 0, fmt.Errorf("invalid slot ID: %d", slotID)
	}

	sm.mu.RLock()
	sc := sm.slots[slotID]
	sm.mu.RUnlock()

	if sc == nil {
		return 0, fmt.Errorf("slot %d is not running", slotID)
	}

	return sc.GetRulesCount()
}

// GetStartedAt returns the StartedAt timestamp for a slot from the lock file.
func (sm *SlotManager) GetStartedAt(slotID int) (string, error) {
	mlf, err := ReadMultiLockFile()
	if err != nil {
		return "", err
	}
	if mlf == nil {
		return "", fmt.Errorf("no lock file found")
	}
	lock, ok := mlf.Instances[slotID]
	if !ok {
		return "", fmt.Errorf("slot %d not found in lock file", slotID)
	}
	return lock.StartedAt, nil
}

// ActiveSlots returns a list of active slot states.
func (sm *SlotManager) ActiveSlots() []*SlotState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var states []*SlotState
	for i := 0; i < MaxSlots; i++ {
		if sm.slots[i] != nil && sm.slots[i].GetStatus() == StatusRunning {
			sm.slots[i].ctrl.mu.RLock()
			states = append(states, &SlotState{
				SlotID:           i,
				Mode:             sm.slotModes[i],
				NodeName:         sm.slotNodes[i],
				SubID:            sm.slotSubIDs[i],
				Port:             sm.slots[i].ctrl.port,
				APIPort:          sm.slots[i].ctrl.apiPort,
				PID:              sm.slots[i].ctrl.pid,
				UpstreamEnabled:  sm.slots[i].ctrl.upstreamEnabled,
				UpstreamProtocol: sm.slots[i].ctrl.upstreamProtocol,
				UpstreamServer:   sm.slots[i].ctrl.upstreamServer,
				UpstreamPort:     sm.slots[i].ctrl.upstreamPort,
			})
			sm.slots[i].ctrl.mu.RUnlock()
		}
	}
	return states
}

// ActiveSlotCount returns the number of running slots.
func (sm *SlotManager) ActiveSlotCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	count := 0
	for i := 0; i < MaxSlots; i++ {
		if sm.slots[i] != nil && sm.slots[i].GetStatus() == StatusRunning {
			count++
		}
	}
	return count
}

// HasRunningSlots returns true if any slot is running.
func (sm *SlotManager) HasRunningSlots() bool {
	return sm.ActiveSlotCount() > 0
}

// StopAll stops all running slots.
func (sm *SlotManager) StopAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i := 0; i < MaxSlots; i++ {
		if sm.slots[i] != nil {
			_ = sm.slots[i].Stop()
			sm.cleanSlotLocked(i)
		}
	}
	sm.writeLockFileLocked()
}

// SyncFromLockFile reads the multi lock file and restores slots.
func (sm *SlotManager) SyncFromLockFile() {
	mlf, err := ReadMultiLockFile()
	if err != nil || mlf == nil {
		if err != nil {
			logger.Warn("Failed to read proxy.lock: %v", err)
		}
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// First, restore slot history from config (for stopped slots that we want to remember)
	if sm.cfg != nil {
		for _, h := range sm.cfg.UI.SlotHistory {
			if h.SlotID >= 0 && h.SlotID < MaxSlots {
				sm.slotHistory[h.SlotID] = &h
			}
		}
	}

	for slotID, lock := range mlf.Instances {
		if slotID < 0 || slotID >= MaxSlots {
			continue
		}

		// Check if the instance is alive
		tmpCtrl := &Controller{
			apiHost:   "127.0.0.1",
			proxyHost: "127.0.0.1",
		}
		if !tmpCtrl.lockFileLooksAlive(&lock) {
			logger.Warn("Slot %d lock entry not alive (pid=%d, port=%d, host=%s): skipping",
				slotID, lock.PID, lock.Port, lock.ProxyHost)
			// Try to kill stale process by PID if it's still around
			if lock.PID > 0 && isProcessAlive(lock.PID) {
				_ = terminateProcess(lock.PID, 2*time.Second)
				logger.Info("Killed stale process PID %d for slot %d", lock.PID, slotID)
			}
			continue
		}

		sc := NewSlotController(slotID, sm.cfg)
		lockCopy := lock
		sc.AttachFromLock(&lockCopy)

		sm.slots[slotID] = sc
		// Restore mode from config history; default to "rule" if not saved
		restoredMode := "rule"
		if sm.slotHistory[slotID] != nil && sm.slotHistory[slotID].Mode != "" {
			restoredMode = sm.slotHistory[slotID].Mode
		}
		sm.slotModes[slotID] = restoredMode
		sm.slotNodes[slotID] = lock.Node
		sm.slotSubIDs[slotID] = lock.SubID
		if lock.Node != "" {
			sm.nodeToSlot[lock.Node] = slotID
		}
		// Update history with restored running slot
		sm.slotHistory[slotID] = &config.SlotHistoryEntry{
			SlotID:   slotID,
			NodeName: lock.Node,
			Mode:     restoredMode,
			SubID:    lock.SubID,
		}

		logger.Info("Restored slot %d from lock file (pid=%d, port=%d, node=%s)", slotID, lock.PID, lock.Port, lock.Node)
		sm.logCh <- LogEntry{Level: "info", Message: fmt.Sprintf("Restored slot %d (PID: %d, port: %d)", slotID, lock.PID, lock.Port)}

		// Apply non-default mode to mihomo after it's ready
		if restoredMode != "rule" {
			go func(id int, mode string) {
				time.Sleep(1 * time.Second)
				_ = sm.SetSlotMode(id, mode)
			}(slotID, restoredMode)
		}

		go sm.forwardLogs(slotID, sc)
	}

	// Rewrite lock file to remove dead entries
	sm.writeLockFileLocked()
}

// Slot0Port returns slot 0's port if running, or the configured default.
func (sm *SlotManager) Slot0Port() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sc := sm.slots[0]; sc != nil && sc.GetStatus() == StatusRunning {
		return sc.CurrentPort()
	}
	if sm.cfg != nil && sm.cfg.Proxy.MixedPort > 0 {
		return sm.cfg.Proxy.MixedPort
	}
	return 0
}

// -- internal helpers --

func (sm *SlotManager) cleanSlotLocked(slotID int) {
	if slotID < 0 || slotID >= MaxSlots {
		return
	}
	nodeName := sm.slotNodes[slotID]
	if nodeName != "" {
		if existing, ok := sm.nodeToSlot[nodeName]; ok && existing == slotID {
			delete(sm.nodeToSlot, nodeName)
		}
	}
	sm.slots[slotID] = nil
	sm.slotModes[slotID] = ""
	sm.slotNodes[slotID] = ""
	sm.slotSubIDs[slotID] = ""
	sm.slotHistory[slotID] = nil
}

func (sm *SlotManager) collectForbiddenPortsLocked() map[int]struct{} {
	forbidden := make(map[int]struct{})
	for i := 0; i < MaxSlots; i++ {
		if sc := sm.slots[i]; sc != nil && sc.GetStatus() == StatusRunning {
			if p := sc.CurrentPort(); p > 0 {
				forbidden[p] = struct{}{}
			}
			if p := sc.APIPort(); p > 0 {
				forbidden[p] = struct{}{}
			}
		}
	}
	return forbidden
}

func (sm *SlotManager) writeLockFileLocked() {
	mlf := &MultiLockFile{
		Version:   2,
		Instances: make(map[int]LockFile),
	}
	for i := 0; i < MaxSlots; i++ {
		sc := sm.slots[i]
		if sc == nil || sc.GetStatus() != StatusRunning {
			continue
		}
		sc.ctrl.mu.RLock()
		lock := LockFile{
			Port:             sc.ctrl.port,
			APIPort:          sc.ctrl.apiPort,
			PID:              sc.ctrl.pid,
			SubID:            sm.slotSubIDs[i],
			Node:             sm.slotNodes[i],
			ProxyHost:        sc.ctrl.proxyHost,
			APIHost:          sc.ctrl.apiHost,
			ConfigPath:       sc.ctrl.configPath,
			LogPath:          sc.ctrl.logPath,
			Secret:           sc.ctrl.secret,
			StartedAt:        time.Now().UTC().Format(time.RFC3339),
			Detached:         true,
			EffectiveProxy:   sc.ctrl.effectiveProxy,
			UpstreamEnabled:  sc.ctrl.upstreamEnabled,
			UpstreamProtocol: sc.ctrl.upstreamProtocol,
			UpstreamServer:   sc.ctrl.upstreamServer,
			UpstreamPort:     sc.ctrl.upstreamPort,
		}
		sc.ctrl.mu.RUnlock()
		mlf.Instances[i] = lock
	}
	if err := writeMultiLockFile(mlf); err != nil {
		logger.Warn("Failed to write multi lock file: %v", err)
	}
	// Also save slot history to config for UI display on restart
	sm.saveSlotHistoryLocked()
}

func (sm *SlotManager) saveSlotHistoryLocked() {
	if sm.cfg == nil {
		return
	}
	history := make([]config.SlotHistoryEntry, 0, MaxSlots)
	for i := 0; i < MaxSlots; i++ {
		// Use running slot state if available, otherwise use persistent history
		if sm.slots[i] != nil && sm.slotNodes[i] != "" && sm.slotModes[i] != "" {
			// Update history from running slot
			sm.slotHistory[i] = &config.SlotHistoryEntry{
				SlotID:   i,
				NodeName: sm.slotNodes[i],
				Mode:     sm.slotModes[i],
				SubID:    sm.slotSubIDs[i],
			}
		}
		// Collect all non-nil history entries
		if sm.slotHistory[i] != nil {
			history = append(history, *sm.slotHistory[i])
		}
	}
	sm.cfg.UI.SlotHistory = history

	// Call the custom save function if set (for async config persistence)
	if sm.saveHistoryFunc != nil {
		historyCopy := make([]config.SlotHistoryEntry, len(history))
		copy(historyCopy, history)
		go sm.saveHistoryFunc(historyCopy)
	}
}

func (sm *SlotManager) forwardLogs(slotID int, sc *SlotController) {
	ch := sc.LogChannel()
	for entry := range ch {
		prefixed := LogEntry{
			Level:   entry.Level,
			Message: fmt.Sprintf("[slot%d] %s", slotID, entry.Message),
		}
		select {
		case sm.logCh <- prefixed:
		default:
		}
	}
}

func (sm *SlotManager) monitorSlot(slotID int, sc *SlotController) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if sc.GetStatus() != StatusRunning {
			// Only clean up if we started this slot (not attached from lockfile)
			if !sc.IsAttached() {
				sm.mu.Lock()
				if sm.slots[slotID] == sc {
					sm.cleanSlotLocked(slotID)
					sm.writeLockFileLocked()
				}
				sm.mu.Unlock()
			}
			return
		}
	}
}
