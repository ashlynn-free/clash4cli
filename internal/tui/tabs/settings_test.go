package tabs

import (
	"runtime"
	"testing"

	"rocket4cli/internal/config"
)

func TestSettingsTab_FocusSkipsHeaderAndDisabledFields(t *testing.T) {
	cfg := config.DefaultConfig()
	tab := NewSettingsTab(cfg)

	// 0 -> 1
	tab.focusNext()
	if tab.focusedField != 1 {
		t.Fatalf("expected focusedField=1, got %d", tab.focusedField)
	}
	if tab.fields[tab.focusedField].key != "allow_lan" {
		t.Fatalf("expected key allow_lan, got %q", tab.fields[tab.focusedField].key)
	}

	// 1 -> 2
	tab.focusNext()
	if tab.fields[tab.focusedField].key != "update_via_proxy" {
		t.Fatalf("expected to focus update_via_proxy, got %q", tab.fields[tab.focusedField].key)
	}

	// 2 -> (skip header) -> linux-only (on linux) or language (non-linux)
	tab.focusNext()
	if runtime.GOOS == "linux" {
		if tab.fields[tab.focusedField].key != "tun_enabled" {
			t.Fatalf("expected to focus tun_enabled on linux, got %q", tab.fields[tab.focusedField].key)
		}
	} else {
		if tab.fields[tab.focusedField].key != "language" {
			t.Fatalf("expected to skip linux-only fields and focus language, got %q", tab.fields[tab.focusedField].key)
		}
	}
}
