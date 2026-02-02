package proxy

import (
	"errors"
	"net"
	"strconv"
	"testing"
)

type fakeListener struct {
	addr net.Addr
}

func (l *fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (l *fakeListener) Close() error              { return nil }
func (l *fakeListener) Addr() net.Addr            { return l.addr }

func TestPickTCPPort_PreferredInUse(t *testing.T) {
	prev := listenTCP
	defer func() { listenTCP = prev }()

	listenTCP = func(network, address string) (net.Listener, error) {
		_, portStr, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		p, _ := strconv.Atoi(portStr)
		switch p {
		case 10000:
			return nil, errors.New("in use")
		default:
			return &fakeListener{addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p}}, nil
		}
	}

	got, err := pickTCPPort("127.0.0.1", 10000, 20, nil)
	if err != nil {
		t.Fatalf("pickTCPPort: %v", err)
	}
	if got != 10001 {
		t.Fatalf("expected 10001, got %d", got)
	}
}

func TestPickTCPPort_RangeExhausted(t *testing.T) {
	prev := listenTCP
	defer func() { listenTCP = prev }()

	listenTCP = func(network, address string) (net.Listener, error) {
		return nil, errors.New("in use")
	}

	_, err := pickTCPPort("127.0.0.1", 10000, 1, nil)
	if err == nil {
		t.Fatalf("expected error when all ports are unavailable")
	}
}

func TestPickTCPPort_SkipsForbidden(t *testing.T) {
	prev := listenTCP
	defer func() { listenTCP = prev }()

	listenTCP = func(network, address string) (net.Listener, error) {
		_, portStr, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		p, _ := strconv.Atoi(portStr)
		return &fakeListener{addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p}}, nil
	}

	forbidden := map[int]struct{}{10000: {}}
	got, err := pickTCPPort("127.0.0.1", 10000, 50, forbidden)
	if err != nil {
		t.Fatalf("pickTCPPort: %v", err)
	}
	if _, blocked := forbidden[got]; blocked {
		t.Fatalf("expected port not in forbidden set, got %d", got)
	}
	if got != 10001 {
		t.Fatalf("expected 10001, got %d", got)
	}
}
