package main

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"rocket4cli/internal/proxy"
)

// maxShebangDepth bounds recursive shebang resolution (script -> interpreter -> ...).
const maxShebangDepth = 4

// runTarget describes the executable that will actually be launched.
type runTarget struct {
	// Path is the resolved executable that receives the dynamic-linker injection.
	Path string
	// Argv is what should be passed to exec (may be rewritten for #!/usr/bin/env scripts).
	Argv []string
	// Script is true when the target is a script (executed via its interpreter).
	Script bool
}

// resolveRunTarget resolves args[0], follows shebangs, and returns the command to run.
// It rewrites "#!/usr/bin/env <program>" scripts to invoke <program> directly:
// on macOS /usr/bin/env is SIP-protected and strips DYLD_INSERT_LIBRARIES, which
// would silently disable proxychains for the whole child tree.
func resolveRunTarget(args []string, validate bool) (*runTarget, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return resolveRunTargetDepth(args, 0, validate)
}

func resolveRunTargetDepth(args []string, depth int, validate bool) (*runTarget, error) {
	if depth > maxShebangDepth {
		return nil, fmt.Errorf("too many nested interpreters while resolving %q", args[0])
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		return nil, fmt.Errorf("cannot find command %q: %w", args[0], err)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		realPath = path
	}

	interp, interpArgs, ok, err := readShebang(realPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		if validate {
			if err := validateInterceptable(realPath); err != nil {
				return nil, err
			}
		}
		return &runTarget{Path: realPath, Argv: append([]string{path}, args[1:]...)}, nil
	}

	if isEnvInterpreter(interp) {
		program, programArgs, err := parseEnvShebang(interpArgs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", realPath, err)
		}
		programPath, err := exec.LookPath(program)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot find interpreter %q in PATH: %w", realPath, program, err)
		}
		programReal, err := filepath.EvalSymlinks(programPath)
		if err != nil {
			programReal = programPath
		}
		if validate {
			if err := validateInterceptable(programReal); err != nil {
				return nil, err
			}
		}
		// The kernel would hand the script to /usr/bin/env, which strips DYLD
		// injection on macOS. Run the interpreter ourselves with the script as argv[1].
		argv := make([]string, 0, len(programArgs)+2+len(args)-1)
		argv = append(argv, programPath)
		argv = append(argv, programArgs...)
		argv = append(argv, realPath)
		argv = append(argv, args[1:]...)
		return &runTarget{Path: programReal, Argv: argv, Script: true}, nil
	}

	interpPath := interp
	if !filepath.IsAbs(interpPath) {
		p, err := exec.LookPath(interpPath)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot find interpreter %q: %w", realPath, interp, err)
		}
		interpPath = p
	}
	interpReal, err := filepath.EvalSymlinks(interpPath)
	if err != nil {
		interpReal = interpPath
	}
	if validate {
		if err := validateInterceptable(interpReal); err != nil {
			return nil, err
		}
	}
	return &runTarget{Path: realPath, Argv: append([]string{path}, args[1:]...), Script: true}, nil
}

// readShebang returns the interpreter and its arguments for a script.
// ok is false when the file is not a script or cannot be read.
func readShebang(path string) (interp string, interpArgs []string, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, false, nil
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, readErr := f.Read(buf)
	if readErr != nil && readErr != io.EOF {
		return "", nil, false, nil
	}
	if n < 2 || buf[0] != '#' || buf[1] != '!' {
		return "", nil, false, nil
	}
	line := string(buf[2:n])
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimRight(line, "\x00\r")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil, false, nil
	}
	return fields[0], fields[1:], true, nil
}

func isEnvInterpreter(interp string) bool {
	base := filepath.Base(interp)
	return base == "env"
}

// parseEnvShebang extracts the program and its arguments from `env` arguments.
// It supports the common forms: `env prog`, `env -S prog args...`,
// `env --split-string="prog args..."`, and ignores env options/assignments.
func parseEnvShebang(envArgs []string) (program string, programArgs []string, err error) {
	for i := 0; i < len(envArgs); i++ {
		a := envArgs[i]
		switch {
		case a == "-S" || a == "--split-string":
			if i+1 >= len(envArgs) {
				return "", nil, fmt.Errorf("env -S requires a program")
			}
			parts := strings.Fields(envArgs[i+1])
			if len(parts) == 0 {
				return "", nil, fmt.Errorf("env -S requires a program")
			}
			return parts[0], append(parts[1:], envArgs[i+2:]...), nil
		case strings.HasPrefix(a, "--split-string="):
			parts := strings.Fields(strings.TrimPrefix(a, "--split-string="))
			if len(parts) == 0 {
				return "", nil, fmt.Errorf("env --split-string requires a program")
			}
			return parts[0], append(parts[1:], envArgs[i+1:]...), nil
		case a == "-i" || a == "--ignore-environment":
			continue
		case a == "-u" || a == "--unset":
			i++ // skip the variable name
		case strings.HasPrefix(a, "--unset="):
			continue
		case strings.HasPrefix(a, "-"):
			continue // unknown env option: ignore
		case strings.Contains(a, "="):
			continue // NAME=VALUE assignment
		default:
			return a, envArgs[i+1:], nil
		}
	}
	return "", nil, fmt.Errorf("env shebang does not name a program")
}

// validateInterceptable rejects executables that would silently bypass the proxy.
func validateInterceptable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSetuid != 0 || info.Mode()&os.ModeSetgid != 0 {
		return fmt.Errorf("%s is setuid/setgid; the system ignores LD_PRELOAD/DYLD injection for such binaries, so its traffic would NOT be proxied", path)
	}
	if runtime.GOOS == "darwin" && isSIPProtectedDarwinPath(path) {
		return fmt.Errorf("%s is a macOS SIP-protected system binary; DYLD injection is stripped, so its traffic would NOT be proxied", path)
	}
	if runtime.GOOS == "linux" {
		static, err := isStaticELF(path)
		if err == nil && static {
			return fmt.Errorf("%s is statically linked; LD_PRELOAD cannot intercept it, so its traffic would NOT be proxied", path)
		}
		if isGoELF(path) {
			return fmt.Errorf("%s is a Go binary; Go uses raw syscalls for TCP on Linux, bypassing LD_PRELOAD, so its traffic would NOT be proxied (use TUN mode instead)", path)
		}
	}
	return nil
}

// isGoELF reports whether path is a Go-produced ELF binary. Go's net package
// uses raw syscalls on Linux, so LD_PRELOAD hooks never see its connections.
func isGoELF(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return f.Section(".note.go.buildid") != nil || f.Section(".gopclntab") != nil
}

// validateSlotEgress refuses slots whose mihomo core would send traffic out
// directly (no node/upstream selected, or the core itself is in direct mode).
// The per-process sandbox cannot see this: mihomo runs outside it.
func validateSlotEgress(state *proxy.SlotState) error {
	if state == nil {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(state.Mode), "direct") {
		return fmt.Errorf("slot %d is in DIRECT mode; its traffic would not use a proxy", state.SlotID)
	}
	if !state.UpstreamEnabled && strings.TrimSpace(state.NodeName) == "" {
		return fmt.Errorf("slot %d has no node selected (DIRECT); its traffic would leave the machine directly", state.SlotID)
	}
	return nil
}

// validatePreload checks that the proxychains shared library matches the target
// ELF class/architecture. Mismatched preloads are silently ignored by ld.so
// ("cannot be preloaded ... ignored"), which would let the target run unproxied.
func validatePreload(libPath, targetPath string) error {
	lib, err := elf.Open(libPath)
	if err != nil {
		return fmt.Errorf("proxychains library %s is not a valid ELF: %w", libPath, err)
	}
	defer lib.Close()

	target, err := elf.Open(targetPath)
	if err != nil {
		// Scripts/non-ELF targets are validated through their interpreter.
		return nil
	}
	defer target.Close()

	if lib.Class != target.Class || lib.Machine != target.Machine {
		return fmt.Errorf("proxychains library %s (%s/%s) does not match target %s (%s/%s); the loader would ignore LD_PRELOAD and traffic would NOT be proxied",
			libPath, lib.Class, lib.Machine, targetPath, target.Class, target.Machine)
	}
	return nil
}

// isStaticELF reports whether path is an ELF executable without a dynamic
// interpreter (PT_INTERP). Non-ELF files return false.
func isStaticELF(path string) (bool, error) {
	f, err := elf.Open(path)
	if err != nil {
		return false, nil
	}
	defer f.Close()
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			return false, nil
		}
	}
	return true, nil
}
