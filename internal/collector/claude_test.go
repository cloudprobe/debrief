package collector

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

func TestClaudeCollector_ParseSampleFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testdataDir := filepath.Join(wd, "..", "..", "testdata")

	c := &ClaudeCollector{homeDir: "", showCost: true, pricingTable: directTable()}
	dr := model.DateRange{
		Start: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
	}

	accums := make(map[string]*projectAccum)
	assistantMsgs := make(map[string]deferredMsg)
	if err := c.scanFile(filepath.Join(testdataDir, "claude_sample.jsonl"), dr, time.UTC, accums, assistantMsgs); err != nil {
		t.Fatalf("scanFile: %v", err)
	}
	activities := c.buildActivities(accums, assistantMsgs, time.UTC)

	if len(activities) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(activities))
	}

	tests := []struct {
		name          string
		project       string
		model         string
		tokensIn      int
		tokensOut     int
		interactions  int
		branch        string
		filesCreated  int
		filesModified int
		toolCount     int
	}{
		{
			name:          "myproject - sonnet",
			project:       "myproject",
			model:         "claude-sonnet-4-6",
			tokensIn:      3500,
			tokensOut:     1300,
			interactions:  2,
			branch:        "main",
			filesCreated:  1, // Write → cmd/main.go (relative)
			filesModified: 1, // Edit → cmd/main_test.go (relative)
			toolCount:     3, // Write + Edit + Read
		},
		{
			name:          "other-project - opus",
			project:       "other-project",
			model:         "claude-opus-4-6",
			tokensIn:      3000,
			tokensOut:     1000,
			interactions:  1,
			branch:        "feature/auth",
			filesCreated:  0,
			filesModified: 0,
			toolCount:     1, // Bash
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found *model.Activity
			for i := range activities {
				if activities[i].Project == tt.project {
					found = &activities[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("project %s not found in activities", tt.project)
				return // unreachable; keeps staticcheck SA5011 quiet
			}
			if found.Model != tt.model {
				t.Errorf("model: got %q, want %q", found.Model, tt.model)
			}
			if found.TokensIn != tt.tokensIn {
				t.Errorf("tokens_in: got %d, want %d", found.TokensIn, tt.tokensIn)
			}
			if found.TokensOut != tt.tokensOut {
				t.Errorf("tokens_out: got %d, want %d", found.TokensOut, tt.tokensOut)
			}
			if found.Interactions != tt.interactions {
				t.Errorf("interactions: got %d, want %d", found.Interactions, tt.interactions)
			}
			if found.Branch != tt.branch {
				t.Errorf("branch: got %q, want %q", found.Branch, tt.branch)
			}
			if found.Source != "claude-code" {
				t.Errorf("source: got %q, want %q", found.Source, "claude-code")
			}
			if found.CostUSD <= 0 {
				t.Error("cost should be positive")
			}
			if len(found.FilesCreated) != tt.filesCreated {
				t.Errorf("files_created: got %d, want %d (%v)", len(found.FilesCreated), tt.filesCreated, found.FilesCreated)
			}
			if len(found.FilesModified) != tt.filesModified {
				t.Errorf("files_modified: got %d, want %d (%v)", len(found.FilesModified), tt.filesModified, found.FilesModified)
			}
			var totalTools int
			for _, v := range found.ToolBreakdown {
				totalTools += v
			}
			if totalTools != tt.toolCount {
				t.Errorf("tool_count: got %d, want %d (%v)", totalTools, tt.toolCount, found.ToolBreakdown)
			}

			// Verify file paths are relative, not absolute.
			for _, fp := range found.FilesCreated {
				if filepath.IsAbs(fp) {
					t.Errorf("file_created should be relative, got absolute: %s", fp)
				}
			}
			for _, fp := range found.FilesModified {
				if filepath.IsAbs(fp) {
					t.Errorf("file_modified should be relative, got absolute: %s", fp)
				}
			}
		})
	}
}

func TestClaudeInternal_Filtered(t *testing.T) {
	if !isClaudeInternal("/Users/test/.claude/plans/foo.md") {
		t.Error("expected .claude/plans path to be filtered")
	}
	if !isClaudeInternal("/Users/test/.claude/projects/memory/bar.md") {
		t.Error("expected .claude/projects/memory path to be filtered")
	}
	if isClaudeInternal("/Users/test/myproject/internal/model.go") {
		t.Error("regular file should not be filtered")
	}
}

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		tokensIn   int
		tokensOut  int
		cacheRead  int
		cacheWrite int
		wantCost   float64
		wantKnown  bool
	}{
		{
			name:       "opus basic",
			model:      "claude-opus-4-6",
			tokensIn:   1000,
			tokensOut:  500,
			cacheRead:  0,
			cacheWrite: 0,
			wantCost:   (1000.0*5 + 500.0*25) / 1_000_000,
			wantKnown:  true,
		},
		{
			name:       "sonnet with cache",
			model:      "claude-sonnet-4-6",
			tokensIn:   1000,
			tokensOut:  500,
			cacheRead:  200,
			cacheWrite: 100,
			// input: 1000 * 3/1M = 0.003, output: 500 * 15/1M = 0.0075
			// cache read: 200 * 3 * 0.10 / 1M = 0.00006
			// cache write: 100 * 3 * 1.25 / 1M = 0.000375
			wantCost:  0.003 + 0.0075 + 0.00006 + 0.000375,
			wantKnown: true,
		},
		{
			name:      "unknown model",
			model:     "unknown-model",
			tokensIn:  1000,
			tokensOut: 500,
			wantCost:  0,
			wantKnown: false,
		},
		{
			// claude-sonnet-4-5-20250929 isn't in the table but its base name
			// claude-sonnet-4-5-20250514 is — fallback should match.
			name:      "sonnet date-versioned fallback (dash)",
			model:     "claude-sonnet-4-5-20250929",
			tokensIn:  1000,
			tokensOut: 500,
			wantCost:  (1000.0*3 + 500.0*15) / 1_000_000,
			wantKnown: true,
		},
		{
			// Same fallback but with @ separator (Vertex AI style).
			name:      "haiku date-versioned fallback (at)",
			model:     "claude-haiku-4-5@20261201",
			tokensIn:  1000,
			tokensOut: 500,
			wantCost:  (1000.0*1 + 500.0*5) / 1_000_000,
			wantKnown: true,
		},
	}

	table := directTable()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := CalculateCost(table, tt.model, tt.tokensIn, tt.tokensOut, tt.cacheRead, tt.cacheWrite)
			if known != tt.wantKnown {
				t.Errorf("CalculateCost known = %v, want %v", known, tt.wantKnown)
			}
			if math.Abs(got-tt.wantCost) > 0.000001 {
				t.Errorf("CalculateCost = %f, want %f", got, tt.wantCost)
			}
		})
	}
}

func TestProjectFromCWD(t *testing.T) {
	tests := []struct {
		cwd  string
		want string
	}{
		{"/Users/test/work/myproject", "myproject"},
		{"/home/user/code/backend", "backend"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		got := projectFromCWD(tt.cwd)
		if got != tt.want {
			t.Errorf("projectFromCWD(%q) = %q, want %q", tt.cwd, got, tt.want)
		}
	}
}

func TestSessionNotes(t *testing.T) {
	tests := []struct {
		name      string
		blocks    []string // text block contents to feed
		wantCount int      // expected number of notes
		wantAll   bool     // true = all qualifying blocks must appear
	}{
		{
			name:      "single qualifying block",
			blocks:    []string{"Fixed the auth refactor. The implementation is now complete."},
			wantCount: 1,
		},
		{
			name: "multiple qualifying blocks all collected",
			blocks: []string{
				"Fixed the auth refactor. The implementation is now complete.",
				"Updated the token refresh logic. All tests pass.",
			},
			wantCount: 2,
			wantAll:   true,
		},
		{
			name:      "non-qualifying block skipped",
			blocks:    []string{"sure", "Fixed the auth refactor. The implementation is now complete."},
			wantCount: 1,
		},
		{
			name:      "no qualifying blocks",
			blocks:    []string{"sure", "ok", "understood"},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notes []string
			for _, text := range tt.blocks {
				if note := extractSessionNote(text); note != "" {
					notes = append(notes, note)
				}
			}
			if len(notes) < tt.wantCount {
				t.Errorf("got %d notes, want at least %d; notes: %v", len(notes), tt.wantCount, notes)
			}
			if tt.wantAll && len(notes) != tt.wantCount {
				t.Errorf("wantAll=true: got %d notes, want exactly %d; notes: %v", len(notes), tt.wantCount, notes)
			}
			if !tt.wantAll && tt.wantCount == 0 && len(notes) != 0 {
				t.Errorf("expected no notes, got %d: %v", len(notes), notes)
			}
		})
	}
}

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"**bold text**", "bold text"},
		{"*italic*", "italic"},
		{"__underline__", "underline"},
		{"_emphasis_", "emphasis"},
		{"`inline code`", "inline code"},
		{"- list item", "list item"},
		{"• bullet item", "bullet item"},
		{"**bold** and *italic*", "bold and italic"},
		{"unterminated **bold", "unterminated bold"},
		{"unterminated `code", "unterminated code"},
		{"plain text", "plain text"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("stripMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanNote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Done. README now shows the version", "README now shows the version"},
		{"Done — removed from tracking", "Removed from tracking"},
		{"Done: config saved", "Config saved"},
		{"Done, tests pass", "Tests pass"},
		{"Done! All good", "All good"},
		{"Done.", ""},
		{"Done", ""},
		{"Done!", ""},
		{"I've added the feature", "Added the feature"},
		{"I have updated the config", "Updated the config"},
		{"Fixed the bug", "Fixed the bug"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanNote(tt.input)
			if got != tt.want {
				t.Errorf("cleanNote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNumberedListSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"captured items: 1.", true},
		{"step 2.", true},
		{"the following: 12.", true},
		{"plain sentence.", false},
		{"ends with number 42", false},
		{"", false},
		{"a.", false},  // single letter, not digit
		{"a 1", false}, // no trailing period
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := numberedListSuffix(tt.input)
			if got != tt.want {
				t.Errorf("numberedListSuffix(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
