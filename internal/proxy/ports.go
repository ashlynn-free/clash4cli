package proxy

import (
	"fmt"
	"net"
	"strconv"
)

var listenTCP = net.Listen

func canListenTCP(host string, port int) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := listenTCP("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func pickTCPPort(host string, preferred int, scan int, forbidden map[int]struct{}) (int, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if scan <= 0 {
		scan = 200
	}
	if forbidden == nil {
		forbidden = map[int]struct{}{}
	}

	if preferred <= 0 {
		for i := 0; i < 20; i++ {
			ln, err := listenTCP("tcp", net.JoinHostPort(host, "0"))
			if err != nil {
				return 0, err
			}
			port := ln.Addr().(*net.TCPAddr).Port
			_ = ln.Close()
			if _, blocked := forbidden[port]; blocked {
				continue
			}
			return port, nil
		}
		return 0, fmt.Errorf("failed to allocate a free port")
	}

	for p := preferred; p < preferred+scan && p < 65536; p++ {
		if _, blocked := forbidden[p]; blocked {
			continue
		}
		if canListenTCP(host, p) {
			return p, nil
		}
	}

	end := preferred + scan - 1
	if end > 65535 {
		end = 65535
	}
	return 0, fmt.Errorf("no free port available on %s in range %d-%d", host, preferred, end)
}
