package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Proxy     ProxyConfig     `yaml:"proxy"`
	Language  string          `yaml:"language"`
	SpeedTest SpeedTestConfig `yaml:"speedtest"`
	UI        UIConfig        `yaml:"ui"`
}

// ProxyConfig represents proxy port configuration
type ProxyConfig struct {
	MixedPort   int    `yaml:"mixed_port"` // Single port for both SOCKS5 and HTTP
	AllowLan    bool   `yaml:"allow_lan"`
	BindAddress string `yaml:"bind_address"`
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
	ActiveSubscriptionID string `yaml:"active_subscription_id,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Proxy: ProxyConfig{
			MixedPort:   17890,
			AllowLan:    true,
			BindAddress: "0.0.0.0",
		},
		Language: "en_us",
		SpeedTest: SpeedTestConfig{
			Concurrent: 10,
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
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
