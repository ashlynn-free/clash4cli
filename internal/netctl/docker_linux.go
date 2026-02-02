//go:build linux

package netctl

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const dockerChainName = "CLASH4CLI_DOCKER"

var runIptables = func(args ...string) (string, error) {
	cmd := exec.Command("iptables", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("iptables %s: %w\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}

func ensureChain() error {
	if _, err := runIptables("-t", "nat", "-S", dockerChainName); err == nil {
		_, _ = runIptables("-t", "nat", "-F", dockerChainName)
		return nil
	}

	// Create new chain.
	if _, err := runIptables("-t", "nat", "-N", dockerChainName); err != nil {
		// If another process created it concurrently, proceed.
		if strings.Contains(strings.ToLower(err.Error()), "chain already exists") {
			_, _ = runIptables("-t", "nat", "-F", dockerChainName)
			return nil
		}
		return err
	}
	return nil
}

func ensureJumpRule(inIface string) error {
	checkArgs := []string{"-t", "nat", "-C", "PREROUTING", "-i", inIface, "-p", "tcp", "-j", dockerChainName}
	if _, err := runIptables(checkArgs...); err == nil {
		return nil
	}
	_, err := runIptables("-t", "nat", "-I", "PREROUTING", "1", "-i", inIface, "-p", "tcp", "-j", dockerChainName)
	return err
}

func deleteJumpRule(inIface string) error {
	for {
		checkArgs := []string{"-t", "nat", "-C", "PREROUTING", "-i", inIface, "-p", "tcp", "-j", dockerChainName}
		if _, err := runIptables(checkArgs...); err != nil {
			return nil
		}
		if _, err := runIptables("-t", "nat", "-D", "PREROUTING", "-i", inIface, "-p", "tcp", "-j", dockerChainName); err != nil {
			return err
		}
	}
}

// EnableDockerTCPRedirect installs iptables rules that redirect Docker containers' TCP traffic to mihomo redir-port.
//
// This only targets packets entering the host from Docker bridge interfaces (docker0 + br-*) via nat/PREROUTING.
// It intentionally excludes RFC1918 and other local ranges to primarily affect "internet" traffic.
func EnableDockerTCPRedirect(redirPort int) error {
	if redirPort <= 0 || redirPort >= 65536 {
		return fmt.Errorf("invalid redir port: %d", redirPort)
	}

	if err := ensureChain(); err != nil {
		return err
	}

	excludes := []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}
	for _, cidr := range excludes {
		if _, err := runIptables("-t", "nat", "-A", dockerChainName, "-d", cidr, "-j", "RETURN"); err != nil {
			return err
		}
	}

	if _, err := runIptables("-t", "nat", "-A", dockerChainName, "-p", "tcp", "-j", "REDIRECT", "--to-ports", strconv.Itoa(redirPort)); err != nil {
		return err
	}

	// Typical Docker bridge interfaces:
	// - docker0 (default bridge)
	// - br-* (user-defined bridge networks)
	if err := ensureJumpRule("docker0"); err != nil {
		return err
	}
	if err := ensureJumpRule("br+"); err != nil {
		return err
	}
	return nil
}

// DisableDockerTCPRedirect removes iptables rules and deletes the dedicated nat chain.
func DisableDockerTCPRedirect() error {
	// Remove jump rules first.
	if err := deleteJumpRule("docker0"); err != nil {
		return err
	}
	if err := deleteJumpRule("br+"); err != nil {
		return err
	}

	// Flush & delete chain (ignore if missing).
	_, _ = runIptables("-t", "nat", "-F", dockerChainName)
	_, _ = runIptables("-t", "nat", "-X", dockerChainName)
	return nil
}

// DockerTCPRedirectStatus returns true if the PREROUTING jump rule exists (requires root).
func DockerTCPRedirectStatus() (bool, error) {
	if _, err := runIptables("-t", "nat", "-C", "PREROUTING", "-i", "docker0", "-p", "tcp", "-j", dockerChainName); err == nil {
		return true, nil
	}
	if _, err := runIptables("-t", "nat", "-C", "PREROUTING", "-i", "br+", "-p", "tcp", "-j", dockerChainName); err == nil {
		return true, nil
	}
	return false, nil
}
