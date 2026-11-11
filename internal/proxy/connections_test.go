package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"rocket4cli/internal/config"
)

func TestController_GetConnections(t *testing.T) {
	cfg := config.DefaultConfig()
	c := NewController(cfg)
	c.status = StatusRunning
	c.apiHost = "127.0.0.1"
	c.apiPort = 9090
	c.apiClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/connections" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body := `{
			"downloadTotal": 123,
			"uploadTotal": 456,
			"connections": [{
				"id": "conn-1",
				"metadata": {
					"network": "tcp",
					"type": "HTTPS",
					"sourceIP": "127.0.0.1",
					"sourcePort": "5555",
					"destinationIP": "1.2.3.4",
					"destinationPort": "443",
					"host": "api.example.com",
					"processPath": "/usr/bin/curl"
				},
				"upload": 10,
				"download": 20,
				"start": "2026-01-01T00:00:00Z",
				"chains": ["Proxy", "HK-01"],
				"rule": "DomainSuffix",
				"rulePayload": "example.com"
			}, {
				"id": "conn-inner",
				"metadata": {
					"network": "tcp",
					"type": "Inner",
					"sourceIP": "",
					"sourcePort": "0",
					"destinationIP": "1.2.3.4",
					"destinationPort": "8443"
				},
				"upload": 5,
				"download": 5,
				"start": "2026-01-01T00:00:00Z",
				"chains": ["HK-01"]
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})

	snapshot, err := c.GetConnections()
	if err != nil {
		t.Fatalf("GetConnections: %v", err)
	}
	if snapshot.UploadTotal != 456 || snapshot.DownloadTotal != 123 {
		t.Fatalf("unexpected totals: up=%d down=%d", snapshot.UploadTotal, snapshot.DownloadTotal)
	}
	if len(snapshot.Connections) != 1 {
		t.Fatalf("expected 1 connection with Inner filtered, got %d", len(snapshot.Connections))
	}
	conn := snapshot.Connections[0]
	if conn.Host != "api.example.com" || conn.DestinationPort != "443" {
		t.Fatalf("unexpected host: %#v", conn)
	}
	if strings.Join(conn.Chains, " > ") != "Proxy > HK-01" {
		t.Fatalf("unexpected chains: %v", conn.Chains)
	}
	if conn.Rule != "DomainSuffix" || conn.RulePayload != "example.com" {
		t.Fatalf("unexpected rule: %q %q", conn.Rule, conn.RulePayload)
	}
	if conn.Upload != 10 || conn.Download != 20 {
		t.Fatalf("unexpected traffic: up=%d down=%d", conn.Upload, conn.Download)
	}
}
