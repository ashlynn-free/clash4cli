//go:build !darwin

package main

import "fmt"

// strictSandboxSupported reports whether a kernel-level fail-closed sandbox is
// available. Only macOS provides one that does not require root.
func strictSandboxSupported() bool {
	return false
}

func strictSandboxCommand(argv []string, proxyPort int) ([]string, error) {
	return nil, fmt.Errorf("strict fail-closed mode is only supported on macOS; on Linux use mihomo TUN mode for full traffic coverage")
}
