//go:build unix

package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceLock_Exclusive(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "c4c.lock")

	l1, err := tryAcquireInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("tryAcquireInstanceLock #1: %v", err)
	}
	defer l1.release()

	_, err = tryAcquireInstanceLock(lockPath)
	if !errors.Is(err, errInstanceAlreadyRunning) {
		t.Fatalf("expected errInstanceAlreadyRunning, got %v", err)
	}

	// Release and acquire again should work.
	l1.release()
	l1 = nil

	l3, err := tryAcquireInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("tryAcquireInstanceLock #3: %v", err)
	}
	l3.release()
}

func TestReadInstancePID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "c4c.lock")
	if err := os.WriteFile(lockPath, []byte("123\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := readInstancePID(lockPath); got != 123 {
		t.Fatalf("expected pid=123, got %d", got)
	}
}
