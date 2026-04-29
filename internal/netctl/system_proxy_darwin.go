//go:build darwin

package netctl

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"rocket4cli/internal/config"
)

type SystemProxyServiceState struct {
	Service       string   `json:"service" yaml:"service"`
	WebEnabled    bool     `json:"web_enabled" yaml:"web_enabled"`
	WebHost       string   `json:"web_host,omitempty" yaml:"web_host,omitempty"`
	WebPort       int      `json:"web_port,omitempty" yaml:"web_port,omitempty"`
	SecureEnabled bool     `json:"secure_enabled" yaml:"secure_enabled"`
	SecureHost    string   `json:"secure_host,omitempty" yaml:"secure_host,omitempty"`
	SecurePort    int      `json:"secure_port,omitempty" yaml:"secure_port,omitempty"`
	PACEnabled    bool     `json:"pac_enabled" yaml:"pac_enabled"`
	PACURL        string   `json:"pac_url,omitempty" yaml:"pac_url,omitempty"`
	BypassDomains []string `json:"bypass_domains,omitempty" yaml:"bypass_domains,omitempty"`
}

type SystemProxySnapshot struct {
	Services []SystemProxyServiceState `json:"services" yaml:"services"`
}

type SystemProxyStatus struct {
	Enabled bool     `json:"enabled" yaml:"enabled"`
	Host    string   `json:"host,omitempty" yaml:"host,omitempty"`
	Port    int      `json:"port,omitempty" yaml:"port,omitempty"`
	PACMode bool     `json:"pac_mode" yaml:"pac_mode"`
	PACURL  string   `json:"pac_url,omitempty" yaml:"pac_url,omitempty"`
	Bypass  []string `json:"bypass,omitempty" yaml:"bypass,omitempty"`
}

var runNetworksetup = func(args ...string) (string, error) {
	cmd := exec.Command("networksetup", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("networksetup %s: %w\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}

func listAllNetworkServices() ([]string, error) {
	out, err := runNetworksetup("-listallnetworkservices")
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func parseProxyState(output string) (enabled bool, host string, port int) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			enabled = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Enabled:")), "Yes")
		}
		if strings.HasPrefix(line, "Server:") {
			host = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
		}
		if strings.HasPrefix(line, "Port:") {
			p, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Port:")))
			port = p
		}
	}
	return
}

func parsePACState(output string) (enabled bool, url string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			enabled = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Enabled:")), "Yes")
		}
		if strings.HasPrefix(line, "URL:") {
			url = strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
		}
	}
	return
}

func parseBypassDomains(output string) []string {
	var domains []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Enabled:") || strings.HasPrefix(line, "There aren't any") {
			continue
		}
		domains = append(domains, line)
	}
	return domains
}

func normalizeBypassDomains(values []string, defaults []string, includeDefaults bool, validate bool) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values)+len(defaults))
	appendValue := func(v string) error {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		if validate {
			if strings.Contains(v, " ") {
				return fmt.Errorf("invalid bypass entry: %q", v)
			}
		}
		if _, ok := seen[v]; ok {
			return nil
		}
		seen[v] = struct{}{}
		result = append(result, v)
		return nil
	}
	if includeDefaults {
		for _, v := range defaults {
			if err := appendValue(v); err != nil {
				return nil, err
			}
		}
	}
	for _, v := range values {
		if err := appendValue(v); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func defaultSystemProxyBypass() []string {
	return []string{"127.0.0.1", "localhost", "192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "*.local", "<local>"}
}

func ReadSystemProxyStatus() (*SystemProxyStatus, error) {
	services, err := listAllNetworkServices()
	if err != nil {
		return nil, err
	}
	status := &SystemProxyStatus{}
	for _, service := range services {
		webOut, err := runNetworksetup("-getwebproxy", service)
		if err != nil {
			return nil, err
		}
		webEnabled, webHost, webPort := parseProxyState(webOut)
		secureOut, err := runNetworksetup("-getsecurewebproxy", service)
		if err != nil {
			return nil, err
		}
		secureEnabled, secureHost, securePort := parseProxyState(secureOut)
		pacOut, err := runNetworksetup("-getautoproxyurl", service)
		if err != nil {
			return nil, err
		}
		pacEnabled, pacURL := parsePACState(pacOut)
		if pacEnabled || webEnabled || secureEnabled {
			status.Enabled = true
		}
		if status.Host == "" {
			if webHost != "" {
				status.Host = webHost
				status.Port = webPort
			} else if secureHost != "" {
				status.Host = secureHost
				status.Port = securePort
			}
		}
		if pacEnabled {
			status.PACMode = true
			status.PACURL = pacURL
		}
	}
	return status, nil
}

func SnapshotSystemProxy() (*SystemProxySnapshot, error) {
	services, err := listAllNetworkServices()
	if err != nil {
		return nil, err
	}
	snapshot := &SystemProxySnapshot{Services: make([]SystemProxyServiceState, 0, len(services))}
	for _, service := range services {
		webOut, err := runNetworksetup("-getwebproxy", service)
		if err != nil {
			return nil, err
		}
		webEnabled, webHost, webPort := parseProxyState(webOut)
		secureOut, err := runNetworksetup("-getsecurewebproxy", service)
		if err != nil {
			return nil, err
		}
		secureEnabled, secureHost, securePort := parseProxyState(secureOut)
		pacOut, err := runNetworksetup("-getautoproxyurl", service)
		if err != nil {
			return nil, err
		}
		pacEnabled, pacURL := parsePACState(pacOut)
		bypassOut, err := runNetworksetup("-getproxybypassdomains", service)
		if err != nil {
			return nil, err
		}
		bypass := parseBypassDomains(bypassOut)
		state := SystemProxyServiceState{
			Service:       service,
			WebEnabled:    webEnabled,
			WebHost:       webHost,
			WebPort:       webPort,
			SecureEnabled: secureEnabled,
			SecureHost:    secureHost,
			SecurePort:    securePort,
			PACEnabled:    pacEnabled,
			PACURL:        pacURL,
			BypassDomains: bypass,
		}
		snapshot.Services = append(snapshot.Services, state)
	}
	return snapshot, nil
}

func ApplySystemProxy(cfg config.SystemProxyConfig) error {
	services, err := listAllNetworkServices()
	if err != nil {
		return err
	}
	bypass, err := normalizeBypassDomains(cfg.Bypass, defaultSystemProxyBypass(), cfg.AlwaysUseDefaultBypass, cfg.ValidateBypassFormat)
	if err != nil {
		return err
	}
	for _, service := range services {
		if cfg.PACMode {
			pacURL := fmt.Sprintf("http://%s:%d/proxy.pac", cfg.Host, cfg.Port)
			if _, err := runNetworksetup("-setautoproxyurl", service, pacURL); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setautoproxystate", service, "on"); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setwebproxystate", service, "off"); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setsecurewebproxystate", service, "off"); err != nil {
				return err
			}
		} else {
			if _, err := runNetworksetup("-setwebproxy", service, cfg.Host, strconv.Itoa(cfg.Port), "off"); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setwebproxystate", service, "on"); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setsecurewebproxy", service, cfg.Host, strconv.Itoa(cfg.Port), "off"); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setsecurewebproxystate", service, "on"); err != nil {
				return err
			}
			if _, err := runNetworksetup("-setautoproxystate", service, "off"); err != nil {
				return err
			}
		}
		args := append([]string{"-setproxybypassdomains", service}, bypass...)
		if len(bypass) == 0 {
			args = []string{"-setproxybypassdomains", service, ""}
		}
		if _, err := runNetworksetup(args...); err != nil {
			return err
		}
	}
	return nil
}

func RestoreSystemProxy(snapshot *SystemProxySnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("nil system proxy snapshot")
	}
	for _, service := range snapshot.Services {
		if service.PACURL != "" {
			if _, err := runNetworksetup("-setautoproxyurl", service.Service, service.PACURL); err != nil {
				return err
			}
		}
		if _, err := runNetworksetup("-setautoproxystate", service.Service, boolToState(service.PACEnabled)); err != nil {
			return err
		}
		if service.WebHost != "" || service.WebPort > 0 {
			if _, err := runNetworksetup("-setwebproxy", service.Service, service.WebHost, strconv.Itoa(service.WebPort), "off"); err != nil {
				return err
			}
		}
		if _, err := runNetworksetup("-setwebproxystate", service.Service, boolToState(service.WebEnabled)); err != nil {
			return err
		}
		if service.SecureHost != "" || service.SecurePort > 0 {
			if _, err := runNetworksetup("-setsecurewebproxy", service.Service, service.SecureHost, strconv.Itoa(service.SecurePort), "off"); err != nil {
				return err
			}
		}
		if _, err := runNetworksetup("-setsecurewebproxystate", service.Service, boolToState(service.SecureEnabled)); err != nil {
			return err
		}
		args := append([]string{"-setproxybypassdomains", service.Service}, service.BypassDomains...)
		if len(service.BypassDomains) == 0 {
			args = []string{"-setproxybypassdomains", service.Service, ""}
		}
		if _, err := runNetworksetup(args...); err != nil {
			return err
		}
	}
	return nil
}

func boolToState(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
