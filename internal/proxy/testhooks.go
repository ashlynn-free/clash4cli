package proxy

import (
	"net"
	"time"
)

var dialTimeout = net.DialTimeout

// SetDialTimeoutForTests overrides the dialer used by internal TCP reachability checks.
// Intended for unit tests only.
func SetDialTimeoutForTests(fn func(network, address string, timeout time.Duration) (net.Conn, error)) (restore func()) {
	prev := dialTimeout
	if fn == nil {
		dialTimeout = net.DialTimeout
	} else {
		dialTimeout = fn
	}
	return func() { dialTimeout = prev }
}
