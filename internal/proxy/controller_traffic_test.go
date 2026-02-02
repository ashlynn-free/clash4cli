package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"rocket4cli/internal/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestController_GetTrafficTotals(t *testing.T) {
	const secret = "test-secret"

	cfg := config.DefaultConfig()
	c := NewController(cfg)
	c.status = StatusRunning
	c.apiHost = "127.0.0.1"
	c.apiPort = 9090
	c.secret = secret
	c.apiClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/connections" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Fatalf("expected Authorization header, got %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"downloadTotal":123,"uploadTotal":456}`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})

	up, down, err := c.GetTrafficTotals()
	if err != nil {
		t.Fatalf("GetTrafficTotals: %v", err)
	}
	if up != 456 || down != 123 {
		t.Fatalf("unexpected totals: up=%d down=%d", up, down)
	}
}
