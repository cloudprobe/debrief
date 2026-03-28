package collector

import "strings"

// ModelPricing holds per-million-token rates for a model.
// CacheWritePerMillion and CacheReadPerMillion are populated from LiteLLM data
// when available; if zero the standard ratios (1.25× and 0.10× input) apply.
type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheWritePerMillion float64
	CacheReadPerMillion  float64
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

// directTable is the hardcoded fallback used when LiteLLM is unreachable.
// Rates are in USD per 1 million tokens.
// Reference: https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json
func directTable() map[string]ModelPricing {
	return map[string]ModelPricing{
		// Claude 4.x
		"claude-opus-4-6":            {InputPerMillion: 5.0, OutputPerMillion: 25.0},
		"claude-opus-4-5":            {InputPerMillion: 5.0, OutputPerMillion: 25.0},
		"claude-sonnet-4-6":          {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		"claude-sonnet-4-5-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		"claude-haiku-4-5":           {InputPerMillion: 1.0, OutputPerMillion: 5.0},
		"claude-haiku-4-5-20251001":  {InputPerMillion: 1.0, OutputPerMillion: 5.0},
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
// Current Claude 4.x global endpoint pricing matches the direct API.
func vertexTable() map[string]ModelPricing {
	return directTable()
}

// bedrockTable returns pricing for AWS Bedrock.
// Current Claude 4.x on-demand global endpoint pricing matches the direct API.
func bedrockTable() map[string]ModelPricing {
	return directTable()
}

// CalculateCost returns (cost, known).
// known=false means the model has no configured price; caller should display "—".
//
// Cache rates come from the model's explicit CacheWritePerMillion/CacheReadPerMillion
// fields when set (populated from LiteLLM). When zero, standard ratios apply:
// cache reads at 10% of input, cache writes at 125% of input.
//
// If the exact model name isn't in the table, falls back to stripping the trailing
// date suffix (-YYYYMMDD or @YYYYMMDD) so e.g. "claude-sonnet-4-5-20250929"
// matches "claude-sonnet-4-5-20250514".
func CalculateCost(table map[string]ModelPricing, modelName string, tokensIn, tokensOut, cacheRead, cacheWrite int) (float64, bool) {
	pricing, ok := table[modelName]
	if !ok {
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

	cacheWriteRate := pricing.CacheWritePerMillion
	if cacheWriteRate == 0 {
		cacheWriteRate = pricing.InputPerMillion * 1.25
	}
	cacheReadRate := pricing.CacheReadPerMillion
	if cacheReadRate == 0 {
		cacheReadRate = pricing.InputPerMillion * 0.10
	}

	inputCost := float64(tokensIn) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(tokensOut) * pricing.OutputPerMillion / 1_000_000
	cacheWriteCost := float64(cacheWrite) * cacheWriteRate / 1_000_000
	cacheReadCost := float64(cacheRead) * cacheReadRate / 1_000_000

	return inputCost + outputCost + cacheWriteCost + cacheReadCost, true
}

// stripDateSuffix removes a trailing date version from a model name.
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
