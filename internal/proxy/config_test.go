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

	m, err := GenerateConfig(nodes, selected, cfg, 0, 12345, 19090, "127.0.0.1", 23456)
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

	// TUN should be present on linux and darwin when enabled.
	cfg.Proxy.TunEnabled = true
	m2, err := GenerateConfig(nodes, selected, cfg, 0, 12345, 19090, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if m2.Tun == nil || !m2.Tun.Enable {
			t.Fatalf("expected Tun enabled on %s when cfg.Proxy.TunEnabled=true, got %#v", runtime.GOOS, m2.Tun)
		}
		wantStack := "system"
		if runtime.GOOS == "darwin" {
			wantStack = "mixed"
		}
		if m2.Tun.Stack != wantStack {
			t.Fatalf("expected Tun.Stack=%q on %s, got %q", wantStack, runtime.GOOS, m2.Tun.Stack)
		}
		if runtime.GOOS == "darwin" && m2.Tun.Device != "" {
			t.Fatalf("expected Tun.Device empty on darwin, got %q", m2.Tun.Device)
		}

		m3, err := GenerateConfig(nodes, selected, cfg, 1, 12346, 19091, "127.0.0.1", 0)
		if err != nil {
			t.Fatalf("GenerateConfig for slot 1: %v", err)
		}
		if m3.Tun != nil {
			t.Fatalf("expected Tun nil on non-zero slot, got %#v", m3.Tun)
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
	var globalGroup *ProxyGroup
	for i := range m.ProxyGroups {
		g := &m.ProxyGroups[i]
		switch g.Name {
		case "Proxy":
			proxyGroup = g
		case "Auto":
			autoGroup = g
		case "GLOBAL":
			globalGroup = g
		}
	}
	if proxyGroup == nil || autoGroup == nil || globalGroup == nil {
		t.Fatalf("expected Proxy, Auto and GLOBAL groups, got %#v", m.ProxyGroups)
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
	wantGlobal := []string{"A", "Proxy", "Auto", "DIRECT", "REJECT", "B"}
	if len(globalGroup.Proxies) != len(wantGlobal) {
		t.Fatalf("unexpected GLOBAL group entries: %#v", globalGroup.Proxies)
	}
	for i := range wantGlobal {
		if globalGroup.Proxies[i] != wantGlobal[i] {
			t.Fatalf("unexpected GLOBAL group order: %#v", globalGroup.Proxies)
		}
	}
}

func TestGenerateConfig_DirectOnlyMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.AllowLan = false

	m, err := GenerateConfig(nil, nil, cfg, 0, 0, 0, "", 0)
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
	if len(m.Proxies) != 0 {
		t.Fatalf("expected 0 proxies in direct-only mode, got %d", len(m.Proxies))
	}

	// Direct-only mode should still create Proxy and GLOBAL groups defaulting to DIRECT.
	if len(m.ProxyGroups) != 2 {
		t.Fatalf("expected 2 proxy groups in direct-only mode, got %d: %#v", len(m.ProxyGroups), m.ProxyGroups)
	}
	var proxyGroup, globalGroup *ProxyGroup
	for i := range m.ProxyGroups {
		switch m.ProxyGroups[i].Name {
		case "Proxy":
			proxyGroup = &m.ProxyGroups[i]
		case "GLOBAL":
			globalGroup = &m.ProxyGroups[i]
		}
	}
	if proxyGroup == nil || globalGroup == nil {
		t.Fatalf("expected Proxy and GLOBAL groups")
	}
	wantProxy := []string{"DIRECT", "REJECT"}
	if len(proxyGroup.Proxies) != len(wantProxy) {
		t.Fatalf("unexpected Proxy group: %#v", proxyGroup.Proxies)
	}
	for i := range wantProxy {
		if proxyGroup.Proxies[i] != wantProxy[i] {
			t.Fatalf("unexpected Proxy group entry %d: got %q want %q", i, proxyGroup.Proxies[i], wantProxy[i])
		}
	}
	wantGlobal := []string{"DIRECT", "Proxy", "REJECT"}
	if len(globalGroup.Proxies) != len(wantGlobal) {
		t.Fatalf("unexpected GLOBAL group: %#v", globalGroup.Proxies)
	}
	for i := range wantGlobal {
		if globalGroup.Proxies[i] != wantGlobal[i] {
			t.Fatalf("unexpected GLOBAL group entry %d: got %q want %q", i, globalGroup.Proxies[i], wantGlobal[i])
		}
	}
}

func TestGenerateConfig_SlotUpstreamUsesDialerProxy(t *testing.T) {
	cfg := config.DefaultConfig()
	upID := cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "Test Gateway",
		Protocol: "https",
		Server:   "gateway.example.com",
		Port:     443,
		Username: "alice",
		Password: "secret",
	})
	cfg.SetSlotUpstreamRef(2, upID)

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
	}
	selected := &nodes[0]

	m, err := GenerateConfig(nodes, selected, cfg, 2, 17892, 19092, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	if len(m.Proxies) != 2 {
		t.Fatalf("expected 2 proxies including upstream, got %d", len(m.Proxies))
	}

	var upstream map[string]any
	for _, proxyEntry := range m.Proxies {
		if proxyEntry["name"] == "slot-2-upstream" {
			upstream = proxyEntry
			break
		}
	}
	if upstream == nil {
		t.Fatalf("expected generated slot upstream proxy, got %#v", m.Proxies)
	}
	if upstream["type"] != "http" {
		t.Fatalf("expected upstream type=http for https protocol, got %#v", upstream["type"])
	}
	if upstream["tls"] != true {
		t.Fatalf("expected upstream tls=true, got %#v", upstream["tls"])
	}
	if upstream["dialer-proxy"] != "A" {
		t.Fatalf("expected upstream dialer-proxy=A, got %#v", upstream["dialer-proxy"])
	}
	if upstream["username"] != "alice" || upstream["password"] != "secret" {
		t.Fatalf("unexpected upstream credentials: %#v", upstream)
	}

	var proxyGroup *ProxyGroup
	var globalGroup *ProxyGroup
	for i := range m.ProxyGroups {
		switch m.ProxyGroups[i].Name {
		case "Proxy":
			proxyGroup = &m.ProxyGroups[i]
		case "GLOBAL":
			globalGroup = &m.ProxyGroups[i]
		}
	}
	if proxyGroup == nil || globalGroup == nil {
		t.Fatalf("expected Proxy and GLOBAL groups, got %#v", m.ProxyGroups)
	}
	want := []string{"slot-2-upstream", "DIRECT", "REJECT"}
	if len(proxyGroup.Proxies) != len(want) {
		t.Fatalf("unexpected Proxy group entries: %#v", proxyGroup.Proxies)
	}
	for i := range want {
		if proxyGroup.Proxies[i] != want[i] {
			t.Fatalf("unexpected Proxy group entries: %#v", proxyGroup.Proxies)
		}
	}
	wantGlobal := []string{"slot-2-upstream", "Proxy", "Auto", "DIRECT", "REJECT", "A"}
	if len(globalGroup.Proxies) != len(wantGlobal) {
		t.Fatalf("unexpected GLOBAL group entries: %#v", globalGroup.Proxies)
	}
	for i := range wantGlobal {
		if globalGroup.Proxies[i] != wantGlobal[i] {
			t.Fatalf("unexpected GLOBAL group entries: %#v", globalGroup.Proxies)
		}
	}
}

func TestGenerateConfigWithMode_Global(t *testing.T) {
	cfg := config.DefaultConfig()

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
	selected := &nodes[1]

	m, err := GenerateConfigWithMode(nodes, selected, cfg, 0, 12345, 19090, "127.0.0.1", 0, "global")
	if err != nil {
		t.Fatalf("GenerateConfigWithMode: %v", err)
	}
	if m.Mode != "global" {
		t.Fatalf("expected Mode=global, got %q", m.Mode)
	}

	var globalGroup *ProxyGroup
	for i := range m.ProxyGroups {
		if m.ProxyGroups[i].Name == "GLOBAL" {
			globalGroup = &m.ProxyGroups[i]
			break
		}
	}
	if globalGroup == nil {
		t.Fatalf("expected GLOBAL group, got %#v", m.ProxyGroups)
	}
	if len(globalGroup.Proxies) == 0 || globalGroup.Proxies[0] != "B" {
		t.Fatalf("expected selected node B to be first GLOBAL entry, got %#v", globalGroup.Proxies)
	}
}

func TestGenerateConfig_UpstreamOnlyNoNode(t *testing.T) {
	cfg := config.DefaultConfig()
	upID := cfg.AddUpstreamProxy(config.UpstreamProxyEntry{
		Label:    "Corporate Proxy",
		Protocol: "http",
		Server:   "proxy.corp.com",
		Port:     8080,
	})
	cfg.SetSlotUpstreamRef(1, upID)

	// No nodes, no selected node — upstream-only mode
	m, err := GenerateConfig(nil, nil, cfg, 1, 17891, 9091, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("GenerateConfig upstream-only: %v", err)
	}

	// Should have exactly 1 proxy (the upstream)
	if len(m.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d: %#v", len(m.Proxies), m.Proxies)
	}
	if m.Proxies[0]["name"] != "slot-1-upstream" {
		t.Fatalf("expected upstream proxy name, got %v", m.Proxies[0]["name"])
	}
	// No dialer-proxy should be set (upstream-only, no chaining)
	if _, hasDial := m.Proxies[0]["dialer-proxy"]; hasDial {
		t.Fatalf("expected no dialer-proxy in upstream-only mode, got %v", m.Proxies[0]["dialer-proxy"])
	}

	// Should have Proxy and GLOBAL groups (no Auto)
	if len(m.ProxyGroups) != 2 {
		t.Fatalf("expected 2 proxy groups, got %d: %#v", len(m.ProxyGroups), m.ProxyGroups)
	}
	var proxyGroup, globalGroup *ProxyGroup
	for i := range m.ProxyGroups {
		switch m.ProxyGroups[i].Name {
		case "Proxy":
			proxyGroup = &m.ProxyGroups[i]
		case "GLOBAL":
			globalGroup = &m.ProxyGroups[i]
		}
	}
	if proxyGroup == nil || globalGroup == nil {
		t.Fatalf("expected Proxy and GLOBAL groups")
	}
	// Proxy group default should be upstream
	wantProxy := []string{"slot-1-upstream", "DIRECT", "REJECT"}
	if len(proxyGroup.Proxies) != len(wantProxy) {
		t.Fatalf("unexpected Proxy group: %#v", proxyGroup.Proxies)
	}
	for i := range wantProxy {
		if proxyGroup.Proxies[i] != wantProxy[i] {
			t.Fatalf("unexpected Proxy group entry %d: got %q want %q", i, proxyGroup.Proxies[i], wantProxy[i])
		}
	}
	wantGlobal := []string{"slot-1-upstream", "Proxy", "DIRECT", "REJECT"}
	if len(globalGroup.Proxies) != len(wantGlobal) {
		t.Fatalf("unexpected GLOBAL group: %#v", globalGroup.Proxies)
	}
	for i := range wantGlobal {
		if globalGroup.Proxies[i] != wantGlobal[i] {
			t.Fatalf("unexpected GLOBAL group entry %d: got %q want %q", i, globalGroup.Proxies[i], wantGlobal[i])
		}
	}
}
