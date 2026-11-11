package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ConnectionInfo describes one active mihomo connection.
type ConnectionInfo struct {
	ID              string   `json:"id"`
	Host            string   `json:"host"`
	DestinationIP   string   `json:"destination_ip"`
	DestinationPort string   `json:"destination_port"`
	SourceIP        string   `json:"source_ip"`
	SourcePort      string   `json:"source_port"`
	Network         string   `json:"network"`
	Type            string   `json:"type"`
	Rule            string   `json:"rule"`
	RulePayload     string   `json:"rule_payload"`
	Chains          []string `json:"chains"`
	Upload          int64    `json:"upload"`
	Download        int64    `json:"download"`
	Start           string   `json:"start"`
	ProcessPath     string   `json:"process_path"`
}

// ConnectionsSnapshot is the active-connection state for one proxy slot.
type ConnectionsSnapshot struct {
	UploadTotal   int64            `json:"upload_total"`
	DownloadTotal int64            `json:"download_total"`
	Connections   []ConnectionInfo `json:"connections"`
}

type mihomoConnectionsResponse struct {
	UploadTotal   int64                  `json:"uploadTotal"`
	DownloadTotal int64                  `json:"downloadTotal"`
	Connections   []mihomoConnectionInfo `json:"connections"`
}

type mihomoConnectionInfo struct {
	ID       string `json:"id"`
	Metadata struct {
		Network         string `json:"network"`
		Type            string `json:"type"`
		SourceIP        string `json:"sourceIP"`
		SourcePort      string `json:"sourcePort"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		Host            string `json:"host"`
		ProcessPath     string `json:"processPath"`
	} `json:"metadata"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Start       string   `json:"start"`
	Chains      []string `json:"chains"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
}

// GetConnections returns active connections and traffic totals for this proxy.
func (c *Controller) GetConnections() (ConnectionsSnapshot, error) {
	c.mu.RLock()
	if c.status != StatusRunning {
		c.mu.RUnlock()
		return ConnectionsSnapshot{}, fmt.Errorf("proxy is not running")
	}

	apiHost := c.apiHost
	if apiHost == "" {
		apiHost = "127.0.0.1"
	}
	apiPort := c.apiPort
	if apiPort == 0 {
		apiPort = 9090
	}
	secret := c.secret
	c.mu.RUnlock()

	apiURL := fmt.Sprintf("http://%s:%d/connections", apiHost, apiPort)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return ConnectionsSnapshot{}, fmt.Errorf("failed to create request: %w", err)
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := c.doAPI(req)
	if err != nil {
		return ConnectionsSnapshot{}, fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		msg := strings.TrimSpace(string(body))
		if msg != "" {
			return ConnectionsSnapshot{}, fmt.Errorf("API returned status %d: %s", resp.StatusCode, msg)
		}
		return ConnectionsSnapshot{}, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var out mihomoConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ConnectionsSnapshot{}, fmt.Errorf("failed to decode response: %w", err)
	}

	snapshot := ConnectionsSnapshot{
		UploadTotal:   out.UploadTotal,
		DownloadTotal: out.DownloadTotal,
		Connections:   make([]ConnectionInfo, 0, len(out.Connections)),
	}
	for _, item := range out.Connections {
		// mihomo tracks the physical dial to a dialer-proxy upstream as a
		// separate "Inner" connection. It is an internal transport segment,
		// not a client connection, so keep it out of the UI and counts.
		if strings.EqualFold(item.Metadata.Type, "Inner") {
			continue
		}
		snapshot.Connections = append(snapshot.Connections, ConnectionInfo{
			ID:              item.ID,
			Host:            item.Metadata.Host,
			DestinationIP:   item.Metadata.DestinationIP,
			DestinationPort: item.Metadata.DestinationPort,
			SourceIP:        item.Metadata.SourceIP,
			SourcePort:      item.Metadata.SourcePort,
			Network:         item.Metadata.Network,
			Type:            item.Metadata.Type,
			Rule:            item.Rule,
			RulePayload:     item.RulePayload,
			Chains:          item.Chains,
			Upload:          item.Upload,
			Download:        item.Download,
			Start:           item.Start,
			ProcessPath:     item.Metadata.ProcessPath,
		})
	}
	return snapshot, nil
}

// GetTrafficAndConnCount returns traffic totals and active connection count.
func (c *Controller) GetTrafficAndConnCount() (uploadTotal int64, downloadTotal int64, connCount int, err error) {
	snapshot, err := c.GetConnections()
	if err != nil {
		return 0, 0, 0, err
	}
	return snapshot.UploadTotal, snapshot.DownloadTotal, len(snapshot.Connections), nil
}
