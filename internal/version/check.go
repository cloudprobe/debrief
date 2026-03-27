package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	releasesAPI = "https://api.github.com/repos/cloudprobe/debrief/releases/latest"
	checkTTL    = 24 * time.Hour
	httpTimeout = 2 * time.Second
	cacheFile   = "version-check"
)

// UpdateInfo is returned when a newer version is available.
type UpdateInfo struct {
	Current string
	Latest  string
}

// cache is the on-disk structure for the cached version check.
type cache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

// CheckForUpdate checks if a newer version of debrief is available.
// cacheDir is the config directory (e.g. ~/.config/debrief).
// current is the running version string (e.g. "v0.1.0" or "dev").
//
// Returns (UpdateInfo, true) when a newer version was found.
// Returns (UpdateInfo{}, false) for any error, timeout, or up-to-date state.
// Never panics, never blocks longer than httpTimeout.
func CheckForUpdate(cacheDir, current string) (UpdateInfo, bool) {
	cachePath := filepath.Join(cacheDir, cacheFile)

	// Read existing cache.
	var c cache
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &c)
	}

	// Determine latest version: use cache if fresh, otherwise fetch.
	var latest string
	if time.Since(c.CheckedAt) < checkTTL && c.LatestVersion != "" {
		// Cache is still fresh — use it.
		latest = c.LatestVersion
	} else {
		// Fetch from GitHub with a short timeout.
		fetched, err := fetchLatest()
		if err != nil {
			// Network failure: use stale cache if available, otherwise skip.
			if c.LatestVersion != "" {
				latest = c.LatestVersion
			} else {
				return UpdateInfo{}, false
			}
		} else {
			latest = fetched
			// Write cache (best-effort — ignore write errors).
			c = cache{CheckedAt: time.Now().UTC(), LatestVersion: latest}
			if data, err := json.Marshal(c); err == nil {
				_ = os.MkdirAll(cacheDir, 0700)
				_ = os.WriteFile(cachePath, data, 0600)
			}
		}
	}

	// Compare versions. Skip if current is "dev" (not a release build).
	if current == "dev" || latest == "" {
		return UpdateInfo{}, false
	}

	// Normalize: strip leading "v" for comparison.
	cur := strings.TrimPrefix(current, "v")
	lat := strings.TrimPrefix(latest, "v")

	if lat <= cur {
		return UpdateInfo{}, false
	}

	return UpdateInfo{Current: current, Latest: latest}, true
}

// fetchLatest calls the GitHub releases API and returns the latest tag name.
func fetchLatest() (string, error) {
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(releasesAPI)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return release.TagName, nil
}
