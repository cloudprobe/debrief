package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/devrecap/internal/model"
)

func TestRenderText_Empty(t *testing.T) {
	s := model.DaySummary{}
	got := RenderText(s, RenderOptions{})
	if !strings.Contains(got, "No activity") {
		t.Errorf("expected 'No activity' message, got: %s", got)
	}
}

func TestRenderText_ShowsProject(t *testing.T) {
	s := model.DaySummary{
		Date: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		Activities: []model.Activity{
			{Project: "devrecap", Interactions: 10, TokensIn: 50000, TokensOut: 10000},
		},
		TotalTokens:  60000,
		Interactions: 10,
		ByProject: map[string]model.ProjectSummary{
			"devrecap": {
				Name:         "devrecap",
				Interactions: 10,
				TotalTokens:  60000,
				Models:       []string{"claude-opus-4-6"},
				FilesCreated: []string{"main.go", "model.go", "collector.go"},
			},
		},
		ByModel: map[string]model.ModelSummary{
			"claude-opus-4-6": {Name: "claude-opus-4-6", TokensIn: 50000, TokensOut: 10000},
		},
	}

	got := RenderText(s, RenderOptions{})

	checks := []string{
		"devrecap",
		"10 interactions",
		"60.0K tokens",
		"opus 4.6",
		"3 files created",
		"main.go",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderText_CostHiddenByDefault(t *testing.T) {
	s := model.DaySummary{
		Date:       time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		Activities: []model.Activity{{Project: "x"}},
		TotalCost:  1.50,
		ByProject: map[string]model.ProjectSummary{
			"x": {Name: "x", TotalCost: 1.50},
		},
		ByModel: map[string]model.ModelSummary{},
	}

	got := RenderText(s, RenderOptions{ShowCost: false})
	if strings.Contains(got, "$") {
		t.Errorf("cost should be hidden by default:\n%s", got)
	}

	got = RenderText(s, RenderOptions{ShowCost: true})
	if !strings.Contains(got, "$1.50") {
		t.Errorf("cost should show with --cost:\n%s", got)
	}
}

func TestRenderStandup_Conversational(t *testing.T) {
	s := model.DaySummary{
		Date: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		Activities: []model.Activity{
			{Project: "bigproject", Interactions: 20},
			{Project: "smallfix", Interactions: 2},
		},
		ByProject: map[string]model.ProjectSummary{
			"bigproject": {
				Name:         "bigproject",
				Interactions: 20,
				FilesCreated: []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"},
			},
			"smallfix": {
				Name:         "smallfix",
				Interactions: 2,
			},
		},
	}

	got := RenderStandup(s, RenderOptions{})

	if !strings.Contains(got, "Built out") {
		t.Errorf("expected 'Built out' for large project:\n%s", got)
	}
	if !strings.Contains(got, "Minor work on") {
		t.Errorf("expected 'Minor work on' for small project:\n%s", got)
	}
}

func TestShortModelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-opus-4-6", "opus 4.6"},
		{"claude-sonnet-4-6", "sonnet 4.6"},
		{"claude-haiku-4-5-20251001", "haiku 4.5"},
		{"gpt-4o", "gpt-4o"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := shortModelName(tt.input)
		if got != tt.want {
			t.Errorf("shortModelName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{500, "500"},
		{1500, "1.5K"},
		{1_500_000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "file"); got != "1 file" {
		t.Errorf("got %q", got)
	}
	if got := plural(3, "file"); got != "3 files" {
		t.Errorf("got %q", got)
	}
}
