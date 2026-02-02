package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
)

func TestParse_URIList_ShadowSocks(t *testing.T) {
	content := []byte("ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#TestNode\n")

	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeSS {
		t.Fatalf("expected type %q, got %q", TypeSS, n.Type)
	}
	if n.Name != "TestNode" {
		t.Fatalf("expected name %q, got %q", "TestNode", n.Name)
	}
	if n.Server != "example.com" || n.Port != 8388 {
		t.Fatalf("expected server example.com:8388, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["cipher"] != "aes-128-gcm" {
		t.Fatalf("expected cipher aes-128-gcm, got %#v", n.Settings["cipher"])
	}
	if n.Settings["password"] != "pass" {
		t.Fatalf("expected password pass, got %#v", n.Settings["password"])
	}
}

func TestParse_Base64_WithWhitespace(t *testing.T) {
	decoded := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#TestNode\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(decoded))
	// Simulate browser copy with whitespace/newlines.
	withWhitespace := encoded[:10] + "\n" + encoded[10:30] + " \n\t" + encoded[30:]

	nodes, err := Parse([]byte(withWhitespace))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Name != "TestNode" {
		t.Fatalf("expected name %q, got %q", "TestNode", nodes[0].Name)
	}
}

func TestParse_ClashYAML(t *testing.T) {
	content := []byte(`
proxies:
  - name: test
    type: ss
    server: 1.2.3.4
    port: 8388
    cipher: aes-128-gcm
    password: pass
`)
	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Name != "test" {
		t.Fatalf("expected name %q, got %q", "test", n.Name)
	}
	if n.Type != "ss" {
		t.Fatalf("expected type %q, got %q", "ss", n.Type)
	}
	if n.Server != "1.2.3.4" || n.Port != 8388 {
		t.Fatalf("expected server 1.2.3.4:8388, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["cipher"] != "aes-128-gcm" || n.Settings["password"] != "pass" {
		t.Fatalf("expected settings cipher/password, got %#v", n.Settings)
	}
}

func TestParse_SIP008_JSON(t *testing.T) {
	cfg := SIP008Config{
		Version: 1,
		Servers: []struct {
			ID         string `json:"id"`
			Remarks    string `json:"remarks"`
			Server     string `json:"server"`
			ServerPort int    `json:"server_port"`
			Password   string `json:"password"`
			Method     string `json:"method"`
			Plugin     string `json:"plugin,omitempty"`
			PluginOpts string `json:"plugin_opts,omitempty"`
		}{
			{
				ID:         "1",
				Remarks:    "sip",
				Server:     "1.2.3.4",
				ServerPort: 8388,
				Password:   "pass",
				Method:     "aes-128-gcm",
			},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	nodes, err := Parse(b)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeSS {
		t.Fatalf("expected type %q, got %q", TypeSS, n.Type)
	}
	if n.Name != "sip" {
		t.Fatalf("expected name %q, got %q", "sip", n.Name)
	}
	if n.Server != "1.2.3.4" || n.Port != 8388 {
		t.Fatalf("expected server 1.2.3.4:8388, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["cipher"] != "aes-128-gcm" || n.Settings["password"] != "pass" {
		t.Fatalf("expected settings cipher/password, got %#v", n.Settings)
	}
}

func TestParse_URIList_Shadowsocks_PluginObfs(t *testing.T) {
	plugin := url.QueryEscape("obfs-local;obfs=http;obfs-host=example.com")
	content := []byte("ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388/?plugin=" + plugin + "#Obfs\n")

	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeSS {
		t.Fatalf("expected type %q, got %q", TypeSS, n.Type)
	}
	if n.Name != "Obfs" {
		t.Fatalf("expected name %q, got %q", "Obfs", n.Name)
	}
	if n.Settings["plugin"] != "obfs" {
		t.Fatalf("expected plugin=obfs, got %#v", n.Settings["plugin"])
	}
	opts, ok := n.Settings["plugin-opts"].(map[string]any)
	if !ok {
		t.Fatalf("expected plugin-opts map, got %T", n.Settings["plugin-opts"])
	}
	if opts["mode"] != "http" || opts["host"] != "example.com" {
		t.Fatalf("unexpected plugin-opts: %#v", opts)
	}
}

func TestParse_URIList_VMess(t *testing.T) {
	cfg := map[string]any{
		"v":    "2",
		"ps":   "vmess-node",
		"add":  "vmess.example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
		"aid":  "0",
		"net":  "ws",
		"type": "none",
		"host": "host.example.com",
		"path": "/ws",
		"tls":  "tls",
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	content := []byte("vmess://" + base64.StdEncoding.EncodeToString(b) + "\n")

	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeVMess {
		t.Fatalf("expected type %q, got %q", TypeVMess, n.Type)
	}
	if n.Name != "vmess-node" {
		t.Fatalf("expected name %q, got %q", "vmess-node", n.Name)
	}
	if n.Server != "vmess.example.com" || n.Port != 443 {
		t.Fatalf("expected server vmess.example.com:443, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["id"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("expected id setting, got %#v", n.Settings["id"])
	}
	if n.Settings["net"] != "ws" || n.Settings["path"] != "/ws" {
		t.Fatalf("expected net/path settings, got %#v", n.Settings)
	}
}

func TestParse_URIList_VLESS(t *testing.T) {
	content := []byte("vless://550e8400-e29b-41d4-a716-446655440000@example.com:8443?encryption=none&security=tls&type=ws&path=%2Fws#My%20VLESS\n")
	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeVLESS {
		t.Fatalf("expected type %q, got %q", TypeVLESS, n.Type)
	}
	if n.Name != "My VLESS" {
		t.Fatalf("expected name %q, got %q", "My VLESS", n.Name)
	}
	if n.Server != "example.com" || n.Port != 8443 {
		t.Fatalf("expected server example.com:8443, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["uuid"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("expected uuid, got %#v", n.Settings["uuid"])
	}
	if n.Settings["security"] != "tls" || n.Settings["path"] != "/ws" {
		t.Fatalf("expected query settings, got %#v", n.Settings)
	}
}

func TestParse_URIList_Trojan(t *testing.T) {
	content := []byte("trojan://passw0rd@trojan.example.com:443?security=tls&sni=example.com#Trojan%20Node\n")
	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeTrojan {
		t.Fatalf("expected type %q, got %q", TypeTrojan, n.Type)
	}
	if n.Name != "Trojan Node" {
		t.Fatalf("expected name %q, got %q", "Trojan Node", n.Name)
	}
	if n.Server != "trojan.example.com" || n.Port != 443 {
		t.Fatalf("expected server trojan.example.com:443, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["password"] != "passw0rd" || n.Settings["security"] != "tls" {
		t.Fatalf("expected password/security settings, got %#v", n.Settings)
	}
}

func TestParse_URIList_Hysteria2(t *testing.T) {
	content := []byte("hy2://auth@hy2.example.com:443?insecure=1&alpn=h3#HY2\n")
	nodes, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != TypeHysteria2 {
		t.Fatalf("expected type %q, got %q", TypeHysteria2, n.Type)
	}
	if n.Name != "HY2" {
		t.Fatalf("expected name %q, got %q", "HY2", n.Name)
	}
	if n.Server != "hy2.example.com" || n.Port != 443 {
		t.Fatalf("expected server hy2.example.com:443, got %s:%d", n.Server, n.Port)
	}
	if n.Settings["password"] != "auth" || n.Settings["alpn"] != "h3" {
		t.Fatalf("expected password/alpn settings, got %#v", n.Settings)
	}
}

func TestParse_URIList_NoValidNodes_ReturnsError(t *testing.T) {
	_, err := Parse([]byte("vmess://not_base64\n"))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
