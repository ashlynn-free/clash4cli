//go:build !darwin

package netctl

import (
	"fmt"

	"rocket4cli/internal/config"
)

type SystemProxyServiceState struct {
	Service       string
	WebEnabled    bool
	WebHost       string
	WebPort       int
	SecureEnabled bool
	SecureHost    string
	SecurePort    int
	PACEnabled    bool
	PACURL        string
	BypassDomains []string
}

type SystemProxySnapshot struct {
	Services []SystemProxyServiceState
}

type SystemProxyStatus struct {
	Enabled bool
	Host    string
	Port    int
	PACMode bool
	PACURL  string
	Bypass  []string
}

func ReadSystemProxyStatus() (*SystemProxyStatus, error) {
	return nil, fmt.Errorf("system proxy is supported on macOS only")
}

func SnapshotSystemProxy() (*SystemProxySnapshot, error) {
	return nil, nil
}

func ApplySystemProxy(cfg config.SystemProxyConfig) error {
	return fmt.Errorf("system proxy is supported on macOS only")
}

func RestoreSystemProxy(snapshot *SystemProxySnapshot) error {
	return nil
}
