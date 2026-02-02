package node

import (
	"rocket4cli/internal/subscription"
)

// NodeStatus represents the test status of a node
type NodeStatus int

const (
	StatusUnknown NodeStatus = iota
	StatusTesting
	StatusOnline
	StatusTimeout
	StatusError
)

// String returns the string representation of NodeStatus
func (s NodeStatus) String() string {
	switch s {
	case StatusTesting:
		return "Testing"
	case StatusOnline:
		return "Online"
	case StatusTimeout:
		return "Timeout"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// TestResult represents the result of a node speed test
type TestResult struct {
	Node    *subscription.Node
	Latency int64   // in milliseconds, -1 means failed
	Speed   int64   // download speed in bytes per second
	Loss    float64 // packet loss percentage (0-100)
	Status  NodeStatus
	Error   error
}

// SpeedTestOptions configures speed test behavior
type SpeedTestOptions struct {
	Concurrent  int  // number of concurrent tests
	TestLatency bool // whether to test latency
	TestSpeed   bool // whether to test download speed
	TestLoss    bool // whether to test packet loss
}

// DefaultSpeedTestOptions returns default speed test options
func DefaultSpeedTestOptions() SpeedTestOptions {
	return SpeedTestOptions{
		Concurrent:  50,
		TestLatency: true,
		TestSpeed:   true,
		TestLoss:    true,
	}
}
