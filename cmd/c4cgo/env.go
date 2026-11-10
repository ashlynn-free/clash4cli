package main

import "strings"

// loaderEnvKeys are environment variables that can suppress or redirect the
// dynamic-linker injection. A leftover value from the parent environment would
// otherwise shadow the value c4cgo appends (getenv returns the first match),
// silently disabling proxychains.
var loaderEnvKeys = []string{
	"LD_PRELOAD",
	"LD_LIBRARY_PATH",
	"LD_AUDIT",
	"DYLD_INSERT_LIBRARIES",
	"DYLD_LIBRARY_PATH",
	"DYLD_FRAMEWORK_PATH",
	"DYLD_FALLBACK_LIBRARY_PATH",
	"DYLD_FALLBACK_FRAMEWORK_PATH",
	"DYLD_FORCE_FLAT_NAMESPACE",
	"PROXYCHAINS_CONF_FILE",
	"PROXYCHAINS_QUIET_MODE",
}

// proxyEnvKeys are removed before c4cgo sets its own proxy variables, so the
// child cannot inherit a conflicting HTTP(S)_PROXY/NO_PROXY from the shell.
var proxyEnvKeys = []string{
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"ALL_PROXY",
	"NO_PROXY",
	"FTP_PROXY",
	"RSYNC_PROXY",
	"http_proxy",
	"https_proxy",
	"all_proxy",
	"no_proxy",
	"ftp_proxy",
	"rsync_proxy",
}

// sanitizeLoaderEnv removes variables that could disable or hijack the
// proxychains injection. Matching is exact (Unix env names are case-sensitive).
func sanitizeLoaderEnv(env []string) []string {
	return filterEnv(env, loaderEnvKeys, true)
}

// sanitizeProxyEnv removes HTTP_PROXY and friends. Matching is case-insensitive
// because both upper- and lower-case variants are commonly consumed.
func sanitizeProxyEnv(env []string) []string {
	return filterEnv(env, proxyEnvKeys, false)
}

func filterEnv(env []string, keys []string, exact bool) []string {
	drop := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if !exact {
			key = strings.ToUpper(key)
		}
		drop[key] = struct{}{}
	}

	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if !exact {
			name = strings.ToUpper(name)
		}
		if _, found := drop[name]; found {
			continue
		}
		out = append(out, entry)
	}
	return out
}
