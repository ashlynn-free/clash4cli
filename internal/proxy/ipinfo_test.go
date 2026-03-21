package proxy

import (
	"fmt"
	"testing"
	"time"
)

func TestProbeIPInfoViaProxy_WithHook(t *testing.T) {
	expected := IPInfoResult{
		IP:      "1.2.3.4",
		City:    "Tokyo",
		Region:  "Tokyo",
		Country: "JP",
		Org:     "AS1234 Example ISP",
	}

	restore := SetFetchIPInfoForTests(func(proxyHost string, proxyPort int, timeout time.Duration) IPInfoResult {
		if proxyHost != "127.0.0.1" {
			t.Errorf("unexpected proxyHost: %s", proxyHost)
		}
		if proxyPort != 7890 {
			t.Errorf("unexpected proxyPort: %d", proxyPort)
		}
		return expected
	})
	defer restore()

	result := ProbeIPInfoViaProxy("127.0.0.1", 7890, 5*time.Second)
	if result.IP != expected.IP {
		t.Errorf("IP = %q, want %q", result.IP, expected.IP)
	}
	if result.City != expected.City {
		t.Errorf("City = %q, want %q", result.City, expected.City)
	}
	if result.Country != expected.Country {
		t.Errorf("Country = %q, want %q", result.Country, expected.Country)
	}
	if result.Org != expected.Org {
		t.Errorf("Org = %q, want %q", result.Org, expected.Org)
	}
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestProbeIPInfoViaProxy_Error(t *testing.T) {
	restore := SetFetchIPInfoForTests(func(proxyHost string, proxyPort int, timeout time.Duration) IPInfoResult {
		return IPInfoResult{Error: "connection refused"}
	})
	defer restore()

	result := ProbeIPInfoViaProxy("127.0.0.1", 9999, 5*time.Second)
	if result.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", result.Error, "connection refused")
	}
	if result.IP != "" {
		t.Errorf("IP should be empty on error, got %q", result.IP)
	}
}

func TestSetFetchIPInfoForTests_Restore(t *testing.T) {
	originalFunc := fetchIPInfoFunc

	restore := SetFetchIPInfoForTests(func(proxyHost string, proxyPort int, timeout time.Duration) IPInfoResult {
		return IPInfoResult{IP: "stubbed"}
	})

	result := ProbeIPInfoViaProxy("127.0.0.1", 1234, time.Second)
	if result.IP != "stubbed" {
		t.Errorf("expected stubbed result, got %q", result.IP)
	}

	restore()

	// After restore, the original function should be back.
	// We can't easily call it (requires real network), but we can verify the pointer changed back.
	if fmt.Sprintf("%p", fetchIPInfoFunc) != fmt.Sprintf("%p", originalFunc) {
		t.Error("restore did not reset fetchIPInfoFunc")
	}
}
