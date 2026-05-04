package proxy

import (
	"encoding/json"
	"os"
	"time"

	"rocket4cli/internal/config"
)

// CompatibilityProjection describes daemon-authored projection freshness metadata.
type CompatibilityProjection struct {
	Source           string `json:"source"`
	DaemonPID        int    `json:"daemon_pid"`
	DaemonAPIVersion string `json:"daemon_api_version"`
	DaemonStartedAt  string `json:"daemon_started_at"`
	Revision         uint64 `json:"revision"`
	GeneratedAt      string `json:"generated_at"`
	MaxAgeSeconds    int    `json:"max_age_seconds"`
}

// ProjectionMeta is the metadata used when writing daemon-authored projections.
type ProjectionMeta struct {
	Source           string
	DaemonPID        int
	DaemonAPIVersion string
	DaemonStartedAt  string
	Revision         uint64
	GeneratedAt      time.Time
	MaxAge           time.Duration
}

// MultiLockFile is the v2 lock file format that supports multiple proxy instances.
type MultiLockFile struct {
	Version    int                     `json:"version"`
	Projection *CompatibilityProjection `json:"projection,omitempty"`
	Instances  map[int]LockFile        `json:"instances"`
}

// ReadMultiLockFile reads the proxy.lock file and returns a MultiLockFile.
// If the file contains a v1 (plain LockFile) format, it wraps it as slot 0.
// If the file does not exist, it returns (nil, nil).
func ReadMultiLockFile() (*MultiLockFile, error) {
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Probe for the "version" field to distinguish v1 from v2.
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}

	if probe.Version >= 2 {
		var mlf MultiLockFile
		if err := json.Unmarshal(data, &mlf); err != nil {
			return nil, err
		}
		if mlf.Instances == nil {
			mlf.Instances = make(map[int]LockFile)
		}
		return &mlf, nil
	}

	// v1 format: plain LockFile without a version field.
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	return &MultiLockFile{
		Version:   2,
		Instances: map[int]LockFile{0: lf},
	}, nil
}

// writeMultiLockFile atomically writes a MultiLockFile to the standard lock path.
// If all instance slots are empty, the lock file is deleted instead.
func writeMultiLockFile(mlf *MultiLockFile) error {
	lockPath, err := config.GetLockFilePath()
	if err != nil {
		return err
	}

	// If there are no instances, remove the file.
	if len(mlf.Instances) == 0 {
		err := os.Remove(lockPath)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	data, err := json.MarshalIndent(mlf, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := lockPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, lockPath); err != nil {
		// Windows cannot rename over an existing file.
		_ = os.Remove(lockPath)
		return os.Rename(tmpPath, lockPath)
	}
	return nil
}

// updateMultiLockFile reads the multi lock file, applies a mutation, and writes it back.
func updateMultiLockFile(mut func(*MultiLockFile)) error {
	mlf, err := ReadMultiLockFile()
	if err != nil {
		return err
	}
	if mlf == nil {
		mlf = &MultiLockFile{
			Version:   2,
			Instances: make(map[int]LockFile),
		}
	}
	mut(mlf)
	return writeMultiLockFile(mlf)
}

func readSlotLockFile(slotID int) (*LockFile, error) {
	mlf, err := ReadMultiLockFile()
	if err != nil {
		return nil, err
	}
	if mlf == nil || len(mlf.Instances) == 0 {
		return nil, os.ErrNotExist
	}

	lock, ok := mlf.Instances[slotID]
	if !ok {
		return nil, os.ErrNotExist
	}

	lockCopy := lock
	return &lockCopy, nil
}

func WriteProjectedMultiLockFile(mlf *MultiLockFile, meta ProjectionMeta) error {
	if mlf == nil {
		mlf = &MultiLockFile{}
	}
	if mlf.Instances == nil {
		mlf.Instances = make(map[int]LockFile)
	}
	if meta.Source == "" {
		meta.Source = "c4cd"
	}
	generatedAt := meta.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	maxAgeSeconds := int(meta.MaxAge / time.Second)
	if maxAgeSeconds < 0 {
		maxAgeSeconds = 0
	}
	mlf.Version = 2
	mlf.Projection = &CompatibilityProjection{
		Source:           meta.Source,
		DaemonPID:        meta.DaemonPID,
		DaemonAPIVersion: meta.DaemonAPIVersion,
		DaemonStartedAt:  meta.DaemonStartedAt,
		Revision:         meta.Revision,
		GeneratedAt:      generatedAt.Format(time.RFC3339Nano),
		MaxAgeSeconds:    maxAgeSeconds,
	}
	return writeMultiLockFile(mlf)
}

func removeSlotLockFile(slotID int) error {
	return updateMultiLockFile(func(mlf *MultiLockFile) {
		delete(mlf.Instances, slotID)
	})
}

// ProjectionFresh reports whether a daemon-authored projection is still fresh.
func ProjectionFresh(mlf *MultiLockFile, now time.Time) bool {
	if mlf == nil || mlf.Projection == nil {
		return false
	}
	p := mlf.Projection
	if p.Source != "c4cd" || p.GeneratedAt == "" || p.MaxAgeSeconds <= 0 {
		return false
	}
	generatedAt, err := time.Parse(time.RFC3339Nano, p.GeneratedAt)
	if err != nil {
		return false
	}
	if now.Before(generatedAt) {
		return false
	}
	return now.Sub(generatedAt) <= time.Duration(p.MaxAgeSeconds)*time.Second
}
