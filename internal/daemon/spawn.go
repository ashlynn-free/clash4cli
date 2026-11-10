package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var errDaemonDifferentHome = errors.New("c4cd on the local port belongs to a different CLASH4CLI_HOME")

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

	// The authority files can be lost while a daemon is still running (for
	// example if a failed duplicate start cleaned them up). Probe the fixed
	// local port before spawning a second daemon, which would only fail with
	// "address already in use".
	if adoptedClient, adoptedInfo, adoptErr := adoptRunningDaemon(ctx, defaultPort); adoptErr == nil {
		return adoptedClient, adoptedInfo, nil
	} else if errors.Is(adoptErr, errDaemonDifferentHome) {
		return nil, nil, adoptErr
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

// adoptRunningDaemon attaches to a healthy daemon already listening on the
// given loopback port and rewrites its authority files so future launches find
// it normally. It only adopts a daemon that belongs to the current home.
func adoptRunningDaemon(ctx context.Context, port int) (*Client, *AuthorityInfo, error) {
	client := NewClientFromPort(port)
	info, _, err := client.Health(ctx)
	if err != nil {
		return nil, nil, err
	}
	if info == nil || info.PID <= 0 {
		return nil, nil, fmt.Errorf("daemon on port %d returned an invalid identity", port)
	}

	fingerprint, err := HomeFingerprint()
	if err != nil {
		return nil, nil, err
	}
	if info.HomeFingerprint != "" && info.HomeFingerprint != fingerprint {
		return nil, nil, fmt.Errorf("%w (pid %d, port %d)", errDaemonDifferentHome, info.PID, port)
	}

	// Self-heal the authority files. These failures are not fatal: the client
	// is already connected and can still be used for this session.
	_ = WriteDaemonPID(info.PID)
	_ = WriteAuthorityInfo(info)

	return client, info, nil
}
