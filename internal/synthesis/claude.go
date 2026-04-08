package synthesis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudprobe/debrief/internal/journal"
	"github.com/cloudprobe/debrief/internal/model"
)

// ErrNoClaude is returned when the claude binary is not found in PATH.
var ErrNoClaude = errors.New("claude binary not found in PATH")

// ErrEmptyOutput is returned when claude -p exits successfully but produces no output.
var ErrEmptyOutput = errors.New("claude returned empty output")

// Executor runs claude -p with the given stdin and returns stdout.
type Executor interface {
	Run(ctx context.Context, stdin string) (string, error)
}

// Options configures Synthesize. Zero value uses defaults.
type Options struct {
	Timeout         time.Duration   // default 90s
	MaxPayload      int             // default 50_000 bytes
	Executor        Executor        // default: real claude -p
	JournalEntries  []journal.Entry // from internal/journal
	PreviousStandup string
	PreviousDate    string
}

// Synthesize produces a Claude-powered standup from the collected day summaries.
// dateLabel is the period header (e.g. "Week of Apr 1–7, 2026"); empty for single day.
// Returns ErrNoClaude if claude is not installed.
func Synthesize(ctx context.Context, days []model.DaySummary, totalDays int, dateLabel string, opts Options) (string, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 90 * time.Second
	}
	if opts.MaxPayload == 0 {
		opts.MaxPayload = defaultMaxPayload
	}
	if opts.Executor == nil {
		ex, err := newClaudeExecutor()
		if err != nil {
			return "", ErrNoClaude
		}
		opts.Executor = ex
	}

	extras := renderExtras(opts.JournalEntries, opts.PreviousStandup, opts.PreviousDate)
	payload := BuildPayload(days, totalDays, dateLabel, opts.MaxPayload)
	if extras != "" {
		payload = extras + payload
	}
	stdin := SystemPrompt + "\n\n---\n\n" + payload

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	out, err := opts.Executor.Run(ctx, stdin)
	if err != nil {
		return "", fmt.Errorf("synthesis failed: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", ErrEmptyOutput
	}
	return out + "\n", nil
}

// claudeExecutor shells out to `claude -p`.
type claudeExecutor struct {
	bin string
}

func newClaudeExecutor() (*claudeExecutor, error) {
	bin := os.Getenv("DEBRIEF_CLAUDE_BIN")
	if bin != "" {
		if !filepath.IsAbs(bin) {
			return nil, fmt.Errorf("DEBRIEF_CLAUDE_BIN must be an absolute path, got: %q", bin)
		}
		return &claudeExecutor{bin: bin}, nil
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, ErrNoClaude
	}
	return &claudeExecutor{bin: bin}, nil
}

func (e *claudeExecutor) Run(ctx context.Context, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, e.bin, "-p", "--output-format", "text")
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err() // return unwrapped so errors.Is works at call site
		}
		stderrStr := strings.TrimSpace(stderr.String())
		const maxStderrLen = 200
		if len(stderrStr) > maxStderrLen {
			stderrStr = stderrStr[:maxStderrLen] + "...[truncated]"
		}
		return "", fmt.Errorf("%w (stderr: %s)", err, stderrStr)
	}
	return stdout.String(), nil
}
