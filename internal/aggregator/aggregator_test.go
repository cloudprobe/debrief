package aggregator

import (
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

func TestAggregate_Empty(t *testing.T) {
	s := Aggregate(nil)
	if len(s.Activities) != 0 {
		t.Error("expected no activities")
	}
	if len(s.ByProject) != 0 {
		t.Error("expected no projects")
	}
}

func TestAggregate_SingleActivity(t *testing.T) {
	now := time.Now()
	activities := []model.Activity{
		{
			Source:       "claude-code",
			Project:      "myproject",
			Model:        "claude-opus-4-6",
			Timestamp:    now,
			EndTime:      now.Add(10 * time.Minute),
			TokensIn:     1000,
			TokensOut:    500,
			CostUSD:      0.05,
			Interactions: 5,
			FilesCreated: []string{"main.go"},
			ToolBreakdown: map[string]int{
				"Write": 3,
				"Read":  2,
			},
		},
	}

	s := Aggregate(activities)

	if s.TotalTokens != 1500 {
		t.Errorf("total_tokens: got %d, want 1500", s.TotalTokens)
	}
	if s.Interactions != 5 {
		t.Errorf("interactions: got %d, want 5", s.Interactions)
	}
	if s.TotalCost != 0.05 {
		t.Errorf("total_cost: got %f, want 0.05", s.TotalCost)
	}

	p, ok := s.ByProject["myproject"]
	if !ok {
		t.Fatal("myproject not found")
	}
	if p.TotalTokens != 1500 {
		t.Errorf("project total_tokens: got %d, want 1500", p.TotalTokens)
	}
	if len(p.FilesCreated) != 1 || p.FilesCreated[0] != "main.go" {
		t.Errorf("files_created: got %v", p.FilesCreated)
	}
	if p.ToolBreakdown["Write"] != 3 {
		t.Errorf("tool Write: got %d, want 3", p.ToolBreakdown["Write"])
	}
	if len(p.Models) != 1 || p.Models[0] != "claude-opus-4-6" {
		t.Errorf("models: got %v", p.Models)
	}

	m, ok := s.ByModel["claude-opus-4-6"]
	if !ok {
		t.Fatal("model not found")
	}
	if m.TokensIn != 1000 || m.TokensOut != 500 {
		t.Errorf("model tokens: got in=%d out=%d", m.TokensIn, m.TokensOut)
	}
}

func TestAggregate_MultipleProjects(t *testing.T) {
	now := time.Now()
	activities := []model.Activity{
		{
			Source:        "claude-code",
			Project:       "projectA",
			Model:         "claude-opus-4-6",
			Timestamp:     now,
			TokensIn:      1000,
			TokensOut:     500,
			Interactions:  3,
			FilesCreated:  []string{"a.go"},
			FilesModified: []string{"b.go"},
			ToolBreakdown: map[string]int{"Write": 1, "Edit": 1},
		},
		{
			Source:         "git",
			Project:        "projectA",
			Timestamp:      now,
			CommitCount:    2,
			CommitMessages: []string{"fix bug", "add test"},
		},
		{
			Source:       "claude-code",
			Project:      "projectB",
			Model:        "claude-sonnet-4-6",
			Timestamp:    now.Add(time.Hour),
			TokensIn:     2000,
			TokensOut:    800,
			Interactions: 7,
		},
	}

	s := Aggregate(activities)

	if len(s.ByProject) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(s.ByProject))
	}
	if s.Interactions != 10 {
		t.Errorf("total interactions: got %d, want 10", s.Interactions)
	}

	pA := s.ByProject["projectA"]
	if pA.CommitCount != 2 {
		t.Errorf("projectA commits: got %d, want 2", pA.CommitCount)
	}
	if len(pA.CommitMessages) != 2 {
		t.Errorf("projectA commit messages: got %d, want 2", len(pA.CommitMessages))
	}
	if len(pA.Sources) != 2 {
		t.Errorf("projectA sources: got %v, want [claude-code git]", pA.Sources)
	}
	if len(pA.FilesCreated) != 1 {
		t.Errorf("projectA files_created: got %d, want 1", len(pA.FilesCreated))
	}

	pB := s.ByProject["projectB"]
	if pB.Interactions != 7 {
		t.Errorf("projectB interactions: got %d, want 7", pB.Interactions)
	}
}

func TestAggregate_DeduplicatesModelsAndFiles(t *testing.T) {
	now := time.Now()
	activities := []model.Activity{
		{
			Project:      "proj",
			Model:        "claude-opus-4-6",
			Source:       "claude-code",
			Timestamp:    now,
			FilesCreated: []string{"main.go", "util.go"},
		},
		{
			Project:      "proj",
			Model:        "claude-opus-4-6",
			Source:       "claude-code",
			Timestamp:    now.Add(time.Minute),
			FilesCreated: []string{"main.go", "config.go"},
		},
	}

	s := Aggregate(activities)
	p := s.ByProject["proj"]

	if len(p.Models) != 1 {
		t.Errorf("models should be deduplicated: got %v", p.Models)
	}
	// Files: main.go, util.go, config.go (main.go deduplicated)
	if len(p.FilesCreated) != 3 {
		t.Errorf("files_created should be deduplicated: got %v", p.FilesCreated)
	}
}
