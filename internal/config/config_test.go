package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAllDirs_CreatesExpectedDirs(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", base)

	if err := EnsureAllDirs(); err != nil {
		t.Fatalf("EnsureAllDirs: %v", err)
	}

	wantDirs := []string{
		base,
		filepath.Join(base, "cache"),
		filepath.Join(base, "logs"),
		filepath.Join(base, "subscriptions"),
		filepath.Join(base, "mihomo"),
	}
	for _, d := range wantDirs {
		st, err := os.Stat(d)
		if err != nil {
			t.Fatalf("expected dir %s to exist: %v", d, err)
		}
		if !st.IsDir() {
			t.Fatalf("expected %s to be a directory", d)
		}
	}
}

func TestLoad_Save_RoundTrip(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CLASH4CLI_HOME", base)

	// First load should create a config.yaml.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected config")
	}

	configPath := filepath.Join(base, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config.yaml to exist: %v", err)
	}

	// Mutate and save.
	cfg.Language = "zh_cn"
	cfg.Proxy.MixedPort = 19999
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify persisted fields.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Language != "zh_cn" {
		t.Fatalf("expected Language=zh_cn, got %q", cfg2.Language)
	}
	if cfg2.Proxy.MixedPort != 19999 {
		t.Fatalf("expected MixedPort=19999, got %d", cfg2.Proxy.MixedPort)
	}
}

func TestGetBaseDir_EnvOverride(t *testing.T) {
	base := filepath.Join(t.TempDir(), "  base  ")
	t.Setenv("CLASH4CLI_HOME", base)

	got, err := GetBaseDir()
	if err != nil {
		t.Fatalf("GetBaseDir: %v", err)
	}
	want := filepath.Clean(strings.TrimSpace(base))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
