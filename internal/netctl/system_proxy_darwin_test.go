//go:build darwin

package netctl

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"rocket4cli/internal/config"
)

func TestNormalizeBypassDomains(t *testing.T) {
	got, err := normalizeBypassDomains([]string{" localhost ", "127.0.0.1", "localhost"}, []string{"127.0.0.1"}, true, true)
	if err != nil {
		t.Fatalf("normalizeBypassDomains: %v", err)
	}
	want := []string{"127.0.0.1", "localhost"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected bypass list: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeBypassDomains_Invalid(t *testing.T) {
	if _, err := normalizeBypassDomains([]string{"bad value"}, nil, false, true); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestApplySystemProxyManualMode(t *testing.T) {
	orig := runNetworksetup
	defer func() { runNetworksetup = orig }()

	var calls [][]string
	runNetworksetup = func(args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(args) == 1 && args[0] == "-listallnetworkservices" {
			return "Wi-Fi\nAX88179A\n", nil
		}
		return "", nil
	}

	err := ApplySystemProxy(config.SystemProxyConfig{
		Host:                   "127.0.0.1",
		Port:                   17890,
		PACMode:                false,
		AlwaysUseDefaultBypass: true,
		ValidateBypassFormat:   true,
	})
	if err != nil {
		t.Fatalf("ApplySystemProxy: %v", err)
	}

	joined := make([]string, 0, len(calls))
	for _, c := range calls {
		joined = append(joined, strings.Join(c, " "))
	}
	mustContain := []string{
		"-setwebproxy Wi-Fi 127.0.0.1 17890 off",
		"-setsecurewebproxy Wi-Fi 127.0.0.1 17890 off",
		"-setwebproxystate Wi-Fi on",
		"-setsecurewebproxystate Wi-Fi on",
	}
	for _, want := range mustContain {
		found := false
		for _, got := range joined {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected call %q not found in %#v", want, joined)
		}
	}
}

func TestSnapshotSystemProxy_CapturesBypassDomains(t *testing.T) {
	orig := runNetworksetup
	defer func() { runNetworksetup = orig }()

	runNetworksetup = func(args ...string) (string, error) {
		if len(args) == 1 && args[0] == "-listallnetworkservices" {
			return "Wi-Fi\n", nil
		}
		if len(args) == 2 && args[0] == "-getwebproxy" {
			return "Enabled: No\nServer: \nPort: 0\n", nil
		}
		if len(args) == 2 && args[0] == "-getsecurewebproxy" {
			return "Enabled: No\nServer: \nPort: 0\n", nil
		}
		if len(args) == 2 && args[0] == "-getautoproxyurl" {
			return "Enabled: No\nURL: \n", nil
		}
		if len(args) == 2 && args[0] == "-getproxybypassdomains" {
			return "127.0.0.1\nlocalhost\n", nil
		}
		return "", nil
	}

	snapshot, err := SnapshotSystemProxy()
	if err != nil {
		t.Fatalf("SnapshotSystemProxy: %v", err)
	}
	if len(snapshot.Services) != 1 || len(snapshot.Services[0].BypassDomains) != 2 {
		t.Fatalf("expected bypass domains in snapshot, got %#v", snapshot)
	}
}

func TestApplySystemProxyRollbackFriendlyError(t *testing.T) {
	orig := runNetworksetup
	defer func() { runNetworksetup = orig }()

	runNetworksetup = func(args ...string) (string, error) {
		if len(args) == 1 && args[0] == "-listallnetworkservices" {
			return "Wi-Fi\n", nil
		}
		if len(args) >= 2 && args[0] == "-setwebproxy" {
			return "", fmt.Errorf("boom")
		}
		return "", nil
	}

	err := ApplySystemProxy(config.SystemProxyConfig{Host: "127.0.0.1", Port: 17890})
	if err == nil {
		t.Fatalf("expected error")
	}
}
