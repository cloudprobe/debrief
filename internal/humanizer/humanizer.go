// Package humanizer rewrites bullet lists via a subprocess call to the Claude CLI.
package humanizer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Humanizer rewrites a prompt string and returns the result.
// A non-nil error signals the caller should fall back to original content.
// Returning ("", nil) is the NoOp contract — caller must also fall back.
type Humanizer interface {
	Rewrite(ctx context.Context, prompt string) (string, error)
}

// NoOp always returns ("", nil), causing callers to use original content.
type NoOp struct{}

func (NoOp) Rewrite(_ context.Context, _ string) (string, error) {
	return "", nil
}

// maxStdoutBytes caps the stdout buffer when reading claude output.
// A response exceeding this limit triggers an error and the caller falls back.
const maxStdoutBytes = 1024 * 1024 // 1 MB

// ClaudeCLI calls the Claude CLI binary with -p <prompt> and returns stdout.
// Binary defaults to "claude" resolved via PATH. Timeout defaults to 20s.
type ClaudeCLI struct {
	Binary  string
	Timeout time.Duration
}

func (c ClaudeCLI) Rewrite(ctx context.Context, prompt string) (string, error) {
	bin := c.Binary
	if bin == "" {
		bin = "claude"
	}

	// Verify binary is reachable before constructing the command.
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("humanizer: binary %q not found: %w", bin, err)
	}

	timeout := c.Timeout
	if timeout == 0 {
		timeout = 20 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, resolved, "-p", prompt)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// Discard stderr to avoid polluting the user's terminal.
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("humanizer: claude exited with error: %w", err)
	}

	if stdout.Len() > maxStdoutBytes {
		return "", fmt.Errorf("humanizer: response too large (%d bytes, limit %d)", stdout.Len(), maxStdoutBytes)
	}

	out := stdout.String()
	if out == "" {
		return "", fmt.Errorf("humanizer: empty response from claude")
	}
	return out, nil
}
