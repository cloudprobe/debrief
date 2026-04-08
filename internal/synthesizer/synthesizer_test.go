package synthesizer

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

func TestSignificantWords(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"add authentication module", []string{"authentication", "module"}},
		{"fix the crash", []string{"crash"}},                                    // "the" < 4 chars
		{"refactor with helpers", []string{"refactor", "helpers"}},              // "with" is noise
		{"from this into that", []string{}},                                     // all noise/short
		{"fixed, updated; deployed.", []string{"fixed", "updated", "deployed"}}, // punctuation stripped
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := significantWords(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("significantWords(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("significantWords(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCoveredByNotes(t *testing.T) {
	tests := []struct {
		name      string
		commitMsg string
		notes     []string
		want      bool
	}{
		{
			name:      "commit words mostly in note",
			commitMsg: "refactor authentication module",
			notes:     []string{"Refactored the authentication module to reduce complexity"},
			want:      true,
		},
		{
			name:      "commit words not in notes",
			commitMsg: "implement payment gateway",
			notes:     []string{"Fixed authentication bug"},
			want:      false,
		},
		{
			name:      "empty notes",
			commitMsg: "refactor authentication",
			notes:     []string{},
			want:      false,
		},
		{
			name:      "commit with only short/noise words",
			commitMsg: "fix the bug with this",
			notes:     []string{"Fixed something"},
			want:      false, // no significant words → false
		},
		{
			name:      "partial match across multiple notes",
			commitMsg: "implement search feature",
			notes:     []string{"Added login page", "Implemented search feature with filters"},
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coveredByNotes(tt.commitMsg, tt.notes)
			if got != tt.want {
				t.Errorf("coveredByNotes(%q, %v) = %v, want %v", tt.commitMsg, tt.notes, got, tt.want)
			}
		})
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"feat: add login page", "Add login page"},
		{"fix: resolve crash", "Resolve crash"},
		{"refactor: clean up auth", "Clean up auth"},
		{"chore: bump deps", "Bump deps"},
		{"docs: update readme", "Update readme"},
		{"feat(auth): add OAuth", "Add OAuth"},
		{"unknown: something", "unknown: something"}, // non-standard prefix kept
		{"no prefix at all", "no prefix at all"},
		{"feat: ", "feat: "}, // empty after colon → keep original
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := stripPrefix(tt.msg)
			if got != tt.want {
				t.Errorf("stripPrefix(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestSortedProjects(t *testing.T) {
	byProject := map[string]model.ProjectSummary{
		"low":    {Name: "low", CommitCount: 1, Interactions: 0},
		"medium": {Name: "medium", CommitCount: 3, Interactions: 2},
		"high":   {Name: "high", CommitCount: 5, Interactions: 10},
	}
	got := sortedProjects(byProject)
	if len(got) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(got))
	}
	// high should come first (highest score), low should be last.
	if got[0].Name != "high" {
		t.Errorf("expected high-activity project first, got %q", got[0].Name)
	}
	if got[len(got)-1].Name != "low" {
		t.Errorf("expected low-activity project last, got %q", got[len(got)-1].Name)
	}
}

func TestSynthesizeSmart(t *testing.T) {
	makeDay := func(date time.Time, projects map[string]model.ProjectSummary) model.DaySummary {
		var acts []model.Activity
		for k := range projects {
			acts = append(acts, model.Activity{Project: k, Source: "git"})
		}
		return model.DaySummary{Date: date, Activities: acts, ByProject: projects}
	}

	t.Run("feat commit appears in output", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:           "myproject",
				CommitMessages: []string{"feat: add login page"},
				CommitCount:    1,
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", false)
		if !strings.Contains(got, "Add login page") {
			t.Errorf("expected 'Add login page' in output, got:\n%s", got)
		}
	})

	t.Run("chore commit does NOT appear", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:           "myproject",
				CommitMessages: []string{"chore: bump deps"},
				CommitCount:    1,
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", false)
		if strings.Contains(got, "bump deps") || strings.Contains(got, "Bump deps") {
			t.Errorf("chore commit should not appear, got:\n%s", got)
		}
	})

	t.Run("note with decided keyword appears first", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:         "myproject",
				SessionNotes: []string{"Decided to use gRPC instead of REST for the internal service calls"},
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", false)
		if !strings.Contains(got, "gRPC instead of REST") {
			t.Errorf("expected decided note in output, got:\n%s", got)
		}
	})

	t.Run("note with bare commit hash is dropped", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:         "myproject",
				SessionNotes: []string{"Fixed. a1b2c3d."},
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", false)
		if strings.Contains(got, "Fixed. a1b2c3d") {
			t.Errorf("note with bare hash should not appear, got:\n%s", got)
		}
	})

	t.Run("multi-day with dateLabel is flat, no per-day headers, has summary line", func(t *testing.T) {
		days := []model.DaySummary{
			makeDay(time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
				"proj-a": {Name: "proj-a", CommitMessages: []string{"feat: add search"}, CommitCount: 1},
			}),
			makeDay(time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
				"proj-b": {Name: "proj-b", CommitMessages: []string{"fix: resolve panic"}, CommitCount: 1},
			}),
		}
		got := SynthesizeSmart(days, 2, "Week of Apr 6", false)
		if !strings.Contains(got, "Week of Apr 6") {
			t.Errorf("expected dateLabel header, got:\n%s", got)
		}
		if strings.Contains(got, "Mon, Apr 6 2026") || strings.Contains(got, "Tue, Apr 7 2026") {
			t.Errorf("multi-day should have no per-day date headers, got:\n%s", got)
		}
		// No summary line in SynthesizeSmart
		if strings.Contains(got, "projects ·") {
			t.Errorf("SynthesizeSmart should not contain summary line, got:\n%s", got)
		}
	})

	t.Run("single day has date header, no summary line", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:         "myproject",
				SessionNotes: []string{"Implemented the new authentication module using JWT tokens"},
				CommitCount:  2,
				Interactions: 3,
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", false)
		if !strings.Contains(got, "Wed, Apr 8 2026") {
			t.Errorf("expected single-day date header, got:\n%s", got)
		}
		if strings.Contains(got, "active") {
			t.Errorf("single-day should not have summary line, got:\n%s", got)
		}
	})

	t.Run("empty days returns noActivity", func(t *testing.T) {
		got := SynthesizeSmart([]model.DaySummary{}, 0, "", false)
		if got != noActivity {
			t.Errorf("expected noActivity, got: %q", got)
		}
	})

	t.Run("session note covered by commit — commit not duplicated", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:           "myproject",
				SessionNotes:   []string{"Added authentication module with JWT token support for the API"},
				CommitMessages: []string{"feat: add authentication module"},
				CommitCount:    1,
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", false)
		count := strings.Count(got, "authentication")
		if count > 1 {
			t.Errorf("authentication should appear only once (note wins over commit), got:\n%s", got)
		}
	})

	t.Run("slack format uses bold header and dash bullets", func(t *testing.T) {
		day := makeDay(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), map[string]model.ProjectSummary{
			"myproject": {
				Name:           "myproject",
				CommitMessages: []string{"feat: add login page"},
				CommitCount:    1,
			},
		})
		got := SynthesizeSmart([]model.DaySummary{day}, 1, "", true)
		if !strings.Contains(got, "`Wed, Apr 8 2026`") {
			t.Errorf("slack mode: expected backtick date header, got:\n%s", got)
		}
		if !strings.Contains(got, "- Add login page") {
			t.Errorf("slack mode: expected '- ' bullet prefix, got:\n%s", got)
		}
	})
}
