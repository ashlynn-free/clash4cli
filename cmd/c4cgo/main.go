package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"rocket4cli/internal/daemon"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

// Version information (set via ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var (
	verboseMode    = false
	specifiedPort  = 0
	specifiedSlot  = -1     // -1 means auto-detect (first active slot)
	runMode        = "auto" // auto|proxychains|env
	strictMode     = runtime.GOOS == "darwin"
	strictExplicit = false
)

func main() {
	args := os.Args[1:]

	// Handle flags
	for len(args) > 0 {
		switch args[0] {
		case "--version":
			fmt.Printf("c4cgo %s\n", Version)
			fmt.Printf("Build time: %s\n", BuildTime)
			return
		case "-h", "--help":
			printUsage()
			return
		case "-v", "--verbose":
			verboseMode = true
			args = args[1:]
		case "--mode":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Error: --mode requires a value (auto|proxychains|env)\n")
				os.Exit(1)
			}
			switch args[1] {
			case "auto", "proxychains", "env":
				runMode = args[1]
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid --mode value: %s (expected auto|proxychains|env)\n", args[1])
				os.Exit(1)
			}
			args = args[2:]
		case "-s", "--slot":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Error: --slot requires a value (0-9)\n")
				os.Exit(1)
			}
			s, err := strconv.Atoi(args[1])
			if err != nil || s < 0 || s > 9 {
				fmt.Fprintf(os.Stderr, "Error: invalid slot number: %s (expected 0-9)\n", args[1])
				os.Exit(1)
			}
			specifiedSlot = s
			args = args[2:]
		case "--port":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Error: --port requires a value\n")
				os.Exit(1)
			}
			p, err := strconv.Atoi(args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid port number: %s\n", args[1])
				os.Exit(1)
			}
			specifiedPort = p
			args = args[2:]
		case "--strict":
			strictMode = true
			strictExplicit = true
			args = args[1:]
		case "--no-strict":
			strictMode = false
			strictExplicit = true
			args = args[1:]
		default:
			goto runCommand
		}
	}

runCommand:
	if len(args) == 0 {
		listSlots()
		return
	}

	port := specifiedPort
	proxyHost := "127.0.0.1"

	// If no port specified, read from daemon runtime
	if port == 0 {
		snapshot, _, err := daemon.LoadRuntimeSnapshot(context.Background())
		if err != nil || snapshot == nil || len(snapshot.Active) == 0 {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: Cannot read daemon runtime\033[0m\n")
			fmt.Fprintf(os.Stderr, "        c4cd is not running, unreachable, or has no active slots.\n")
			fmt.Fprintf(os.Stderr, "        Please start c4c/c4cd and select a node first.\n")
			os.Exit(1)
		}

		var state *proxy.SlotState
		var slotID int
		if specifiedSlot >= 0 {
			for _, candidate := range snapshot.Active {
				if candidate != nil && candidate.SlotID == specifiedSlot {
					state = candidate
					slotID = candidate.SlotID
					break
				}
			}
			if state == nil {
				fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: Slot %d is not active\033[0m\n", specifiedSlot)
				fmt.Fprintf(os.Stderr, "        Run 'c4cgo' with no arguments to see active slots.\n")
				os.Exit(1)
			}
		} else {
			active := append([]*proxy.SlotState(nil), snapshot.Active...)
			sort.Slice(active, func(i, j int) bool { return active[i].SlotID < active[j].SlotID })
			state = active[0]
			slotID = state.SlotID
		}

		if err := validateSlotEgress(state); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: %v\033[0m\n", err)
			fmt.Fprintf(os.Stderr, "        Refusing to run to prevent direct traffic. Select a node in c4c first.\n")
			os.Exit(1)
		}

		port = state.Port
		if verboseMode {
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] c4cgo %s\033[0m\n", Version)
			slotInfo := ""
			if len(snapshot.Active) > 1 || specifiedSlot >= 0 {
				slotInfo = fmt.Sprintf(" slot:%d", slotID)
			}
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Using daemon runtime at %s:%d (%s%s)\033[0m\n", proxyHost, port, verboseStateSummary(state), slotInfo)
		}
	}

	// Verify proxy is actually reachable
	if !verifyProxy(proxyHost, port) {
		fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: Proxy at %s:%d is not responding\033[0m\n", proxyHost, port)
		fmt.Fprintf(os.Stderr, "       The proxy may have crashed or the port is incorrect.\n")
		fmt.Fprintf(os.Stderr, "       Please check c4c status and restart if necessary.\n")
		os.Exit(1)
	}

	// Try to find proxychains
	proxychains, libPath := findProxychains()

	// Decide execution mode
	useProxychains := false
	switch runMode {
	case "proxychains":
		if proxychains == "" {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: proxychains4 not found (mode=proxychains)\033[0m\n")
			fmt.Fprintf(os.Stderr, "       Install proxychains4 or use --mode env\n")
			os.Exit(1)
		}
		useProxychains = true
	case "env":
		useProxychains = false
	default: // auto
		if proxychains == "" {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: proxychains4 not found\033[0m\n")
			fmt.Fprintf(os.Stderr, "       Cannot guarantee traffic safety without proxychains.\n")
			fmt.Fprintf(os.Stderr, "       Install proxychains4, or use --mode env at your own risk.\n")
			os.Exit(1)
		}
		useProxychains = true
	}

	// Resolve the real target (following #!/usr/bin/env shims). Refuse commands
	// that cannot be intercepted: running them would silently leak their traffic.
	plan, err := resolveRunTarget(args, useProxychains)
	if err != nil {
		// Hardened-runtime binaries (e.g. native codex/claude) ignore DYLD
		// injection, so proxychains can never cover them. In auto mode fall
		// back to proxy environment variables; the strict sandbox still keeps
		// their unproxied traffic from leaving the machine.
		var blocked *notInterceptableError
		if useProxychains && runMode == "auto" && runtime.GOOS == "darwin" && errors.As(err, &blocked) {
			useProxychains = false
			plan, err = resolveRunTarget(args, false)
			if err == nil {
				fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Warning: %s uses the macOS hardened runtime; DYLD injection is ignored.\033[0m\n", blocked.path)
				fmt.Fprintf(os.Stderr, "\033[33m         Falling back to environment-variable proxy mode (the app must honor HTTP(S)_PROXY/ALL_PROXY).\033[0m\n")
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: cannot run command: %v\033[0m\n", err)
			if useProxychains {
				fmt.Fprintf(os.Stderr, "       Refusing to run: its traffic would bypass the proxy.\n")
				if errors.As(err, &blocked) {
					fmt.Fprintf(os.Stderr, "       Use --mode env so the app can reach the proxy through environment variables.\n")
				} else {
					fmt.Fprintf(os.Stderr, "       Install a non-system build (e.g. Homebrew) or use --mode env at your own risk.\n")
				}
			}
			os.Exit(1)
		}
	}

	// On Linux we inject the library directly. A wrong-architecture preload is
	// silently ignored by ld.so, so verify the match before running anything.
	if useProxychains && runtime.GOOS == "linux" && libPath != "" {
		if err := validatePreload(libPath, plan.Path); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: %v\033[0m\n", err)
			os.Exit(1)
		}
	}

	// Strict mode is the kernel-level fail-closed sandbox. It is on by default
	// on macOS; on other platforms --strict is an explicit error.
	if strictMode {
		if !strictSandboxSupported() {
			if strictExplicit {
				fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: --strict is not supported on this platform\033[0m\n")
				fmt.Fprintf(os.Stderr, "       On Linux, use mihomo TUN mode for full traffic coverage.\n")
				os.Exit(1)
			}
			strictMode = false
			fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Warning: strict sandbox unavailable; falling back to best-effort proxychains (UDP/direct traffic is not guaranteed to be blocked).\033[0m\n")
		}
	}
	if useProxychains && !strictMode {
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Warning: proxychains only proxies TCP+DNS; UDP/QUIC/ICMP and non-injectable binaries are not covered.\033[0m\n")
		fmt.Fprintf(os.Stderr, "\033[33m          Use --strict on macOS or mihomo TUN mode for fail-closed coverage.\033[0m\n")
	} else if !useProxychains && !strictMode {
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Warning: --mode env relies on the app honoring proxy environment variables; traffic may leak.\033[0m\n")
	}

	if useProxychains {
		if verboseMode {
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Mode: proxychains4\033[0m\n")
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Binary: %s\033[0m\n", proxychains)
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Proxy: socks5://%s:%d\033[0m\n", proxyHost, port)
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Command: %s\033[0m\n", plan.Argv[0])
			if strictMode {
				fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Sandbox: strict (block all remote network except the proxy)\033[0m\n")
			}
			fmt.Fprintf(os.Stderr, "\033[90m─────────────────────────────────────\033[0m\n")
		}
		exitCode, stderr, err := runWithProxychains(proxychains, libPath, proxyHost, port, plan.Argv, strictMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: proxychains failed - refusing to continue to prevent traffic leak\033[0m\n")
			if stderr != "" {
				fmt.Fprintf(os.Stderr, "       %s\n", firstLine(stderr))
			}
			fmt.Fprintf(os.Stderr, "       Fix proxychains or use --mode env at your own risk.\n")
			os.Exit(1)
		}
		os.Exit(exitCode)
	}

	// Environment variable proxy mode (--mode env only)
	// WARNING: programs that ignore HTTP_PROXY/ALL_PROXY will leak traffic.
	if verboseMode {
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Mode: environment variables (traffic may leak!)\033[0m\n")
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] HTTP_PROXY: http://%s:%d\033[0m\n", proxyHost, port)
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] ALL_PROXY: socks5://%s:%d\033[0m\n", proxyHost, port)
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Command: %s\033[0m\n", plan.Argv[0])
		if strictMode {
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Sandbox: strict (block all remote network except the proxy)\033[0m\n")
		}
		fmt.Fprintf(os.Stderr, "\033[90m─────────────────────────────────────\033[0m\n")
	}
	exitCode, _ := runWithEnvProxy(proxyHost, port, plan.Argv, strictMode)
	os.Exit(exitCode)
}

func printUsage() {
	fmt.Printf(`c4cgo - Run commands through clash4cli proxy (v%s)

Usage: c4cgo [options] command [args...]

Running c4cgo with no arguments shows all proxy slots and their status.

Options:
  -s, --slot N   Use proxy from slot N (0-9, default: first active slot)
  --port PORT    Specify proxy port (default: read from c4cd runtime)
  --mode MODE    Run mode: auto|proxychains|env (default: auto)
  --strict       Fail closed: block all remote network except the proxy
                 (default on macOS; unsupported on Linux — use TUN instead)
  --no-strict    Disable the fail-closed sandbox (UDP/direct traffic may leak)
  -v, --verbose  Show detailed status messages before running command
  --version      Show version
  -h, --help     Show this help

Examples:
  c4cgo                                              Show active proxy slots
  c4cgo curl https://www.google.com
  c4cgo -s 0 curl https://ip.sb
  c4cgo --port 17890 wget https://example.com/file.zip
  c4cgo --mode env curl https://ip.sb
  c4cgo --no-strict npm install
  c4cgo git clone https://github.com/user/repo.git
  c4cgo -v curl https://ip.sb                        Show proxy details

Safety:
  proxychains only intercepts TCP and DNS. Without --strict, UDP/QUIC/ICMP
  and statically linked/setuid binaries can bypass the proxy. On macOS,
  --strict uses a kernel sandbox to deny all remote network except the proxy,
  so leaks fail closed. On Linux, use mihomo TUN mode for full coverage.

  macOS hardened-runtime binaries (for example the native claude and codex
  builds) ignore DYLD injection. In auto mode c4cgo detects this and falls
  back to proxy environment variables; use --mode env to force that mode.

Note: Make sure c4cd is running and a slot is active.
`, Version)
}

// queryMihomoMode queries the mihomo API for the current proxy mode (rule/global/direct).
func queryMihomoMode(lf proxy.LockFile) string {
	host := lf.APIHost
	if host == "" {
		host = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s:%d/configs", host, lf.APIPort)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	if lf.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+lf.Secret)
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}
	return strings.ToLower(result.Mode)
}

// modeEmoji returns the emoji representation for a mihomo proxy mode.
func modeEmoji(mode string) string {
	switch mode {
	case "rule":
		return "📜"
	case "global":
		return "🌐"
	default:
		return "  "
	}
}

func listSlots() {
	snapshot, _, err := daemon.LoadRuntimeSnapshot(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: Cannot read daemon runtime: %v\033[0m\n", err)
		os.Exit(1)
	}
	if snapshot == nil || len(snapshot.Active) == 0 {
		fmt.Println("No active slots. Start c4c/c4cd and select a node first.")
		return
	}

	subNames := loadSubscriptionNames()
	ids := make([]int, 0, len(snapshot.Active))
	byID := make(map[int]*proxy.SlotState, len(snapshot.Active))
	for _, state := range snapshot.Active {
		if state == nil {
			continue
		}
		ids = append(ids, state.SlotID)
		byID[state.SlotID] = state
	}
	sort.Ints(ids)
	activeSet := make(map[int]bool, len(ids))
	for _, id := range ids {
		activeSet[id] = true
	}

	for i := 0; i < 10; i++ {
		if state, ok := byID[i]; ok && activeSet[i] {
			subName := subscriptionDisplayName(subNames, state.SubID)
			mode := strings.ToLower(strings.TrimSpace(state.Mode))
			emoji := modeEmoji(mode)
			fmt.Printf("  %s Slot %d  \033[32m●\033[0m running at %d, %s (%s)\n", emoji, i, state.Port, verboseStateSummary(state), subName)
		} else {
			fmt.Printf("     Slot %d  \033[90m○\033[0m empty\n", i)
		}
	}
}

func loadSubscriptionNames() map[string]string {
	subNames := make(map[string]string)
	mgr := subscription.NewManager()
	if err := mgr.Load(); err != nil {
		return subNames
	}
	for _, sub := range mgr.List() {
		if sub == nil || sub.ID == "" {
			continue
		}
		subNames[sub.ID] = sub.Name
	}
	return subNames
}

func subscriptionDisplayName(subNames map[string]string, subID string) string {
	if subID == "" {
		return "-"
	}
	if name := strings.TrimSpace(subNames[subID]); name != "" {
		return name
	}
	return subID
}

func verboseStateSummary(state *proxy.SlotState) string {
	if state == nil {
		return "node: -"
	}
	if state.UpstreamEnabled {
		return fmt.Sprintf("chain: %s -> %s", firstNonEmpty(state.NodeName, "-"), upstreamStateEndpoint(state))
	}
	return fmt.Sprintf("node: %s", firstNonEmpty(state.NodeName, "-"))
}

func upstreamStateEndpoint(state *proxy.SlotState) string {
	if state == nil {
		return "proxy://-"
	}
	protocol := strings.TrimSpace(state.UpstreamProtocol)
	if protocol == "" {
		protocol = "proxy"
	}
	host := strings.TrimSpace(state.UpstreamServer)
	if host == "" {
		host = "-"
	}
	if state.UpstreamPort > 0 {
		return fmt.Sprintf("%s://%s:%d", protocol, host, state.UpstreamPort)
	}
	return fmt.Sprintf("%s://%s", protocol, host)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func verifyProxy(host string, port int) bool {
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// findProxychains looks for proxychains binary and library
// Returns (binaryPath, libPath) or ("", "") if not found
func findProxychains() (string, string) {
	execPath, err := os.Executable()
	if err != nil {
		execPath = ""
	}
	execDir := filepath.Dir(execPath)

	cwd := ""
	if d, err := os.Getwd(); err == nil {
		cwd = d
	}

	env := proxychainsDiscoveryEnv{
		goos:      runtime.GOOS,
		goarch:    runtime.GOARCH,
		execDir:   execDir,
		cwd:       cwd,
		lookPath:  exec.LookPath,
		getenv:    os.Getenv,
		fileExist: fileExists,
		packagedDirs: []string{
			"/usr/lib/clash4cli",
			"/usr/lib64/clash4cli",
			"/usr/local/lib/clash4cli",
			"/opt/homebrew/lib/clash4cli",
		},
	}

	return findProxychainsWith(env)
}

type proxychainsDiscoveryEnv struct {
	goos         string
	goarch       string
	execDir      string
	cwd          string
	lookPath     func(string) (string, error)
	getenv       func(string) string
	fileExist    func(string) bool
	packagedDirs []string
}

func findProxychainsWith(env proxychainsDiscoveryEnv) (string, string) {
	if env.lookPath == nil {
		env.lookPath = exec.LookPath
	}
	if env.getenv == nil {
		env.getenv = os.Getenv
	}
	if env.fileExist == nil {
		env.fileExist = fileExists
	}

	// Determine library names based on OS.
	// proxychains-ng builds the library as libproxychains4.so even on macOS.
	libNames := []string{"libproxychains4.so"}
	if env.goos == "darwin" {
		libNames = []string{"libproxychains4.so", "libproxychains4.dylib"}
	}

	// 1. Check bundled proxychains (same directory as c4cgo)
	for _, libName := range libNames {
		if bundledBin, bundledLib := findBundledProxychains(env.execDir, libName, env.fileExist); bundledBin != "" {
			return bundledBin, bundledLib
		}
	}

	// 2. Check repo-style third_party layout (useful for dev builds)
	for _, libName := range libNames {
		if thirdPartyBin, thirdPartyLib := findThirdPartyProxychains(env.execDir, env.goos, env.goarch, libName, env.fileExist); thirdPartyBin != "" {
			return thirdPartyBin, thirdPartyLib
		}
	}
	if env.cwd != "" {
		for _, libName := range libNames {
			if thirdPartyBin, thirdPartyLib := findThirdPartyProxychains(env.cwd, env.goos, env.goarch, libName, env.fileExist); thirdPartyBin != "" {
				return thirdPartyBin, thirdPartyLib
			}
		}
	}

	// 3. Check packaged layout (deb/rpm/Homebrew-style)
	for _, libName := range libNames {
		if bin, lib := findPackagedProxychains(env.packagedDirs, libName, env.fileExist); bin != "" {
			return bin, lib
		}
	}

	// 4. Check system proxychains4
	if path, err := env.lookPath("proxychains4"); err == nil {
		// Try to find the library near the binary or in common locations
		libPath := ""
		for _, libName := range libNames {
			if p := findProxychainsLib(path, libName, env.getenv, env.fileExist); p != "" {
				libPath = p
				break
			}
		}
		return path, libPath
	}

	// 5. Check system proxychains
	if path, err := env.lookPath("proxychains"); err == nil {
		libPath := ""
		for _, libName := range libNames {
			if p := findProxychainsLib(path, libName, env.getenv, env.fileExist); p != "" {
				libPath = p
				break
			}
		}
		return path, libPath
	}

	return "", ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findBundledProxychains(execDir, libName string, exists func(string) bool) (string, string) {
	bundledBin := filepath.Join(execDir, "proxychains4")
	bundledLib := filepath.Join(execDir, libName)
	if exists(bundledBin) && exists(bundledLib) {
		return bundledBin, bundledLib
	}
	return "", ""
}

func findThirdPartyProxychains(baseDir, goos, goarch, libName string, exists func(string) bool) (string, string) {
	platformDir := fmt.Sprintf("%s-%s", goos, goarch)
	thirdPartyDir := filepath.Join(baseDir, "third_party", "proxychains", platformDir)
	thirdPartyBin := filepath.Join(thirdPartyDir, "proxychains4")
	thirdPartyLib := filepath.Join(thirdPartyDir, libName)
	if exists(thirdPartyBin) && exists(thirdPartyLib) {
		return thirdPartyBin, thirdPartyLib
	}
	return "", ""
}

func findPackagedProxychains(candidateDirs []string, libName string, exists func(string) bool) (string, string) {
	if len(candidateDirs) == 0 {
		return "", ""
	}
	for _, dir := range candidateDirs {
		bin := filepath.Join(dir, "proxychains4")
		lib := filepath.Join(dir, libName)
		if exists(bin) && exists(lib) {
			return bin, lib
		}
	}
	return "", ""
}

// findProxychainsLib searches for the proxychains library in common locations
func findProxychainsLib(binaryPath, libName string, getenv func(string) string, exists func(string) bool) string {
	binDir := filepath.Dir(binaryPath)

	// Common library locations relative to binary
	candidates := []string{
		// Same directory as binary
		filepath.Join(binDir, libName),
		// ../lib relative to binary (e.g., /usr/bin/../lib)
		filepath.Join(binDir, "..", "lib", libName),
		// ../lib64 for some distros
		filepath.Join(binDir, "..", "lib64", libName),
		// Standard system paths
		"/usr/lib/" + libName,
		"/usr/lib64/" + libName,
		"/usr/local/lib/" + libName,
		"/usr/local/lib64/" + libName,
	}

	// Also check LD_LIBRARY_PATH
	if ldPath := getenv("LD_LIBRARY_PATH"); ldPath != "" {
		for _, dir := range filepath.SplitList(ldPath) {
			candidates = append(candidates, filepath.Join(dir, libName))
		}
	}

	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate
		}
	}

	return ""
}

// firstLine returns the first non-empty line of s, or "" if s is empty.
func shouldFallbackToEnv(stderr string) bool {
	text := strings.ToLower(stderr)
	if !strings.Contains(text, "libproxychains4") {
		return false
	}
	return strings.Contains(text, "glibc_") ||
		strings.Contains(text, "cannot open shared object file") ||
		strings.Contains(text, "wrong elf class")
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

func runWithProxychains(proxychains, libPath string, host string, port int, targetArgs []string, strict bool) (int, string, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	// Write the config with a private, unpredictable name (0600). A predictable
	// 0644 file in /tmp could be pre-created as a symlink and clobbered.
	configFile, err := os.CreateTemp("", "c4cgo-proxychains-*.conf")
	if err != nil {
		return 1, "", fmt.Errorf("failed to create proxychains config: %w", err)
	}
	configPath := configFile.Name()
	defer os.Remove(configPath)

	configContent := fmt.Sprintf(`# Generated by c4cgo
quiet_mode
strict_chain
proxy_dns
remote_dns_subnet 224
tcp_read_time_out 15000
tcp_connect_time_out 8000

[ProxyList]
socks5 %s %d
`, host, port)
	if _, err := configFile.WriteString(configContent); err != nil {
		configFile.Close()
		return 1, "", fmt.Errorf("failed to write proxychains config: %w", err)
	}
	if err := configFile.Close(); err != nil {
		return 1, "", fmt.Errorf("failed to write proxychains config: %w", err)
	}

	// Drop inherited loader/proxy variables so the child cannot shadow the
	// injection we set here (getenv returns the first duplicate).
	oldLibPath := os.Getenv("LD_LIBRARY_PATH")
	env := sanitizeProxyEnv(sanitizeLoaderEnv(os.Environ()))

	var argv []string
	if runtime.GOOS == "darwin" || libPath == "" {
		// Use the proxychains4 wrapper; it sets the correct DYLD/LD variables.
		argv = append([]string{proxychains, "-q", "-f", configPath}, targetArgs...)
	} else {
		// Skip the wrapper and inject the library directly. This avoids
		// proxychains4 re-discovering the library with its own logic.
		argv = append([]string{targetArgs[0]}, targetArgs[1:]...)
		libDir := filepath.Dir(libPath)
		libPathValue := libDir
		if oldLibPath != "" {
			libPathValue = libDir + string(os.PathListSeparator) + oldLibPath
		}
		env = append(env,
			"PROXYCHAINS_CONF_FILE="+configPath,
			"LD_PRELOAD="+libPath,
			"LD_LIBRARY_PATH="+libPathValue,
		)
	}

	if strict {
		wrapped, err := strictSandboxCommand(argv, port)
		if err != nil {
			return 1, "", err
		}
		argv = wrapped
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(newProxychainsBannerFilter(os.Stderr), &stderrBuf)

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// The target ran and returned a non-zero status; that is not a
			// proxychains failure.
			return exitErr.ExitCode(), stderrBuf.String(), nil
		}
		return 1, stderrBuf.String(), err
	}
	return 0, stderrBuf.String(), nil
}

func runWithEnvProxy(host string, port int, targetArgs []string, strict bool) (int, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	httpProxy := "http://" + hostPort
	socksProxy := "socks5://" + hostPort

	// Remove inherited proxy variables first; otherwise the child may see the
	// parent's (possibly stale or direct) proxy settings instead of ours.
	env := sanitizeProxyEnv(os.Environ())
	env = append(env,
		"HTTP_PROXY="+httpProxy,
		"HTTPS_PROXY="+httpProxy,
		"http_proxy="+httpProxy,
		"https_proxy="+httpProxy,
		"ALL_PROXY="+socksProxy,
		"all_proxy="+socksProxy,
		"NO_PROXY=localhost,127.0.0.1,::1",
		"no_proxy=localhost,127.0.0.1,::1",
	)

	argv := append([]string(nil), targetArgs...)
	if strict {
		wrapped, err := strictSandboxCommand(argv, port)
		if err != nil {
			return 1, err
		}
		argv = wrapped
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// isSIPProtectedDarwinPath returns true if the executable lives in a SIP-protected system directory.
// DYLD_* injection (used by proxychains) is ignored for these binaries on a typical macOS setup.
func isSIPProtectedDarwinPath(p string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return isSIPProtectedPath(p)
}

func isSIPProtectedPath(p string) bool {
	clean := filepath.Clean(p)
	protectedPrefixes := []string{
		"/usr/bin/",
		"/bin/",
		"/usr/sbin/",
		"/sbin/",
		"/System/",
	}
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

// proxychainsBannerFilter wraps an io.Writer and suppresses the
// "[proxychains] DLL init: proxychains-ng ..." banner line that
// the shared library prints on startup (before quiet_mode takes effect).
type proxychainsBannerFilter struct {
	inner io.Writer
}

func newProxychainsBannerFilter(w io.Writer) *proxychainsBannerFilter {
	return &proxychainsBannerFilter{inner: w}
}

func (f *proxychainsBannerFilter) Write(p []byte) (int, error) {
	n := len(p)
	// Fast path: if the chunk doesn't contain the banner prefix, pass through.
	if !bytes.Contains(p, []byte("[proxychains] DLL init")) {
		f.inner.Write(p)
		return n, nil
	}
	// Filter out lines containing the banner.
	var filtered []byte
	for _, line := range bytes.SplitAfter(p, []byte("\n")) {
		if bytes.Contains(line, []byte("[proxychains] DLL init")) {
			continue
		}
		filtered = append(filtered, line...)
	}
	if len(filtered) > 0 {
		f.inner.Write(filtered)
	}
	return n, nil
}
