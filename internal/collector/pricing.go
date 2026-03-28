package collector

import (
	"strings"

	"github.com/cloudprobe/debrief/internal/config"
)

// ModelPricing holds per-million-token rates for a model.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// EffectivePricing returns the pricing table to use, merging preset + overrides.
func EffectivePricing(cfg config.PricingConfig) map[string]ModelPricing {
	base := presetTable(cfg.Preset)
	if len(cfg.Overrides) == 0 {
		return base
	}
	// Copy base and apply overrides.
	merged := make(map[string]ModelPricing, len(base))
	for k, v := range base {
		merged[k] = v
	}
	for model, rate := range cfg.Overrides {
		merged[model] = ModelPricing{
			InputPerMillion:  rate.InputPerMillion,
			OutputPerMillion: rate.OutputPerMillion,
		}
	}
	return merged
}

func presetTable(preset string) map[string]ModelPricing {
	switch preset {
	case "vertex":
		return vertexTable()
	case "bedrock":
		return bedrockTable()
	default: // "direct" or ""
		return directTable()
	}
}

// directTable returns the current Anthropic direct API pricing rates.
// Rates are in USD per 1 million tokens.
// Source: https://platform.claude.com/docs/en/about-claude/pricing (2026-03-26)
func directTable() map[string]ModelPricing {
	return map[string]ModelPricing{
		// Anthropic Claude 4.x
		"claude-opus-4-6":            {InputPerMillion: 5.0, OutputPerMillion: 25.0},
		"claude-opus-4-5":            {InputPerMillion: 5.0, OutputPerMillion: 25.0},
		"claude-sonnet-4-6":          {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		"claude-sonnet-4-5-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		"claude-sonnet-4-5@20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		"claude-haiku-4-5":           {InputPerMillion: 1.0, OutputPerMillion: 5.0},
		"claude-haiku-4-5-20251001":  {InputPerMillion: 1.0, OutputPerMillion: 5.0},
		"claude-haiku-4-5@20251001":  {InputPerMillion: 1.0, OutputPerMillion: 5.0},
		"claude-haiku-3-5":           {InputPerMillion: 0.80, OutputPerMillion: 4.0},
		// OpenAI
		"gpt-4o":      {InputPerMillion: 2.5, OutputPerMillion: 10.0},
		"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
		"o3":          {InputPerMillion: 10.0, OutputPerMillion: 40.0},
		"o3-mini":     {InputPerMillion: 1.10, OutputPerMillion: 4.40},
		// Google
		"gemini-2.5-pro":   {InputPerMillion: 1.25, OutputPerMillion: 10.0},
		"gemini-2.5-flash": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	}
}

// vertexTable returns pricing for Google Vertex AI.
// For current Claude 4.x models, global endpoint pricing matches direct API.
// Source: https://cloud.google.com/vertex-ai/pricing (2026-03-26)
func vertexTable() map[string]ModelPricing {
	return directTable()
}

// bedrockTable returns pricing for AWS Bedrock.
// For current Claude 4.x models, on-demand global endpoint pricing matches direct API.
// Source: https://aws.amazon.com/bedrock/pricing (2026-03-26)
func bedrockTable() map[string]ModelPricing {
	return directTable()
}

// CalculateCost returns (cost, known).
// known=false means the model has no configured price; caller should display "—".
// Cache read tokens are charged at 10% of the input rate.
// Cache write tokens are charged at 125% of the input rate.
//
// If the exact model name isn't in the table, the lookup falls back to stripping
// any trailing date version (-YYYYMMDD or @YYYYMMDD) from both sides, so that
// e.g. "claude-sonnet-4-5-20250929" matches "claude-sonnet-4-5-20250514".
func CalculateCost(table map[string]ModelPricing, modelName string, tokensIn, tokensOut, cacheRead, cacheWrite int) (float64, bool) {
	pricing, ok := table[modelName]
	if !ok {
		// Strip date suffix and try matching any table entry with the same base name.
		base := stripDateSuffix(modelName)
		if base != modelName {
			for k, v := range table {
				if stripDateSuffix(k) == base {
					pricing = v
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return 0, false
	}

	inputCost := float64(tokensIn) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(tokensOut) * pricing.OutputPerMillion / 1_000_000
	cacheReadCost := float64(cacheRead) * pricing.InputPerMillion * 0.10 / 1_000_000
	cacheWriteCost := float64(cacheWrite) * pricing.InputPerMillion * 1.25 / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheWriteCost, true
}

// stripDateSuffix removes a trailing date version from a model name.
// Handles both separator styles used by Anthropic and Vertex:
//
//	claude-sonnet-4-5-20250929  →  claude-sonnet-4-5
//	claude-haiku-4-5@20251001   →  claude-haiku-4-5
func stripDateSuffix(name string) string {
	for _, sep := range []byte{'-', '@'} {
		i := strings.LastIndexByte(name, sep)
		if i <= 0 {
			continue
		}
		suffix := name[i+1:]
		if len(suffix) == 8 && isAllDigits(suffix) {
			return name[:i]
		}
	}
	return name
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
