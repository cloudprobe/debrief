package synthesizer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudprobe/debrief/internal/humanizer"
)

const humanizePromptTemplate = `You are editing bullets for a short end-of-day work log. Each bullet is one line of what I did today.

Rewrite the numbered bullets so they sound like a person wrote them — not an AI. Rules:
- Use contractions (it's, didn't, we're).
- Drop AI filler: utilize, leverage, delve, pivotal, robust, seamless, comprehensive, underscore, testament, tapestry, navigate, meticulous.
- No significance inflation. No "serves as a testament", no "plays a pivotal role".
- No rule-of-three. If I wrote one thing, keep it one thing.
- Keep it plain and specific. Shorter is better.
- Preserve every factual token exactly: file names, function names, PR numbers (#123), commit hashes, error codes, identifiers, numbers, package paths.
- Do not invent facts. If the bullet is terse, leave it terse — do not pad.
- Vary bullet length — a short bullet can stay short; a substantive one can be a full sentence.
- Use contractions and natural cadence (some bullets in past tense, some can be noun phrases).
- Drop bullets that are just URLs with no context (e.g. a bare https://github.com/... line with nothing else).
- Merge near-duplicate bullets into one when they describe the same work.
- Output count may be less than input count (after dropping/merging). Numbering must be monotonic starting at 1.
- Do not add commentary, headers, or a preamble. Output only the numbered list.
- No markdown emphasis (*, _, backticks).

Example input:
1. Refactored the authentication module to leverage JWT tokens for robust session management
2. feat: add login page
3. Decided to utilize gRPC instead of REST for internal service calls
4. https://github.com/cloudprobe/cloudprobe
5. Fixed review blockers around correctness, security, and error handling
6. Fixed review blockers

Example output:
1. Refactored the auth module to use JWT for sessions
2. Added the login page
3. Went with gRPC over REST for internal service calls
4. Fixed review blockers around correctness, security, and error handling

Now rewrite these:
%s`

const humanizeProsePromptTemplate = `You are writing a short narrative summary of someone's work day for a standup update.

Given a list of what was done, write 2–3 short paragraphs of natural prose. Rules:
- First-person implied ("Spent most of the day on...", "Got PR #5 merged after...").
- No bullets, no headers, no preamble, no closing remarks.
- Preserve every factual token exactly: PR numbers (#123), file names, function names, commit hashes, error codes, identifiers.
- Do not invent facts.
- Plain language — no markdown emphasis (*, _, backticks), no filler words.
- Combine related items into flowing sentences. Group thematically across paragraphs.
- Two or three short focused paragraphs. Each paragraph 1–3 sentences.

Example input:
1. Opened PR #12 to fix the auth flow
2. Fixed review blockers around correctness and error handling
3. Handled CodeRabbit feedback on PR #12
4. Merged search feature after review
5. Decided to use Redis for session storage

Example output:
Most of the day went into PR #12 — fixed the auth flow, worked through the CodeRabbit feedback, and addressed correctness and error handling concerns from review.

Also got the search feature merged after clearing the remaining review comments.

Went with Redis for session storage after weighing the options.

Now write 2–3 paragraphs for these:
%s`

const maxPromptBytes = 100 * 1024 // 100 KB — argv cap guard

var numberedLineRe = regexp.MustCompile(`^\s*(\d+)[.)]\s+(.*)$`)

// humanizeBullets rewrites items via h in a single call and returns a slice of
// at most len(items). On any failure (empty input, prompt too large, runner error,
// parse returning more lines than input, scrambled numbering, or 0 lines) it
// returns the original items unchanged.
// parsed < items is accepted (merges/URL-drops), parsed > items falls back.
// The caller-supplied ctx is forwarded directly to h.Rewrite; timeout management
// is the responsibility of h (e.g. ClaudeCLI applies its own per-call timeout).
func humanizeBullets(ctx context.Context, items []string, h humanizer.Humanizer) []string {
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

// humanizeAsProse rewrites items as 2–3 paragraphs of natural prose via h.
// Returns (text, true) on success, ("", false) on any failure including empty
// input, runner error, or empty response.
func humanizeAsProse(ctx context.Context, items []string, h humanizer.Humanizer) (string, bool) {
	if len(items) == 0 {
		return "", false
	}

	var sb strings.Builder
	for i, item := range items {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, item)
	}
	numbered := strings.TrimRight(sb.String(), "\n")

	prompt := fmt.Sprintf(humanizeProsePromptTemplate, numbered)
	if len(prompt) > maxPromptBytes {
		return "", false
	}

	raw, err := h.Rewrite(ctx, prompt)
	if err != nil || raw == "" {
		return "", false
	}

	return strings.TrimSpace(raw), true
}

// parseNumberedList splits raw on newlines, matches numbered bullet lines, and
// returns a slice when all matched lines are monotonic 1..N and N <= n.
// Returns nil when: any non-empty line fails to match, numbering is not
// monotonic from 1, or N > n (more output than input). N == 0 also returns nil.
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
	if len(out) == 0 || len(out) > n {
		return nil
	}
	return out
}
