package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rocket4cli/internal/logger"
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

// restartDaemonForVersion is a variable so tests can avoid spawning processes.
var restartDaemonForVersion = RestartDaemon

// EnsureDaemon connects to the daemon, starting one if needed, and guarantees
// that c4cd reports the same build version as the client. A healthy daemon
// from a different build is restarted once; if the version still differs after
// the restart, the client must not continue.
func EnsureDaemon(ctx context.Context, clientVersion string) (*Client, *AuthorityInfo, error) {
	client, info, err := ensureDaemon(ctx)
	if err != nil {
		return nil, nil, err
	}
	if daemonVersionMatches(clientVersion, info) && infoAPIVersionMatches(info) {
		return client, info, nil
	}

	runningVersion := daemonVersion(info)
	logger.Warn("c4cd version %s does not match c4c version %s; restarting c4cd", runningVersion, clientVersion)

	client, info, err = restartDaemonForVersion(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"c4cd version %s does not match c4c version %s and the daemon could not be restarted: %w",
			runningVersion, clientVersion, err,
		)
	}
	if !daemonVersionMatches(clientVersion, info) {
		return nil, nil, fmt.Errorf(
			"c4cd version %s still does not match c4c version %s after restart; install c4c and c4cd from the same build",
			daemonVersion(info), clientVersion,
		)
	}
	return client, info, nil
}

// daemonVersionMatches reports whether the daemon's build version satisfies the
// client. An empty client version cannot be verified and is always accepted; a
// daemon that predates version reporting never matches a versioned client.
func daemonVersionMatches(clientVersion string, info *AuthorityInfo) bool {
	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		return true
	}
	return info != nil && strings.TrimSpace(info.Version) == clientVersion
}

func daemonVersion(info *AuthorityInfo) string {
	if info == nil || strings.TrimSpace(info.Version) == "" {
		return "unknown"
	}
	return strings.TrimSpace(info.Version)
}

// infoAPIVersionMatches reports whether the daemon speaks this client's API.
// Health() normally enforces this, but an incompatible-but-alive daemon is
// still returned so version negotiation can replace it.
func infoAPIVersionMatches(info *AuthorityInfo) bool {
	return info != nil && strings.TrimSpace(info.APIVersion) == APIVersion
}

// ensureDaemon attaches to a healthy daemon or starts a new one.
func ensureDaemon(ctx context.Context) (*Client, *AuthorityInfo, error) {
	client, info, err := LoadClientFromAuthorityInfo()
	if err == nil {
		if healthInfo, _, healthErr := client.Health(ctx); healthErr == nil {
			if healthInfo != nil && healthInfo.PID > 0 {
				// The authority file can predate a daemon upgrade. Trust the
				// live daemon (including its reported version) and refresh the
				// file so version negotiation sees the current build.
				info = healthInfo
				_ = WriteDaemonPID(healthInfo.PID)
				_ = WriteAuthorityInfo(healthInfo)
			}
			return client, info, nil
		} else if live := probeDaemonHealth(info.Port); live != nil && live.PID > 0 && isProcessAlive(live.PID) {
			// The daemon is alive but its API version is incompatible, so
			// Health() rejected it. Return its identity anyway; EnsureDaemon
			// restarts it instead of failing to spawn a duplicate.
			fingerprint, fpErr := HomeFingerprint()
			if fpErr == nil && (strings.TrimSpace(live.HomeFingerprint) == "" || live.HomeFingerprint == fingerprint) {
				_ = WriteDaemonPID(live.PID)
				_ = WriteAuthorityInfo(live)
				return client, live, nil
			}
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
