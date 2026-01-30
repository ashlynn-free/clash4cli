package proxy

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"rocket4cli/internal/config"
	"rocket4cli/internal/subscription"
)

// MihomoConfig represents the mihomo configuration
type MihomoConfig struct {
	MixedPort          int              `yaml:"mixed-port,omitempty"`
	Port               int              `yaml:"port,omitempty"`
	SocksPort          int              `yaml:"socks-port,omitempty"`
	AllowLan           bool             `yaml:"allow-lan"`
	Mode               string           `yaml:"mode"`
	LogLevel           string           `yaml:"log-level"`
	ExternalController string           `yaml:"external-controller,omitempty"`
	Proxies            []map[string]any `yaml:"proxies"`
	ProxyGroups        []ProxyGroup     `yaml:"proxy-groups"`
	Rules              []string         `yaml:"rules"`
}

// ProxyGroup represents a proxy group
type ProxyGroup struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Proxies  []string `yaml:"proxies"`
	URL      string   `yaml:"url,omitempty"`
	Interval int      `yaml:"interval,omitempty"`
}

// GenerateConfig generates mihomo configuration from nodes
func GenerateConfig(nodes []subscription.Node, selectedNode *subscription.Node, cfg *config.Config) (*MihomoConfig, error) {
	mihomoConfig := &MihomoConfig{
		MixedPort:          cfg.Proxy.MixedPort, // Single port for both SOCKS5 and HTTP
		AllowLan:           false,
		Mode:               "rule", // Drive all traffic via our "Proxy" selector group
		LogLevel:           "info",
		ExternalController: "127.0.0.1:9090",
		Proxies:            make([]map[string]any, 0, len(nodes)),
		Rules: []string{
			"MATCH,Proxy",
		},
	}

	proxyNames := make([]string, 0, len(nodes))

	for _, node := range nodes {
		proxy := nodeToProxy(node)
		if proxy != nil {
			mihomoConfig.Proxies = append(mihomoConfig.Proxies, proxy)
			proxyNames = append(proxyNames, node.Name)
		}
	}

	// Create proxy groups - GLOBAL will use these
	if len(proxyNames) > 0 {
		// Build selector list for "Proxy" group.
		// For type=select, the first entry is the default. If a node is selected, prefer it.
		allProxies := make([]string, 0, len(proxyNames)+2)
		if selectedNode != nil && selectedNode.Name != "" {
			allProxies = append(allProxies, selectedNode.Name)
		}
		allProxies = append(allProxies, "DIRECT", "REJECT")
		for _, name := range proxyNames {
			if selectedNode != nil && name == selectedNode.Name {
				continue
			}
			allProxies = append(allProxies, name)
		}

		mihomoConfig.ProxyGroups = []ProxyGroup{
			{
				Name:    "Proxy",
				Type:    "select",
				Proxies: allProxies,
			},
			{
				Name:     "Auto",
				Type:     "url-test",
				Proxies:  proxyNames,
				URL:      "http://www.gstatic.com/generate_204",
				Interval: 300,
			},
		}
	}

	return mihomoConfig, nil
}

// nodeToProxy converts a Node to mihomo proxy format
func nodeToProxy(node subscription.Node) map[string]any {
	proxy := map[string]any{
		"name":   node.Name,
		"type":   node.Type,
		"server": node.Server,
		"port":   node.Port,
	}

	// Copy all settings
	for k, v := range node.Settings {
		proxy[k] = v
	}

	return proxy
}

// SaveConfig saves mihomo config to file
func SaveConfig(cfg *MihomoConfig) (string, error) {
	workDir, err := config.GetMihomoWorkDir()
	if err != nil {
		return "", err
	}

	if err := config.EnsureDir(workDir); err != nil {
		return "", err
	}

	configPath := filepath.Join(workDir, "config.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return configPath, nil
}
