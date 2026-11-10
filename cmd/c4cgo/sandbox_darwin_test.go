//go:build darwin

package main

import (
	"strings"
	"testing"
)

func TestStrictSandboxProfile(t *testing.T) {
	profile := strictSandboxProfile(17890)
	for _, want := range []string{
		"(deny network*)",
		`(allow network-outbound (remote ip "localhost:*"))`,
		"(allow network-outbound (remote unix-socket))",
	} {
		if !strings.Contains(profile, want) {
			t.Fatalf("profile missing %q:\n%s", want, profile)
		}
	}
}

func TestStrictSandboxCommandWrapsArgv(t *testing.T) {
	if !strictSandboxSupported() {
		t.Skip("sandbox-exec not available")
	}
	argv, err := strictSandboxCommand([]string{"/usr/bin/curl", "https://example.com"}, 17890)
	if err != nil {
		t.Fatalf("strictSandboxCommand: %v", err)
	}
	if len(argv) < 4 || argv[0] != sandboxExecPath || argv[1] != "-p" {
		t.Fatalf("unexpected wrapper: %v", argv)
	}
	if argv[len(argv)-1] != "https://example.com" {
		t.Fatalf("target args not preserved: %v", argv)
	}
}
