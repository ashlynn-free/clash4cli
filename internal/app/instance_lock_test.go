//go:build unix

package app

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
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

func TestEnsureSingleInstanceDaemon_RefusesOnValidationFailure(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "daemon.lock")
	cmd := startLockHelper(t, lockPath, true)
	defer stopHelper(t, cmd)

	_, err := ensureSingleInstanceDaemon(lockPath, daemonTakeoverOptions{
		ValidatePID: func(pid int) error {
			return errors.New("metadata mismatch")
		},
		RetryTimeout:  500 * time.Millisecond,
		RetryInterval: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}

	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("helper should still be alive: %v", err)
	}
}

func TestEnsureSingleInstanceDaemon_TakeoverForceKill(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "daemon.lock")
	cmd := startLockHelper(t, lockPath, true)
	defer stopHelper(t, cmd)

	killerDone := make(chan error, 1)
	killer := func(pid int) error {
		if pid != cmd.Process.Pid {
			return errors.New("unexpected pid")
		}
		if err := cmd.Process.Kill(); err != nil {
			return err
		}
		go func() { killerDone <- cmd.Wait() }()
		select {
		case err := <-killerDone:
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					return nil
				}
			}
			return nil
		case <-time.After(3 * time.Second):
			return errors.New("helper did not exit after kill")
		}
	}

	lock, err := ensureSingleInstanceDaemon(lockPath, daemonTakeoverOptions{
		ValidatePID: func(pid int) error {
			if pid != cmd.Process.Pid {
				return errors.New("unexpected pid")
			}
			return nil
		},
		KillProcess:   killer,
		RetryTimeout:  5 * time.Second,
		RetryInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("ensureSingleInstanceDaemon: %v", err)
	}
	defer lock.release()
}


func TestInstanceLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_INSTANCE_LOCK_HELPER") != "1" {
		return
	}

	lockPath := os.Getenv("LOCK_PATH")
	ignoreTerm := os.Getenv("IGNORE_SIGTERM") == "1"
	if lockPath == "" {
		os.Exit(2)
	}

	lock, err := tryAcquireInstanceLock(lockPath)
	if err != nil {
		os.Exit(3)
	}
	defer lock.release()

	if ignoreTerm {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		go func() {
			for range ch {
			}
		}()
	}

	readyPath := lockPath + ".ready"
	_ = os.WriteFile(readyPath, []byte(strconv.Itoa(os.Getpid())), 0o644)
	for {
		time.Sleep(200 * time.Millisecond)
	}
}

func startLockHelper(t *testing.T, lockPath string, ignoreTerm bool) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestInstanceLockHelperProcess")
	env := append(os.Environ(), "GO_WANT_INSTANCE_LOCK_HELPER=1", "LOCK_PATH="+lockPath)
	if ignoreTerm {
		env = append(env, "IGNORE_SIGTERM=1")
	}
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	readyPath := lockPath + ".ready"
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyPath); err == nil {
			return cmd
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	t.Fatal("helper did not become ready")
	return nil
}

func stopHelper(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}
