package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudprobe/debrief/internal/humanizer"
)

// fakeHumanizer is a test-only Humanizer that returns a caller-configured
// result, letting us drive the wrapper through its state transitions.
type fakeHumanizer struct {
	out string
	err error
}

func (f fakeHumanizer) Rewrite(_ context.Context, _ string) (string, error) {
	return f.out, f.err
}

func TestHumanizeFallbackWrapper_Banner(t *testing.T) {
	tests := []struct {
		name     string
		disabled bool
		inner    humanizer.Humanizer
		// call controls whether we actually invoke Rewrite before checking the
		// banner — lets us verify the "not attempted" path.
		call     bool
		wantSub  string // substring that must appear in the banner
		wantNone bool   // if true, banner must be empty
	}{
		{
			name:     "not attempted — no banner",
			disabled: false,
			inner:    fakeHumanizer{out: "anything"},
			call:     false,
			wantNone: true,
		},
		{
			name:     "explicitly disabled — raw output banner",
			disabled: true,
			inner:    humanizer.NoOp{},
			call:     true,
			wantSub:  "--no-humanize",
		},
		{
			name:     "succeeded — humanized banner",
			disabled: false,
			inner:    fakeHumanizer{out: "1. rewritten bullet"},
			call:     true,
			wantSub:  "humanized via claude-code",
		},
		{
			name:     "failed — missing binary banner",
			disabled: false,
			inner:    fakeHumanizer{err: errors.New("binary not found")},
			call:     true,
			wantSub:  "install the `claude` CLI",
		},
		{
			name: "attempted but NoOp-like empty result — no banner",
			// inner returns ("", nil) — the NoOp contract. We don't know if
			// that's "humanization ran and opted out" vs "never would've worked",
			// so emitting no banner is the safe behavior.
			disabled: false,
			inner:    humanizer.NoOp{},
			call:     true,
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &humanizeFallbackWrapper{inner: tt.inner, disabled: tt.disabled}
			if tt.call {
				_, _ = w.Rewrite(context.Background(), "prompt")
			}
			got := w.banner()
			if tt.wantNone {
				if got != "" {
					t.Errorf("expected empty banner, got %q", got)
				}
				return
			}
			if got == "" {
				t.Errorf("expected banner containing %q, got empty string", tt.wantSub)
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("banner %q does not contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestBuildHumanizer(t *testing.T) {
	tests := []struct {
		name       string
		noFlag     bool
		envVal     string
		wantNoOp   bool
		wantDisabl bool
	}{
		{
			name:       "no flag, no env — ClaudeCLI, not disabled",
			noFlag:     false,
			envVal:     "",
			wantNoOp:   false,
			wantDisabl: false,
		},
		{
			name:       "--no-humanize flag — NoOp, disabled",
			noFlag:     true,
			envVal:     "",
			wantNoOp:   true,
			wantDisabl: true,
		},
		{
			name:       "DEBRIEF_HUMANIZE=0 — NoOp, disabled",
			noFlag:     false,
			envVal:     "0",
			wantNoOp:   true,
			wantDisabl: true,
		},
		{
			name:       "DEBRIEF_HUMANIZE=false — NoOp, disabled",
			noFlag:     false,
			envVal:     "false",
			wantNoOp:   true,
			wantDisabl: true,
		},
		{
			name:       "DEBRIEF_HUMANIZE=off — NoOp, disabled",
			noFlag:     false,
			envVal:     "off",
			wantNoOp:   true,
			wantDisabl: true,
		},
		{
			name:       "DEBRIEF_HUMANIZE=1 — ClaudeCLI, not disabled",
			noFlag:     false,
			envVal:     "1",
			wantNoOp:   false,
			wantDisabl: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEBRIEF_HUMANIZE", tt.envVal)
			h, disabled := buildHumanizer(tt.noFlag)
			if disabled != tt.wantDisabl {
				t.Errorf("disabled = %v, want %v", disabled, tt.wantDisabl)
			}
			_, isNoOp := h.(humanizer.NoOp)
			if isNoOp != tt.wantNoOp {
				t.Errorf("isNoOp = %v, want %v (got type %T)", isNoOp, tt.wantNoOp, h)
			}
		})
	}
}
