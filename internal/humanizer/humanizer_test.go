package humanizer_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

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
