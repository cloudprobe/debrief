// Package synthesizer produces standup summaries from locally collected activity data.
// It uses completion statements extracted from Claude session JSONL files (what Claude
// said it did) combined with signal commits — no external API calls required.
package synthesizer

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/humanizer"
	"github.com/cloudprobe/debrief/internal/model"
)

var bareHashRe = regexp.MustCompile(`\b[0-9a-f]{7,}\b`)

// prSquashRe matches the trailing "(#NNN)" pattern that GitHub/GitLab append to
// squash-merged commit messages. Presence indicates the commit went through code
// review and shipped, regardless of its conventional-commit prefix.
var prSquashRe = regexp.MustCompile(`\(#\d+\)\s*$`)

// NoActivity is the sentinel returned when there is nothing to report at all —
// no commits, no session notes. Callers can compare against this to avoid
// overwriting real saved state.
const NoActivity = "No activity to report.\n"

// QuietDay is the sentinel returned when commits existed but were all filtered
// as chore/test/docs noise. Signals "tool worked, day was just quiet" rather
// than "tool found nothing at all".
const QuietDay = "Quiet day — just chores and lints. Nothing shipped worth writing up.\n"

const (
	noActivity     = NoActivity
	quietDay       = QuietDay
	bucketShipped  = "shipped"
	bucketSkip     = "skip"
	bucketDecided  = "decided"
	bucketInvestig = "investigated"
	bucketRisk     = "risk"
)

// coveredByNotes checks if a commit message describes work already captured in notes.
func coveredByNotes(commitMsg string, notes []string) bool {
	words := significantWords(commitMsg)
	if len(words) == 0 {
		return false
	}
	for _, note := range notes {
		noteWords := significantWords(note)
		matches := 0
		for _, w := range words {
			for _, nw := range noteWords {
				if w == nw {
					matches++
					break
				}
			}
		}
		if matches*2 >= len(words) {
			return true
		}
	}
	return false
}

// significantWords extracts lowercase words > 4 chars, skipping noise words.
func significantWords(s string) []string {
	noise := map[string]bool{
		"with": true, "from": true, "that": true, "this": true, "into": true,
		"have": true, "been": true, "when": true, "also": true, "then": true,
	}
	var out []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,;:!?()'\"")
		if len(w) > 4 && !noise[w] {
			out = append(out, w)
		}
	}
	return out
}

// stripPrefix removes conventional commit prefixes ("fix:", "feat(scope):") for display.
func stripPrefix(msg string) string {
	colonIdx := strings.Index(msg, ":")
	if colonIdx < 0 {
		return msg
	}
	prefix := strings.ToLower(msg[:colonIdx])
	if i := strings.Index(prefix, "("); i > 0 {
		prefix = prefix[:i]
	}
	known := map[string]bool{
		"feat": true, "fix": true, "refactor": true, "perf": true,
		"build": true, "chore": true, "docs": true, "ci": true, "test": true,
	}
	if known[prefix] {
		s := strings.TrimSpace(msg[colonIdx+1:])
		if s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return msg
}

// dedup removes exact duplicates while preserving order.
func dedup(notes []string) []string {
	seen := make(map[string]bool, len(notes))
	var out []string
	for _, n := range notes {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// SynthesizeSmart produces a flat standup using local classification — no API calls.
// Bullets are ordered Decided → Shipped → Investigated → Risk with no section headers.
// For single-day input it shows a date header; for multi-day (dateLabel non-empty)
// it produces a flat rollup across all days.
// If slack is true, the header is bold (*date*) and bullets use Slack's "- " prefix.
func SynthesizeSmart(days []model.DaySummary, dateLabel string, slack bool) string {
	return SynthesizeSmartWith(context.Background(), days, dateLabel, slack, false, humanizer.NoOp{})
}

// SynthesizeSmartWith is SynthesizeSmart with an injectable humanizer, caller context,
// and prose mode. ctx is forwarded to h.Rewrite for cancellation and deadline propagation.
// When prose is true and the prose humanizer succeeds, emits header + blank line + prose
// paragraphs instead of bullets. On prose failure falls back to bullet mode (with bolder
// humanizer). When prose is false, h rewrites the final bullet list before rendering;
// pass humanizer.NoOp{} for deterministic output (identical to SynthesizeSmart).
func SynthesizeSmartWith(ctx context.Context, days []model.DaySummary, dateLabel string, slack, prose bool, h humanizer.Humanizer) string {
	type bucket struct{ decided, shipped, investigated, risk []string }
	var b bucket

	// filteredCommits counts commits we dropped as chore/test/docs noise.
	// Used to distinguish "nothing happened" from "only noise happened".
	var filteredCommits int

	for _, day := range days {
		for _, p := range sortedProjects(day.ByProject) {
			// Collect surviving notes first
			var goodNotes []string
			for _, note := range p.SessionNotes {
				if noteQuality(note) {
					goodNotes = append(goodNotes, note)
				}
			}

			// Classify notes into buckets
			for _, note := range goodNotes {
				switch noteBucket(note) {
				case bucketDecided:
					b.decided = append(b.decided, note)
				case bucketInvestig:
					b.investigated = append(b.investigated, note)
				case bucketRisk:
					b.risk = append(b.risk, note)
				default:
					b.shipped = append(b.shipped, note)
				}
			}

			// Classify commits; skip if covered by a note
			for _, msg := range p.CommitMessages {
				if commitBucket(msg) == bucketSkip {
					filteredCommits++
					continue
				}
				stripped := stripPrefix(msg)
				if coveredByNotes(stripped, goodNotes) {
					continue
				}
				b.shipped = append(b.shipped, stripped)
			}
		}
	}

	// Dedup each bucket
	b.decided = dedup(b.decided)
	b.shipped = dedup(b.shipped)
	b.investigated = dedup(b.investigated)
	b.risk = dedup(b.risk)

	if len(b.decided)+len(b.shipped)+len(b.investigated)+len(b.risk) == 0 {
		if filteredCommits > 0 {
			return quietDay
		}
		return noActivity
	}

	var sb strings.Builder

	bulletPrefix := "  - "
	if slack {
		bulletPrefix = "- "
	}

	// Build header
	isMultiDay := dateLabel != ""
	if isMultiDay {
		if slack {
			fmt.Fprintf(&sb, "`%s`\n\n", dateLabel)
		} else {
			fmt.Fprintf(&sb, "%s\n\n", dateLabel)
		}
	} else if len(days) > 0 {
		dateStr := days[0].Date.Format("Mon, Jan 2 2006")
		if slack {
			fmt.Fprintf(&sb, "`%s`\n\n", dateStr)
		} else {
			fmt.Fprintf(&sb, "%s\n\n", dateStr)
		}
	}

	// Track bucket boundaries so we can re-emit section headers after the
	// humanizer rewrites the flat list. Order is Decided → Shipped →
	// Investigated → Watch (the display label for the "risk" bucket).
	bucketSections := []struct {
		label string
		items []string
	}{
		{"Decided", b.decided},
		{"Shipped", b.shipped},
		{"Investigated", b.investigated},
		{"Watch", b.risk},
	}
	var allItems []string
	for _, s := range bucketSections {
		allItems = append(allItems, s.items...)
	}

	// Prose mode: attempt to produce 2–3 paragraphs; fall back to bullets on failure.
	if prose {
		if proseText, ok := humanizeAsProse(ctx, allItems, h); ok {
			header := strings.TrimRight(sb.String(), "\n")
			if header == "" {
				return proseText + "\n"
			}
			return header + "\n\n" + proseText + "\n"
		}
		// Fall through to bullet rendering (bolder humanizer still applies).
	}

	humanized := humanizeBullets(ctx, allItems, h)

	// Slack mode stays flat — section headers would clutter a Slack paste.
	// We also fall back to flat if the humanizer returned a different-length
	// slice than we sent in (defensive; bucket boundaries would no longer align).
	if slack || len(humanized) != len(allItems) {
		for _, item := range humanized {
			fmt.Fprintf(&sb, "%s%s\n", bulletPrefix, item)
		}
		return strings.TrimRight(sb.String(), "\n") + "\n"
	}

	// Text mode: emit a plain-text section label above each non-empty bucket
	// so the 4-bucket classifier is visible in the output. Header style matches
	// the date header (plain line, no markdown #) for terminal readability.
	idx := 0
	firstSection := true
	for _, s := range bucketSections {
		n := len(s.items)
		if n == 0 {
			continue
		}
		if !firstSection {
			sb.WriteByte('\n')
		}
		firstSection = false
		fmt.Fprintf(&sb, "%s\n", s.label)
		for j := 0; j < n; j++ {
			fmt.Fprintf(&sb, "%s%s\n", bulletPrefix, humanized[idx+j])
		}
		idx += n
	}

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// commitBucket classifies a commit message into a routing bucket.
// Returns bucketShipped to include the commit, bucketSkip to drop it.
// True merge commits (prefix "Merge pull request" / "Merge branch") are always
// shipped. For chore/test/docs prefixes, a PR-squash suffix "(#N)" combined with
// a substantive body (>20 chars after prefix strip) is treated as evidence the
// work was reviewed and shipped, overriding the usual skip.
func commitBucket(msg string) string {
	// True merge commits — always include.
	if strings.HasPrefix(msg, "Merge pull request") || strings.HasPrefix(msg, "Merge branch") {
		return bucketShipped
	}

	colonIdx := strings.Index(msg, ":")
	if colonIdx < 0 {
		// No conventional prefix — include by default.
		return bucketShipped
	}

	prefix := strings.ToLower(msg[:colonIdx])
	if i := strings.Index(prefix, "("); i > 0 {
		prefix = prefix[:i]
	}

	// Substantive PR-squashed commits override chore/test/docs skipping.
	// Rationale: work that went through review and merged is worth surfacing
	// even if the author tagged it chore or test.
	prSquashSubstantive := prSquashRe.MatchString(msg) && len(stripPrefix(msg)) > 20

	switch prefix {
	case "feat", "fix", "perf", "refactor", "build", "ci":
		return bucketShipped
	case "docs":
		if len(stripPrefix(msg)) > 20 {
			return bucketShipped
		}
		return bucketSkip
	case "chore", "test":
		if prSquashSubstantive {
			return bucketShipped
		}
		return bucketSkip
	default:
		return bucketShipped
	}
}

// noteQuality returns true if the note is worth including in the standup.
func noteQuality(note string) bool {
	if len(note) < 40 {
		return false
	}
	if bareHashRe.MatchString(note) {
		return false
	}
	lower := strings.ToLower(note)
	for _, prefix := range []string{"pushed", "committed", "fixed."} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return note != "All the content."
}

// noteBucket classifies a surviving session note into a standup bucket.
func noteBucket(note string) string {
	lower := strings.ToLower(note)
	for _, kw := range []string{"decided", "went with", "chose", "switched to", "picked"} {
		if strings.Contains(lower, kw) {
			return bucketDecided
		}
	}
	for _, kw := range []string{"found", "discovered", "ruled out", "investigated", "turns out"} {
		if strings.Contains(lower, kw) {
			return bucketInvestig
		}
	}
	for _, kw := range []string{"risk", "concern", "watch out"} {
		if strings.Contains(lower, kw) {
			return bucketRisk
		}
	}
	return bucketShipped
}

// sortedProjects returns projects sorted by activity volume.
func sortedProjects(byProject map[string]model.ProjectSummary) []model.ProjectSummary {
	var ps []model.ProjectSummary
	for _, p := range byProject {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool {
		si := ps[i].CommitCount*3 + ps[i].Interactions + len(ps[i].SessionNotes)*2
		sj := ps[j].CommitCount*3 + ps[j].Interactions + len(ps[j].SessionNotes)*2
		if si != sj {
			return si > sj
		}
		return ps[i].Name < ps[j].Name
	})
	return ps
}
