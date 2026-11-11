//go:build darwin

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const testHardenedRuntimeFlags = "Executable=/tmp/tool\n" +
	"CodeDirectory v=20500 size=1885 flags=0x10000(runtime) hashes=54+2 location=embedded\n"

func TestHasHardenedRuntimeFlag(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{name: "not signed", out: "code object is not signed at all\n", want: false},
		{name: "adhoc", out: "CodeDirectory v=20400 size=1885 flags=0x2(adhoc) hashes=54+2", want: false},
		{name: "runtime", out: testHardenedRuntimeFlags, want: true},
		{name: "adhoc and runtime", out: "flags=0x10002(adhoc,runtime) hashes=1+1", want: true},
		{name: "no flags", out: "Identifier=codex\n", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasHardenedRuntimeFlag(tc.out); got != tc.want {
				t.Fatalf("hasHardenedRuntimeFlag()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestDarwinInjectionBlocked(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{name: "unsigned", out: "code object is not signed at all\n", want: false},
		{name: "adhoc signed", out: "flags=0x2(adhoc)\n", want: false},
		{name: "no entitlements", out: testHardenedRuntimeFlags, want: true},
		{
			name: "allow dyld only",
			out:  testHardenedRuntimeFlags + "<key>" + entitlementAllowDYLDEnv + "</key>\n",
			want: true,
		},
		{
			name: "disable library validation only",
			out:  testHardenedRuntimeFlags + "<key>" + entitlementDisableLibValidation + "</key>\n",
			want: true,
		},
		{
			name: "both entitlements",
			out: testHardenedRuntimeFlags +
				"<key>" + entitlementAllowDYLDEnv + "</key>\n" +
				"<key>" + entitlementDisableLibValidation + "</key>\n",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := codesignDiagnostics
			t.Cleanup(func() { codesignDiagnostics = old })
			codesignDiagnostics = func(string) (string, error) { return tc.out, nil }

			_, blocked := darwinInjectionBlocked("/tmp/tool")
			if blocked != tc.want {
				t.Fatalf("darwinInjectionBlocked()=%v, want %v", blocked, tc.want)
			}
		})
	}
}

func TestValidateInterceptableRejectsHardenedRuntime(t *testing.T) {
	old := codesignDiagnostics
	t.Cleanup(func() { codesignDiagnostics = old })
	codesignDiagnostics = func(string) (string, error) { return testHardenedRuntimeFlags, nil }

	path := filepath.Join(t.TempDir(), "hardened-tool")
	if err := os.WriteFile(path, []byte("\xcf\xfa\xed\xfe"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var blocked *notInterceptableError
	if err := validateInterceptable(path); !errors.As(err, &blocked) {
		t.Fatalf("expected notInterceptableError, got %v", err)
	}
}
