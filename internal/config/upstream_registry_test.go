package config

import (
	"testing"
)

func TestAddAndGetUpstreamProxy(t *testing.T) {
	cfg := DefaultConfig()
	id := cfg.AddUpstreamProxy(UpstreamProxyEntry{
		Label:    "My VPN",
		Protocol: "http",
		Server:   "vpn.example.com",
		Port:     8080,
	})
	if id == "" {
		t.Fatalf("expected non-empty ID")
	}

	entry, ok := cfg.GetUpstreamProxy(id)
	if !ok {
		t.Fatalf("expected to find upstream proxy %q", id)
	}
	if entry.Label != "My VPN" || entry.Server != "vpn.example.com" || entry.Port != 8080 {
		t.Fatalf("unexpected entry: %#v", entry)
	}
}

func TestUpdateUpstreamProxy(t *testing.T) {
	cfg := DefaultConfig()
	id := cfg.AddUpstreamProxy(UpstreamProxyEntry{
		Label:    "Old",
		Protocol: "http",
		Server:   "old.example.com",
		Port:     80,
	})

	err := cfg.UpdateUpstreamProxy(UpstreamProxyEntry{
		ID:       id,
		Label:    "New",
		Protocol: "socks5",
		Server:   "new.example.com",
		Port:     1080,
	})
	if err != nil {
		t.Fatalf("UpdateUpstreamProxy: %v", err)
	}

	entry, ok := cfg.GetUpstreamProxy(id)
	if !ok {
		t.Fatalf("expected to find updated entry")
	}
	if entry.Label != "New" || entry.Protocol != "socks5" || entry.Server != "new.example.com" {
		t.Fatalf("unexpected updated entry: %#v", entry)
	}
}

func TestUpdateUpstreamProxy_NotFound(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.UpdateUpstreamProxy(UpstreamProxyEntry{ID: "nonexistent", Label: "X"})
	if err == nil {
		t.Fatalf("expected error for non-existent ID")
	}
}

func TestDeleteUpstreamProxy(t *testing.T) {
	cfg := DefaultConfig()
	id := cfg.AddUpstreamProxy(UpstreamProxyEntry{
		Label:    "ToDelete",
		Protocol: "http",
		Server:   "del.example.com",
		Port:     80,
	})

	err := cfg.DeleteUpstreamProxy(id)
	if err != nil {
		t.Fatalf("DeleteUpstreamProxy: %v", err)
	}

	_, ok := cfg.GetUpstreamProxy(id)
	if ok {
		t.Fatalf("expected proxy to be deleted")
	}
}

func TestDeleteUpstreamProxy_InUse(t *testing.T) {
	cfg := DefaultConfig()
	id := cfg.AddUpstreamProxy(UpstreamProxyEntry{
		Label:    "InUse",
		Protocol: "http",
		Server:   "inuse.example.com",
		Port:     80,
	})
	cfg.SetSlotUpstreamRef(3, id)

	err := cfg.DeleteUpstreamProxy(id)
	if err == nil {
		t.Fatalf("expected error when deleting in-use upstream proxy")
	}

	slots := cfg.UpstreamProxyUsedBySlots(id)
	if len(slots) != 1 || slots[0] != 3 {
		t.Fatalf("expected slot 3 to use upstream, got %v", slots)
	}
}

func TestEffectiveUpstreamForSlot_Registry(t *testing.T) {
	cfg := DefaultConfig()
	id := cfg.AddUpstreamProxy(UpstreamProxyEntry{
		Label:    "Gateway",
		Protocol: "https",
		Server:   "gw.example.com",
		Port:     443,
		Username: "user",
		Password: "pass",
	})
	cfg.SetSlotUpstreamRef(1, id)

	upstream, ok := cfg.EffectiveUpstreamForSlot(1)
	if !ok {
		t.Fatalf("expected effective upstream for slot 1")
	}
	if upstream.Protocol != "https" || upstream.Server != "gw.example.com" || upstream.Port != 443 {
		t.Fatalf("unexpected upstream: %#v", upstream)
	}
	if upstream.Username != "user" || upstream.Password != "pass" {
		t.Fatalf("unexpected credentials: %#v", upstream)
	}
}

func TestEffectiveUpstreamForSlot_MissingRef(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetSlotUpstreamRef(1, "nonexistent")

	_, ok := cfg.EffectiveUpstreamForSlot(1)
	if ok {
		t.Fatalf("expected no effective upstream for missing ref")
	}
}

func TestEffectiveUpstreamForSlot_NoRef(t *testing.T) {
	cfg := DefaultConfig()
	_, ok := cfg.EffectiveUpstreamForSlot(5)
	if ok {
		t.Fatalf("expected no effective upstream for unconfigured slot")
	}
}

func TestEffectiveUpstreamForSlot_LegacyFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetSlotUpstream(2, UpstreamProxyConfig{
		Enabled:  true,
		Protocol: "socks5",
		Server:   "legacy.example.com",
		Port:     1080,
	})

	upstream, ok := cfg.EffectiveUpstreamForSlot(2)
	if !ok {
		t.Fatalf("expected legacy fallback to work")
	}
	if upstream.Protocol != "socks5" || upstream.Server != "legacy.example.com" {
		t.Fatalf("unexpected upstream: %#v", upstream)
	}
}

func TestMigrateInlineUpstreams(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Slots = []SlotConfig{
		{
			ID: 1,
			Upstream: UpstreamProxyConfig{
				Enabled:  true,
				Protocol: "http",
				Server:   "migrate.example.com",
				Port:     8080,
			},
		},
		{
			ID: 2,
			Upstream: UpstreamProxyConfig{
				Enabled:  false,
				Protocol: "socks5",
				Server:   "disabled.example.com",
				Port:     1080,
			},
		},
	}

	cfg.migrateInlineUpstreams()
	cfg.normalizeSlots()

	// Should have created 2 registry entries.
	entries := cfg.ListUpstreamProxies()
	if len(entries) != 2 {
		t.Fatalf("expected 2 migrated upstream entries, got %d", len(entries))
	}

	// Slot 1 was enabled -> should have UpstreamID set.
	ref1, ok := cfg.GetSlotUpstreamRef(1)
	if !ok || ref1 == "" {
		t.Fatalf("expected slot 1 to have upstream ref after migration")
	}
	upstream1, ok := cfg.EffectiveUpstreamForSlot(1)
	if !ok {
		t.Fatalf("expected effective upstream for migrated slot 1")
	}
	if upstream1.Server != "migrate.example.com" {
		t.Fatalf("unexpected migrated upstream: %#v", upstream1)
	}

	// Slot 2 was disabled -> should NOT have UpstreamID set.
	ref2, _ := cfg.GetSlotUpstreamRef(2)
	if ref2 != "" {
		t.Fatalf("expected slot 2 (disabled) to have empty upstream ref, got %q", ref2)
	}
}

func TestNormalizeSlots_KeepsUpstreamID(t *testing.T) {
	cfg := DefaultConfig()
	id := cfg.AddUpstreamProxy(UpstreamProxyEntry{
		Label:    "Keep",
		Protocol: "http",
		Server:   "keep.example.com",
		Port:     80,
	})
	cfg.Slots = []SlotConfig{
		{ID: 0, UpstreamID: id},
	}

	cfg.normalizeSlots()

	if len(cfg.Slots) != 1 {
		t.Fatalf("expected slot with UpstreamID to be kept, got %d slots", len(cfg.Slots))
	}
	if cfg.Slots[0].UpstreamID != id {
		t.Fatalf("expected UpstreamID=%q, got %q", id, cfg.Slots[0].UpstreamID)
	}
}
