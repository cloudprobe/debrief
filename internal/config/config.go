package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds debrief configuration.
type Config struct {
	// GitRepoPaths are directories to scan for git repos.
	GitRepoPaths []string `yaml:"git_repo_paths"`

	// GitDiscoveryDepth controls how many directory levels deep to scan for git repos.
	// Default is 2.
	GitDiscoveryDepth int `yaml:"git_discovery_depth,omitempty"`

	// ClaudeDir overrides the default ~/.claude/projects/ path.
	ClaudeDir string `yaml:"claude_dir,omitempty"`

	// CodexDir overrides the default ~/.codex/sessions/ path.
	CodexDir string `yaml:"codex_dir,omitempty"`

	// GeminiDir overrides the default ~/.gemini/tmp/ path.
	GeminiDir string `yaml:"gemini_dir,omitempty"`

	// DefaultFormat is the output format: "tui", "text", or "json".
	DefaultFormat string `yaml:"default_format"`

	// Pricing controls how API costs are calculated.
	Pricing PricingConfig `yaml:"pricing,omitempty"`
}

// PricingConfig controls how API costs are calculated.
type PricingConfig struct {
	// Preset selects a provider pricing table: "direct" (default), "vertex", "bedrock".
	// Overrides are applied on top of the preset.
	Preset string `yaml:"preset,omitempty"`

	// Overrides maps model slugs to custom per-million-token rates.
	// A model listed here takes precedence over the preset table.
	Overrides map[string]ModelRateConfig `yaml:"overrides,omitempty"`
}

// ModelRateConfig holds per-million-token rates for a single model.
type ModelRateConfig struct {
	InputPerMillion  float64 `yaml:"input_per_million"`
	OutputPerMillion float64 `yaml:"output_per_million"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		GitRepoPaths: []string{
			filepath.Join(home, "work"),
			filepath.Join(home, "projects"),
			filepath.Join(home, "code"),
		},
		GitDiscoveryDepth: 2,
		DefaultFormat:     "text",
	}
}

// ConfigDir returns the directory containing debrief's config file.
// Respects XDG_CONFIG_HOME if set; otherwise ~/.config/debrief.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "debrief")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "debrief")
}

// configPath returns the path to the config file.
func configPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// Load reads the config from disk, returning defaults if the file doesn't exist.
func Load() Config {
	cfg := DefaultConfig()

	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}

	_ = yaml.Unmarshal(data, &cfg)
	return cfg
}
