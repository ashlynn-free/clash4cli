package proxy

import (
	"net"
	"os/exec"
	"reflect"
	"strconv"
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

func TestSlotManager_StartSlot_DirectOnly_NoMihomo(t *testing.T) {
	setupTestHome(t)

	// Mock listenTCP so port checks pass without binding real ports
	prev := listenTCP
	defer func() { listenTCP = prev }()
	listenTCP = func(network, address string) (net.Listener, error) {
		_, portStr, _ := net.SplitHostPort(address)
		p, _ := strconv.Atoi(portStr)
		return &fakeListener{addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p}}, nil
	}

	restoreFMB := SetFindMihomoBinaryForTests(func() (string, error) {
		return "/fake/mihomo", nil
	})
	defer restoreFMB()

	restoreSMP := SetStartMihomoProcessForTests(func(mihomoPath, workDir, cfgPath, logPath string) (*exec.Cmd, error) {
		cmd := exec.Command("sleep", "10")
		if err := cmd.Start(); err != nil {
			t.Fatalf("stub start: %v", err)
		}
		return cmd, nil
	})
	defer restoreSMP()

	sm := NewSlotManager(config.DefaultConfig())

	// Start slot 0 with no nodes (direct-only)
	err := sm.StartSlotWithMode(0, "", nil, nil, "rule")
	if err != nil {
		t.Fatalf("StartSlotWithMode: %v", err)
	}

	defer func() {
		// Kill the stub process directly (Stop() checks process name which won't match "sleep")
		if sc := sm.slots[0]; sc != nil {
			sc.ctrl.mu.Lock()
			if sc.ctrl.cmd != nil && sc.ctrl.cmd.Process != nil {
				_ = sc.ctrl.cmd.Process.Kill()
				_ = sc.ctrl.cmd.Wait()
			}
			sc.ctrl.status = StatusStopped
			sc.ctrl.mu.Unlock()
		}
	}()

	if sm.ActiveSlotCount() != 1 {
		t.Fatalf("expected 1 active slot, got %d", sm.ActiveSlotCount())
	}

	state := sm.GetSlotState(0)
	if state == nil {
		t.Fatal("expected slot 0 state")
	}
	if state.Mode != "rule" {
		t.Fatalf("expected mode rule, got %s", state.Mode)
	}
	if state.NodeName != "" {
		t.Fatalf("expected empty node name for direct-only, got %q", state.NodeName)
	}
	if state.Port == 0 {
		t.Fatal("expected non-zero port")
	}
}
