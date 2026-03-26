package collector

// ModelPricing holds per-million-token rates for a model.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// PricingTable maps model identifiers to their pricing.
// Rates are in USD per 1 million tokens.
var PricingTable = map[string]ModelPricing{
	// Anthropic
	"claude-opus-4-6":            {InputPerMillion: 5.0, OutputPerMillion: 25.0},
	"claude-sonnet-4-6":          {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-haiku-4-5":           {InputPerMillion: 1.0, OutputPerMillion: 5.0},
	"claude-haiku-4-5-20251001":  {InputPerMillion: 1.0, OutputPerMillion: 5.0},
	"claude-sonnet-4-5-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	// OpenAI
	"gpt-4o":      {InputPerMillion: 2.5, OutputPerMillion: 10.0},
	"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	"o3":          {InputPerMillion: 10.0, OutputPerMillion: 40.0},
	"o3-mini":     {InputPerMillion: 1.10, OutputPerMillion: 4.40},
	// Google
	"gemini-2.5-pro":   {InputPerMillion: 1.25, OutputPerMillion: 10.0},
	"gemini-2.5-flash": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
}

// CalculateCost computes the USD cost for a given token usage and model.
// Cache read tokens are charged at 10% of the input rate.
// Cache write tokens are charged at the normal input rate.
func CalculateCost(modelName string, tokensIn, tokensOut, cacheRead, cacheWrite int) float64 {
	pricing, ok := PricingTable[modelName]
	if !ok {
		return 0
	}

	inputCost := float64(tokensIn) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(tokensOut) * pricing.OutputPerMillion / 1_000_000
	cacheReadCost := float64(cacheRead) * pricing.InputPerMillion * 0.10 / 1_000_000
	cacheWriteCost := float64(cacheWrite) * pricing.InputPerMillion / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheWriteCost
}
