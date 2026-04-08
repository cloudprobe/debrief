package journal

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestAppend_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 7, 9, 32, 0, 0, time.UTC)

	if err := Append(dir, ts, "decided to use postgres"); err != nil {
		t.Fatalf("Append: %v", err)
	}

	data, err := os.ReadFile(Path(dir, ts))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# Journal — 2026-04-07") {
		t.Errorf("missing header, got:\n%s", content)
	}
	if !strings.Contains(content, "## Entries") {
		t.Errorf("missing Entries section, got:\n%s", content)
	}
	if !strings.Contains(content, "- [09:32] decided to use postgres") {
		t.Errorf("missing bullet, got:\n%s", content)
	}
}

func TestAppend_SecondEntry(t *testing.T) {
	dir := t.TempDir()
	ts1 := time.Date(2026, 4, 7, 9, 32, 0, 0, time.UTC)
	ts2 := time.Date(2026, 4, 7, 10, 15, 0, 0, time.UTC)

	if err := Append(dir, ts1, "first entry"); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := Append(dir, ts2, "second entry"); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	data, err := os.ReadFile(Path(dir, ts1))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	headerCount := strings.Count(content, "# Journal")
	if headerCount != 1 {
		t.Errorf("expected 1 header, got %d, content:\n%s", headerCount, content)
	}
	if !strings.Contains(content, "- [09:32] first entry") {
		t.Errorf("missing first bullet, got:\n%s", content)
	}
	if !strings.Contains(content, "- [10:15] second entry") {
		t.Errorf("missing second bullet, got:\n%s", content)
	}
}

func TestReadEntries_Missing(t *testing.T) {
	dir := t.TempDir()
	entries, err := ReadEntries(dir, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestReadEntries_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 7, 9, 32, 0, 0, time.UTC)
	ts2 := time.Date(2026, 4, 7, 10, 15, 0, 0, time.UTC)

	if err := Append(dir, ts, "first message"); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := Append(dir, ts2, "second message"); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	entries, err := ReadEntries(dir, ts)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Time != "09:32" || entries[0].Message != "first message" {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if entries[1].Time != "10:15" || entries[1].Message != "second message" {
		t.Errorf("entry[1] = %+v", entries[1])
	}
}

func TestReadEntries_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 7, 9, 0, 0, 0, time.UTC)

	// Write a file with mixed valid/invalid lines manually.
	content := "# Journal — 2026-04-07\n\n## Entries\n- [09:00] valid entry\n- [bad] malformed\n- no bracket at all\n- [10:00] another valid\n"
	p := Path(dir, ts)
	if err := os.MkdirAll(strings.TrimSuffix(p, "/2026-04-07.md"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadEntries(dir, ts)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Time != "09:00" || entries[0].Message != "valid entry" {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if entries[1].Time != "10:00" || entries[1].Message != "another valid" {
		t.Errorf("entry[1] = %+v", entries[1])
	}
}

func TestWriteReadLastStandup(t *testing.T) {
	dir := t.TempDir()
	text := "shipped: added postgres support\ndecided: use migrations"
	date := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)

	if err := WriteLastStandup(dir, text, date); err != nil {
		t.Fatalf("WriteLastStandup: %v", err)
	}

	gotText, gotDate, err := ReadLastStandup(dir)
	if err != nil {
		t.Fatalf("ReadLastStandup: %v", err)
	}
	if gotText != text {
		t.Errorf("text mismatch: got %q, want %q", gotText, text)
	}
	if !gotDate.Equal(date) {
		t.Errorf("date mismatch: got %v, want %v", gotDate, date)
	}
}

func TestReadLastStandup_Missing(t *testing.T) {
	dir := t.TempDir()

	text, date, err := ReadLastStandup(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if !date.IsZero() {
		t.Errorf("expected zero date, got %v", date)
	}
}

func TestAppend_EmptyMessage(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 7, 9, 0, 0, 0, time.UTC)

	if err := Append(dir, ts, ""); err != nil {
		t.Fatalf("Append with empty message: %v", err)
	}

	entries, err := ReadEntries(dir, ts)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Time != "09:00" {
		t.Errorf("time mismatch: %q", entries[0].Time)
	}
}
