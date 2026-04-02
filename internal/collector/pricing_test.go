package collector

import (
	"testing"

	"github.com/cloudprobe/debrief/internal/config"
)

func TestEffectivePricing_DirectPreset(t *testing.T) {
	table := EffectivePricing(config.PricingConfig{Preset: "direct"})
	if _, ok := table["claude-opus-4-6"]; !ok {
		t.Error("direct preset: expected claude-opus-4-6 in table")
	}
	if _, ok := table["claude-sonnet-4-6"]; !ok {
		t.Error("direct preset: expected claude-sonnet-4-6 in table")
	}
}

func TestEffectivePricing_EmptyPresetDefaultsToDirect(t *testing.T) {
	table := EffectivePricing(config.PricingConfig{})
	if _, ok := table["claude-opus-4-6"]; !ok {
		t.Error("empty preset: expected direct table (opus should be present)")
	}
}

func TestEffectivePricing_VertexPreset(t *testing.T) {
	table := EffectivePricing(config.PricingConfig{Preset: "vertex"})
	if len(table) == 0 {
		t.Error("vertex preset: expected non-empty pricing table")
	}
	if _, ok := table["claude-opus-4-6"]; !ok {
		t.Error("vertex preset: expected claude-opus-4-6 in table")
	}
}

func TestEffectivePricing_BedrockPreset(t *testing.T) {
	table := EffectivePricing(config.PricingConfig{Preset: "bedrock"})
	if len(table) == 0 {
		t.Error("bedrock preset: expected non-empty pricing table")
	}
	if _, ok := table["claude-opus-4-6"]; !ok {
		t.Error("bedrock preset: expected claude-opus-4-6 in table")
	}
}

func TestEffectivePricing_Overrides(t *testing.T) {
	cfg := config.PricingConfig{
		Preset: "direct",
		Overrides: map[string]config.ModelRateConfig{
			"my-custom-model": {InputPerMillion: 99.0, OutputPerMillion: 199.0},
		},
	}
	table := EffectivePricing(cfg)

	p, ok := table["my-custom-model"]
	if !ok {
		t.Fatal("override not applied: my-custom-model missing from table")
	}
	if p.InputPerMillion != 99.0 {
		t.Errorf("InputPerMillion = %v, want 99.0", p.InputPerMillion)
	}
	if p.OutputPerMillion != 199.0 {
		t.Errorf("OutputPerMillion = %v, want 199.0", p.OutputPerMillion)
	}
	// Base table entries must still be present.
	if _, ok := table["claude-opus-4-6"]; !ok {
		t.Error("base table entry claude-opus-4-6 lost after applying overrides")
	}
}

func TestEffectivePricing_OverrideReplacesBaseEntry(t *testing.T) {
	cfg := config.PricingConfig{
		Preset: "direct",
		Overrides: map[string]config.ModelRateConfig{
			"claude-opus-4-6": {InputPerMillion: 1.0, OutputPerMillion: 2.0},
		},
	}
	table := EffectivePricing(cfg)
	p := table["claude-opus-4-6"]
	if p.InputPerMillion != 1.0 {
		t.Errorf("override should replace base: InputPerMillion = %v, want 1.0", p.InputPerMillion)
	}
}

func TestParseRepoSlug(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/cloudprobe/debrief.git", "cloudprobe/debrief"},
		{"https://github.com/org/repo", "org/repo"},
		{"git@github.com:cloudprobe/debrief.git", "cloudprobe/debrief"},
		{"git@gitlab.com:group/project.git", "group/project"},
		{"git@github.com:org/repo-name.git", "org/repo-name"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := parseRepoSlug(tt.url)
			if got != tt.want {
				t.Errorf("parseRepoSlug(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
