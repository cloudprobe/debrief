package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
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
			{Project: "cloudprobe/devrecap", Source: "claude-code", Interactions: 10, TokensIn: 50000, TokensOut: 10000},
		},
		TotalTokens:  60000,
		Interactions: 10,
		ByProject: map[string]model.ProjectSummary{
			"cloudprobe/devrecap": {
				Name:         "cloudprobe/devrecap",
				Interactions: 10,
				TotalTokens:  60000,
				Sources:      []string{"claude-code"},
				FilesCreated: []string{"main.go", "model.go", "collector.go"},
			},
		},
		ByModel: map[string]model.ModelSummary{},
	}

	got := RenderText(s, RenderOptions{SingleDay: true})

	checks := []string{
		"Your day",
		"cloudprobe/devrecap",
		"with Claude",
		"Created 3 files",
		"main.go",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}

	// Should NOT contain tokens or model names.
	unwanted := []string{"tokens", "opus", "interactions"}
	for _, bad := range unwanted {
		if strings.Contains(got, bad) {
			t.Errorf("output should not contain %q:\n%s", bad, got)
		}
	}
}

func TestRenderText_CommitMessages(t *testing.T) {
	s := model.DaySummary{
		Date: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		Activities: []model.Activity{
			{Project: "dotfiles", Source: "git", CommitCount: 2},
		},
		ByProject: map[string]model.ProjectSummary{
			"dotfiles": {
				Name:           "dotfiles",
				CommitCount:    2,
				CommitMessages: []string{"add zsh aliases", "update gitconfig"},
				Sources:        []string{"git"},
			},
		},
		ByModel: map[string]model.ModelSummary{},
	}

	got := RenderText(s, RenderOptions{})

	if !strings.Contains(got, "Committed:") {
		t.Errorf("expected commit messages:\n%s", got)
	}
	if !strings.Contains(got, "add zsh aliases") {
		t.Errorf("expected commit message text:\n%s", got)
	}
}

func TestRenderText_GitOnlyProject(t *testing.T) {
	s := model.DaySummary{
		Date: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		Activities: []model.Activity{
			{Project: "dotfiles", Source: "git", CommitCount: 2},
		},
		ByProject: map[string]model.ProjectSummary{
			"dotfiles": {
				Name:           "dotfiles",
				CommitCount:    2,
				CommitMessages: []string{"fix bug", "add test"},
				Sources:        []string{"git"},
			},
		},
		ByModel: map[string]model.ModelSummary{},
	}

	got := RenderText(s, RenderOptions{})

	// Git-only should NOT say "with Claude".
	if strings.Contains(got, "Claude") {
		t.Errorf("git-only project should not mention Claude:\n%s", got)
	}
}

func TestRenderStandup_PlainText(t *testing.T) {
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
	// No markdown bold.
	if strings.Contains(got, "**") {
		t.Errorf("standup should not contain markdown bold:\n%s", got)
	}
	// Should include year.
	if !strings.Contains(got, "2026") {
		t.Errorf("standup should include year:\n%s", got)
	}
}

func TestRenderCost(t *testing.T) {
	s := model.DaySummary{
		Date:      time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		TotalCost: 2.50,
		Activities: []model.Activity{
			{Project: "cloudprobe/devrecap", Model: "claude-opus-4-6", CostUSD: 2.50, Source: "claude-code"},
		},
		ByProject: map[string]model.ProjectSummary{
			"cloudprobe/devrecap": {
				Name:      "cloudprobe/devrecap",
				TotalCost: 2.50,
			},
		},
		ByModel: map[string]model.ModelSummary{},
	}

	got := RenderCost(s, RenderOptions{ShowCost: true})
	if !strings.Contains(got, "$2.50") {
		t.Errorf("expected cost in output:\n%s", got)
	}
	if !strings.Contains(got, "opus 4.6") {
		t.Errorf("expected model name in cost view:\n%s", got)
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

func TestFormatCommitMessages(t *testing.T) {
	got := formatCommitMessages([]string{"add auth", "fix bug"}, 2)
	if !strings.Contains(got, "Committed:") {
		t.Errorf("expected Committed prefix: %s", got)
	}
	if !strings.Contains(got, "add auth") {
		t.Errorf("expected message text: %s", got)
	}
}
