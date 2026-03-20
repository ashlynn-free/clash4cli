package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

// Manager manages nodes and their test results
type Manager struct {
	subManager        *subscription.Manager
	speedTester       *SpeedTester
	results           map[string]*TestResult // key: server:port
	mu                sync.RWMutex
	selected          *subscription.Node
	probeLockProvider func(*subscription.Node) *proxy.LockFile
}

// NewManager creates a new node manager
func NewManager(subMgr *subscription.Manager, concurrent int, timeout time.Duration) *Manager {
	opts := SpeedTestOptions{
		Concurrent:  concurrent,
		TestLatency: true,
		TestSpeed:   false, // Disable by default for faster testing
		TestLoss:    false, // Disable by default for faster testing
	}

	return &Manager{
		subManager:  subMgr,
		speedTester: NewSpeedTester(opts, timeout),
		results:     make(map[string]*TestResult),
	}
}

// SetProbeLockProvider resolves a running slot lock for a node, when available.
func (m *Manager) SetProbeLockProvider(provider func(*subscription.Node) *proxy.LockFile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probeLockProvider = provider
	m.speedTester.SetProbeLockProvider(provider)
}

// GetProbeLock returns the current running slot lock for a node, if any.
func (m *Manager) GetProbeLock(node *subscription.Node) *proxy.LockFile {
	m.mu.RLock()
	provider := m.probeLockProvider
	m.mu.RUnlock()
	if provider == nil || node == nil {
		return nil
	}
	return provider(node)
}

// GetNodes returns all nodes from all subscriptions
func (m *Manager) GetNodes() []subscription.Node {
	return m.subManager.GetAllNodes()
}

func (m *Manager) GetNodesForSubscription(subscriptionID string) []subscription.Node {
	return m.subManager.GetNodesForSubscription(subscriptionID)
}

// GetNodeWithResult returns a node with its test result
func (m *Manager) GetNodeWithResult(node *subscription.Node) *TestResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := nodeKey(node)
	if result, ok := m.results[key]; ok {
		return result
	}

	return &TestResult{
		Node:   node,
		Status: StatusUnknown,
	}
}

// TestAll tests all nodes
func (m *Manager) TestAll(ctx context.Context, progressCh chan<- TestResult) {
	nodes := m.GetNodes()
	resultCh := make(chan TestResult, len(nodes))

	go func() {
		m.speedTester.TestAll(ctx, nodes, resultCh)
		close(resultCh)
	}()

	for result := range resultCh {
		m.mu.Lock()
		m.results[nodeKey(result.Node)] = &result
		m.mu.Unlock()

		if progressCh != nil {
			select {
			case progressCh <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (m *Manager) TestNodes(ctx context.Context, nodes []subscription.Node, progressCh chan<- TestResult) {
	resultCh := make(chan TestResult, len(nodes))

	go func() {
		m.speedTester.TestAll(ctx, nodes, resultCh)
		close(resultCh)
	}()

	for result := range resultCh {
		m.mu.Lock()
		m.results[nodeKey(result.Node)] = &result
		m.mu.Unlock()

		if progressCh != nil {
			select {
			case progressCh <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

// TestNode tests a single node
func (m *Manager) TestNode(ctx context.Context, node *subscription.Node) TestResult {
	result := m.speedTester.TestNode(ctx, node)

	m.mu.Lock()
	m.results[nodeKey(node)] = &result
	m.mu.Unlock()

	return result
}

// Select selects a node for connection
func (m *Manager) Select(node *subscription.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.selected = node
}

// GetSelected returns the currently selected node
func (m *Manager) GetSelected() *subscription.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selected
}

// ClearResults clears all test results
func (m *Manager) ClearResults() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = make(map[string]*TestResult)
}

func nodeKey(node *subscription.Node) string {
	return fmt.Sprintf("%s:%d", node.Server, node.Port)
}
