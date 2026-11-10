//go:build darwin

package main

import (
	"fmt"
	"os"
)

const sandboxExecPath = "/usr/bin/sandbox-exec"

// strictSandboxSupported reports whether the kernel-level fail-closed sandbox
// can be used on this machine.
func strictSandboxSupported() bool {
	info, err := os.Stat(sandboxExecPath)
	return err == nil && !info.IsDir()
}

// strictSandboxCommand wraps argv so that every process in the tree is denied
// remote network access; only loopback (proxy + local services) and unix
// sockets are allowed. Remote TCP, UDP/QUIC, DNS and ICMP fail closed.
func strictSandboxCommand(argv []string, proxyPort int) ([]string, error) {
	if !strictSandboxSupported() {
		return nil, fmt.Errorf("strict mode requires %s", sandboxExecPath)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	profile := strictSandboxProfile(proxyPort)
	wrapped := make([]string, 0, len(argv)+3)
	wrapped = append(wrapped, sandboxExecPath, "-p", profile)
	wrapped = append(wrapped, argv...)
	return wrapped, nil
}

func strictSandboxProfile(proxyPort int) string {
	// `(allow default)` keeps normal process/file/mach behavior; the network
	// rules below are the only restriction. loopback is allowed so proxychains
	// can reach 127.0.0.1:<proxyPort> and local services keep working.
	return `(version 1)
(allow default)
(deny network*)
(allow network-outbound (remote ip "localhost:*"))
(allow network-inbound (local ip "localhost:*"))
(allow network-bind (local ip "localhost:*"))
(allow network-outbound (remote unix-socket))
(allow network-inbound (local unix-socket))
(allow network-bind (local unix-socket))`
}
