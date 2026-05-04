package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"rocket4cli/internal/config"
)

// AuthorityInfo stores daemon identity used by takeover validation.
type AuthorityInfo struct {
	PID             int    `json:"pid"`
	Port            int    `json:"port"`
	APIVersion      string `json:"apiVersion"`
	HomeFingerprint string `json:"homeFingerprint"`
	Executable      string `json:"executable"`
	StartedAt       string `json:"startedAt"`
	Privileged      bool   `json:"privileged"`
}

// AuthorityPaths contains daemon authority file paths.
type AuthorityPaths struct {
	LockPath string
	PIDPath  string
	InfoPath string
	LogPath  string
}

func GetAuthorityPaths() (*AuthorityPaths, error) {
	lockPath, err := config.GetDaemonLockFilePath()
	if err != nil {
		return nil, err
	}
	pidPath, err := config.GetDaemonPIDFilePath()
	if err != nil {
		return nil, err
	}
	infoPath, err := config.GetDaemonInfoFilePath()
	if err != nil {
		return nil, err
	}
	logPath, err := config.GetDaemonLogFilePath()
	if err != nil {
		return nil, err
	}
	return &AuthorityPaths{LockPath: lockPath, PIDPath: pidPath, InfoPath: infoPath, LogPath: logPath}, nil
}

func HomeFingerprint() (string, error) {
	baseDir, err := config.GetBaseDir()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(baseDir)
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:]), nil
}

func ExecutableIdentity() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Clean(resolved), nil
}

func NewAuthorityInfo(port int, apiVersion string, privileged bool) (*AuthorityInfo, error) {
	fp, err := HomeFingerprint()
	if err != nil {
		return nil, err
	}
	exe, err := ExecutableIdentity()
	if err != nil {
		return nil, err
	}
	return &AuthorityInfo{
		PID:             os.Getpid(),
		Port:            port,
		APIVersion:      strings.TrimSpace(apiVersion),
		HomeFingerprint: fp,
		Executable:      exe,
		StartedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Privileged:      privileged,
	}, nil
}

func ReadAuthorityInfo() (*AuthorityInfo, error) {
	infoPath, err := config.GetDaemonInfoFilePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, err
	}
	var info AuthorityInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, fmt.Errorf("unmarshal daemon authority info: %w", err)
	}
	return &info, nil
}

func WriteAuthorityInfo(info *AuthorityInfo) error {
	if info == nil {
		return errors.New("authority info is nil")
	}
	infoPath, err := config.GetDaemonInfoFilePath()
	if err != nil {
		return err
	}
	if err := ensureSafeExistingFile(infoPath); err != nil {
		return err
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return writeAtomicFile(infoPath, b, 0o600)
}

func ReadDaemonPID() int {
	pidPath, err := config.GetDaemonPIDFilePath()
	if err != nil {
		return 0
	}
	b, err := os.ReadFile(pidPath)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func WriteDaemonPID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}
	pidPath, err := config.GetDaemonPIDFilePath()
	if err != nil {
		return err
	}
	if err := ensureSafeExistingFile(pidPath); err != nil {
		return err
	}
	return writeAtomicFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0o600)
}

func CleanupStaleAuthorityFiles() error {
	pid := ReadDaemonPID()
	if pid > 0 && isProcessAlive(pid) {
		return nil
	}
	paths, err := GetAuthorityPaths()
	if err != nil {
		return err
	}
	for _, p := range []string{paths.LockPath, paths.PIDPath, paths.InfoPath} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func MatchesExpectedIdentity(info *AuthorityInfo, expectedHomeFingerprint, expectedExecutable string) error {
	if info == nil {
		return errors.New("missing authority info")
	}
	if strings.TrimSpace(info.HomeFingerprint) != strings.TrimSpace(expectedHomeFingerprint) {
		return fmt.Errorf("home fingerprint mismatch")
	}
	if filepath.Clean(strings.TrimSpace(info.Executable)) != filepath.Clean(strings.TrimSpace(expectedExecutable)) {
		return fmt.Errorf("executable mismatch")
	}
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	tmp := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ensureSafeExistingFile(path string) error {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("insecure permissions on %s", path)
	}
	if runtime.GOOS != "windows" {
		sys := fi.Sys()
		v := reflect.ValueOf(sys)
		if v.IsValid() {
			if v.Kind() == reflect.Pointer {
				v = v.Elem()
			}
			if v.IsValid() {
				f := v.FieldByName("Uid")
				if f.IsValid() && f.CanUint() {
					if uint32(f.Uint()) != uint32(os.Geteuid()) {
						return fmt.Errorf("unexpected file owner on %s", path)
					}
				}
			}
		}
	}
	return nil
}
