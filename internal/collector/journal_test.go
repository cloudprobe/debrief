package collector

import (
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/journal"
	"github.com/cloudprobe/debrief/internal/model"
)

func TestJournalCollector_Name(t *testing.T) {
	jc := NewJournalCollector(t.TempDir())
	if jc.Name() != "journal" {
		t.Errorf("Name() = %q, want %q", jc.Name(), "journal")
	}
}

func TestJournalCollector_Available_MissingDir(t *testing.T) {
	jc := NewJournalCollector(t.TempDir())
	// journal subdir doesn't exist yet
	if jc.Available() {
		t.Error("Available() should be false when journal dir does not exist")
	}
}

func TestJournalCollector_Available_PresentDir(t *testing.T) {
	dir := t.TempDir()
	// Write an entry to create the journal dir.
	if err := journal.Append(dir, time.Now(), "hello"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	jc := NewJournalCollector(dir)
	if !jc.Available() {
		t.Error("Available() should be true when journal dir exists")
	}
}

func TestJournalCollector_Collect_Empty(t *testing.T) {
	dir := t.TempDir()
	jc := NewJournalCollector(dir)
	dr := model.DateRange{
		Start: time.Now().Truncate(24 * time.Hour),
		End:   time.Now().Truncate(24 * time.Hour),
	}
	activities, err := jc.Collect(dr)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(activities) != 0 {
		t.Errorf("expected 0 activities, got %d", len(activities))
	}
}

func TestJournalCollector_Collect_ReturnsEntries(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	if err := journal.Append(dir, day, "decided to use postgres"); err != nil {
		t.Fatalf("setup append: %v", err)
	}
	if err := journal.Append(dir, day, "found a race condition"); err != nil {
		t.Fatalf("setup append: %v", err)
	}

	jc := NewJournalCollector(dir)
	dr := model.DateRange{Start: day, End: day}
	activities, err := jc.Collect(dr)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}
	act := activities[0]
	if act.Source != "journal" {
		t.Errorf("Source = %q, want %q", act.Source, "journal")
	}
	if act.Project != "journal" {
		t.Errorf("Project = %q, want %q", act.Project, "journal")
	}
	if len(act.SessionNotes) != 2 {
		t.Errorf("SessionNotes len = %d, want 2", len(act.SessionNotes))
	}
}

func TestJournalCollector_Collect_MultiDay(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 16, 9, 0, 0, 0, time.UTC)

	if err := journal.Append(dir, day1, "note on day 1"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := journal.Append(dir, day2, "note on day 2"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	jc := NewJournalCollector(dir)
	dr := model.DateRange{Start: day1, End: day2}
	activities, err := jc.Collect(dr)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(activities) != 2 {
		t.Errorf("expected 2 activities, got %d", len(activities))
	}
}
