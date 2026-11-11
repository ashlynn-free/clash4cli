package daemon

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

func TestLogStreamHubCloseAll(t *testing.T) {
	hub := newLogStreamHub()
	_, ch, unsubscribe := hub.subscribe()

	hub.closeAll()

	if _, ok := <-ch; ok {
		t.Fatalf("expected channel to be closed")
	}
	// Unsubscribing after closeAll must not panic or double-close.
	unsubscribe()
}

func TestServerStopClosesLogStreamsPromptly(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	cfg := config.DefaultConfig()
	s := NewServer(cfg, proxy.NewSlotManager(cfg), "test")
	s.port = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	port := s.listener.Addr().(*net.TCPAddr).Port

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil}, // never route loopback through a proxy
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/logs/stream", port))
	if err != nil {
		t.Fatalf("open log stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read ready event: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			break
		}
	}

	start := time.Now()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stop took %v; an open SSE stream is blocking shutdown", elapsed)
	}
}
