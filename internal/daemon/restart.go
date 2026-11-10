package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"rocket4cli/internal/config"
)

const restartReadyTimeout = 5 * time.Second

// RestartDaemon stops the current daemon (if any) and starts a new detached one
// for the current CLASH4CLI_HOME. c4cd restores slot history from config on
// startup, so previously configured slots come back.
func RestartDaemon(ctx context.Context) (*Client, *AuthorityInfo, error) {
	if _, err := StopDaemon(); err != nil {
		return nil, nil, err
	}
	return startDaemonDetached(ctx)
}

func startDaemonDetached(ctx context.Context) (*Client, *AuthorityInfo, error) {
	binary, err := FindDaemonBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot find c4cd binary: %w", err)
	}

	// Do not use CommandContext here: cancelling the caller's context after the
	// command returns must not kill the freshly started daemon.
	cmd := exec.Command(binary)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil

	bootLogPath := ""
	if logFile, path, err := openDaemonBootLog(); err == nil {
		bootLogPath = path
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close() // the child keeps its own descriptor
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start c4cd: %w", err)
	}
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(restartReadyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)

		if client, info, err := LoadClientFromAuthorityInfo(); err == nil {
			if _, _, healthErr := client.Health(ctx); healthErr == nil {
				return client, info, nil
			} else {
				lastErr = healthErr
			}
		} else {
			lastErr = err
		}

		// The daemon may be healthy before its authority files are visible;
		// adoptRunningDaemon also self-heals those files.
		if client, info, err := adoptRunningDaemon(ctx, defaultPort); err == nil {
			return client, info, nil
		} else {
			lastErr = err
		}
	}

	msg := "c4cd did not become healthy in time"
	if bootLogPath != "" {
		msg = fmt.Sprintf("%s (see %s)", msg, bootLogPath)
	}
	if lastErr != nil {
		return nil, nil, fmt.Errorf("%s: %w", msg, lastErr)
	}
	return nil, nil, fmt.Errorf("%s", msg)
}

func openDaemonBootLog() (*os.File, string, error) {
	logDir, err := config.GetLogDir()
	if err != nil {
		return nil, "", err
	}
	if err := config.EnsureDir(logDir); err != nil {
		return nil, "", err
	}
	path := filepath.Join(logDir, "c4cd-boot.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}
