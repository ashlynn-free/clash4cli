package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAllDirs_CreatesExpectedDirs(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", base)

	if err := EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	wantDirs := []string{
		base,
		filepath.Join(base, "cache"),
		filepath.Join(base, "logs"),
		filepath.Join(base, "subscriptions"),
		filepath.Join(base, "mihomo"),
	}
	for _, d := range wantDirs {
		st, err := os.Stat(d)
		if err != nil {
			t.Fatalf("expected dir %s to exist: %v", d, err)
		}
		if !st.IsDir() {
			t.Fatalf("expected %s to be a directory", d)
		}
	}
}

func TestLoad_Save_RoundTrip(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", base)

	// First load should create a config.yaml.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected config")
	}

	configPath := filepath.Join(base, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config.yaml to exist: %v", err)
	}

	// Mutate and save.
	cfg.Language = "zh_cn"
	cfg.Proxy.MixedPort = 19999
	cfg.SetSlotNodeRef(2, SlotNodeRef{
		SubID:      "sub-1",
		NodeName:   "NodeA",
		NodeServer: "node.example.com",
		NodePort:   443,
		NodeType:   "ss",
	})
	cfg.SetSlotStartMode(2, "global")
	cfg.SetSlotManualMixedPort(2, 21090)
	cfg.SetSlotUpstream(2, UpstreamProxyConfig{
		Enabled:  true,
		Protocol: "https",
		Server:   "gateway.example.com",
		Port:     443,
		Username: "alice",
		Password: "secret",
	})
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify persisted fields.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Language != "zh_cn" {
		t.Fatalf("expected Language=zh_cn, got %q", cfg2.Language)
	}
	if cfg2.Proxy.MixedPort != 19999 {
		t.Fatalf("expected MixedPort=19999, got %d", cfg2.Proxy.MixedPort)
	}
	if cfg2.Proxy.BaseMixedPort != 19999 {
		t.Fatalf("expected BaseMixedPort=19999, got %d", cfg2.Proxy.BaseMixedPort)
	}
	ref, ok := cfg2.GetSlotNodeRef(2)
	if !ok {
		t.Fatalf("expected slot 2 node ref to persist")
	}
	if ref.SubID != "sub-1" || ref.NodeName != "NodeA" || ref.NodeServer != "node.example.com" || ref.NodePort != 443 {
		t.Fatalf("unexpected slot node ref: %#v", ref)
	}
	if got := cfg2.GetSlotStartMode(2); got != "global" {
		t.Fatalf("expected slot 2 start mode global, got %q", got)
	}
	if port, manual, ok := cfg2.SlotMixedPort(2); !ok || !manual || port != 21090 {
		t.Fatalf("unexpected slot 2 manual port: port=%d manual=%v ok=%v", port, manual, ok)
	}
	// After reload, the inline upstream is migrated to the registry.
	// EffectiveUpstreamForSlot should still resolve it.
	upstream, ok := cfg2.EffectiveUpstreamForSlot(2)
	if !ok {
		t.Fatalf("expected slot 2 upstream config to persist via registry")
	}
	if upstream.Protocol != "https" || upstream.Server != "gateway.example.com" || upstream.Port != 443 {
		t.Fatalf("unexpected upstream config after reload: %#v", upstream)
	}
}

func TestGetBaseDir_EnvOverride(t *testing.T) {
	base := filepath.Join(t.TempDir(), "  base  ")
	t.Setenv("CLASH4CLI_HOME", base)

	got, err := GetBaseDir()
	if err != nil {
		t.Fatalf("GetBaseDir: %v", err)
	}
	want := filepath.Clean(strings.TrimSpace(base))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestConfig_NormalizeSlots(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Slots = []SlotConfig{
		{ID: 12, Upstream: UpstreamProxyConfig{Enabled: true, Protocol: "http", Server: "bad", Port: 80}},
		{ID: 2, SubID: " sub-1 ", NodeName: " NodeA ", PortMode: "manual", ManualMixedPort: 19090, Upstream: UpstreamProxyConfig{Enabled: true, Protocol: " https ", Server: " gateway ", Port: 443}},
		{ID: 2, Upstream: UpstreamProxyConfig{Enabled: true, Protocol: "socks5", Server: "duplicate", Port: 1080}},
		{ID: 1, Upstream: UpstreamProxyConfig{}},
	}

	cfg.normalizeSlots()

	if len(cfg.Slots) != 1 {
		t.Fatalf("expected only one normalized slot entry, got %#v", cfg.Slots)
	}
	if cfg.Slots[0].ID != 2 {
		t.Fatalf("expected slot id 2, got %#v", cfg.Slots[0])
	}
	if cfg.Slots[0].SubID != "sub-1" || cfg.Slots[0].NodeName != "NodeA" {
		t.Fatalf("unexpected normalized slot node ref: %#v", cfg.Slots[0])
	}
	if cfg.Slots[0].PortMode != "manual" || cfg.Slots[0].ManualMixedPort != 19090 {
		t.Fatalf("unexpected normalized port config: %#v", cfg.Slots[0])
	}
	if cfg.Slots[0].Upstream.Protocol != "https" || cfg.Slots[0].Upstream.Server != "gateway" {
		t.Fatalf("unexpected normalized upstream: %#v", cfg.Slots[0].Upstream)
	}
}
