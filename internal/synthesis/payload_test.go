package synthesis

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

func dayFixture() model.DaySummary {
	return model.DaySummary{
		Date:      time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
		TotalCost: 0.05,
		ByModel: map[string]model.ModelSummary{
			"claude-sonnet-4": {TotalCost: 0.05},
		},
		ByProject: map[string]model.ProjectSummary{
			"debrief": {
				Name:           "debrief",
				CommitCount:    3,
				CommitMessages: []string{"feat: synthesis", "fix: fields", "chore: tests"},
				Insertions:     120,
				Deletions:      30,
				Interactions:   8,
				SessionNotes:   []string{"built synthesis pipeline", "fixed model field names"},
				FilesCreated:   []string{"internal/synthesis/claude.go"},
				FilesModified:  []string{"cmd/debrief/main.go"},
			},
		},
	}
}

func TestBuildPayload_Basic(t *testing.T) {
	days := []model.DaySummary{dayFixture()}
	result := BuildPayload(days, 1, "", 0)

	if !strings.Contains(result, "=== 2026-04-07") {
		t.Errorf("expected date header '=== 2026-04-07', got:\n%s", result)
	}
	if !strings.Contains(result, "project: debrief") {
		t.Errorf("expected 'project: debrief', got:\n%s", result)
	}
	if !strings.Contains(result, "session_notes:") {
		t.Errorf("expected 'session_notes:', got:\n%s", result)
	}
}

func TestBuildPayload_Truncation(t *testing.T) {
	// Build a fixture large enough to exceed 50KB by adding many session notes.
	notes := make([]string, 500)
	for i := range notes {
		notes[i] = strings.Repeat("this is a very long session note that takes up space in the payload buffer ", 3)
	}
	msgs := make([]string, 200)
	for i := range msgs {
		msgs[i] = "feat: commit message number " + strings.Repeat("x", 80)
	}

	day := model.DaySummary{
		Date: time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
		ByProject: map[string]model.ProjectSummary{
			"bigproject": {
				Name:           "bigproject",
				CommitCount:    200,
				CommitMessages: msgs,
				Interactions:   100,
				SessionNotes:   notes,
			},
		},
	}

	maxBytes := 50_000
	result := BuildPayload([]model.DaySummary{day}, 1, "big range", maxBytes)

	if len(result) > maxBytes {
		t.Errorf("payload exceeds maxBytes: got %d, want <= %d", len(result), maxBytes)
	}
	// Top project should still be present
	if !strings.Contains(result, "project: bigproject") && !strings.Contains(result, "[truncated]") {
		t.Errorf("expected top project or truncation marker in result")
	}
}

func TestBuildPayload_Empty(t *testing.T) {
	result := BuildPayload([]model.DaySummary{}, 1, "", 0)
	if result == "" {
		t.Error("expected non-empty string for empty days slice, got empty string")
	}
	// Should not panic and should return something (at least a newline from header section)
	_ = result
}

func TestBuildPayload_NeverExceedsMax(t *testing.T) {
	cases := []struct {
		name     string
		days     []model.DaySummary
		maxBytes int
	}{
		{"single day", []model.DaySummary{dayFixture()}, 100},
		{"single day", []model.DaySummary{dayFixture()}, 1000},
		{"single day", []model.DaySummary{dayFixture()}, 50_000},
		{"empty", []model.DaySummary{}, 50_000},
		{"tiny max", []model.DaySummary{dayFixture()}, 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildPayload(tc.days, 1, "test period", tc.maxBytes)
			if len(result) > tc.maxBytes {
				t.Errorf("maxBytes=%d: payload length %d exceeds limit", tc.maxBytes, len(result))
			}
		})
	}
}
