package subscription

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"rocket4cli/internal/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewManager_UsesNoSystemProxy(t *testing.T) {
	m := NewManager()
	tr, ok := m.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", m.httpClient.Transport)
	}
	if tr.Proxy != nil {
		t.Fatalf("expected http transport Proxy=nil (do not use system proxy)")
	}
}

func TestManager_Add_Load_Update_URL(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	m := NewManager()

	call := 0
	m.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		call++
		body := ""
		switch call {
		case 1:
			body = "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1\n"
		default:
			body = strings.Join([]string{
				"ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1",
				"ss://YWVzLTEyOC1nY206cGFzcw==@example.org:8388#N2",
				"",
			}, "\n")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})

	sub, err := m.Add("test", "https://example.com/sub")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sub.Source != "url" {
		t.Fatalf("expected Source=url, got %q", sub.Source)
	}
	if sub.URL != "https://example.com/sub" {
		t.Fatalf("expected URL to be saved, got %q", sub.URL)
	}
	if len(sub.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(sub.Nodes))
	}
	if sub.LastUpdateAttemptAt.IsZero() {
		t.Fatalf("expected LastUpdateAttemptAt to be set")
	}
	if sub.LastUpdateError != "" {
		t.Fatalf("expected LastUpdateError to be empty, got %q", sub.LastUpdateError)
	}

	before := sub.UpdatedAt
	beforeAttempt := sub.LastUpdateAttemptAt
	time.Sleep(10 * time.Millisecond)
	if err := m.Update(sub.ID); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, ok := m.Get(sub.ID)
	if !ok || got == nil {
		t.Fatalf("expected subscription to exist after update")
	}
	if got.UpdatedAt.Equal(before) {
		t.Fatalf("expected UpdatedAt to change")
	}
	if !got.LastUpdateAttemptAt.After(beforeAttempt) {
		t.Fatalf("expected LastUpdateAttemptAt to advance")
	}
	if got.LastUpdateError != "" {
		t.Fatalf("expected LastUpdateError to be empty, got %q", got.LastUpdateError)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after update, got %d", len(got.Nodes))
	}
	if got.Nodes[0].Name != "N1" || got.Nodes[1].Name != "N2" {
		t.Fatalf("expected refreshed node names [N1 N2], got [%s %s]", got.Nodes[0].Name, got.Nodes[1].Name)
	}

	// Reload from disk should include updated nodes.
	m2 := NewManager()
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got2, ok := m2.Get(sub.ID)
	if !ok || got2 == nil {
		t.Fatalf("expected subscription to be loadable from disk")
	}
	if len(got2.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after reload, got %d", len(got2.Nodes))
	}
	if got2.Nodes[0].Name != "N1" || got2.Nodes[1].Name != "N2" {
		t.Fatalf("expected persisted node names [N1 N2], got [%s %s]", got2.Nodes[0].Name, got2.Nodes[1].Name)
	}
	if got2.LastUpdateAttemptAt.IsZero() {
		t.Fatalf("expected LastUpdateAttemptAt to be persisted")
	}
	if got2.LastUpdateError != "" {
		t.Fatalf("expected persisted LastUpdateError to be empty, got %q", got2.LastUpdateError)
	}
}

func TestManager_Update_PersistsFailureWithoutOverwritingNodes(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	m := NewManager()

	m.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})

	sub, err := m.Add("test", "https://example.com/sub")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	beforeUpdatedAt := sub.UpdatedAt
	beforeAttempt := sub.LastUpdateAttemptAt
	beforeNodes := len(sub.Nodes)

	// Next update fails.
	m.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("oops")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})

	time.Sleep(10 * time.Millisecond)
	if err := m.Update(sub.ID); err == nil {
		t.Fatalf("expected Update to fail")
	}

	got, ok := m.Get(sub.ID)
	if !ok || got == nil {
		t.Fatalf("expected subscription to exist after failed update")
	}
	if got.UpdatedAt != beforeUpdatedAt {
		t.Fatalf("expected UpdatedAt to remain unchanged on failure")
	}
	if !got.LastUpdateAttemptAt.After(beforeAttempt) {
		t.Fatalf("expected LastUpdateAttemptAt to advance on failure")
	}
	if got.LastUpdateError == "" {
		t.Fatalf("expected LastUpdateError to be set on failure")
	}
	if len(got.Nodes) != beforeNodes {
		t.Fatalf("expected nodes to remain unchanged on failure, got %d want %d", len(got.Nodes), beforeNodes)
	}

	// Reload should keep the error metadata.
	m2 := NewManager()
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got2, ok := m2.Get(sub.ID)
	if !ok || got2 == nil {
		t.Fatalf("expected subscription to be loadable from disk")
	}
	if got2.LastUpdateError == "" {
		t.Fatalf("expected persisted LastUpdateError to be set")
	}
}

func TestManager_Load_InferSource_BackCompat(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	if err := config.EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	subsDir, err := config.GetSubscriptionsDir()
	if err != nil {
		t.Fatalf("GetSubscriptionsDir: %v", err)
	}

	sub := &Subscription{
		ID:        "legacy",
		Name:      "legacy",
		URL:       "https://example.com/sub",
		Nodes:     nil,
		UpdatedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	b, err := yaml.Marshal(sub)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subsDir, "legacy.yaml"), b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := NewManager()
	if err := m.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := m.Get("legacy")
	if !ok || got == nil {
		t.Fatalf("expected legacy subscription to be loaded")
	}
	if got.Source != "url" {
		t.Fatalf("expected inferred Source=url, got %q", got.Source)
	}
}

func TestManager_AddManual_Load_Update(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	m := NewManager()
	content := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1\n"

	sub, err := m.AddManual("manual", content)
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}
	if sub.Source != "manual" {
		t.Fatalf("expected Source=manual, got %q", sub.Source)
	}
	if sub.URL != "" {
		t.Fatalf("expected URL empty for manual subscription, got %q", sub.URL)
	}
	if sub.Content != content {
		t.Fatalf("expected Content to be preserved")
	}
	if len(sub.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(sub.Nodes))
	}

	// Reload from disk should preserve source/content and parsed nodes.
	m2 := NewManager()
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := m2.Get(sub.ID)
	if !ok || got == nil {
		t.Fatalf("expected subscription to be loadable from disk")
	}
	if got.Source != "manual" {
		t.Fatalf("expected Source=manual after reload, got %q", got.Source)
	}
	if got.Content != content {
		t.Fatalf("expected Content to be preserved after reload")
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("expected 1 node after reload, got %d", len(got.Nodes))
	}

	// Update manual content should re-parse.
	got.Content = strings.Join([]string{
		"ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1",
		"ss://YWVzLTEyOC1nY206cGFzcw==@example.org:8388#N2",
		"",
	}, "\n")
	if err := m2.saveSub(got); err != nil {
		t.Fatalf("saveSub: %v", err)
	}
	if err := m2.Update(got.ID); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, ok := m2.Get(got.ID)
	if !ok || got2 == nil {
		t.Fatalf("expected subscription to exist after update")
	}
	if len(got2.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after manual update, got %d", len(got2.Nodes))
	}
	if got2.Nodes[0].Name != "N1" || got2.Nodes[1].Name != "N2" {
		t.Fatalf("expected manual refresh node names [N1 N2], got [%s %s]", got2.Nodes[0].Name, got2.Nodes[1].Name)
	}
}

func TestManager_UpdateAll_PartialFailure(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	m := NewManager()

	countByURL := map[string]int{}
	m.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		u := r.URL.String()
		countByURL[u]++

		switch u {
		case "https://ok/sub":
			body := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1\n"
			if countByURL[u] > 1 {
				body = strings.Join([]string{
					"ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#N1",
					"ss://YWVzLTEyOC1nY206cGFzcw==@example.org:8388#N2",
					"",
				}, "\n")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		case "https://bad/sub":
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("blocked")),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		}
	})

	okSub, err := m.Add("ok", "https://ok/sub")
	if err != nil {
		t.Fatalf("Add(ok): %v", err)
	}
	if len(okSub.Nodes) != 1 {
		t.Fatalf("expected 1 node on initial add, got %d", len(okSub.Nodes))
	}

	badSub, err := m.Add("bad", "https://bad/sub")
	if err != nil {
		t.Fatalf("Add(bad): %v", err)
	}
	if len(badSub.Nodes) != 0 {
		t.Fatalf("expected 0 nodes for failing add, got %d", len(badSub.Nodes))
	}

	if err := m.UpdateAll(); err == nil {
		t.Fatalf("expected UpdateAll to return error when one subscription fails")
	}

	gotOk, ok := m.Get(okSub.ID)
	if !ok || gotOk == nil {
		t.Fatalf("expected ok subscription to exist")
	}
	if len(gotOk.Nodes) != 2 {
		t.Fatalf("expected ok subscription to update to 2 nodes, got %d", len(gotOk.Nodes))
	}
}
