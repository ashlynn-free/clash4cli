package main

import (
	"testing"

	"rocket4cli/internal/proxy"
)

func TestShouldFallbackToEnv(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name:   "glibc mismatch",
			stderr: "curl: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.34' not found (required by /tmp/libproxychains4.so)",
			want:   true,
		},
		{
			name:   "shared library loader error",
			stderr: "error while loading shared libraries: libproxychains4.so: cannot open shared object file: No such file or directory",
			want:   true,
		},
		{
			name:   "wrong elf class",
			stderr: "error while loading shared libraries: libproxychains4.so: wrong ELF class: ELFCLASS32",
			want:   true,
		},
		{
			name:   "no proxychains lib mentioned",
			stderr: "some other error",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackToEnv(tc.stderr); got != tc.want {
				t.Fatalf("shouldFallbackToEnv()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestVerboseStateSummary(t *testing.T) {
	state := &proxy.SlotState{
		NodeName:         "Node-A",
		UpstreamEnabled:  true,
		UpstreamProtocol: "https",
		UpstreamServer:   "gateway.example.com",
		UpstreamPort:     443,
	}

	if got := verboseStateSummary(state); got != "chain: Node-A -> https://gateway.example.com:443" {
		t.Fatalf("unexpected verboseStateSummary: %q", got)
	}
}
