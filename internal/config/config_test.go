package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig_SensibleDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.GitDiscoveryDepth != 2 {
		t.Errorf("GitDiscoveryDepth = %d, want 2", cfg.GitDiscoveryDepth)
	}
	if cfg.DefaultFormat != "text" {
		t.Errorf("DefaultFormat = %q, want %q", cfg.DefaultFormat, "text")
	}
	if len(cfg.GitRepoPaths) == 0 {
		t.Error("GitRepoPaths should not be empty")
	}
	// Each default path should be an absolute path (starts with /).
	for _, p := range cfg.GitRepoPaths {
		if !filepath.IsAbs(p) {
			t.Errorf("GitRepoPaths entry %q is not absolute", p)
		}
	}
}

func TestDefaultConfig_ContainsCommonDirs(t *testing.T) {
	cfg := DefaultConfig()
	home, _ := os.UserHomeDir()

	want := []string{
		filepath.Join(home, "work"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "code"),
	}
	for _, w := range want {
		found := false
		for _, p := range cfg.GitRepoPaths {
			if p == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected GitRepoPaths to contain %q", w)
		}
	}
}

func TestLoad_ReturnsDefaultsWhenNoFile(t *testing.T) {
	// Point XDG_CONFIG_HOME at an empty temp dir so there is no config file.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := Load()

	if cfg.GitDiscoveryDepth != 2 {
		t.Errorf("GitDiscoveryDepth = %d, want 2", cfg.GitDiscoveryDepth)
	}
	if cfg.DefaultFormat != "text" {
		t.Errorf("DefaultFormat = %q, want %q", cfg.DefaultFormat, "text")
	}
}

func TestLoad_ReadsConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "debrief")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yaml := `
git_repo_paths:
  - /tmp/repos
git_discovery_depth: 5
default_format: json
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), 0600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg := Load()

	if cfg.GitDiscoveryDepth != 5 {
		t.Errorf("GitDiscoveryDepth = %d, want 5", cfg.GitDiscoveryDepth)
	}
	if cfg.DefaultFormat != "json" {
		t.Errorf("DefaultFormat = %q, want %q", cfg.DefaultFormat, "json")
	}
	if len(cfg.GitRepoPaths) != 1 || cfg.GitRepoPaths[0] != "/tmp/repos" {
		t.Errorf("GitRepoPaths = %v, want [/tmp/repos]", cfg.GitRepoPaths)
	}
}

func TestLoad_MergesWithDefaults(t *testing.T) {
	// A partial config: only sets DefaultFormat, leaving GitRepoPaths as default.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "debrief")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yaml := `default_format: tui`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), 0600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg := Load()
	if cfg.DefaultFormat != "tui" {
		t.Errorf("DefaultFormat = %q, want tui", cfg.DefaultFormat)
	}
	// GitRepoPaths should still hold defaults because yaml.Unmarshal doesn't zero them.
	if len(cfg.GitRepoPaths) == 0 {
		t.Error("GitRepoPaths should retain defaults when not specified in config file")
	}
}

func TestLoad_PricingPreset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "debrief")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yaml := `
pricing:
  preset: vertex
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), 0600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg := Load()
	if cfg.Pricing.Preset != "vertex" {
		t.Errorf("Pricing.Preset = %q, want vertex", cfg.Pricing.Preset)
	}
}

func TestConfigDir_NonEmpty(t *testing.T) {
	dir := ConfigDir()
	if dir == "" {
		t.Error("ConfigDir() returned empty string")
	}
}

func TestConfigDir_RespectsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	dir := ConfigDir()
	if !strings.HasPrefix(dir, "/tmp/xdg-test") {
		t.Errorf("ConfigDir() = %q, expected prefix /tmp/xdg-test", dir)
	}
	if !strings.HasSuffix(dir, "debrief") {
		t.Errorf("ConfigDir() = %q, expected suffix debrief", dir)
	}
}

func TestConfigDir_DefaultsToHomeConfig(t *testing.T) {
	// Unset XDG_CONFIG_HOME to test the fallback path.
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := ConfigDir()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "debrief")
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}
