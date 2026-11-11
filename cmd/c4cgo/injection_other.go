//go:build !darwin

package main

// darwinInjectionBlocked is only meaningful on macOS; other platforms use
// their own ELF checks in validateInterceptable.
func darwinInjectionBlocked(string) (string, bool) {
	return "", false
}
