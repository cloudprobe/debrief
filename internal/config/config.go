package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds devrecap configuration.
type Config struct {
	// GitRepoPaths are directories to scan for git repos.
	GitRepoPaths []string `yaml:"git_repo_paths"`

	// ClaudeDir overrides the default ~/.claude/projects/ path.
	ClaudeDir string `yaml:"claude_dir,omitempty"`

	// CodexDir overrides the default ~/.codex/sessions/ path.
	CodexDir string `yaml:"codex_dir,omitempty"`

	// GeminiDir overrides the default ~/.gemini/tmp/ path.
	GeminiDir string `yaml:"gemini_dir,omitempty"`

	// DefaultFormat is the output format: "tui", "text", or "json".
	DefaultFormat string `yaml:"default_format"`
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
		DefaultFormat: "text",
	}
}

// configPath returns the path to the config file.
func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "devrecap", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "devrecap", "config.yaml")
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
