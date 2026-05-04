package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// GetBaseDir returns the base directory.
//
// Default: ~/.clash4cli
func GetBaseDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("CLASH4CLI_HOME")); v != "" {
		return filepath.Clean(v), nil
	}

	home, err := userHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".clash4cli"), nil
}

func userHomeDir() (string, error) {
	// When running under sudo, use the original invoking user's home directory so
	// root-only helpers (iptables) operate on the correct ~/.clash4cli.
	if os.Geteuid() == 0 {
		if sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER")); sudoUser != "" && sudoUser != "root" {
			if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
				return u.HomeDir, nil
			}
		}
	}
	return os.UserHomeDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetConfigDir returns the configuration directory
func GetConfigDir() (string, error) {
	return GetBaseDir()
}

// GetDataDir returns the data directory
func GetDataDir() (string, error) {
	return GetBaseDir()
}

// GetCacheDir returns the cache directory
func GetCacheDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "cache"), nil
}

// GetLogDir returns the log directory
func GetLogDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "logs"), nil
}

// GetSubscriptionsDir returns the subscriptions storage directory
func GetSubscriptionsDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "subscriptions"), nil
}

// GetMihomoWorkDir returns the mihomo working directory
func GetMihomoWorkDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "mihomo"), nil
}

// GetLockFilePath returns the proxy.lock file path
func GetLockFilePath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "proxy.lock"), nil
}

// GetInstanceLockFilePath returns the single-instance lock file path.
func GetInstanceLockFilePath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "c4c.lock"), nil
}

// GetDaemonLockFilePath returns the daemon authority lock file path.
func GetDaemonLockFilePath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "daemon.lock"), nil
}

// GetDaemonPIDFilePath returns the daemon PID file path.
func GetDaemonPIDFilePath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "daemon.pid"), nil
}

// GetDaemonInfoFilePath returns the daemon info metadata file path.
func GetDaemonInfoFilePath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "daemon.info.json"), nil
}

// GetDaemonLogFilePath returns the daemon log file path.
func GetDaemonLogFilePath() (string, error) {
	logDir, err := GetLogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(logDir, "c4cd.log"), nil
}

// EnsureDir creates directory if it doesn't exist
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// EnsureAllDirs creates all required directories
func EnsureAllDirs() error {
	dirs := []func() (string, error){
		GetBaseDir,
		GetCacheDir,
		GetLogDir,
		GetSubscriptionsDir,
		GetMihomoWorkDir,
	}

	for _, getDirFunc := range dirs {
		dir, err := getDirFunc()
		if err != nil {
			return err
		}
		if err := EnsureDir(dir); err != nil {
			return err
		}
	}
	return nil
}
