package synthesizer

import (
	"context"
	"strings"
	"testing"
)

// fakeHumanizer is a test double for humanizer.Humanizer.
type fakeHumanizer struct {
	out   string
	err   error
	calls int
}

func (f *fakeHumanizer) Rewrite(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.out, f.err
}

func TestHumanizeBulletsHappyPath(t *testing.T) {
	f := &fakeHumanizer{
		out: "1. Fixed the auth bug\n2. Shipped the login page\n3. Went with gRPC",
	}
	items := []string{
		"Fixed the authentication bug in the module",
		"feat: add login page",
		"Decided to utilize gRPC instead of REST",
	}
	got := humanizeBullets(items, f)
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(got), got)
	}
	if got[0] != "Fixed the auth bug" {
		t.Errorf("[0] = %q, want %q", got[0], "Fixed the auth bug")
	}
	if got[1] != "Shipped the login page" {
		t.Errorf("[1] = %q, want %q", got[1], "Shipped the login page")
	}
	if got[2] != "Went with gRPC" {
		t.Errorf("[2] = %q, want %q", got[2], "Went with gRPC")
	}
}

func TestHumanizeBulletsCountMismatch(t *testing.T) {
	f := &fakeHumanizer{
		// Returns only 2 items for 3 inputs → fall back to originals.
		out: "1. Fixed the auth bug\n2. Shipped the login page",
	}
	items := []string{"a longer original item one", "b longer original item two", "c longer original item three"}
	got := humanizeBullets(items, f)
	if len(got) != 3 {
		t.Fatalf("expected 3 (originals), got %d", len(got))
	}
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want original %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsEmptyStdout(t *testing.T) {
	f := &fakeHumanizer{out: ""}
	items := []string{"first item to return as original", "second item to return as original"}
	got := humanizeBullets(items, f)
	if len(got) != 2 {
		t.Fatalf("expected 2 (originals), got %d", len(got))
	}
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsErrorFromRunner(t *testing.T) {
	f := &fakeHumanizer{
		out: "1. something rewritten\n2. something else\n3. third one",
		err: context.DeadlineExceeded,
	}
	items := []string{"original one here for test", "original two here for test", "original three for test"}
	got := humanizeBullets(items, f)
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want original %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsScrambledNumbering(t *testing.T) {
	// Out-of-order numbering must cause fallback.
	f := &fakeHumanizer{
		out: "2. Something first\n1. Something second\n3. Something third",
	}
	items := []string{"original first item here", "original second item here", "original third item here"}
	got := humanizeBullets(items, f)
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want original %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsEmptyInput(t *testing.T) {
	f := &fakeHumanizer{out: "1. something"}
	got := humanizeBullets(nil, f)
	if len(got) != 0 {
		t.Fatalf("expected empty slice for nil input, got %v", got)
	}
	if f.calls != 0 {
		t.Fatalf("runner should not be called for empty input, got %d calls", f.calls)
	}

	f2 := &fakeHumanizer{out: "1. something"}
	got2 := humanizeBullets([]string{}, f2)
	if len(got2) != 0 {
		t.Fatalf("expected empty slice for empty input, got %v", got2)
	}
	if f2.calls != 0 {
		t.Fatalf("runner should not be called for empty input, got %d calls", f2.calls)
	}
}

func TestHumanizeBulletsMergeCommitPRPreservation(t *testing.T) {
	f := &fakeHumanizer{
		out: "1. Merged #42 from foo/bar",
	}
	items := []string{"Merge pull request #42 from foo/bar"}
	got := humanizeBullets(items, f)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if !strings.Contains(got[0], "#42") {
		t.Errorf("PR number #42 not preserved in %q", got[0])
	}
}

// TestHumanizeBulletsNoMarkdownStripping documents that the parser accepts lines
// containing markdown emphasis — stripping markdown is a prompt-level constraint,
// not a parser responsibility.
func TestHumanizeBulletsNoMarkdownStripping(t *testing.T) {
	f := &fakeHumanizer{
		out: "1. Fixed *bold* issue in module",
	}
	items := []string{"Fixed the bold issue in the module"}
	got := humanizeBullets(items, f)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	// Parser must accept and return the line as-is (markdown not stripped by parser).
	if got[0] != "Fixed *bold* issue in module" {
		t.Errorf("got %q, want %q", got[0], "Fixed *bold* issue in module")
	}
}

func TestHumanizeBulletsPromptTooLarge(t *testing.T) {
	f := &fakeHumanizer{out: "1. something"}
	// Build items that, when joined into a prompt, exceed 100 KB.
	item := strings.Repeat("x", 1000)
	items := make([]string, 200)
	for i := range items {
		items[i] = item
	}
	got := humanizeBullets(items, f)
	if f.calls != 0 {
		t.Fatalf("runner must not be called when prompt > 100KB, got %d calls", f.calls)
	}
	if len(got) != len(items) {
		t.Fatalf("expected %d originals, got %d", len(items), len(got))
	}
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] mismatch", i)
		}
	}
}
