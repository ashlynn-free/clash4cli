package app

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"rocket4cli/internal/config"
	"rocket4cli/internal/daemon"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

type daemonSlotService struct {
	client *daemon.Client
	mu     sync.RWMutex
	histFn func([]config.SlotHistoryEntry)
}

func newDaemonSlotService(client *daemon.Client) *daemonSlotService {
	return &daemonSlotService{client: client}
}

func (d *daemonSlotService) SetSaveHistoryFunc(fn func([]config.SlotHistoryEntry)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.histFn = fn
}

func (d *daemonSlotService) SyncFromLockFile() {}

func (d *daemonSlotService) ActiveSlots() []*proxy.SlotState {
	if d == nil || d.client == nil {
		return nil
	}
	snapshot, err := d.client.Runtime(context.Background())
	if err != nil || snapshot == nil {
		return nil
	}
	return snapshot.Active
}

func (d *daemonSlotService) ActiveSlotCount() int {
	return len(d.ActiveSlots())
}

func (d *daemonSlotService) Slot0Port() int {
	state := d.GetSlotState(0)
	if state == nil {
		return 0
	}
	return state.Port
}

func (d *daemonSlotService) GetSlotState(slotID int) *proxy.SlotState {
	if d == nil || d.client == nil {
		return nil
	}
	state, err := d.client.Slot(context.Background(), slotID)
	if err != nil {
		return nil
	}
	return (*proxy.SlotState)(state)
}

func (d *daemonSlotService) NodeSlots(nodeName string) []int {
	if nodeName == "" {
		return nil
	}
	active := d.ActiveSlots()
	ids := make([]int, 0, len(active))
	for _, state := range active {
		if state != nil && state.NodeName == nodeName {
			ids = append(ids, state.SlotID)
		}
	}
	sort.Ints(ids)
	return ids
}

func (d *daemonSlotService) SetSlotMode(slotID int, mode string) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("daemon client is nil")
	}
	return d.client.SetSlotMode(context.Background(), slotID, mode)
}

func (d *daemonSlotService) StartSlotWithMode(slotID int, subID string, nodes []subscription.Node, selected *subscription.Node, mode string) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("daemon client is nil")
	}
	return d.client.StartSlotWithMode(context.Background(), slotID, subID, nodes, selected, mode)
}

func (d *daemonSlotService) StopSlot(slotID int) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("daemon client is nil")
	}
	return d.client.StopSlot(context.Background(), slotID)
}

func (d *daemonSlotService) StopAll() {
	for _, state := range d.ActiveSlots() {
		if state == nil {
			continue
		}
		_ = d.StopSlot(state.SlotID)
	}
}

func (d *daemonSlotService) GetTrafficAndConnCountForSlot(slotID int) (int64, int64, int, error) {
	if d == nil || d.client == nil {
		return 0, 0, 0, fmt.Errorf("daemon client is nil")
	}
	traffic, err := d.client.Traffic(context.Background(), slotID)
	if err != nil {
		return 0, 0, 0, err
	}
	return traffic.UploadTotal, traffic.DownloadTotal, traffic.ConnCount, nil
}

func (d *daemonSlotService) GetConnectionsForSlot(slotID int) (proxy.ConnectionsSnapshot, error) {
	if d == nil || d.client == nil {
		return proxy.ConnectionsSnapshot{}, fmt.Errorf("daemon client is nil")
	}
	snapshot, err := d.client.Connections(context.Background(), slotID)
	if err != nil {
		return proxy.ConnectionsSnapshot{}, err
	}
	return *snapshot, nil
}

func (d *daemonSlotService) GetCoreVersion(slotID int) (string, error) {
	state := d.GetSlotState(slotID)
	if state == nil {
		return "", nil
	}
	return fmt.Sprintf("mihomo (pid %d)", state.PID), nil
}

func (d *daemonSlotService) GetRulesCount(slotID int) (int, error) {
	return 0, nil
}

func (d *daemonSlotService) GetStartedAt(slotID int) (string, error) {
	return "", nil
}
func (d *daemonSlotService) SaveConfig(cfg *config.Config) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("daemon client is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	return d.client.SaveConfig(context.Background(), cfg)
}
