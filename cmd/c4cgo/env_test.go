package main

import (
	"strings"
	"testing"
)

func TestSanitizeLoaderEnvRemovesInjectionOverrides(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"LD_PRELOAD=/tmp/evil.so",
		"LD_LIBRARY_PATH=/tmp/lib",
		"LD_AUDIT=/tmp/audit.so",
		"DYLD_INSERT_LIBRARIES=/tmp/evil.dylib",
		"DYLD_LIBRARY_PATH=/tmp/dyld",
		"DYLD_FORCE_FLAT_NAMESPACE=1",
		"PROXYCHAINS_CONF_FILE=/tmp/other.conf",
		"PROXYCHAINS_QUIET_MODE=1",
		"HOME=/Users/me",
	}
	got := sanitizeLoaderEnv(env)
	joined := strings.Join(got, "\n")
	for _, banned := range loaderEnvKeys {
		if strings.Contains(joined, banned+"=") {
			t.Fatalf("expected %s to be removed, got:\n%s", banned, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "HOME=/Users/me") {
		t.Fatalf("expected unrelated variables to survive, got:\n%s", joined)
	}
}

func TestSanitizeProxyEnvIsCaseInsensitive(t *testing.T) {
	env := []string{
		"HTTP_PROXY=http://old:1",
		"http_proxy=http://old:2",
		"HTTPS_PROXY=http://old:3",
		"ALL_PROXY=socks5://old:4",
		"NO_PROXY=*",
		"PATH=/usr/bin",
		"GIT_PROXY_COMMAND=ssh",
	}
	got := sanitizeProxyEnv(env)
	joined := strings.Join(got, "\n")
	for _, banned := range []string{"HTTP_PROXY=", "http_proxy=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY="} {
		if strings.Contains(joined, banned) {
			t.Fatalf("expected %s to be removed, got:\n%s", banned, joined)
		}
	}
	// Only exact proxy variable names are stripped; GIT_PROXY_COMMAND is not ours.
	if !strings.Contains(joined, "GIT_PROXY_COMMAND=ssh") || !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("expected unrelated variables to survive, got:\n%s", joined)
	}
}

func TestFilterEnvDropsMalformedEntries(t *testing.T) {
	got := filterEnv([]string{"NO_EQUALS", "A=1"}, []string{"A"}, true)
	if len(got) != 0 {
		t.Fatalf("expected malformed entries to be dropped, got %v", got)
	}
}
