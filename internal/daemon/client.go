package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rocket4cli/internal/logger"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

// SlotTraffic is the traffic summary for one slot.
type SlotTraffic struct {
	UploadTotal   int64 `json:"upload_total"`
	DownloadTotal int64 `json:"download_total"`
	ConnCount     int   `json:"conn_count"`
}

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClientFromPort(port int) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
	}
}

func NewClientFromAuthorityInfo(info *AuthorityInfo) (*Client, error) {
	if info == nil || info.Port <= 0 {
		return nil, fmt.Errorf("invalid daemon authority info")
	}
	return NewClientFromPort(info.Port), nil
}

func LoadClientFromAuthorityInfo() (*Client, *AuthorityInfo, error) {
	info, err := ReadAuthorityInfo()
	if err != nil {
		return nil, nil, err
	}
	client, err := NewClientFromAuthorityInfo(info)
	if err != nil {
		return nil, nil, err
	}
	return client, info, nil
}

func (c *Client) Health(ctx context.Context) (*AuthorityInfo, uint64, error) {
	var out struct {
		APIVersion string         `json:"api_version"`
		Daemon     *AuthorityInfo `json:"daemon"`
		Revision   uint64         `json:"revision"`
	}
	if err := c.getJSON(ctx, http.MethodGet, "/health", nil, &out); err != nil {
		return nil, 0, err
	}
	if out.APIVersion != APIVersion {
		return nil, 0, fmt.Errorf("daemon API version mismatch: got %s want %s", out.APIVersion, APIVersion)
	}
	return out.Daemon, out.Revision, nil
}

func (c *Client) Runtime(ctx context.Context) (*RuntimeSnapshot, error) {
	var out RuntimeSnapshot
	if err := c.getJSON(ctx, http.MethodGet, "/runtime", nil, &out); err != nil {
		return nil, err
	}
	if out.APIVersion != APIVersion {
		return nil, fmt.Errorf("daemon API version mismatch: got %s want %s", out.APIVersion, APIVersion)
	}
	return &out, nil
}

func (c *Client) Slot(ctx context.Context, slotID int) (*SlotResponse, error) {
	var out SlotResponse
	if err := c.getJSON(ctx, http.MethodGet, "/slots/"+strconv.Itoa(slotID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StartSlotWithMode(ctx context.Context, slotID int, subID string, nodes []subscription.Node, selected *subscription.Node, mode string) error {
	payload := StartSlotRequest{SubID: subID, Nodes: nodes, SelectedNode: selected, Mode: mode}
	return c.getJSON(ctx, http.MethodPost, "/slots/"+strconv.Itoa(slotID)+"/start", payload, nil)
}

func (c *Client) StopSlot(ctx context.Context, slotID int) error {
	return c.getJSON(ctx, http.MethodPost, "/slots/"+strconv.Itoa(slotID)+"/stop", map[string]any{}, nil)
}

// Traffic returns the traffic totals and active connection count for a slot.
func (c *Client) Traffic(ctx context.Context, slotID int) (*SlotTraffic, error) {
	var out SlotTraffic
	if err := c.getJSON(ctx, http.MethodGet, "/slots/"+strconv.Itoa(slotID)+"/traffic", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Connections returns the active connections for a slot.
func (c *Client) Connections(ctx context.Context, slotID int) (*proxy.ConnectionsSnapshot, error) {
	var out proxy.ConnectionsSnapshot
	if err := c.getJSON(ctx, http.MethodGet, "/slots/"+strconv.Itoa(slotID)+"/connections", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SetSlotMode(ctx context.Context, slotID int, mode string) error {
	payload := SetModeRequest{Mode: mode}
	return c.getJSON(ctx, http.MethodPut, "/slots/"+strconv.Itoa(slotID)+"/mode", payload, nil)
}

func (c *Client) StreamLogs(ctx context.Context) (<-chan logger.Entry, func(), error) {
	if c == nil {
		return nil, nil, fmt.Errorf("daemon client is nil")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/logs/stream", nil)
	if err != nil {
		return nil, nil, err
	}
	// Use a short-lived client with no timeout so the SSE stream stays open.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	ch := make(chan logger.Entry, 64)
	stop := func() {
		_ = resp.Body.Close()
	}
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			var msg struct {
				Time     string `json:"time"`
				Level    string `json:"level"`
				Message  string `json:"message"`
				Revision uint64 `json:"revision"`
			}
			if err := json.Unmarshal([]byte(payload), &msg); err != nil {
				continue
			}
			if msg.Level == "" && msg.Message == "" {
				continue
			}
			entryTime := time.Now()
			if t, err := time.Parse(time.RFC3339Nano, msg.Time); err == nil {
				entryTime = t
			}
			entry := logger.Entry{
				Time:    entryTime,
				Level:   parseLevel(msg.Level),
				Message: msg.Message,
			}
			ch <- entry
		}
	}()
	return ch, stop, nil
}

func parseLevel(s string) logger.Level {
	switch s {
	case "DEBUG":
		return logger.LevelDebug
	case "INFO":
		return logger.LevelInfo
	case "WARN":
		return logger.LevelWarn
	case "ERROR":
		return logger.LevelError
	default:
		return logger.LevelInfo
	}
}

func (c *Client) SaveConfig(ctx context.Context, cfg any) error {
	return c.getJSON(ctx, http.MethodPut, "/state/config", cfg, nil)
}

func (c *Client) getJSON(ctx context.Context, method, path string, body any, out any) error {
	if c == nil {
		return fmt.Errorf("daemon client is nil")
	}
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		trimmed := strings.TrimSpace(string(bodyBytes))
		if trimmed == "" {
			return fmt.Errorf("daemon returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, trimmed)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func LoadRuntimeSnapshot(ctx context.Context) (*RuntimeSnapshot, *AuthorityInfo, error) {
	client, info, err := LoadClientFromAuthorityInfo()
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := client.Runtime(ctx)
	if err != nil {
		return nil, nil, err
	}
	return snapshot, info, nil
}
