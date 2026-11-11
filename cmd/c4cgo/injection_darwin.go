//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// entitlement keys relevant to DYLD injection. macOS honors DYLD_* variables
// for a hardened-runtime binary only when the binary opts in with
// allow-dyld-environment-variables, and the injected library must pass library
// validation. The bundled proxychains library is ad-hoc signed (no team ID),
// so it also requires the target to disable library validation.
const (
	entitlementAllowDYLDEnv         = "com.apple.security.cs.allow-dyld-environment-variables"
	entitlementDisableLibValidation = "com.apple.security.cs.disable-library-validation"
)

// codesignDiagnostics is a variable so tests can stub the external call.
var codesignDiagnostics = func(path string) (string, error) {
	out, err := exec.Command("/usr/bin/codesign", "-d", "--entitlements", ":-", "-vvv", path).CombinedOutput()
	return string(out), err
}

// darwinInjectionBlocked reports whether dyld will ignore DYLD_INSERT_LIBRARIES
// for path. reason is a human-readable explanation when blocked is true.
func darwinInjectionBlocked(path string) (reason string, blocked bool) {
	out, _ := codesignDiagnostics(path)
	if !hasHardenedRuntimeFlag(out) {
		return "", false
	}

	allowDYLD := strings.Contains(out, entitlementAllowDYLDEnv)
	libValidationDisabled := strings.Contains(out, entitlementDisableLibValidation)

	switch {
	case !allowDYLD && !libValidationDisabled:
		return "is a hardened-runtime binary without the " + entitlementAllowDYLDEnv +
			" and " + entitlementDisableLibValidation + " entitlements", true
	case !allowDYLD:
		return "is a hardened-runtime binary without the " + entitlementAllowDYLDEnv + " entitlement", true
	case !libValidationDisabled:
		return "is a hardened-runtime binary with library validation enabled; " +
			"the ad-hoc-signed proxychains library would be rejected", true
	}
	return "", false
}

// hasHardenedRuntimeFlag reports whether `codesign -d` output contains the
// hardened-runtime flag, e.g. `flags=0x10000(runtime)`.
func hasHardenedRuntimeFlag(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, "flags=")
		if idx < 0 {
			continue
		}
		flagText := line[idx+len("flags="):]
		if end := strings.IndexAny(flagText, " \t\r"); end >= 0 {
			flagText = flagText[:end]
		}
		open := strings.IndexByte(flagText, '(')
		closing := strings.IndexByte(flagText, ')')
		if open < 0 || closing < open {
			return false
		}
		for _, flag := range strings.Split(flagText[open+1:closing], ",") {
			if flag == "runtime" {
				return true
			}
		}
		return false
	}
	return false
}
