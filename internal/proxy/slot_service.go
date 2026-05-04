package proxy

import (
	"rocket4cli/internal/config"
	"rocket4cli/internal/subscription"
)

// SlotService defines the runtime slot control surface used by clients.
// SlotManager satisfies this interface.
type SlotService interface {
	SetSaveHistoryFunc(func([]config.SlotHistoryEntry))
	SyncFromLockFile()
	ActiveSlots() []*SlotState
	ActiveSlotCount() int
	Slot0Port() int
	GetSlotState(slotID int) *SlotState
	NodeSlots(nodeName string) []int
	SetSlotMode(slotID int, mode string) error
	StartSlotWithMode(slotID int, subID string, nodes []subscription.Node, selected *subscription.Node, mode string) error
	StopSlot(slotID int) error
	StopAll()
	GetTrafficAndConnCountForSlot(slotID int) (int64, int64, int, error)
	GetCoreVersion(slotID int) (string, error)
	GetRulesCount(slotID int) (int, error)
	GetStartedAt(slotID int) (string, error)
}
