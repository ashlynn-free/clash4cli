package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashConfig represents a Clash configuration file structure
type ClashConfig struct {
	Proxies []map[string]any `yaml:"proxies"`
}

// SIP008Config represents SIP008 subscription format
type SIP008Config struct {
	Version int `json:"version"`
	Servers []struct {
		ID         string `json:"id"`
		Remarks    string `json:"remarks"`
		Server     string `json:"server"`
		ServerPort int    `json:"server_port"`
		Password   string `json:"password"`
		Method     string `json:"method"`
		Plugin     string `json:"plugin,omitempty"`
		PluginOpts string `json:"plugin_opts,omitempty"`
	} `json:"servers"`
}

func looksLikeURIList(text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ss://") ||
			strings.HasPrefix(line, "ssr://") ||
			strings.HasPrefix(line, "vmess://") ||
			strings.HasPrefix(line, "vless://") ||
			strings.HasPrefix(line, "trojan://") ||
			strings.HasPrefix(line, "hysteria://") ||
			strings.HasPrefix(line, "hysteria2://") ||
			strings.HasPrefix(line, "hy2://") {
			return true
		}
	}
	return false
}

// DetectFormat detects the subscription format from content
func DetectFormat(content []byte) SubFormat {
	text := string(content)
	text = strings.TrimSpace(text)

	// Try YAML first (Clash format)
	var clashConfig ClashConfig
	if err := yaml.Unmarshal(content, &clashConfig); err == nil && len(clashConfig.Proxies) > 0 {
		return FormatClashYAML
	}

	// Try SIP008 JSON
	var sip008 SIP008Config
	if err := json.Unmarshal(content, &sip008); err == nil && len(sip008.Servers) > 0 {
		return FormatSIP008
	}

	// Try raw URI list (ss://, vmess://, ...)
	if looksLikeURIList(text) {
		return FormatURIList
	}

	// Try Base64
	decoded := tryBase64Decode(text)
	if len(decoded) > 0 {
		// Check if decoded content is a known subscription format.
		var decodedClashConfig ClashConfig
		if err := yaml.Unmarshal(decoded, &decodedClashConfig); err == nil && len(decodedClashConfig.Proxies) > 0 {
			return FormatBase64
		}
		var decodedSIP008 SIP008Config
		if err := json.Unmarshal(decoded, &decodedSIP008); err == nil && len(decodedSIP008.Servers) > 0 {
			return FormatBase64
		}
		if looksLikeURIList(strings.TrimSpace(string(decoded))) {
			return FormatBase64
		}
	}

	return FormatUnknown
}

// tryBase64Decode tries various base64 decodings
func tryBase64Decode(text string) []byte {
	// Remove any whitespace/newlines (browser copy may include line breaks)
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), "")

	// Try standard base64
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err == nil {
		return decoded
	}

	// Try URL-safe base64
	decoded, err = base64.URLEncoding.DecodeString(text)
	if err == nil {
		return decoded
	}

	// Try with padding normalization
	// Add padding if needed
	padded := text
	switch len(text) % 4 {
	case 2:
		padded += "=="
	case 3:
		padded += "="
	}

	decoded, err = base64.StdEncoding.DecodeString(padded)
	if err == nil {
		return decoded
	}

	// Try RawStdEncoding (no padding)
	decoded, err = base64.RawStdEncoding.DecodeString(text)
	if err == nil {
		return decoded
	}

	// Try RawURLEncoding
	decoded, err = base64.RawURLEncoding.DecodeString(text)
	if err == nil {
		return decoded
	}

	return nil
}

// Parse parses subscription content and returns nodes
func Parse(content []byte) ([]Node, error) {
	format := DetectFormat(content)

	switch format {
	case FormatClashYAML:
		return parseClashYAML(content)
	case FormatBase64:
		return parseBase64(content)
	case FormatSIP008:
		return parseSIP008(content)
	case FormatURIList:
		return parseURIList(content)
	default:
		return nil, fmt.Errorf("unknown subscription format")
	}
}

func parseClashYAML(content []byte) ([]Node, error) {
	var config ClashConfig
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	var nodes []Node
	for _, proxy := range config.Proxies {
		node := Node{
			Settings: make(map[string]any),
		}

		if name, ok := proxy["name"].(string); ok {
			node.Name = name
		}
		if t, ok := proxy["type"].(string); ok {
			node.Type = t
		}
		if server, ok := proxy["server"].(string); ok {
			node.Server = server
		}
		if port, ok := proxy["port"].(int); ok {
			node.Port = port
		} else if port, ok := proxy["port"].(float64); ok {
			node.Port = int(port)
		}

		// Store all other settings
		for k, v := range proxy {
			if k != "name" && k != "type" && k != "server" && k != "port" {
				node.Settings[k] = v
			}
		}

		if node.Name != "" && node.Type != "" {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

func parseBase64(content []byte) ([]Node, error) {
	text := strings.TrimSpace(string(content))

	decoded := tryBase64Decode(text)
	if decoded == nil {
		return nil, fmt.Errorf("failed to decode base64")
	}

	// Some providers base64-encode Clash YAML / SIP008 JSON directly.
	var clashConfig ClashConfig
	if err := yaml.Unmarshal(decoded, &clashConfig); err == nil && len(clashConfig.Proxies) > 0 {
		return parseClashYAML(decoded)
	}
	var sip008 SIP008Config
	if err := json.Unmarshal(decoded, &sip008); err == nil && len(sip008.Servers) > 0 {
		return parseSIP008(decoded)
	}

	return parseURIList(decoded)
}

func parseURIList(content []byte) ([]Node, error) {
	var nodes []Node
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var node *Node
		var parseErr error

		switch {
		case strings.HasPrefix(line, "ss://"):
			node, parseErr = parseShadowsocksURI(line)
		case strings.HasPrefix(line, "vmess://"):
			node, parseErr = parseVMessURI(line)
		case strings.HasPrefix(line, "vless://"):
			node, parseErr = parseVLESSURI(line)
		case strings.HasPrefix(line, "trojan://"):
			node, parseErr = parseTrojanURI(line)
		case strings.HasPrefix(line, "hysteria2://"), strings.HasPrefix(line, "hy2://"):
			node, parseErr = parseHysteria2URI(line)
		default:
			continue
		}

		if parseErr != nil {
			continue
		}

		if node != nil && node.Name != "" {
			nodes = append(nodes, *node)
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no valid nodes found")
	}

	return nodes, nil
}

func parseSIP008(content []byte) ([]Node, error) {
	var config SIP008Config
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	var nodes []Node
	for _, server := range config.Servers {
		node := Node{
			Name:   server.Remarks,
			Type:   TypeSS,
			Server: server.Server,
			Port:   server.ServerPort,
			Settings: map[string]any{
				"password": server.Password,
				"cipher":   server.Method,
			},
		}

		if server.Plugin != "" {
			node.Settings["plugin"] = server.Plugin
			node.Settings["plugin-opts"] = server.PluginOpts
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func parseShadowsocksURI(uri string) (*Node, error) {
	// ss://base64(method:password)@host:port/?params#name
	// or ss://base64(method:password@host:port)#name

	uri = strings.TrimPrefix(uri, "ss://")

	// Extract name from fragment
	var name string
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		name, _ = url.QueryUnescape(uri[idx+1:])
		uri = uri[:idx]
	}

	// Extract query params
	var queryParams url.Values
	if idx := strings.Index(uri, "?"); idx != -1 {
		queryStr := uri[idx+1:]
		uri = uri[:idx]
		queryParams, _ = url.ParseQuery(queryStr)
	}

	// Remove trailing slash
	uri = strings.TrimSuffix(uri, "/")

	// Try to parse as base64 encoded
	var userInfo, hostPort string

	if atIdx := strings.LastIndex(uri, "@"); atIdx != -1 {
		// Format: base64(method:password)@host:port
		userInfoB64 := uri[:atIdx]
		hostPort = uri[atIdx+1:]

		decoded := tryBase64Decode(userInfoB64)
		if decoded == nil {
			return nil, fmt.Errorf("failed to decode userinfo")
		}
		userInfo = string(decoded)
	} else {
		// Format: base64(method:password@host:port)
		decoded := tryBase64Decode(uri)
		if decoded == nil {
			return nil, fmt.Errorf("failed to decode")
		}

		fullURI := string(decoded)
		if atIdx := strings.LastIndex(fullURI, "@"); atIdx != -1 {
			userInfo = fullURI[:atIdx]
			hostPort = fullURI[atIdx+1:]
		} else {
			return nil, fmt.Errorf("invalid ss URI format")
		}
	}

	// Parse method:password
	parts := strings.SplitN(userInfo, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid userinfo format")
	}
	method, password := parts[0], parts[1]

	// Parse host:port
	host, portStr, err := splitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	settings := map[string]any{
		"cipher":   method,
		"password": password,
	}

	// Add plugin settings if present
	// Plugin format: obfs-local;obfs=http;obfs-host=xxx or v2ray-plugin;...
	if plugin := queryParams.Get("plugin"); plugin != "" {
		parsePluginSettings(plugin, settings)
	}

	return &Node{
		Name:     name,
		Type:     TypeSS,
		Server:   host,
		Port:     port,
		Settings: settings,
	}, nil
}

// parsePluginSettings parses SS plugin string and adds to settings map
// Input format: obfs-local;obfs=http;obfs-host=xxx;tfo=1
// Output format for mihomo: plugin: obfs-local, plugin-opts: {mode: http, host: xxx}
func parsePluginSettings(plugin string, settings map[string]any) {
	parts := strings.Split(plugin, ";")
	if len(parts) == 0 {
		return
	}

	pluginName := parts[0]

	// Map plugin names
	switch pluginName {
	case "obfs-local", "obfs":
		settings["plugin"] = "obfs"
	case "v2ray-plugin":
		settings["plugin"] = "v2ray-plugin"
	case "simple-obfs":
		settings["plugin"] = "obfs"
	default:
		settings["plugin"] = pluginName
	}

	pluginOpts := make(map[string]any)

	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, value := kv[0], kv[1]

		switch key {
		case "obfs":
			pluginOpts["mode"] = value
		case "obfs-host", "host":
			pluginOpts["host"] = value
		case "path":
			pluginOpts["path"] = value
		case "tls":
			pluginOpts["tls"] = value == "1" || value == "true"
		case "mux":
			pluginOpts["mux"] = value == "1" || value == "true"
		case "tfo":
			// TCP Fast Open - mihomo doesn't use this in plugin-opts
			// Skip it
		default:
			pluginOpts[key] = value
		}
	}

	if len(pluginOpts) > 0 {
		settings["plugin-opts"] = pluginOpts
	}
}

func parseVMessURI(uri string) (*Node, error) {
	// vmess://base64(json)
	uri = strings.TrimPrefix(uri, "vmess://")

	decoded, err := base64.StdEncoding.DecodeString(uri)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(uri)
		if err != nil {
			return nil, fmt.Errorf("failed to decode vmess: %w", err)
		}
	}

	var config map[string]any
	if err := json.Unmarshal(decoded, &config); err != nil {
		return nil, fmt.Errorf("failed to parse vmess json: %w", err)
	}

	node := &Node{
		Type:     TypeVMess,
		Settings: make(map[string]any),
	}

	if ps, ok := config["ps"].(string); ok {
		node.Name = ps
	}
	if add, ok := config["add"].(string); ok {
		node.Server = add
	}
	if port, ok := config["port"].(string); ok {
		node.Port, _ = strconv.Atoi(port)
	} else if port, ok := config["port"].(float64); ok {
		node.Port = int(port)
	}

	// Store relevant settings
	for _, key := range []string{"id", "aid", "net", "type", "host", "path", "tls", "sni"} {
		if v, ok := config[key]; ok {
			node.Settings[key] = v
		}
	}

	if node.Name == "" {
		node.Name = fmt.Sprintf("%s:%d", node.Server, node.Port)
	}

	return node, nil
}

func firstQueryValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if got := strings.TrimSpace(values.Get(key)); got != "" {
			return got
		}
	}
	return ""
}

func queryBoolValue(values url.Values, keys ...string) bool {
	raw := strings.ToLower(firstQueryValue(values, keys...))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizeURINetwork(values url.Values) string {
	rawType := strings.ToLower(firstQueryValue(values, "type"))
	headerType := strings.ToLower(firstQueryValue(values, "headerType", "header_type"))

	switch rawType {
	case "", "tcp":
		if headerType == "http" {
			return "http"
		}
		return "tcp"
	case "h2":
		return "h2"
	case "http":
		return "http"
	case "grpc":
		return "grpc"
	case "ws":
		return "ws"
	default:
		return rawType
	}
}

func applyTLSSettings(settings map[string]any, values url.Values) {
	security := strings.ToLower(firstQueryValue(values, "security"))
	switch security {
	case "tls", "reality":
		settings["tls"] = true
	}

	if serverName := firstQueryValue(values, "sni", "servername", "serverName"); serverName != "" {
		settings["servername"] = serverName
	}
	if queryBoolValue(values, "allowInsecure", "insecure", "skip-cert-verify") {
		settings["skip-cert-verify"] = true
	}
	if fp := firstQueryValue(values, "fp", "fingerprint", "client-fingerprint", "clientFingerprint"); fp != "" {
		settings["client-fingerprint"] = fp
	}
	if alpn := splitCSV(firstQueryValue(values, "alpn")); len(alpn) > 0 {
		settings["alpn"] = alpn
	}

	realityOpts := map[string]any{}
	if publicKey := firstQueryValue(values, "pbk", "public-key", "publicKey"); publicKey != "" {
		realityOpts["public-key"] = publicKey
	}
	if shortID := firstQueryValue(values, "sid", "short-id", "shortId"); shortID != "" {
		realityOpts["short-id"] = shortID
	}
	if spiderX := firstQueryValue(values, "spx", "spider-x", "spiderX"); spiderX != "" {
		realityOpts["spider-x"] = spiderX
	}
	if len(realityOpts) > 0 {
		settings["tls"] = true
		settings["reality-opts"] = realityOpts
	}
}

func applyTransportSettings(settings map[string]any, values url.Values) {
	network := normalizeURINetwork(values)
	if network == "" || network == "tcp" {
		return
	}

	settings["network"] = network

	path := firstQueryValue(values, "path")
	hostValues := splitCSV(firstQueryValue(values, "host"))

	switch network {
	case "ws":
		opts := map[string]any{}
		if path != "" {
			opts["path"] = path
		}
		if len(hostValues) > 0 {
			opts["headers"] = map[string]any{"Host": hostValues[0]}
		}
		if len(opts) > 0 {
			settings["ws-opts"] = opts
		}
	case "http":
		opts := map[string]any{}
		if path != "" {
			opts["path"] = []string{path}
		}
		if len(hostValues) > 0 {
			headers := map[string]any{"Host": hostValues}
			opts["headers"] = headers
		}
		if len(opts) > 0 {
			settings["http-opts"] = opts
		}
	case "h2":
		opts := map[string]any{}
		if len(hostValues) > 0 {
			opts["host"] = hostValues
		}
		if path != "" {
			opts["path"] = path
		}
		if len(opts) > 0 {
			settings["h2-opts"] = opts
		}
	case "grpc":
		opts := map[string]any{}
		if serviceName := firstQueryValue(values, "serviceName", "service_name"); serviceName != "" {
			opts["grpc-service-name"] = serviceName
		}
		if len(opts) > 0 {
			settings["grpc-opts"] = opts
		}
	}
}

func parseVLESSURI(uri string) (*Node, error) {
	// vless://uuid@host:port?params#name
	uri = strings.TrimPrefix(uri, "vless://")

	// Extract name
	var name string
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		name, _ = url.QueryUnescape(uri[idx+1:])
		uri = uri[:idx]
	}

	// Parse URL
	parsed, err := url.Parse("vless://" + uri)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(parsed.Port())
	if port == 0 {
		port = 443
	}

	node := &Node{
		Name:   name,
		Type:   TypeVLESS,
		Server: parsed.Hostname(),
		Port:   port,
		Settings: map[string]any{
			"uuid": parsed.User.Username(),
		},
	}

	values := parsed.Query()
	if encryption := firstQueryValue(values, "encryption"); encryption != "" {
		node.Settings["encryption"] = encryption
	}
	if flow := firstQueryValue(values, "flow"); flow != "" {
		node.Settings["flow"] = flow
	}
	if packetEncoding := firstQueryValue(values, "packetEncoding", "packet-encoding"); packetEncoding != "" {
		node.Settings["packet-encoding"] = packetEncoding
	}
	applyTLSSettings(node.Settings, values)
	applyTransportSettings(node.Settings, values)

	if node.Name == "" {
		node.Name = fmt.Sprintf("%s:%d", node.Server, node.Port)
	}

	return node, nil
}

func parseTrojanURI(uri string) (*Node, error) {
	// trojan://password@host:port?params#name
	uri = strings.TrimPrefix(uri, "trojan://")

	var name string
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		name, _ = url.QueryUnescape(uri[idx+1:])
		uri = uri[:idx]
	}

	parsed, err := url.Parse("trojan://" + uri)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(parsed.Port())
	if port == 0 {
		port = 443
	}

	node := &Node{
		Name:   name,
		Type:   TypeTrojan,
		Server: parsed.Hostname(),
		Port:   port,
		Settings: map[string]any{
			"password": parsed.User.Username(),
		},
	}

	values := parsed.Query()
	applyTLSSettings(node.Settings, values)
	applyTransportSettings(node.Settings, values)

	if node.Name == "" {
		node.Name = fmt.Sprintf("%s:%d", node.Server, node.Port)
	}

	return node, nil
}

func parseHysteria2URI(uri string) (*Node, error) {
	// hysteria2://auth@host:port?params#name
	uri = strings.TrimPrefix(uri, "hysteria2://")
	uri = strings.TrimPrefix(uri, "hy2://")

	var name string
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		name, _ = url.QueryUnescape(uri[idx+1:])
		uri = uri[:idx]
	}

	parsed, err := url.Parse("hy2://" + uri)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(parsed.Port())
	if port == 0 {
		port = 443
	}

	node := &Node{
		Name:   name,
		Type:   TypeHysteria2,
		Server: parsed.Hostname(),
		Port:   port,
		Settings: map[string]any{
			"password": parsed.User.Username(),
		},
	}

	for key, values := range parsed.Query() {
		if len(values) > 0 {
			node.Settings[key] = values[0]
		}
	}

	if node.Name == "" {
		node.Name = fmt.Sprintf("%s:%d", node.Server, node.Port)
	}

	return node, nil
}

func splitHostPort(hostPort string) (host, port string, err error) {
	// Handle IPv6
	if strings.HasPrefix(hostPort, "[") {
		idx := strings.LastIndex(hostPort, "]:")
		if idx == -1 {
			return "", "", fmt.Errorf("invalid host:port format")
		}
		return hostPort[1:idx], hostPort[idx+2:], nil
	}

	idx := strings.LastIndex(hostPort, ":")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid host:port format")
	}
	return hostPort[:idx], hostPort[idx+1:], nil
}
