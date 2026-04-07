package synthesis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

// fakeExecutor is a test double for Executor.
type fakeExecutor struct {
	out     string
	err     error
	blockFn func(ctx context.Context) // if set, called before returning
}

func (f *fakeExecutor) Run(ctx context.Context, stdin string) (string, error) {
	if f.blockFn != nil {
		f.blockFn(ctx)
	}
	return f.out, f.err
}

func sampleDays() []model.DaySummary {
	return []model.DaySummary{
		{
			Date: time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
			ByProject: map[string]model.ProjectSummary{
				"debrief": {
					Name:           "debrief",
					CommitCount:    2,
					CommitMessages: []string{"feat: add synthesis", "fix: correct field names"},
					Interactions:   5,
					SessionNotes:   []string{"implemented synthesis pipeline"},
				},
			},
		},
	}
}

func TestSynthesize_Success(t *testing.T) {
	canned := "Tuesday Apr 7 — debrief\n\nShipped\n  - implemented synthesis pipeline"
	fake := &fakeExecutor{out: canned}
	opts := Options{Executor: fake}

	out, err := Synthesize(context.Background(), sampleDays(), 1, "", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be trimmed output + newline
	want := strings.TrimSpace(canned) + "\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestSynthesize_EmptyOutput(t *testing.T) {
	fake := &fakeExecutor{out: ""}
	opts := Options{Executor: fake}

	_, err := Synthesize(context.Background(), sampleDays(), 1, "", opts)
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("expected ErrEmptyOutput, got %v", err)
	}
}

func TestSynthesize_ExecutorError(t *testing.T) {
	execErr := errors.New("exit status 1")
	fake := &fakeExecutor{err: execErr}
	opts := Options{Executor: fake}

	_, err := Synthesize(context.Background(), sampleDays(), 1, "", opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "synthesis failed") {
		t.Errorf("expected wrapped error containing 'synthesis failed', got: %v", err)
	}
	if !errors.Is(err, execErr) {
		t.Errorf("expected error chain to contain execErr, got: %v", err)
	}
}

func TestSynthesize_NoClaude(t *testing.T) {
	// Clear PATH so claude binary cannot be found.
	t.Setenv("PATH", "")
	opts := Options{} // nil Executor — will attempt to find claude in PATH

	_, err := Synthesize(context.Background(), sampleDays(), 1, "", opts)
	if !errors.Is(err, ErrNoClaude) {
		t.Errorf("expected ErrNoClaude, got %v", err)
	}
}

func TestSynthesize_Timeout(t *testing.T) {
	// Executor blocks until context is cancelled.
	fake := &fakeExecutor{
		blockFn: func(ctx context.Context) {
			<-ctx.Done()
		},
		err: context.DeadlineExceeded,
	}
	opts := Options{
		Executor: fake,
		Timeout:  10 * time.Millisecond,
	}

	ctx := context.Background()
	_, err := Synthesize(ctx, sampleDays(), 1, "", opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should be a synthesis failure wrapping a context error
	if !strings.Contains(err.Error(), "synthesis failed") {
		t.Errorf("expected 'synthesis failed' in error, got: %v", err)
	}
}
