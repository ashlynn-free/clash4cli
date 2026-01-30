//go:build !unix

package proxy

import (
	"fmt"
	"os/exec"
	"time"
)

func configureDetachedCommand(cmd *exec.Cmd) {}

func isProcessAlive(pid int) bool { return false }

func terminateProcess(pid int, timeout time.Duration) error {
	return fmt.Errorf("terminateProcess unsupported on this platform")
}

func processLooksLikeMihomo(pid int, configPath string) bool { return false }
