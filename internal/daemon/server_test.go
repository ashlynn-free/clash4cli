package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

func TestServerHealthAndRuntimeHandlers(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	slotMgr := proxy.NewSlotManager(cfg)
	s := NewServer(cfg, slotMgr)
	s.info = &AuthorityInfo{PID: 123, Port: 47189, APIVersion: APIVersion, HomeFingerprint: "fp", Executable: "/tmp/c4cd", StartedAt: "2026-01-01T00:00:00Z"}

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	s.handleHealth(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", healthRec.Code)
	}
	var health map[string]any
	if err := json.Unmarshal(healthRec.Body.Bytes(), &health); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if health["api_version"] != APIVersion {
		t.Fatalf("expected api version %q, got %#v", APIVersion, health["api_version"])
	}

	runtimeReq := httptest.NewRequest(http.MethodGet, "/runtime", nil)
	runtimeRec := httptest.NewRecorder()
	s.handleRuntime(runtimeRec, runtimeReq)
	if runtimeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", runtimeRec.Code)
	}
	var snapshot RuntimeSnapshot
	if err := json.Unmarshal(runtimeRec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("unmarshal runtime: %v", err)
	}
	if snapshot.APIVersion != APIVersion {
		t.Fatalf("expected api version %q, got %q", APIVersion, snapshot.APIVersion)
	}
	if snapshot.Count != 0 {
		t.Fatalf("expected zero active slots, got %d", snapshot.Count)
	}
}

func TestServerLoopbackOnlyListener(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	slotMgr := proxy.NewSlotManager(cfg)
	s := NewServer(cfg, slotMgr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		_ = s.Stop(context.Background())
	}()

	if s.listener == nil {
		t.Fatal("expected listener")
	}
	if got := s.listener.Addr().String(); len(got) == 0 || got[:9] != "127.0.0.1" {
		t.Fatalf("expected loopback listener, got %q", got)
	}
}
