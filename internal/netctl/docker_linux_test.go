//go:build linux

package netctl

import (
	"fmt"
	"strings"
	"testing"
)

func hasCall(calls [][]string, want []string) bool {
	for _, c := range calls {
		if len(c) != len(want) {
			continue
		}
		match := true
		for i := range want {
			if c[i] != want[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestEnableDockerTCPRedirect_InvalidPort(t *testing.T) {
	orig := runIptables
	defer func() { runIptables = orig }()

	calls := 0
	runIptables = func(args ...string) (string, error) {
		calls++
		return "", nil
	}

	if err := EnableDockerTCPRedirect(0); err == nil {
		t.Fatalf("expected error for invalid port")
	}
	if calls != 0 {
		t.Fatalf("expected no iptables calls for invalid port, got %d", calls)
	}
}

func TestEnableDockerTCPRedirect_BuildsRules(t *testing.T) {
	orig := runIptables
	defer func() { runIptables = orig }()

	var calls [][]string
	runIptables = func(args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))

		// Simulate "chain missing" on first probe.
		if len(args) == 4 && args[0] == "-t" && args[1] == "nat" && args[2] == "-S" && args[3] == dockerChainName {
			return "", fmt.Errorf("chain not found")
		}
		// Simulate "jump rule missing" for ensureJumpRule checks.
		if len(args) >= 10 && args[0] == "-t" && args[1] == "nat" && args[2] == "-C" && args[3] == "PREROUTING" {
			return "", fmt.Errorf("rule not found")
		}
		return "", nil
	}

	const redirPort = 12345
	if err := EnableDockerTCPRedirect(redirPort); err != nil {
		t.Fatalf("EnableDockerTCPRedirect: %v", err)
	}

	want := [][]string{
		{"-t", "nat", "-N", dockerChainName},
		{"-t", "nat", "-A", dockerChainName, "-p", "tcp", "-j", "REDIRECT", "--to-ports", "12345"},
		{"-t", "nat", "-I", "PREROUTING", "1", "-i", "docker0", "-p", "tcp", "-j", dockerChainName},
		{"-t", "nat", "-I", "PREROUTING", "1", "-i", "br+", "-p", "tcp", "-j", dockerChainName},
	}
	for _, w := range want {
		if !hasCall(calls, w) {
			t.Fatalf("expected iptables call not found: %q\nall calls:\n%s", strings.Join(w, " "), formatCalls(calls))
		}
	}
}

func TestDisableDockerTCPRedirect_RemovesRules(t *testing.T) {
	orig := runIptables
	defer func() { runIptables = orig }()

	var calls [][]string
	seenCheck := map[string]int{}

	runIptables = func(args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))

		// Make deleteJumpRule loop exactly once per iface:
		// - first "-C PREROUTING -i <iface> ..." succeeds (rule exists)
		// - subsequent "-C ..." fails (rule removed)
		if len(args) >= 10 && args[0] == "-t" && args[1] == "nat" && args[2] == "-C" && args[3] == "PREROUTING" {
			inIface := ""
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "-i" {
					inIface = args[i+1]
					break
				}
			}
			seenCheck[inIface]++
			if seenCheck[inIface] == 1 {
				return "", nil
			}
			return "", fmt.Errorf("rule not found")
		}
		return "", nil
	}

	if err := DisableDockerTCPRedirect(); err != nil {
		t.Fatalf("DisableDockerTCPRedirect: %v", err)
	}

	want := [][]string{
		{"-t", "nat", "-D", "PREROUTING", "-i", "docker0", "-p", "tcp", "-j", dockerChainName},
		{"-t", "nat", "-D", "PREROUTING", "-i", "br+", "-p", "tcp", "-j", dockerChainName},
		{"-t", "nat", "-F", dockerChainName},
		{"-t", "nat", "-X", dockerChainName},
	}
	for _, w := range want {
		if !hasCall(calls, w) {
			t.Fatalf("expected iptables call not found: %q\nall calls:\n%s", strings.Join(w, " "), formatCalls(calls))
		}
	}
}

func formatCalls(calls [][]string) string {
	var b strings.Builder
	for _, c := range calls {
		b.WriteString("  - ")
		b.WriteString(strings.Join(c, " "))
		b.WriteString("\n")
	}
	return b.String()
}
