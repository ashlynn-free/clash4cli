package logger

import (
	"sync"
	"testing"
	"time"
)

func TestSetPublisherCallsPublisher(t *testing.T) {
	defer func() {
		// Reset after test
		once = sync.Once{}
		defaultLogger = nil
	}()

	if err := Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	got := make(chan Entry, 1)
	pub := func(e Entry) {
		got <- e
	}

	SetPublisher(pub)
	Info("hello %s", "world")

	select {
	case e := <-got:
		if e.Message != "hello world" {
			t.Errorf("expected message 'hello world', got %q", e.Message)
		}
		if e.Level != LevelInfo {
			t.Errorf("expected level LevelInfo, got %v", e.Level)
		}
		if e.Time.IsZero() {
			t.Error("expected non-zero time")
		}
	case <-time.After(time.Second):
		t.Fatal("publisher was not called within 1s")
	}
}

func TestSetPublisherWithoutPublisher(t *testing.T) {
	defer func() {
		once = sync.Once{}
		defaultLogger = nil
	}()

	if err := Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Should not panic
	Info("test")
}

func TestClearPublisher(t *testing.T) {
	defer func() {
		once = sync.Once{}
		defaultLogger = nil
	}()

	if err := Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	called := false
	SetPublisher(func(e Entry) {
		called = true
	})
	ClearPublisher()

	Info("test after clear")
	time.Sleep(100 * time.Millisecond)

	if called {
		t.Error("publisher was called after ClearPublisher")
	}
}

func TestSetPublisherBeforeInit(t *testing.T) {
	// Should not panic even if called before Init
	SetPublisher(func(e Entry) {})
}

func TestLogLevelFilter(t *testing.T) {
	defer func() {
		once = sync.Once{}
		defaultLogger = nil
	}()

	if err := Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	SetLevel(LevelWarn)

	called := false
	SetPublisher(func(e Entry) {
		called = true
	})

	Info("should be filtered")

	select {
	case <-time.After(100 * time.Millisecond):
	}

	if called {
		t.Error("publisher was called for Info log when min level is Warn")
	}
}
