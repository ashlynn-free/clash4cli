package subscription

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"rocket4cli/internal/config"
	"rocket4cli/internal/logger"
)

// Manager manages subscriptions
type Manager struct {
	subscriptions  map[string]*Subscription
	mu             sync.RWMutex
	httpClient     *http.Client
	updateViaProxy bool
	proxyProvider  func() (*url.URL, error)
}

// NewManager creates a new subscription manager
func NewManager() *Manager {
	// Create HTTP client that bypasses system proxy
	transport := &http.Transport{
		Proxy: nil, // Don't use system proxy
	}

	return &Manager{
		subscriptions: make(map[string]*Subscription),
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// SetUpdateViaProxy controls whether URL subscriptions can be fetched via the currently running local proxy.
// The actual proxy address is resolved via the proxy provider (if set).
func (m *Manager) SetUpdateViaProxy(enabled bool) {
	m.mu.Lock()
	m.updateViaProxy = enabled
	m.mu.Unlock()
}

// SetProxyProvider sets a callback to resolve the HTTP proxy URL used for subscription updates.
// When the provider returns nil (or errors), updates will fall back to direct fetch.
func (m *Manager) SetProxyProvider(provider func() (*url.URL, error)) {
	m.mu.Lock()
	m.proxyProvider = provider
	m.mu.Unlock()
}

// Load loads all subscriptions from disk
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	subsDir, err := config.GetSubscriptionsDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(subsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var loadErrors []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		filePath := filepath.Join(subsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("failed to read %s: %v", entry.Name(), err))
			continue
		}

		var sub Subscription
		if err := yaml.Unmarshal(data, &sub); err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("failed to parse %s: %v", entry.Name(), err))
			continue
		}

		// Backward compatibility: infer source if missing.
		if sub.Source == "" {
			if sub.URL != "" {
				sub.Source = "url"
			} else if sub.Content != "" {
				sub.Source = "manual"
			}
		}

		m.subscriptions[sub.ID] = &sub
	}

	// Log any errors encountered during loading
	for _, errMsg := range loadErrors {
		logger.Warn("Subscription load: %s", errMsg)
	}

	return nil
}

// Add adds a new subscription
func (m *Manager) Add(name, url string) (*Subscription, error) {
	// Validate inputs
	if name == "" {
		return nil, fmt.Errorf("subscription name cannot be empty")
	}
	if url == "" {
		return nil, fmt.Errorf("subscription URL cannot be empty")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("subscription URL must start with http:// or https://")
	}

	attemptAt := time.Now()

	logger.Info("Adding subscription: %s", name)
	logger.Info("URL: %s", url)

	// Generate ID from URL
	hash := sha256.Sum256([]byte(url))
	id := hex.EncodeToString(hash[:8])
	logger.Info("Generated ID: %s", id)

	m.mu.Lock()
	if _, exists := m.subscriptions[id]; exists {
		m.mu.Unlock()
		logger.Warn("Subscription already exists: %s", id)
		return nil, fmt.Errorf("subscription already exists")
	}
	m.mu.Unlock()

	// Fetch and parse (don't fail if fetch fails, just save with 0 nodes)
	logger.Info("Fetching subscription content...")
	client, viaProxy := m.clientForUpdate()
	nodes, err := m.fetchAndParseWithClient(url, client)
	lastErr := ""
	if err != nil {
		logger.Warn("Failed to fetch/parse subscription: %v (will save with 0 nodes)", err)
		nodes = []Node{} // Empty nodes, but still save the subscription
		lastErr = err.Error()
	}
	logger.Info("Parsed %d nodes from subscription", len(nodes))

	sub := &Subscription{
		ID:                  id,
		Name:                name,
		Source:              "url",
		URL:                 url,
		Nodes:               nodes,
		LastUpdateAttemptAt: attemptAt,
		LastUpdateError:     lastErr,
		LastUpdateViaProxy:  viaProxy,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	m.mu.Lock()
	m.subscriptions[id] = sub
	m.mu.Unlock()

	// Save to disk
	logger.Info("Saving subscription to disk...")
	if err := m.saveSub(sub); err != nil {
		logger.Error("Failed to save subscription: %v", err)
		return nil, err
	}
	logger.Info("Subscription saved successfully")

	return sub, nil
}

// AddManual adds a new subscription from raw (user pasted) content.
func (m *Manager) AddManual(name, content string) (*Subscription, error) {
	if name == "" {
		return nil, fmt.Errorf("subscription name cannot be empty")
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("subscription content cannot be empty")
	}

	attemptAt := time.Now()
	nodes, err := Parse([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse subscription content: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no valid nodes found in subscription content")
	}

	id, err := randomID(8)
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		ID:                  id,
		Name:                name,
		Source:              "manual",
		Content:             content,
		Nodes:               nodes,
		LastUpdateAttemptAt: attemptAt,
		LastUpdateViaProxy:  false,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	m.mu.Lock()
	m.subscriptions[id] = sub
	m.mu.Unlock()

	if err := m.saveSub(sub); err != nil {
		return nil, err
	}

	return sub, nil
}

// Update updates a subscription by fetching new nodes
func (m *Manager) Update(id string) error {
	m.mu.RLock()
	sub, exists := m.subscriptions[id]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("subscription not found")
	}
	source := sub.Source
	url := sub.URL
	content := sub.Content
	useProxy := m.updateViaProxy
	provider := m.proxyProvider
	m.mu.RUnlock()

	// Backward compatibility: infer source if missing
	if source == "" {
		if url != "" {
			source = "url"
		} else if content != "" {
			source = "manual"
		}
	}

	attemptAt := time.Now()

	var nodes []Node
	var err error
	viaProxy := false
	switch source {
	case "manual":
		nodes, err = Parse([]byte(content))
		if err != nil {
			err = fmt.Errorf("failed to parse subscription content: %w", err)
			break
		}
		if len(nodes) == 0 {
			err = fmt.Errorf("no valid nodes found in subscription content")
			break
		}
	case "url":
		if url == "" {
			err = fmt.Errorf("subscription URL is empty")
			break
		}
		// Fetch without holding lock (network operation)
		client := m.httpClient
		if useProxy && provider != nil {
			if proxyURL, perr := provider(); perr == nil && proxyURL != nil && proxyURL.Host != "" {
				client = m.clientWithProxy(proxyURL)
				viaProxy = true
			}
		}
		nodes, err = m.fetchAndParseWithClient(url, client)
		if err != nil {
			break
		}
	default:
		err = fmt.Errorf("unknown subscription source: %s", source)
	}

	if err != nil {
		// Persist the update attempt metadata even on failure.
		m.mu.Lock()
		sub, exists = m.subscriptions[id]
		if !exists {
			m.mu.Unlock()
			return fmt.Errorf("subscription was deleted during update")
		}
		sub.Source = source
		sub.LastUpdateAttemptAt = attemptAt
		sub.LastUpdateError = err.Error()
		sub.LastUpdateViaProxy = viaProxy
		m.mu.Unlock()
		_ = m.saveSub(sub)
		return err
	}

	// Re-check and update while holding lock
	m.mu.Lock()
	sub, exists = m.subscriptions[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("subscription was deleted during update")
	}
	sub.Source = source
	sub.Nodes = nodes
	sub.LastUpdateAttemptAt = attemptAt
	sub.LastUpdateError = ""
	sub.LastUpdateViaProxy = viaProxy
	sub.UpdatedAt = time.Now()
	m.mu.Unlock()

	return m.saveSub(sub)
}

// UpdateAll updates all subscriptions
func (m *Manager) UpdateAll() error {
	m.mu.RLock()
	ids := make([]string, 0, len(m.subscriptions))
	for id := range m.subscriptions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, id := range ids {
		if err := m.Update(id); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Delete removes a subscription
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[id]; !exists {
		return fmt.Errorf("subscription not found")
	}

	delete(m.subscriptions, id)

	// Remove from disk
	subsDir, err := config.GetSubscriptionsDir()
	if err != nil {
		return err
	}

	return os.Remove(filepath.Join(subsDir, id+".yaml"))
}

// Get returns a subscription by ID
func (m *Manager) Get(id string) (*Subscription, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sub, exists := m.subscriptions[id]
	return sub, exists
}

// List returns all subscriptions
func (m *Manager) List() []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subs := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

// GetAllNodes returns all nodes from all subscriptions
func (m *Manager) GetAllNodes() []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var nodes []Node
	for _, sub := range m.subscriptions {
		nodes = append(nodes, sub.Nodes...)
	}
	return nodes
}

// GetNodesForSubscription returns nodes for a specific subscription ID.
// If the subscription does not exist (or id is empty), it returns an empty slice.
func (m *Manager) GetNodesForSubscription(id string) []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if id == "" {
		return nil
	}
	sub, ok := m.subscriptions[id]
	if !ok {
		return nil
	}
	nodes := make([]Node, len(sub.Nodes))
	copy(nodes, sub.Nodes)
	return nodes
}

func (m *Manager) fetchAndParseWithClient(url string, client *http.Client) ([]Node, error) {
	logger.Info("HTTP GET: %s", url)
	if client == nil {
		client = m.httpClient
	}
	resp, err := client.Get(url)
	if err != nil {
		logger.Error("HTTP request failed: %v", err)
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	logger.Info("HTTP Status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	logger.Info("Received %d bytes", len(body))

	nodes, err := Parse(body)
	if err != nil {
		logger.Error("Failed to parse content: %v", err)
		return nil, fmt.Errorf("failed to parse subscription: %w", err)
	}
	logger.Info("Successfully parsed %d nodes", len(nodes))

	// Log first few node names for debugging
	for i, node := range nodes {
		if i >= 3 {
			logger.Info("... and %d more nodes", len(nodes)-3)
			break
		}
		logger.Info("Node %d: %s (%s)", i+1, node.Name, node.Type)
	}

	return nodes, nil
}

func (m *Manager) clientForUpdate() (*http.Client, bool) {
	m.mu.RLock()
	enabled := m.updateViaProxy
	provider := m.proxyProvider
	m.mu.RUnlock()

	if !enabled || provider == nil {
		return m.httpClient, false
	}
	proxyURL, err := provider()
	if err != nil || proxyURL == nil || proxyURL.Host == "" {
		return m.httpClient, false
	}
	return m.clientWithProxy(proxyURL), true
}

func (m *Manager) clientWithProxy(proxyURL *url.URL) *http.Client {
	if proxyURL == nil {
		return m.httpClient
	}

	base := m.httpClient
	timeout := 30 * time.Second
	if base != nil && base.Timeout > 0 {
		timeout = base.Timeout
	}

	var transport *http.Transport
	if base != nil {
		if bt, ok := base.Transport.(*http.Transport); ok && bt != nil {
			transport = bt.Clone()
		}
	}
	if transport == nil {
		transport = &http.Transport{}
	}
	transport.Proxy = http.ProxyURL(proxyURL)

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func (m *Manager) saveSub(sub *Subscription) error {
	subsDir, err := config.GetSubscriptionsDir()
	if err != nil {
		return err
	}

	if err := config.EnsureDir(subsDir); err != nil {
		return err
	}

	data, err := yaml.Marshal(sub)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(subsDir, sub.ID+".yaml"), data, 0644)
}

func randomID(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 8
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
