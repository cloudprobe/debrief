package main_test

import (
	"os"
	"strings"
	"testing"
)

// TestREADME_HasBadges verifies that README.md contains the expected status
// badges at the top so CI, coverage, lint, release, and license status are
// visible to users landing on the project page.
func TestREADME_HasBadges(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("could not read README.md: %v", err)
	}
	content := string(data)

	badges := []struct {
		name    string
		pattern string
	}{
		{"CI workflow badge", "actions/workflows/ci.yml/badge.svg"},
		{"Coveralls coverage badge", "coveralls.io/repos/github/cloudprobe/debrief/badge.svg"},
		{"Go Report Card badge", "goreportcard.com/badge/github.com/cloudprobe/debrief"},
		{"latest release badge", "img.shields.io/github/v/release/cloudprobe/debrief"},
		{"license badge", "img.shields.io/github/license/cloudprobe/debrief"},
	}

	for _, b := range badges {
		if !strings.Contains(content, b.pattern) {
			t.Errorf("README.md missing %s (pattern: %q)", b.name, b.pattern)
		}
	}
}
