package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Proxy           ProxyConfig          `yaml:"proxy"`
	Subscription    SubscriptionConfig   `yaml:"subscription"`
	UpstreamProxies []UpstreamProxyEntry `yaml:"upstream_proxies,omitempty"`
	Slots           []SlotConfig         `yaml:"slots,omitempty"`
	Language        string               `yaml:"language"`
	SpeedTest       SpeedTestConfig      `yaml:"speedtest"`
	UI              UIConfig             `yaml:"ui"`
}

// ProxyConfig represents proxy port configuration
type ProxyConfig struct {
	MixedPort     int    `yaml:"mixed_port"`      // Single port for both SOCKS5 and HTTP
	BaseMixedPort int    `yaml:"base_mixed_port"` // Base mixed port for multi-instance offset
	BaseAPIPort   int    `yaml:"base_api_port"`   // Base API port for multi-instance offset
	AllowLan      bool   `yaml:"allow_lan"`
	BindAddress   string `yaml:"bind_address"`
	TunEnabled    bool   `yaml:"tun_enabled"`
	// DockerRedirectTCP redirects Docker container TCP traffic (internet-bound) to mihomo redir-port via iptables.
	// Linux only; requires root to apply iptables rules.
	DockerRedirectTCP bool `yaml:"docker_redirect_tcp"`
}

// SubscriptionConfig represents subscription-related configuration.
type SubscriptionConfig struct {
	// UpdateViaProxy controls whether URL subscriptions are fetched via the local proxy core (if running).
	UpdateViaProxy bool `yaml:"update_via_proxy"`
}

// UpstreamProxyEntry is a named, reusable upstream proxy definition stored in the registry.
type UpstreamProxyEntry struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	Protocol string `yaml:"protocol,omitempty"`
	Server   string `yaml:"server,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// SlotNodeRef stores the persisted proxy node selection for a slot.
type SlotNodeRef struct {
	SubID      string
	NodeName   string
	NodeServer string
	NodePort   int
	NodeType   string
}

// SlotConfig stores per-slot persisted settings.
type SlotConfig struct {
	ID              int                 `yaml:"id"`
	SubID           string              `yaml:"sub_id,omitempty"`
	NodeName        string              `yaml:"node_name,omitempty"`
	NodeServer      string              `yaml:"node_server,omitempty"`
	NodePort        int                 `yaml:"node_port,omitempty"`
	NodeType        string              `yaml:"node_type,omitempty"`
	StartMode       string              `yaml:"start_mode,omitempty"`
	PortMode        string              `yaml:"port_mode,omitempty"`
	ManualMixedPort int                 `yaml:"manual_mixed_port,omitempty"`
	UpstreamID      string              `yaml:"upstream_id,omitempty"`
	Upstream        UpstreamProxyConfig `yaml:"upstream,omitempty"` // legacy; migrated to UpstreamID
}

// UpstreamProxyConfig describes an optional upstream HTTP/HTTPS/SOCKS5 proxy.
type UpstreamProxyConfig struct {
	Enabled  bool   `yaml:"enabled,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`
	Server   string `yaml:"server,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// SpeedTestConfig represents speed test configuration
type SpeedTestConfig struct {
	Concurrent int           `yaml:"concurrent"`
	Timeout    time.Duration `yaml:"timeout"`
}

// UIConfig represents UI-related configuration.
type UIConfig struct {
	// ServerDisplay controls whether the Nodes tab shows the "Server" column.
	// Supported values: show | blur | off.
	ServerDisplay        string `yaml:"server_display"`
	SplashLastShownAt    string `yaml:"splash_last_shown_at,omitempty"`
	ActiveSubscriptionID string `yaml:"active_subscription_id,omitempty"`
	SelectedNodeSubID    string `yaml:"selected_node_sub_id,omitempty"`
	SelectedNodeName     string `yaml:"selected_node_name,omitempty"`
	SelectedNodeServer   string `yaml:"selected_node_server,omitempty"`
	SelectedNodePort     int    `yaml:"selected_node_port,omitempty"`
	SelectedNodeType     string `yaml:"selected_node_type,omitempty"`
	// SlotHistory saves the last connected slot states for UI display
	SlotHistory []SlotHistoryEntry `yaml:"slot_history,omitempty"`
}

// SlotHistoryEntry records a slot's historical connection state
type SlotHistoryEntry struct {
	SlotID   int    `yaml:"slot_id"`
	NodeName string `yaml:"node_name"`
	Mode     string `yaml:"mode"`
	SubID    string `yaml:"sub_id"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Proxy: ProxyConfig{
			MixedPort:         17890,
			BaseMixedPort:     17890,
			BaseAPIPort:       9090,
			AllowLan:          true,
			BindAddress:       "0.0.0.0",
			TunEnabled:        false,
			DockerRedirectTCP: false,
		},
		Subscription: SubscriptionConfig{
			UpdateViaProxy: false,
		},
		Language: "en_us",
		SpeedTest: SpeedTestConfig{
			Concurrent: 50,
			Timeout:    5 * time.Second,
		},
		UI: UIConfig{
			ServerDisplay: "show",
		},
	}
}

// Load loads configuration from file, or returns default if not exists
func Load() (*Config, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return DefaultConfig(), nil
	}

	configPath := filepath.Join(configDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// First run: return defaults and best-effort write a config file so
			// users can edit it manually.
			cfg := DefaultConfig()
			_ = cfg.Save()
			return cfg, nil
		}
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Proxy.MixedPort > 0 {
		cfg.Proxy.BaseMixedPort = cfg.Proxy.MixedPort
	}
	cfg.migrateInlineUpstreams()
	cfg.normalizeSlots()

	return cfg, nil
}

// Save saves configuration to file
func (c *Config) Save() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	if err := EnsureDir(configDir); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if c.Proxy.MixedPort > 0 {
		c.Proxy.BaseMixedPort = c.Proxy.MixedPort
	}
	c.normalizeSlots()
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
