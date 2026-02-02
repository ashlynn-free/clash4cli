package proxy

import (
	"runtime"
	"testing"

	"rocket4cli/internal/config"
	"rocket4cli/internal/subscription"
)

func TestGenerateConfig_ProxyGroupsOrder(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.AllowLan = true
	cfg.Proxy.BindAddress = ""

	nodes := []subscription.Node{
		{
			Name:   "A",
			Type:   "ss",
			Server: "1.1.1.1",
			Port:   1111,
			Settings: map[string]any{
				"cipher":   "aes-128-gcm",
				"password": "pass",
			},
		},
		{
			Name:   "B",
			Type:   "ss",
			Server: "2.2.2.2",
			Port:   2222,
			Settings: map[string]any{
				"cipher":   "aes-256-gcm",
				"password": "pass2",
			},
		},
	}
	selected := &nodes[0]

	m, err := GenerateConfig(nodes, selected, cfg, 12345, 19090, "127.0.0.1", 23456)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if m.MixedPort != 12345 {
		t.Fatalf("expected MixedPort=12345, got %d", m.MixedPort)
	}
	if m.RedirPort != 23456 {
		t.Fatalf("expected RedirPort=23456, got %d", m.RedirPort)
	}
	if !m.AllowLan {
		t.Fatalf("expected AllowLan=true")
	}
	if m.BindAddress != "0.0.0.0" {
		t.Fatalf("expected BindAddress=0.0.0.0, got %q", m.BindAddress)
	}
	if m.ExternalController != "127.0.0.1:19090" {
		t.Fatalf("expected ExternalController=127.0.0.1:19090, got %q", m.ExternalController)
	}
	if m.Mode != "rule" {
		t.Fatalf("expected Mode=rule, got %q", m.Mode)
	}
	if len(m.Rules) != 1 || m.Rules[0] != "MATCH,Proxy" {
		t.Fatalf("unexpected Rules: %#v", m.Rules)
	}

	// Linux-only: TUN should be present only when enabled and on linux.
	cfg.Proxy.TunEnabled = true
	m2, err := GenerateConfig(nodes, selected, cfg, 12345, 19090, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	if runtime.GOOS == "linux" {
		if m2.Tun == nil || !m2.Tun.Enable {
			t.Fatalf("expected Tun enabled on linux when cfg.Proxy.TunEnabled=true, got %#v", m2.Tun)
		}
	} else {
		if m2.Tun != nil {
			t.Fatalf("expected Tun nil on %s, got %#v", runtime.GOOS, m2.Tun)
		}
	}

	if len(m.Proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(m.Proxies))
	}
	if m.Proxies[0]["name"] != "A" || m.Proxies[1]["name"] != "B" {
		t.Fatalf("unexpected proxies: %#v", m.Proxies)
	}
	if m.Proxies[0]["cipher"] != "aes-128-gcm" || m.Proxies[0]["password"] != "pass" {
		t.Fatalf("unexpected proxy settings for A: %#v", m.Proxies[0])
	}

	if len(m.ProxyGroups) < 2 {
		t.Fatalf("expected >=2 proxy groups, got %d", len(m.ProxyGroups))
	}
	var proxyGroup *ProxyGroup
	var autoGroup *ProxyGroup
	for i := range m.ProxyGroups {
		g := &m.ProxyGroups[i]
		switch g.Name {
		case "Proxy":
			proxyGroup = g
		case "Auto":
			autoGroup = g
		}
	}
	if proxyGroup == nil || autoGroup == nil {
		t.Fatalf("expected Proxy and Auto groups, got %#v", m.ProxyGroups)
	}
	if proxyGroup.Type != "select" {
		t.Fatalf("expected Proxy group type=select, got %q", proxyGroup.Type)
	}
	want := []string{"A", "DIRECT", "REJECT", "B"}
	if len(proxyGroup.Proxies) != len(want) {
		t.Fatalf("unexpected Proxy group entries: %#v", proxyGroup.Proxies)
	}
	for i := range want {
		if proxyGroup.Proxies[i] != want[i] {
			t.Fatalf("unexpected Proxy group order: %#v", proxyGroup.Proxies)
		}
	}
	if autoGroup.Type != "url-test" {
		t.Fatalf("expected Auto group type=url-test, got %q", autoGroup.Type)
	}
	if len(autoGroup.Proxies) != 2 || autoGroup.Proxies[0] != "A" || autoGroup.Proxies[1] != "B" {
		t.Fatalf("unexpected Auto group proxies: %#v", autoGroup.Proxies)
	}
}

func TestGenerateConfig_NoNodesNoGroups(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.AllowLan = false

	m, err := GenerateConfig(nil, nil, cfg, 0, 0, "", 0)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if m.AllowLan {
		t.Fatalf("expected AllowLan=false")
	}
	if m.BindAddress != "" {
		t.Fatalf("expected BindAddress empty, got %q", m.BindAddress)
	}
	if m.MixedPort != cfg.Proxy.MixedPort {
		t.Fatalf("expected MixedPort=%d from cfg, got %d", cfg.Proxy.MixedPort, m.MixedPort)
	}
	if len(m.ProxyGroups) != 0 {
		t.Fatalf("expected no proxy groups when no nodes, got %#v", m.ProxyGroups)
	}
}
