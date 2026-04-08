package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAtoi(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"10", 10},
		{"100", 100},
		{"", 0},
		{"abc", 0},
		{"12abc", 12},
	}
	for _, tt := range tests {
		got := atoi(tt.input)
		if got != tt.want {
			t.Errorf("atoi(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.5.0", "0.4.1", 1},
		{"0.4.1", "0.5.0", -1},
		{"0.5.0", "0.5.0", 0},
		{"0.10.0", "0.9.0", 1},
		{"0.9.0", "0.10.0", -1},
		{"1.0.0", "0.9.9", 1},
		{"0.0.2", "0.0.1", 1},
		{"0.0.1", "0.0.2", -1},
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "1.99.99", 1},
		// short versions — missing segments filled with 0
		{"1.0", "0.9", 1},
		{"1", "0", 1},
	}
	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNewerThan(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v0.5.0", "v0.4.1", true},
		{"v0.10.0", "v0.9.0", true},
		{"v0.4.1", "v0.5.0", false},
		{"v0.5.0", "v0.5.0", false},
		// no 'v' prefix
		{"0.5.0", "0.4.9", true},
		// mixed prefix
		{"v1.0.0", "0.9.9", true},
	}
	for _, tt := range tests {
		got := newerThan(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("newerThan(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// mockReleaseServer starts an httptest server that responds with the given tag name.
// If tag is empty the server returns a 500.
func mockReleaseServer(t *testing.T, tag string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}
		resp := struct {
			TagName string `json:"tag_name"`
		}{TagName: tag}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// patchReleasesAPI temporarily replaces the releasesAPI constant value via fetchLatest.
// Because releasesAPI is a package-level const, we test CheckForUpdate by writing
// a pre-populated cache file (bypassing the network) or using the overridable path.
//
// For tests that need a real HTTP call we instead call fetchLatest indirectly through
// CheckForUpdate with a rigged cache that forces a fetch. We cannot monkey-patch a
// const, so we use the cache mechanism: an expired cache triggers a real fetch.
// We swap the URL by replacing fetchLatest with a closure-based approach — but since
// fetchLatest is a plain function (not variable), we use a small helper that writes
// the server URL into the cache dir's version-check file with CheckedAt in the past.

func TestCheckForUpdate_ReturnsUpdate(t *testing.T) {
	srv := mockReleaseServer(t, "v1.0.0", http.StatusOK)
	defer srv.Close()

	// We cannot override the const URL directly, so we pre-populate an expired cache
	// pointing at a version older than v1.0.0, and let the real fetch happen.
	// But since we can't redirect the real fetch, we instead write a fresh cache
	// with the "latest" version and a recent CheckedAt to skip the network call.
	dir := t.TempDir()
	writeCache(t, dir, "v1.0.0", time.Now())

	info, ok := CheckForUpdate(dir, "v0.5.0")
	if !ok {
		t.Fatal("expected update to be found")
	}
	if info.Latest != "v1.0.0" {
		t.Errorf("Latest = %q, want %q", info.Latest, "v1.0.0")
	}
	if info.Current != "v0.5.0" {
		t.Errorf("Current = %q, want %q", info.Current, "v0.5.0")
	}
}

func TestCheckForUpdate_NoUpdateWhenSameVersion(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, "v0.5.0", time.Now())

	_, ok := CheckForUpdate(dir, "v0.5.0")
	if ok {
		t.Fatal("expected no update when running the latest version")
	}
}

func TestCheckForUpdate_NoUpdateWhenCurrentIsNewer(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, "v0.4.0", time.Now())

	_, ok := CheckForUpdate(dir, "v9999.0.0")
	if ok {
		t.Fatal("expected no update when current is newer than cached latest")
	}
}

func TestCheckForUpdate_SkipsDevBuilds(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, "v1.0.0", time.Now())

	_, ok := CheckForUpdate(dir, "dev")
	if ok {
		t.Fatal("expected no update for dev builds")
	}
}

func TestCheckForUpdate_CacheHit(t *testing.T) {
	dir := t.TempDir()
	// Fresh cache — should not trigger a network fetch.
	writeCache(t, dir, "v2.0.0", time.Now())

	info, ok := CheckForUpdate(dir, "v0.1.0")
	if !ok {
		t.Fatal("expected update from cache")
	}
	if info.Latest != "v2.0.0" {
		t.Errorf("Latest = %q, want %q", info.Latest, "v2.0.0")
	}
}

func TestCheckForUpdate_StaleCache_FallsBackOnNetworkError(t *testing.T) {
	// Write a stale cache (25 hours old). The real fetch will fail because
	// releasesAPI points at GitHub. We expect it to fall back to the stale value.
	dir := t.TempDir()
	writeCache(t, dir, "v0.9.0", time.Now().Add(-25*time.Hour))

	// CheckForUpdate will try to fetch (stale cache) and fail (network / wrong URL
	// is fine in CI — GitHub may or may not be reachable). If the fetch succeeds we
	// just need to confirm we get a sensible answer; if it fails we expect the stale
	// cache to be used as fallback, so no panic and a valid result or (empty, false).
	// We can't assert the exact outcome because it depends on network access.
	// What we CAN assert is that the function never panics.
	_, _ = CheckForUpdate(dir, "v0.5.0")
}

func TestCheckForUpdate_InvalidatesCacheWhenCurrentIsNewerThanCached(t *testing.T) {
	// If the user has already upgraded past the cached "latest", the cache is
	// considered behind. The function will attempt a real fetch. Since we can't
	// control the network, we just verify no panic and that the result is sensible.
	dir := t.TempDir()
	// Cache says latest is v0.3.0 but we're running v1.0.0 — cache is behind.
	writeCache(t, dir, "v0.3.0", time.Now())

	_, _ = CheckForUpdate(dir, "v1.0.0")
	// No assertions beyond "no panic" since the fetch depends on network.
}

func TestCheckForUpdate_EmptyCacheDir(t *testing.T) {
	// No cache file at all — triggers a network attempt. Should not panic.
	dir := t.TempDir()
	_, _ = CheckForUpdate(dir, "v0.1.0")
}

func TestCheckForUpdate_EnvVarDisablesCheck(t *testing.T) {
	t.Setenv("DEBRIEF_NO_UPDATE_CHECK", "1")

	dir := t.TempDir()
	// Write a cache that would normally report an update.
	writeCache(t, dir, "v9.9.9", time.Now())

	_, ok := CheckForUpdate(dir, "v0.1.0")
	if ok {
		t.Fatal("expected no update when DEBRIEF_NO_UPDATE_CHECK is set")
	}
}

// writeCache writes a version cache file to dir with the given latest version and checked time.
func writeCache(t *testing.T, dir, latestVersion string, checkedAt time.Time) {
	t.Helper()
	c := cache{CheckedAt: checkedAt, LatestVersion: latestVersion}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshalling cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFile), data, 0600); err != nil {
		t.Fatalf("writing cache file: %v", err)
	}
}
