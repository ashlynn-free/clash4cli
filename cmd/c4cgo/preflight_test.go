package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rocket4cli/internal/proxy"
)

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestReadShebang(t *testing.T) {
	dir := t.TempDir()

	script := filepath.Join(dir, "tool")
	writeExecutable(t, script, "#!/usr/bin/env node\nconsole.log(1)\n")
	interp, args, ok, err := readShebang(script)
	if err != nil {
		t.Fatalf("readShebang: %v", err)
	}
	if !ok || interp != "/usr/bin/env" || len(args) != 1 || args[0] != "node" {
		t.Fatalf("unexpected shebang: ok=%v interp=%q args=%v", ok, interp, args)
	}

	withArg := filepath.Join(dir, "shell-script")
	writeExecutable(t, withArg, "#!/bin/sh -e\n")
	interp, args, ok, err = readShebang(withArg)
	if err != nil || !ok || interp != "/bin/sh" || len(args) != 1 || args[0] != "-e" {
		t.Fatalf("unexpected shebang with arg: ok=%v interp=%q args=%v err=%v", ok, interp, args, err)
	}

	binary := filepath.Join(dir, "binary")
	writeExecutable(t, binary, "\x7fELFnot-a-script")
	if _, _, ok, _ := readShebang(binary); ok {
		t.Fatalf("expected non-script to have no shebang")
	}
}

func TestParseEnvShebang(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantProg  string
		wantArgs  []string
		wantError bool
	}{
		{name: "plain", args: []string{"node"}, wantProg: "node"},
		{name: "split string", args: []string{"-S", "node", "--inspect"}, wantProg: "node", wantArgs: []string{"--inspect"}},
		{name: "split string equals", args: []string{"--split-string=node --inspect"}, wantProg: "node", wantArgs: []string{"--inspect"}},
		{name: "ignore environment", args: []string{"-i", "node"}, wantProg: "node"},
		{name: "assignment", args: []string{"FOO=bar", "node"}, wantProg: "node"},
		{name: "unset", args: []string{"-u", "FOO", "node"}, wantProg: "node"},
		{name: "empty", args: nil, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, args, err := parseEnvShebang(tc.args)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseEnvShebang: %v", err)
			}
			if prog != tc.wantProg {
				t.Fatalf("program=%q, want %q", prog, tc.wantProg)
			}
			if strings.Join(args, " ") != strings.Join(tc.wantArgs, " ") {
				t.Fatalf("args=%v, want %v", args, tc.wantArgs)
			}
		})
	}
}

func TestResolveRunTargetRewritesEnvShebang(t *testing.T) {
	binDir := t.TempDir()
	fakeNode := filepath.Join(binDir, "fakenode")
	writeExecutable(t, fakeNode, "#!/bin/sh\necho node\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	scriptDir := t.TempDir()
	script := filepath.Join(scriptDir, "mytool")
	writeExecutable(t, script, "#!/usr/bin/env fakenode\n")

	plan, err := resolveRunTarget([]string{script, "--version"}, false)
	if err != nil {
		t.Fatalf("resolveRunTarget: %v", err)
	}
	wantNode, err := filepath.EvalSymlinks(fakeNode)
	if err != nil {
		wantNode = fakeNode
	}
	wantScript, err := filepath.EvalSymlinks(script)
	if err != nil {
		wantScript = script
	}
	if plan.Argv[0] != fakeNode && plan.Argv[0] != wantNode {
		t.Fatalf("argv[0]=%q, want %q", plan.Argv[0], fakeNode)
	}
	if len(plan.Argv) != 3 || plan.Argv[1] != wantScript || plan.Argv[2] != "--version" {
		t.Fatalf("unexpected argv: %v", plan.Argv)
	}
	if !plan.Script {
		t.Fatalf("expected Script=true")
	}
}

func TestResolveRunTargetKeepsDirectInterpreter(t *testing.T) {
	binDir := t.TempDir()
	interp := filepath.Join(binDir, "fakeinterp")
	writeExecutable(t, interp, "#!/bin/sh\necho interp\n")

	script := filepath.Join(binDir, "direct")
	writeExecutable(t, script, "#!"+interp+"\n")

	plan, err := resolveRunTarget([]string{script, "arg1"}, false)
	if err != nil {
		t.Fatalf("resolveRunTarget: %v", err)
	}
	if len(plan.Argv) != 2 || plan.Argv[0] != script || plan.Argv[1] != "arg1" {
		t.Fatalf("unexpected argv: %v", plan.Argv)
	}
	if !plan.Script {
		t.Fatalf("expected Script=true")
	}
}

func TestIsSIPProtectedPath(t *testing.T) {
	protected := []string{"/usr/bin/curl", "/bin/sh", "/usr/sbin/scutil", "/sbin/ping", "/System/Library/x"}
	for _, p := range protected {
		if !isSIPProtectedPath(p) {
			t.Fatalf("expected %s to be SIP-protected", p)
		}
	}
	notProtected := []string{"/opt/homebrew/bin/curl", "/usr/local/bin/node", "/tmp/tool"}
	for _, p := range notProtected {
		if isSIPProtectedPath(p) {
			t.Fatalf("expected %s not to be SIP-protected", p)
		}
	}
}

func TestValidateSlotEgress(t *testing.T) {
	cases := []struct {
		name    string
		state   *proxy.SlotState
		wantErr bool
	}{
		{name: "nil state", state: nil},
		{name: "node selected", state: &proxy.SlotState{SlotID: 0, NodeName: "HK-01", Mode: "rule"}},
		{name: "upstream only", state: &proxy.SlotState{SlotID: 0, UpstreamEnabled: true, Mode: "rule"}},
		{name: "no node", state: &proxy.SlotState{SlotID: 0, Mode: "rule"}, wantErr: true},
		{name: "direct mode", state: &proxy.SlotState{SlotID: 0, NodeName: "HK-01", Mode: "direct"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSlotEgress(tc.state)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateInterceptableRejectsSetuid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("setuid is not available on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "setuid-tool")
	writeExecutable(t, path, "#!/bin/sh\n")
	if err := os.Chmod(path, 0o755|os.ModeSetuid); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := validateInterceptable(path); err == nil {
		t.Fatalf("expected setuid binary to be rejected")
	}
}

func writeMinimalELF(t *testing.T, path string, programHeaders []elfProgramHeader) {
	t.Helper()
	writeMinimalELFArch(t, path, 2, 62, programHeaders)
}

func writeMinimalELFArch(t *testing.T, path string, class byte, machine uint16, programHeaders []elfProgramHeader) {
	t.Helper()
	const ehSize = 64
	const phSize = 56
	buf := make([]byte, ehSize+len(programHeaders)*phSize)
	copy(buf[:4], []byte("\x7fELF"))
	buf[4] = class // ELFCLASS64 / ELFCLASS32
	buf[5] = 1     // ELFDATA2LSB
	buf[6] = 1     // EV_CURRENT
	buf[16] = 2    // ET_EXEC
	binary.LittleEndian.PutUint16(buf[18:], machine)
	binary.LittleEndian.PutUint32(buf[20:], 1) // EV_CURRENT
	binary.LittleEndian.PutUint64(buf[32:], ehSize)
	binary.LittleEndian.PutUint16(buf[52:], ehSize)
	binary.LittleEndian.PutUint16(buf[54:], phSize)
	binary.LittleEndian.PutUint16(buf[56:], uint16(len(programHeaders)))
	for i, ph := range programHeaders {
		off := ehSize + i*phSize
		binary.LittleEndian.PutUint32(buf[off:], ph.typ)
		binary.LittleEndian.PutUint64(buf[off+8:], ph.offset)
		binary.LittleEndian.PutUint64(buf[off+32:], ph.fileSize)
	}
	if err := os.WriteFile(path, buf, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

type elfProgramHeader struct {
	typ      uint32
	offset   uint64
	fileSize uint64
}

func TestValidatePreload(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "libproxychains4.so")
	writeMinimalELFArch(t, lib, 2, 62, nil)

	matching := filepath.Join(dir, "matching")
	writeMinimalELFArch(t, matching, 2, 62, nil)
	if err := validatePreload(lib, matching); err != nil {
		t.Fatalf("matching arch should pass: %v", err)
	}

	mismatched := filepath.Join(dir, "arm64")
	writeMinimalELFArch(t, mismatched, 2, 183, nil)
	if err := validatePreload(lib, mismatched); err == nil {
		t.Fatalf("expected architecture mismatch to be rejected")
	}

	script := filepath.Join(dir, "script")
	writeExecutable(t, script, "#!/bin/sh\n")
	if err := validatePreload(lib, script); err != nil {
		t.Fatalf("non-ELF target should skip the check: %v", err)
	}
}

func TestIsGoELF(t *testing.T) {
	fixture := filepath.Join("..", "..", "third_party", "mihomo", "mihomo-linux-amd64")
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("bundled linux mihomo binary not available")
	}
	if !isGoELF(fixture) {
		t.Fatalf("expected %s to be detected as a Go binary", fixture)
	}

	plain := filepath.Join(t.TempDir(), "plain")
	writeMinimalELF(t, plain, nil)
	if isGoELF(plain) {
		t.Fatalf("expected minimal ELF not to be detected as Go")
	}
}

func TestIsStaticELF(t *testing.T) {
	dir := t.TempDir()

	static := filepath.Join(dir, "static")
	writeMinimalELF(t, static, nil)
	got, err := isStaticELF(static)
	if err != nil || !got {
		t.Fatalf("expected static ELF, got %v err=%v", got, err)
	}

	dynamic := filepath.Join(dir, "dynamic")
	writeMinimalELF(t, dynamic, []elfProgramHeader{{typ: 3 /* PT_INTERP */}})
	got, err = isStaticELF(dynamic)
	if err != nil || got {
		t.Fatalf("expected dynamic ELF, got %v err=%v", got, err)
	}

	if got, err := isStaticELF(filepath.Join(dir, "missing")); err != nil || got {
		t.Fatalf("expected non-ELF to be false, got %v err=%v", got, err)
	}
}
