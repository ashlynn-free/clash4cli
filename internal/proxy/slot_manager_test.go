package proxy

import (
	"reflect"
	"testing"

	"rocket4cli/internal/config"
)

func TestSlotManager_NodeSlotsAllowsSharedNode(t *testing.T) {
	sm := NewSlotManager(config.DefaultConfig())

	sm.slots[1] = &SlotController{ctrl: &Controller{status: StatusRunning}}
	sm.slotNodes[1] = "shared-node"
	sm.slots[3] = &SlotController{ctrl: &Controller{status: StatusRunning}}
	sm.slotNodes[3] = "shared-node"
	sm.slots[5] = &SlotController{ctrl: &Controller{status: StatusStopped}}
	sm.slotNodes[5] = "shared-node"

	if got := sm.NodeSlots("shared-node"); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("expected shared node in slots [1 3], got %v", got)
	}

	if got, ok := sm.NodeSlot("shared-node"); !ok || got != 1 {
		t.Fatalf("expected NodeSlot to return first running slot 1, got slot=%d ok=%v", got, ok)
	}

	sm.cleanSlotLocked(1)

	if got := sm.NodeSlots("shared-node"); !reflect.DeepEqual(got, []int{3}) {
		t.Fatalf("expected remaining shared node slot [3], got %v", got)
	}
}
