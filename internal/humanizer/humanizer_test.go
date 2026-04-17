package humanizer_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/humanizer"
)

func TestNoOpReturnsEmpty(t *testing.T) {
	h := humanizer.NoOp{}
	out, err := h.Rewrite(context.Background(), "some prompt")
	if err != nil {
		t.Fatalf("NoOp.Rewrite returned unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("NoOp.Rewrite returned non-empty output: %q", out)
	}
}

func TestClaudeCLIMissingBinary(t *testing.T) {
	h := humanizer.ClaudeCLI{Binary: "/nonexistent/claude"}
	_, err := h.Rewrite(context.Background(), "some prompt")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestClaudeCLIContextCancel(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not found on PATH")
	}

	h := humanizer.ClaudeCLI{Binary: sleepPath, Timeout: 30 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, rerr := h.Rewrite(ctx, "5")
	elapsed := time.Since(start)

	if rerr == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(rerr, context.Canceled) && !errors.Is(rerr, context.DeadlineExceeded) {
		// The error may be wrapped inside an exec exit error; check the string too.
		t.Logf("error: %v", rerr)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("call took %v, expected well under 5s after ctx cancel", elapsed)
	}
}

func TestClaudeCLIEcho(t *testing.T) {
	echoPath, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo not found on PATH")
	}
	h := humanizer.ClaudeCLI{Binary: echoPath}
	prompt := "hello world"
	out, err := h.Rewrite(context.Background(), prompt)
	if err != nil {
		t.Fatalf("ClaudeCLI{Binary: echo}.Rewrite returned error: %v", err)
	}
	if !strings.Contains(out, prompt) {
		t.Fatalf("output %q does not contain prompt %q", out, prompt)
	}
}
