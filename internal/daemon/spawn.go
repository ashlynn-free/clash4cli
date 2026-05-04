package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func FindDaemonBinary() (string, error) {
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		candidate := filepath.Join(execDir, "c4cd")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	return exec.LookPath("c4cd")
}

func NonPrivilegedFromEnv() bool {
	return os.Geteuid() != 0
}

func EnsureDaemon(ctx context.Context) (*Client, *AuthorityInfo, error) {
	client, info, err := LoadClientFromAuthorityInfo()
	if err == nil {
		if _, _, healthErr := client.Health(ctx); healthErr == nil {
			return client, info, nil
		}
	}

	if info, err := ReadAuthorityInfo(); err == nil && info != nil {
		if info.Privileged && NonPrivilegedFromEnv() {
			if isProcessAlive(info.PID) {
				return nil, nil, fmt.Errorf(
					"existing privileged c4cd (pid %d) must be replaced manually with sudo c4cd; non-privileged auto-start is not allowed",
					info.PID,
				)
			}
		}
		_ = CleanupStaleAuthorityFiles()
	}

	binary, err := FindDaemonBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot find c4cd binary: %w", err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start c4cd: %w", err)
	}
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		client, info, err = LoadClientFromAuthorityInfo()
		if err != nil {
			continue
		}
		if _, _, healthErr := client.Health(ctx); healthErr == nil {
			return client, info, nil
		}
	}
	return nil, nil, fmt.Errorf("c4cd did not become healthy in time")
}
