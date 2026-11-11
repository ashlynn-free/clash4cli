package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"rocket4cli/internal/proxy"
)

func TestClientConnections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slots/0/connections" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(proxy.ConnectionsSnapshot{
			UploadTotal:   100,
			DownloadTotal: 200,
			Connections: []proxy.ConnectionInfo{{
				ID:       "conn-1",
				Host:     "api.example.com",
				Chains:   []string{"Proxy", "HK-01"},
				Rule:     "DomainSuffix",
				Upload:   10,
				Download: 20,
			}},
		})
	}))
	defer srv.Close()

	port := srv.Listener.Addr().(*net.TCPAddr).Port
	client := NewClientFromPort(port)
	snapshot, err := client.Connections(context.Background(), 0)
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if snapshot.UploadTotal != 100 || snapshot.DownloadTotal != 200 {
		t.Fatalf("unexpected totals: %#v", snapshot)
	}
	if len(snapshot.Connections) != 1 || snapshot.Connections[0].ID != "conn-1" {
		t.Fatalf("unexpected connections: %#v", snapshot.Connections)
	}
	if len(snapshot.Connections[0].Chains) != 2 {
		t.Fatalf("unexpected chains: %#v", snapshot.Connections[0].Chains)
	}
}
