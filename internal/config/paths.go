package config

import (
	"os"
	"path/filepath"
)

// GetBaseDir returns the base directory.
//
// Default: ~/.clash4cli
func GetBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".clash4cli"), nil
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
