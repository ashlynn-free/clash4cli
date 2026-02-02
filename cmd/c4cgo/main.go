package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"rocket4cli/internal/proxy"
)

// Version information (set via ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var (
	quietMode     = false
	specifiedPort = 0
	runMode       = "auto" // auto|proxychains|env
)

func main() {
	args := os.Args[1:]

	// Handle flags
	for len(args) > 0 {
		switch args[0] {
		case "-v", "--version":
			fmt.Printf("c4cgo %s\n", Version)
			fmt.Printf("Build time: %s\n", BuildTime)
			return
		case "-h", "--help":
			printUsage()
			return
		case "-q", "--quiet":
			quietMode = true
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
		default:
			goto runCommand
		}
	}

runCommand:
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	port := specifiedPort
	proxyHost := "127.0.0.1"

	// If no port specified, read from lock file
	if port == 0 {
		lockFile, err := proxy.ReadLockFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m[c4cgo] Error: Cannot read proxy.lock file\033[0m\n")
			fmt.Fprintf(os.Stderr, "        c4c proxy is not running or lock file is missing.\n")
			fmt.Fprintf(os.Stderr, "        Please start c4c and select a node first.\n")
			os.Exit(1)
		}
		port = lockFile.Port
		if lockFile.ProxyHost != "" {
			proxyHost = lockFile.ProxyHost
		}

		if !quietMode {
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Using proxy at %s:%d (node: %s)\033[0m\n", proxyHost, port, lockFile.Node)
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
		useProxychains = proxychains != ""
		// On macOS, DYLD injection is blocked for SIP-protected system binaries. Fall back to env proxy.
		if useProxychains && runtime.GOOS == "darwin" {
			if cmdPath, err := exec.LookPath(args[0]); err == nil && isSIPProtectedDarwinPath(cmdPath) {
				useProxychains = false
				if !quietMode {
					fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] macOS SIP may block proxychains for system binaries: %s\033[0m\n", cmdPath)
					fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Falling back to: environment variables (HTTP_PROXY/ALL_PROXY)\033[0m\n")
					fmt.Fprintf(os.Stderr, "\033[90m       Tip: install a non-system build of this command (e.g. Homebrew) or run with --mode proxychains.\033[0m\n")
				}
			}
		}
	}

	if useProxychains {
		if !quietMode {
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Mode: proxychains4\033[0m\n")
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Proxy: socks5://%s:%d\033[0m\n", proxyHost, port)
			fmt.Fprintf(os.Stderr, "\033[36m[c4cgo] Command: %s\033[0m\n", args[0])
			fmt.Fprintf(os.Stderr, "\033[90m─────────────────────────────────────\033[0m\n")
		}
		exitCode, stderr, err := runWithProxychains(proxychains, libPath, proxyHost, port, args)
		if err != nil && runMode == "auto" && shouldFallbackToEnv(stderr) {
			if !quietMode {
				fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] proxychains failed on this system; falling back to environment variables\033[0m\n")
				fmt.Fprintf(os.Stderr, "\033[90m       Tip: rebuild proxychains on the target machine, or run with --mode env\033[0m\n")
				fmt.Fprintf(os.Stderr, "\033[90m─────────────────────────────────────\033[0m\n")
			}
			exitCode, err = runWithEnvProxy(proxyHost, port, args)
		}
		os.Exit(exitCode)
	}

	// Fallback to environment variables
	if !quietMode {
		switch runMode {
		case "env":
			fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Mode: environment variables\033[0m\n")
		case "auto":
			if proxychains == "" {
				fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Mode: environment variables (proxychains not found)\033[0m\n")
			} else {
				fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Mode: environment variables (proxychains disabled)\033[0m\n")
			}
		default:
			fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Mode: environment variables\033[0m\n")
		}
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] HTTP_PROXY: http://%s:%d\033[0m\n", proxyHost, port)
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] ALL_PROXY: socks5://%s:%d\033[0m\n", proxyHost, port)
		fmt.Fprintf(os.Stderr, "\033[33m[c4cgo] Command: %s\033[0m\n", args[0])
		fmt.Fprintf(os.Stderr, "\033[90m─────────────────────────────────────\033[0m\n")
	}
	exitCode, _ := runWithEnvProxy(proxyHost, port, args)
	os.Exit(exitCode)
}

func printUsage() {
	fmt.Printf(`c4cgo - Run commands through clash4cli proxy (v%s)

Usage: c4cgo [options] command [args...]

Options:
  --port PORT    Specify proxy port (default: read from ~/.clash4cli/proxy.lock)
  --mode MODE    Run mode: auto|proxychains|env (default: auto)
  -q, --quiet    Suppress status messages (only show command output)
  -v, --version  Show version
  -h, --help     Show this help

Examples:
  c4cgo curl https://www.google.com
  c4cgo --port 17890 wget https://example.com/file.zip
  c4cgo --mode env curl https://ip.sb
  c4cgo git clone https://github.com/user/repo.git
  c4cgo -q curl -s https://ip.sb > myip.txt

Note: Make sure c4c is running and a node is selected.
`, Version)
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

func shouldFallbackToEnv(stderr string) bool {
	if stderr == "" {
		return false
	}
	lower := strings.ToLower(stderr)
	if !strings.Contains(lower, "libproxychains4") {
		return false
	}

	// Most common: glibc version mismatch when the library is built on a newer distro.
	if strings.Contains(lower, "glibc_") && strings.Contains(lower, "not found") {
		return true
	}

	// Other dynamic loader failures where env proxy is still a better-than-nothing fallback.
	if strings.Contains(lower, "error while loading shared libraries") {
		return true
	}
	if strings.Contains(lower, "wrong elf class") {
		return true
	}

	return false
}

func runWithProxychains(proxychains, libPath string, host string, port int, args []string) (int, string, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	// Create proxychains config
	configContent := fmt.Sprintf(`# Generated by c4cgo
strict_chain
proxy_dns
remote_dns_subnet 224
tcp_read_time_out 15000
tcp_connect_time_out 8000

[ProxyList]
socks5 %s %d
`, host, port)

	// Write config to temp file
	tmpDir := os.TempDir()
	configPath := filepath.Join(tmpDir, fmt.Sprintf("c4cgo_proxychains_%d.conf", os.Getpid()))
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return 1, "", fmt.Errorf("failed to write proxychains config: %w", err)
	}
	defer os.Remove(configPath)

	var cmd *exec.Cmd
	env := os.Environ()

	// On macOS, prefer running proxychains4 wrapper to ensure correct DYLD settings.
	// (Direct DYLD injection is fragile and can be blocked by SIP for system binaries.)
	if runtime.GOOS == "darwin" {
		cmdArgs := append([]string{"-q", "-f", configPath}, args...)
		cmd = exec.Command(proxychains, cmdArgs...)
	} else if libPath != "" {
		// If we have the library path, skip proxychains4 binary and use LD_PRELOAD directly
		// This avoids proxychains4 trying to find the library again with its own logic
		// Run target command directly with LD_PRELOAD
		cmd = exec.Command(args[0], args[1:]...)
		env = append(env, "PROXYCHAINS_CONF_FILE="+configPath)
		libDir := filepath.Dir(libPath)
		if runtime.GOOS == "darwin" {
			env = append(env, "DYLD_INSERT_LIBRARIES="+libPath)
			env = append(env, "DYLD_LIBRARY_PATH="+libDir)
		} else {
			env = append(env, "LD_PRELOAD="+libPath)
			env = append(env, "LD_LIBRARY_PATH="+libDir)
		}
	} else {
		// No library path, use proxychains4 binary normally (it will handle LD_PRELOAD)
		cmdArgs := append([]string{"-q", "-f", configPath}, args...)
		cmd = exec.Command(proxychains, cmdArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	cmd.Env = env

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), stderrBuf.String(), err
		}
		return 1, stderrBuf.String(), err
	}
	return 0, stderrBuf.String(), nil
}

func runWithEnvProxy(host string, port int, args []string) (int, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	httpProxy := "http://" + hostPort
	socksProxy := "socks5://" + hostPort

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(),
		"HTTP_PROXY="+httpProxy,
		"HTTPS_PROXY="+httpProxy,
		"http_proxy="+httpProxy,
		"https_proxy="+httpProxy,
		"ALL_PROXY="+socksProxy,
		"all_proxy="+socksProxy,
		"NO_PROXY=localhost,127.0.0.1,::1",
		"no_proxy=localhost,127.0.0.1,::1",
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
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
