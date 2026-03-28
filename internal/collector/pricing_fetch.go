package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudprobe/debrief/internal/config"
)

const (
	litellmURL         = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	pricingCacheTTL    = 24 * time.Hour
	pricingCacheFile   = "pricing-cache.json"
	pricingHTTPTimeout = 3 * time.Second
)

type pricingCache struct {
	FetchedAt time.Time               `json:"fetched_at"`
	Models    map[string]ModelPricing `json:"models"`
}

// LoadPricing returns the best available pricing table for the given config.
// It tries LiteLLM (cached, refreshed every 24h) then falls back to the
// hardcoded table so the tool always works offline.
// User overrides from config are applied on top of whichever source wins.
func LoadPricing(cacheDir string, cfg config.PricingConfig) map[string]ModelPricing {
	base := loadBaseTable(cacheDir, cfg.Preset)
	if len(cfg.Overrides) == 0 {
		return base
	}
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

func loadBaseTable(cacheDir, preset string) map[string]ModelPricing {
	if cached, err := readPricingCache(cacheDir); err == nil &&
		time.Since(cached.FetchedAt) < pricingCacheTTL &&
		len(cached.Models) > 0 {
		return cached.Models
	}

	fetched, err := fetchLiteLLMPricing()
	if err == nil && len(fetched) > 0 {
		_ = writePricingCache(cacheDir, fetched)
		return fetched
	}

	return presetTable(preset)
}

func readPricingCache(cacheDir string) (pricingCache, error) {
	var c pricingCache
	data, err := os.ReadFile(filepath.Join(cacheDir, pricingCacheFile))
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(data, &c)
}

func writePricingCache(cacheDir string, models map[string]ModelPricing) error {
	data, err := json.Marshal(pricingCache{FetchedAt: time.Now().UTC(), Models: models})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cacheDir, pricingCacheFile), data, 0600)
}

type litellmEntry struct {
	InputCostPerToken           float64 `json:"input_cost_per_token"`
	OutputCostPerToken          float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost     float64 `json:"cache_read_input_token_cost"`
}

func fetchLiteLLMPricing() (map[string]ModelPricing, error) {
	client := &http.Client{Timeout: pricingHTTPTimeout}
	resp, err := client.Get(litellmURL)
	if err != nil {
		return nil, fmt.Errorf("fetch litellm pricing: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("litellm pricing: unexpected status %d", resp.StatusCode)
	}

	var raw map[string]litellmEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("litellm pricing: decode: %w", err)
	}

	table := make(map[string]ModelPricing, 64)
	for name, entry := range raw {
		if entry.InputCostPerToken == 0 {
			continue
		}
		// Normalise key: strip "anthropic/" prefix, keep only claude-* models.
		key := strings.TrimPrefix(name, "anthropic/")
		if !strings.HasPrefix(key, "claude-") {
			continue
		}
		// Prefer the non-prefixed key; skip if already set by the canonical entry.
		if _, exists := table[key]; exists {
			continue
		}
		table[key] = ModelPricing{
			InputPerMillion:      entry.InputCostPerToken * 1_000_000,
			OutputPerMillion:     entry.OutputCostPerToken * 1_000_000,
			CacheWritePerMillion: entry.CacheCreationInputTokenCost * 1_000_000,
			CacheReadPerMillion:  entry.CacheReadInputTokenCost * 1_000_000,
		}
	}
	return table, nil
}
