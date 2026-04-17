package synthesizer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudprobe/debrief/internal/humanizer"
)

const humanizePromptTemplate = `You are editing bullets for a short end-of-day work log. Each bullet is one line of what I did today.

Rewrite each numbered bullet so it sounds like a person wrote it — not an AI. Rules:
- Use contractions (it's, didn't, we're).
- Drop AI filler: utilize, leverage, delve, pivotal, robust, seamless, comprehensive, underscore, testament, tapestry, navigate, meticulous.
- No significance inflation. No "serves as a testament", no "plays a pivotal role".
- No rule-of-three. If I wrote one thing, keep it one thing.
- Keep it plain and specific. Shorter is better. One line per bullet.
- Preserve every factual token exactly: file names, function names, PR numbers (#123), commit hashes, error codes, identifiers, numbers, package paths.
- Do not invent facts. If the bullet is terse, leave it terse — do not pad.
- Do not add commentary, headers, or a preamble. Output only the numbered list, same count, same order.

Example input:
1. Refactored the authentication module to leverage JWT tokens for robust session management
2. feat: add login page
3. Decided to utilize gRPC instead of REST for internal service calls

Example output:
1. Refactored the auth module to use JWT for sessions
2. Added the login page
3. Went with gRPC over REST for internal service calls

Now rewrite these:
%s`

const maxPromptBytes = 100 * 1024 // 100 KB — argv cap guard

var numberedLineRe = regexp.MustCompile(`^\s*(\d+)[.)]\s+(.*)$`)

// humanizeBullets rewrites items via h in a single call and returns a slice of
// the same length. On any failure (empty input, prompt too large, runner error,
// parse mismatch, scrambled numbering) it returns the original items unchanged.
func humanizeBullets(items []string, h humanizer.Humanizer) []string {
	if len(items) == 0 {
		return items
	}

	var sb strings.Builder
	for i, item := range items {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, item)
	}
	numbered := strings.TrimRight(sb.String(), "\n")

	prompt := fmt.Sprintf(humanizePromptTemplate, numbered)
	if len(prompt) > maxPromptBytes {
		return items
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	raw, err := h.Rewrite(ctx, prompt)
	if err != nil || raw == "" {
		return items
	}

	parsed := parseNumberedList(raw, len(items))
	if parsed == nil {
		return items
	}
	return parsed
}

// parseNumberedList splits raw on newlines, matches numbered bullet lines, and
// returns a slice only when every line is present, count matches n, and
// numbering is strictly monotonic 1..n. Returns nil on any violation.
func parseNumberedList(raw string, n int) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]string, 0, n)
	expected := 1
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := numberedLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		num, _ := strconv.Atoi(m[1])
		if num != expected {
			return nil
		}
		out = append(out, m[2])
		expected++
	}
	if len(out) != n {
		return nil
	}
	return out
}
