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

// NoActivity is the sentinel returned when there is nothing to report.
// Callers can compare against this to avoid overwriting real saved state.
const NoActivity = "No activity to report.\n"

const (
	noActivity     = NoActivity
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
	return SynthesizeSmartWith(context.Background(), days, dateLabel, slack, humanizer.NoOp{})
}

// SynthesizeSmartWith is SynthesizeSmart with an injectable humanizer and caller context.
// ctx is forwarded to h.Rewrite for cancellation and deadline propagation.
// h rewrites the final bullet list before rendering; pass humanizer.NoOp{} for
// deterministic output (identical to SynthesizeSmart).
func SynthesizeSmartWith(ctx context.Context, days []model.DaySummary, dateLabel string, slack bool, h humanizer.Humanizer) string {
	type bucket struct{ decided, shipped, investigated, risk []string }
	var b bucket

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

	// Render all buckets as flat bullets (Decided → Shipped → Investigated → Risk).
	// Use a fresh slice to avoid mutating b.decided's backing array.
	allItems := append([]string(nil), b.decided...)
	allItems = append(allItems, b.shipped...)
	allItems = append(allItems, b.investigated...)
	allItems = append(allItems, b.risk...)
	allItems = humanizeBullets(ctx, allItems, h)
	for _, item := range allItems {
		fmt.Fprintf(&sb, "%s%s\n", bulletPrefix, item)
	}

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// commitBucket classifies a commit message into a routing bucket.
// Returns bucketShipped to include the commit, bucketSkip to drop it.
// True merge commits (prefix "Merge pull request" / "Merge branch") are always
// shipped. Squash commits with trailing "(#N)" still go through the conventional
// prefix filter so chore/test are correctly skipped.
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

	switch prefix {
	case "feat", "fix", "perf", "refactor", "build", "ci":
		return bucketShipped
	case "docs":
		if len(stripPrefix(msg)) > 20 {
			return bucketShipped
		}
		return bucketSkip
	case "chore", "test":
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
