package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

func TestRenderCostTable(t *testing.T) {
	tests := []struct {
		name   string
		days   []model.DaySummary
		checks []func(t *testing.T, got string)
	}{
		{
			name: "single day two models",
			days: []model.DaySummary{
				{
					Date:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					TotalCost: 1.55,
					ByModel: map[string]model.ModelSummary{
						"claude-opus-4-6":   {Name: "claude-opus-4-6", TotalCost: 1.24},
						"claude-sonnet-4-6": {Name: "claude-sonnet-4-6", TotalCost: 0.31},
					},
				},
			},
			checks: []func(t *testing.T, got string){
				func(t *testing.T, got string) {
					t.Helper()
					for _, want := range []string{"Date", "Model", "Cost (USD)", "opus 4.6", "sonnet 4.6", "$1.24", "$0.31", "subtotal", "$1.55"} {
						if !strings.Contains(got, want) {
							t.Errorf("expected %q in output:\n%s", want, got)
						}
					}
				},
			},
		},
		{
			name: "two days grand total",
			days: []model.DaySummary{
				{
					Date:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					TotalCost: 1.55,
					ByModel: map[string]model.ModelSummary{
						"claude-opus-4-6":   {Name: "claude-opus-4-6", TotalCost: 1.24},
						"claude-sonnet-4-6": {Name: "claude-sonnet-4-6", TotalCost: 0.31},
					},
				},
				{
					Date:      time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
					TotalCost: 0.18,
					ByModel: map[string]model.ModelSummary{
						"claude-sonnet-4-6": {Name: "claude-sonnet-4-6", TotalCost: 0.18},
					},
				},
			},
			checks: []func(t *testing.T, got string){
				func(t *testing.T, got string) {
					t.Helper()
					if !strings.Contains(got, "$1.73") {
						t.Errorf("expected grand total $1.73 in output:\n%s", got)
					}
					// Each date should appear exactly once.
					for _, date := range []string{"2026-04-01", "2026-04-02"} {
						count := strings.Count(got, date)
						if count != 1 {
							t.Errorf("expected %q exactly once in output, got %d times:\n%s", date, count, got)
						}
					}
				},
			},
		},
		{
			name: "empty ByModel day skipped",
			days: []model.DaySummary{
				{
					Date:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					TotalCost: 0,
					Activities: []model.Activity{
						{Project: "cloudprobe/devrecap", Source: "git"},
					},
					ByModel: map[string]model.ModelSummary{},
				},
			},
			checks: []func(t *testing.T, got string){
				func(t *testing.T, got string) {
					t.Helper()
					want := noCostData
					if got != want {
						t.Errorf("expected %q, got %q", want, got)
					}
				},
			},
		},
		{
			name: "single model day has subtotal row",
			days: []model.DaySummary{
				{
					Date:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					TotalCost: 0.50,
					ByModel: map[string]model.ModelSummary{
						"claude-sonnet-4-6": {Name: "claude-sonnet-4-6", TotalCost: 0.50},
					},
				},
			},
			checks: []func(t *testing.T, got string){
				func(t *testing.T, got string) {
					t.Helper()
					if !strings.Contains(got, "subtotal") {
						t.Errorf("expected subtotal row in single-model day output:\n%s", got)
					}
					if !strings.Contains(got, "$0.50") {
						t.Errorf("expected $0.50 in single-model day output:\n%s", got)
					}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderCostTable(tt.days)
			for _, check := range tt.checks {
				check(t, got)
			}
		})
	}
}

func TestRenderCostTable_GrandTotalAccuracy(t *testing.T) {
	days := []model.DaySummary{
		{
			Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			ByModel: map[string]model.ModelSummary{
				"claude-opus-4-6":   {Name: "claude-opus-4-6", TotalCost: 1.00},
				"claude-sonnet-4-6": {Name: "claude-sonnet-4-6", TotalCost: 0.50},
			},
		},
		{
			Date: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
			ByModel: map[string]model.ModelSummary{
				"claude-opus-4-6": {Name: "claude-opus-4-6", TotalCost: 2.00},
			},
		},
		{
			Date: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
			ByModel: map[string]model.ModelSummary{
				"claude-haiku-4-5-20251001": {Name: "claude-haiku-4-5-20251001", TotalCost: 0.25},
			},
		},
	}
	got := RenderCostTable(days)

	// Grand total = 1.00 + 0.50 + 2.00 + 0.25 = 3.75
	if !strings.Contains(got, "$3.75") {
		t.Errorf("expected grand total $3.75 in output:\n%s", got)
	}
	// Day 1 subtotal = 1.50
	if !strings.Contains(got, "$1.50") {
		t.Errorf("expected day 1 subtotal $1.50 in output:\n%s", got)
	}
	// Day 2 subtotal = 2.00
	if !strings.Contains(got, "$2.00") {
		t.Errorf("expected day 2 subtotal $2.00 in output:\n%s", got)
	}
	// Day 3 subtotal = 0.25
	if !strings.Contains(got, "$0.25") {
		t.Errorf("expected day 3 subtotal $0.25 in output:\n%s", got)
	}
	// Each date should appear in the output
	for _, date := range []string{"2026-04-01", "2026-04-02", "2026-04-03"} {
		if !strings.Contains(got, date) {
			t.Errorf("expected date %q in output:\n%s", date, got)
		}
	}
}

func TestRenderCostTable_EmptyDays(t *testing.T) {
	wantMsg := noCostData

	gotNil := RenderCostTable(nil)
	if gotNil != wantMsg {
		t.Errorf("RenderCostTable(nil) = %q, want %q", gotNil, wantMsg)
	}

	gotEmpty := RenderCostTable([]model.DaySummary{})
	if gotEmpty != wantMsg {
		t.Errorf("RenderCostTable([]DaySummary{}) = %q, want %q", gotEmpty, wantMsg)
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

func TestExtractPRLinks(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "full GitHub URL",
			input: []string{"fix: closes https://github.com/cloudprobe/debrief/pull/42"},
			want:  []string{"https://github.com/cloudprobe/debrief/pull/42"},
		},
		{
			name:  "GitLab URL",
			input: []string{"feat: merge https://gitlab.com/org/repo/-/merge_requests/7"},
			want:  []string{"https://gitlab.com/org/repo/-/merge_requests/7"},
		},
		{
			name:  "bare #N with intent word closes",
			input: []string{"closes #12 — fixed the thing"},
			want:  []string{"#12"},
		},
		{
			name:  "bare #N with intent word fixes",
			input: []string{"fixes #99"},
			want:  []string{"#99"},
		},
		{
			name:  "false positive — no intent word priority",
			input: []string{"bumped #3 priority item"},
			want:  []string{},
		},
		{
			name:  "false positive — no intent word list",
			input: []string{"added #4 on the list"},
			want:  []string{},
		},
		{
			name:  "false positive — issue alone is not intent word",
			input: []string{"issue #5 in the backlog"},
			want:  []string{},
		},
		{
			name:  "deduplication",
			input: []string{"closes #12", "closes #12 again"},
			want:  []string{"#12"},
		},
		{
			name:  "mixed URLs and bare refs — URLs first",
			input: []string{"https://github.com/org/repo/pull/10", "fixes #3"},
			want:  []string{"https://github.com/org/repo/pull/10", "#3"},
		},
		{
			name:  "empty input",
			input: []string{},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPRLinks(tt.input)
			// Normalize nil vs empty slice.
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ExtractPRLinks(%v) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractPRLinks(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
