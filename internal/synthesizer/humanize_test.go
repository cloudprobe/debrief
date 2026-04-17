package synthesizer

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
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
	got := humanizeBullets(context.Background(), items, f)
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
	// Returns 2 items for 3 inputs: accepted (parsed <= items).
	// The new contract allows merged/dropped output; only parsed > items falls back.
	f := &fakeHumanizer{
		out: "1. Fixed the auth bug\n2. Shipped the login page",
	}
	items := []string{"a longer original item one", "b longer original item two", "c longer original item three"}
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 2 {
		t.Fatalf("expected 2 (merged output accepted), got %d", len(got))
	}
	if got[0] != "Fixed the auth bug" {
		t.Errorf("[0] = %q, want %q", got[0], "Fixed the auth bug")
	}
	if got[1] != "Shipped the login page" {
		t.Errorf("[1] = %q, want %q", got[1], "Shipped the login page")
	}
}

func TestHumanizeBulletsEmptyStdout(t *testing.T) {
	f := &fakeHumanizer{out: ""}
	items := []string{"first item to return as original", "second item to return as original"}
	got := humanizeBullets(context.Background(), items, f)
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
	got := humanizeBullets(context.Background(), items, f)
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
	got := humanizeBullets(context.Background(), items, f)
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want original %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsEmptyInput(t *testing.T) {
	f := &fakeHumanizer{out: "1. something"}
	got := humanizeBullets(context.Background(), nil, f)
	if len(got) != 0 {
		t.Fatalf("expected empty slice for nil input, got %v", got)
	}
	if f.calls != 0 {
		t.Fatalf("runner should not be called for empty input, got %d calls", f.calls)
	}

	f2 := &fakeHumanizer{out: "1. something"}
	got2 := humanizeBullets(context.Background(), []string{}, f2)
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
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if !strings.Contains(got[0], "#42") {
		t.Errorf("PR number #42 not preserved in %q", got[0])
	}
}

// TestHumanizeBulletsHappyPathNoMarkdown is a Slack regression guard: the humanizer
// prompt instructs Claude not to produce markdown emphasis. When the runner returns
// clean output, no bullet should contain *, _, or backtick emphasis characters.
func TestHumanizeBulletsHappyPathNoMarkdown(t *testing.T) {
	f := &fakeHumanizer{
		out: "1. Fixed the auth bug\n2. Shipped the login page\n3. Went with gRPC over REST",
	}
	items := []string{
		"Fixed the authentication bug in the module",
		"feat: add login page",
		"Decided to utilize gRPC instead of REST",
	}
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(got), got)
	}
	markdownEmphasis := regexp.MustCompile(`[*_` + "`" + `]`)
	for i, bullet := range got {
		if markdownEmphasis.MatchString(bullet) {
			t.Errorf("bullet[%d] %q contains markdown emphasis character", i, bullet)
		}
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
	got := humanizeBullets(context.Background(), items, f)
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

// --- Bolder bullet mode: parsed < items is now accepted ---

func TestHumanizeBulletsFewerOutputAccepted(t *testing.T) {
	// Fake returns 2 bullets for 3 inputs (merged a near-duplicate).
	// New behavior: accept the shorter slice.
	f := &fakeHumanizer{
		out: "1. Fixed review blockers around correctness and error handling\n2. Shipped the new auth module",
	}
	items := []string{
		"Fixed review blockers around correctness, security, and error handling",
		"Fixed review blockers",
		"Shipped the new auth module",
	}
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 2 {
		t.Fatalf("expected 2 (merged) items, got %d: %v", len(got), got)
	}
	if got[0] != "Fixed review blockers around correctness and error handling" {
		t.Errorf("[0] = %q, unexpected", got[0])
	}
	if got[1] != "Shipped the new auth module" {
		t.Errorf("[1] = %q, unexpected", got[1])
	}
}

func TestHumanizeBulletsMoreOutputFallsBack(t *testing.T) {
	// Fake returns MORE lines than inputs — must fall back.
	f := &fakeHumanizer{
		out: "1. First item\n2. Second item\n3. Third item\n4. Extra item",
	}
	items := []string{"original one", "original two", "original three"}
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 3 {
		t.Fatalf("expected 3 originals on over-count, got %d", len(got))
	}
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want original %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsZeroOutputFallsBack(t *testing.T) {
	// Empty parse (0 lines matched) still falls back.
	f := &fakeHumanizer{out: "Some non-numbered text here with no list items at all."}
	items := []string{"original one item", "original two item"}
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 2 {
		t.Fatalf("expected 2 originals on zero parse, got %d", len(got))
	}
	for i, want := range items {
		if got[i] != want {
			t.Errorf("[%d] = %q, want original %q", i, got[i], want)
		}
	}
}

func TestHumanizeBulletsURLDropPassThrough(t *testing.T) {
	// Fake drops the bare URL bullet — output has 1 line for 2 inputs.
	// Should pass through (parsed < items is now accepted).
	f := &fakeHumanizer{
		out: "1. Opened PR #12 to fix the auth flow",
	}
	items := []string{
		"https://github.com/cloudprobe/cloudprobe",
		"Opened PR #12 to fix the auth flow",
	}
	got := humanizeBullets(context.Background(), items, f)
	if len(got) != 1 {
		t.Fatalf("expected 1 item after URL drop, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "#12") {
		t.Errorf("PR number not preserved: %q", got[0])
	}
}

// --- humanizeAsProse tests ---

func TestHumanizeAsProseHappyPath(t *testing.T) {
	prose := "Spent most of the day fixing auth flow issues and reviewing CodeRabbit feedback.\n\nOpened PR #12 and addressed all the correctness concerns raised in review.\n\nAlso merged the search feature that had been sitting in review."
	f := &fakeHumanizer{out: prose}
	items := []string{
		"Fixed review blockers around correctness and error handling",
		"Opened PR #12 to fix the auth flow",
		"Merged search feature after review",
	}
	got, ok := humanizeAsProse(context.Background(), items, f)
	if !ok {
		t.Fatal("expected ok=true on happy path")
	}
	if got != prose {
		t.Errorf("got %q, want %q", got, prose)
	}
}

func TestHumanizeAsProseErrorFromRunner(t *testing.T) {
	f := &fakeHumanizer{out: "some text", err: context.DeadlineExceeded}
	items := []string{"Fixed the auth bug in the module"}
	got, ok := humanizeAsProse(context.Background(), items, f)
	if ok {
		t.Fatal("expected ok=false on runner error")
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestHumanizeAsProseEmptyStdout(t *testing.T) {
	f := &fakeHumanizer{out: ""}
	items := []string{"Fixed the auth bug in the module"}
	got, ok := humanizeAsProse(context.Background(), items, f)
	if ok {
		t.Fatal("expected ok=false on empty stdout")
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestHumanizeAsProseEmptyInput(t *testing.T) {
	f := &fakeHumanizer{out: "something"}
	got, ok := humanizeAsProse(context.Background(), []string{}, f)
	if ok {
		t.Fatal("expected ok=false on empty input")
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if f.calls != 0 {
		t.Fatalf("runner should not be called for empty input, got %d calls", f.calls)
	}
}

// --- SynthesizeSmartWith prose integration ---

func TestSynthesizeSmartWithProseTrue(t *testing.T) {
	proseText := "Spent the day fixing auth issues and reviewing PRs.\n\nMerged search feature after addressing all feedback."
	f := &fakeHumanizer{out: proseText}

	days := []model.DaySummary{
		{
			Date: mustDate("2026-04-08"),
			ByProject: map[string]model.ProjectSummary{
				"myproject": {
					Name:           "myproject",
					CommitMessages: []string{"feat: add search feature", "fix: resolve auth crash"},
					CommitCount:    2,
				},
			},
		},
	}
	got := SynthesizeSmartWith(context.Background(), days, "", false, true, f)
	if !strings.Contains(got, proseText) {
		t.Errorf("expected prose text in output, got:\n%s", got)
	}
	// Must not contain bullet prefix.
	if strings.Contains(got, "  - ") {
		t.Errorf("prose mode should not contain bullet prefix, got:\n%s", got)
	}
}

func TestSynthesizeSmartWithProseFallsBackToBullets(t *testing.T) {
	f := &fakeHumanizer{out: "", err: context.DeadlineExceeded}

	days := []model.DaySummary{
		{
			Date: mustDate("2026-04-08"),
			ByProject: map[string]model.ProjectSummary{
				"myproject": {
					Name:           "myproject",
					CommitMessages: []string{"feat: add search feature"},
					CommitCount:    1,
				},
			},
		},
	}
	got := SynthesizeSmartWith(context.Background(), days, "", false, true, f)
	// Should fall back to bullets.
	if !strings.Contains(got, "  - ") {
		t.Errorf("expected bullet fallback on prose failure, got:\n%s", got)
	}
	if !strings.Contains(got, "Add search feature") {
		t.Errorf("expected bullet content on prose failure, got:\n%s", got)
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
