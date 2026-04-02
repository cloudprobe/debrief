package synthesizer

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

func TestSynthesizePeriodSummary(t *testing.T) {
	tests := []struct {
		name            string
		days            []model.DaySummary
		totalDays       int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "multi-day shows period summary",
			days: []model.DaySummary{
				{
					Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "claude-code"}},
					ByProject:  map[string]model.ProjectSummary{"proj-a": {Name: "proj-a", CommitCount: 3}},
				},
				{
					Date:       time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-b", Source: "claude-code"}},
					ByProject:  map[string]model.ProjectSummary{"proj-b": {Name: "proj-b", CommitCount: 2}},
				},
				{
					Date:       time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "git"}},
					ByProject:  map[string]model.ProjectSummary{"proj-a": {Name: "proj-a", CommitCount: 4}},
				},
			},
			totalDays:       5,
			wantContains:    []string{"2 projects", "9 commits", "active 3 of 5 days"},
			wantNotContains: []string{},
		},
		{
			name: "single-day no period summary",
			days: []model.DaySummary{
				{
					Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "claude-code"}},
					ByProject:  map[string]model.ProjectSummary{"proj-a": {Name: "proj-a", CommitCount: 1}},
				},
			},
			totalDays:       1,
			wantContains:    []string{},
			wantNotContains: []string{"projects \u2022"},
		},
		{
			name: "totalDays zero falls back to len(days)",
			days: []model.DaySummary{
				{
					Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "claude-code"}},
					ByProject:  map[string]model.ProjectSummary{"proj-a": {Name: "proj-a", CommitCount: 2}},
				},
				{
					Date:       time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-b", Source: "git"}},
					ByProject:  map[string]model.ProjectSummary{"proj-b": {Name: "proj-b", CommitCount: 1}},
				},
			},
			totalDays:       0,
			wantContains:    []string{"active 2 of 2 days"},
			wantNotContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Synthesize(tt.days, tt.totalDays)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q, got:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("expected output to NOT contain %q, got:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestSynthesizeEmptyDaySuppression(t *testing.T) {
	tests := []struct {
		name            string
		days            []model.DaySummary
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "day with activities but only noise commits suppresses header",
			days: []model.DaySummary{
				{
					Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "git"}},
					ByProject: map[string]model.ProjectSummary{
						"proj-a": {
							Name:           "proj-a",
							CommitMessages: []string{"chore: bump deps", "docs: update readme", "ci: fix pipeline"},
						},
					},
				},
			},
			wantNotContains: []string{"Wed, Apr 1 2026:"},
		},
		{
			name: "day with renderable content shows header",
			days: []model.DaySummary{
				{
					Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "claude-code"}},
					ByProject: map[string]model.ProjectSummary{
						"proj-a": {
							Name:         "proj-a",
							SessionNotes: []string{"Added authentication flow"},
						},
					},
				},
			},
			wantContains: []string{"Wed, Apr 1 2026:"},
		},
		{
			name: "multi-day mix: only non-empty day header appears",
			days: []model.DaySummary{
				{
					// Noise-only day — should be suppressed
					Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-a", Source: "git"}},
					ByProject: map[string]model.ProjectSummary{
						"proj-a": {
							Name:           "proj-a",
							CommitMessages: []string{"chore: bump deps", "ci: fix lint"},
						},
					},
				},
				{
					// Real content day — should show
					Date:       time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
					Activities: []model.Activity{{Project: "proj-b", Source: "claude-code"}},
					ByProject: map[string]model.ProjectSummary{
						"proj-b": {
							Name:         "proj-b",
							SessionNotes: []string{"Implemented login page"},
						},
					},
				},
			},
			wantContains:    []string{"Thu, Apr 2 2026:"},
			wantNotContains: []string{"Wed, Apr 1 2026:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Synthesize(tt.days, len(tt.days))
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q, got:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("expected output to NOT contain %q, got:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestSynthesizeActiveDaysCount(t *testing.T) {
	// 2 days with real session notes, 1 day with only noise commits.
	// activeDays counts days with ByProject data — not days that rendered bullets.
	// Per the established test contract (see plan 07-01 deviation), activeDays
	// counts days where len(ByProject) > 0, regardless of whether any bullets rendered.
	days := []model.DaySummary{
		{
			Date:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			Activities: []model.Activity{{Project: "proj-a", Source: "claude-code"}},
			ByProject: map[string]model.ProjectSummary{
				"proj-a": {Name: "proj-a", SessionNotes: []string{"Built auth module"}, CommitCount: 1},
			},
		},
		{
			Date:       time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
			Activities: []model.Activity{{Project: "proj-b", Source: "claude-code"}},
			ByProject: map[string]model.ProjectSummary{
				"proj-b": {Name: "proj-b", SessionNotes: []string{"Wrote unit tests"}, CommitCount: 2},
			},
		},
		{
			// Noise-only day: has ByProject data so counts in activeDays
			Date:       time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
			Activities: []model.Activity{{Project: "proj-a", Source: "git"}},
			ByProject: map[string]model.ProjectSummary{
				"proj-a": {
					Name:           "proj-a",
					CommitMessages: []string{"chore: bump deps", "docs: update readme"},
					CommitCount:    2,
				},
			},
		},
	}
	got := Synthesize(days, 5)
	// activeDays = 3 (all 3 days have ByProject data), totalDays = 5
	if !strings.Contains(got, "active 3 of 5 days") {
		t.Errorf("expected 'active 3 of 5 days' in output, got:\n%s", got)
	}
}

func TestSynthesizePRLinks(t *testing.T) {
	day := func(projects map[string]model.ProjectSummary) model.DaySummary {
		return model.DaySummary{
			Date:       time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
			Activities: []model.Activity{{Project: "testproject", Source: "claude-code"}},
			ByProject:  projects,
		}
	}

	tests := []struct {
		name        string
		commits     []string
		sessionNote string
		wantPRs     bool
		wantPRLink  string
	}{
		{
			name:       "with pr link",
			commits:    []string{"closes https://github.com/cloudprobe/debrief/pull/1"},
			wantPRs:    true,
			wantPRLink: "PRs: https://github.com/cloudprobe/debrief/pull/1",
		},
		{
			name:    "no intent word — no PRs line",
			commits: []string{"bumped #3 priority item"},
			wantPRs: false,
		},
		{
			name:        "no commits — no PRs line",
			commits:     []string{},
			sessionNote: "Added feature X",
			wantPRs:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notes []string
			if tt.sessionNote != "" {
				notes = []string{tt.sessionNote}
			}
			projects := map[string]model.ProjectSummary{
				"testproject": {
					Name:           "testproject",
					CommitMessages: tt.commits,
					SessionNotes:   notes,
					CommitCount:    len(tt.commits),
				},
			}
			got := Synthesize([]model.DaySummary{day(projects)}, 0)
			if tt.wantPRs {
				if !strings.Contains(got, tt.wantPRLink) {
					t.Errorf("expected output to contain %q, got:\n%s", tt.wantPRLink, got)
				}
			} else {
				if strings.Contains(got, "PRs:") {
					t.Errorf("expected output to NOT contain 'PRs:', got:\n%s", got)
				}
			}
		})
	}
}
