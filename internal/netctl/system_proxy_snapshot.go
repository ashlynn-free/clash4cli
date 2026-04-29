package netctl

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"rocket4cli/internal/config"
)

func systemProxySnapshotPath() (string, error) {
	baseDir, err := config.GetDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "system-proxy-snapshot.yaml"), nil
}

func SaveSystemProxySnapshot(snapshot *SystemProxySnapshot) error {
	path, err := systemProxySnapshotPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadSystemProxySnapshot() (*SystemProxySnapshot, error) {
	path, err := systemProxySnapshotPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snapshot SystemProxySnapshot
	if err := yaml.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func ClearSystemProxySnapshot() error {
	path, err := systemProxySnapshotPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
