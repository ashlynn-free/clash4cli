package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func touchFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestFindProxychainsWith_BundledPreferred(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "proxychains4")
	lib := filepath.Join(dir, "libproxychains4.so")
	touchFile(t, bin)
	touchFile(t, lib)

	gotBin, gotLib := findProxychainsWith(proxychainsDiscoveryEnv{
		goos:         "linux",
		goarch:       "amd64",
		execDir:      dir,
		cwd:          "",
		lookPath:     func(string) (string, error) { return "", exec.ErrNotFound },
		getenv:       os.Getenv,
		fileExist:    fileExists,
		packagedDirs: nil,
	})
	if gotBin != bin || gotLib != lib {
		t.Fatalf("expected bundled (%s,%s), got (%s,%s)", bin, lib, gotBin, gotLib)
	}
}

func TestFindProxychainsWith_ThirdPartyPreferredOverSystem(t *testing.T) {
	dir := t.TempDir()
	platformDir := filepath.Join(dir, "third_party", "proxychains", "linux-amd64")
	bin := filepath.Join(platformDir, "proxychains4")
	lib := filepath.Join(platformDir, "libproxychains4.so")
	touchFile(t, bin)
	touchFile(t, lib)

	gotBin, gotLib := findProxychainsWith(proxychainsDiscoveryEnv{
		goos:      "linux",
		goarch:    "amd64",
		execDir:   dir,
		cwd:       "",
		lookPath:  func(string) (string, error) { return "", exec.ErrNotFound },
		getenv:    os.Getenv,
		fileExist: fileExists,
	})
	if gotBin != bin || gotLib != lib {
		t.Fatalf("expected third_party (%s,%s), got (%s,%s)", bin, lib, gotBin, gotLib)
	}
}

func TestFindProxychainsWith_CwdThirdPartyFallback(t *testing.T) {
	execDir := t.TempDir()
	cwd := t.TempDir()

	platformDir := filepath.Join(cwd, "third_party", "proxychains", "linux-amd64")
	bin := filepath.Join(platformDir, "proxychains4")
	lib := filepath.Join(platformDir, "libproxychains4.so")
	touchFile(t, bin)
	touchFile(t, lib)

	gotBin, gotLib := findProxychainsWith(proxychainsDiscoveryEnv{
		goos:      "linux",
		goarch:    "amd64",
		execDir:   execDir,
		cwd:       cwd,
		lookPath:  func(string) (string, error) { return "", exec.ErrNotFound },
		getenv:    os.Getenv,
		fileExist: fileExists,
	})
	if gotBin != bin || gotLib != lib {
		t.Fatalf("expected cwd third_party (%s,%s), got (%s,%s)", bin, lib, gotBin, gotLib)
	}
}

func TestFindProxychainsWith_PackagedFallback(t *testing.T) {
	execDir := t.TempDir()
	pkg := t.TempDir()

	bin := filepath.Join(pkg, "proxychains4")
	lib := filepath.Join(pkg, "libproxychains4.so")
	touchFile(t, bin)
	touchFile(t, lib)

	gotBin, gotLib := findProxychainsWith(proxychainsDiscoveryEnv{
		goos:         "linux",
		goarch:       "amd64",
		execDir:      execDir,
		cwd:          "",
		lookPath:     func(string) (string, error) { return "", exec.ErrNotFound },
		getenv:       os.Getenv,
		fileExist:    fileExists,
		packagedDirs: []string{pkg},
	})
	if gotBin != bin || gotLib != lib {
		t.Fatalf("expected packaged (%s,%s), got (%s,%s)", bin, lib, gotBin, gotLib)
	}
}

func TestFindProxychainsWith_SystemLookPathAndNeighborLib(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	libDir := filepath.Join(root, "lib")
	bin := filepath.Join(binDir, "proxychains4")
	lib := filepath.Join(libDir, "libproxychains4.so")
	touchFile(t, bin)
	touchFile(t, lib)

	gotBin, gotLib := findProxychainsWith(proxychainsDiscoveryEnv{
		goos:      "linux",
		goarch:    "amd64",
		execDir:   t.TempDir(),
		cwd:       "",
		lookPath:  func(name string) (string, error) { return bin, nil },
		getenv:    os.Getenv,
		fileExist: fileExists,
	})
	if gotBin != bin || gotLib != lib {
		t.Fatalf("expected system (%s,%s), got (%s,%s)", bin, lib, gotBin, gotLib)
	}
}

func TestFindProxychainsWith_DarwinAcceptsDylib(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "proxychains4")
	lib := filepath.Join(dir, "libproxychains4.dylib")
	touchFile(t, bin)
	touchFile(t, lib)

	gotBin, gotLib := findProxychainsWith(proxychainsDiscoveryEnv{
		goos:      "darwin",
		goarch:    "arm64",
		execDir:   dir,
		cwd:       "",
		lookPath:  func(string) (string, error) { return "", exec.ErrNotFound },
		getenv:    os.Getenv,
		fileExist: fileExists,
	})
	if gotBin != bin || gotLib != lib {
		t.Fatalf("expected darwin bundled (%s,%s), got (%s,%s)", bin, lib, gotBin, gotLib)
	}
}
