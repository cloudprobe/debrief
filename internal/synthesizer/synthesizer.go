// Package synthesizer produces standup summaries from locally collected activity data.
// It uses completion statements extracted from Claude session JSONL files (what Claude
// said it did) combined with signal commits — no external API calls required.
package synthesizer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/model"
	"github.com/cloudprobe/debrief/internal/ui"
)

var bareHashRe = regexp.MustCompile(`\b[0-9a-f]{7,}\b`)

const noActivity = "No activity to report.\n"

// Synthesize produces a standup summary from one or more day summaries.
// totalDays is the number of calendar days in the requested period (used for
// the period summary line on multi-day views). Pass 0 to default to len(days).
// byProject groups bullets under project name headers; when false bullets are
// rendered as a flat list with no project headers (default for copy-paste use).
func Synthesize(days []model.DaySummary, totalDays int, byProject bool) string {
	var b strings.Builder

	render := renderDayFlat
	if byProject {
		render = renderDay
	}

	renderedCount := 0
	for _, day := range days {
		var dayBuf strings.Builder
		render(&dayBuf, day)
		if dayBuf.Len() > 0 {
			if renderedCount > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s:\n\n", day.Date.Format("Mon, Jan 2 2006"))
			b.WriteString(dayBuf.String())
			renderedCount++
		}
	}

	if len(days) > 1 {
		td := totalDays
		if td <= 0 {
			td = len(days)
		}
		projectSet := make(map[string]bool)
		var totalCommits int
		activeDays := 0
		for _, day := range days {
			if len(day.ByProject) > 0 {
				activeDays++
				for k, p := range day.ByProject {
					projectSet[k] = true
					totalCommits += p.CommitCount
				}
			}
		}
		b.WriteString("\n")
		b.WriteString(strings.Repeat("\u2500", 40))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%d projects \u2022 %d commits \u2022 active %d of %d days\n",
			len(projectSet), totalCommits, activeDays, td)
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return noActivity
	}
	return out + "\n"
}

func renderDay(b *strings.Builder, day model.DaySummary) {
	if len(day.Activities) == 0 {
		return
	}

	projects := sortedProjects(day.ByProject)
	any := false

	for _, p := range projects {
		bullets := bulletsForProject(p)
		if len(bullets) == 0 {
			continue
		}
		if any {
			b.WriteString("\n")
		}
		fmt.Fprintf(b, "%s\n", p.Name)
		for _, bullet := range bullets {
			fmt.Fprintf(b, "  \u2022 %s\n", bullet)
		}
		if links := ui.ExtractPRLinks(p.CommitMessages); len(links) > 0 {
			fmt.Fprintf(b, "  PRs: %s\n", strings.Join(links, "  "))
		}
		any = true
	}
}

// renderDayFlat renders all bullets from all projects as a flat list with no
// project name headers — suitable for copy-pasting into Slack or standup docs.
func renderDayFlat(b *strings.Builder, day model.DaySummary) {
	if len(day.Activities) == 0 {
		return
	}
	projects := sortedProjects(day.ByProject)
	for _, p := range projects {
		for _, bullet := range bulletsForProject(p) {
			fmt.Fprintf(b, "  \u2022 %s\n", bullet)
		}
		if links := ui.ExtractPRLinks(p.CommitMessages); len(links) > 0 {
			fmt.Fprintf(b, "  PRs: %s\n", strings.Join(links, "  "))
		}
	}
}

// SynthesizeSlack produces a Slack-formatted standup summary from one or more
// day summaries. Always groups by project with bold Slack headers.
// totalDays is used for the period summary line (pass 0 to default to len(days)).
func SynthesizeSlack(days []model.DaySummary, totalDays int) string {
	var b strings.Builder

	renderedCount := 0
	for _, day := range days {
		var dayBuf strings.Builder
		renderDaySlack(&dayBuf, day)
		if dayBuf.Len() > 0 {
			if renderedCount > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "*%s*\n\n", day.Date.Format("2006-01-02"))
			b.WriteString(dayBuf.String())
			renderedCount++
		}
	}

	if len(days) > 1 {
		td := totalDays
		if td <= 0 {
			td = len(days)
		}
		projectSet := make(map[string]bool)
		var totalCommits int
		activeDays := 0
		for _, day := range days {
			if len(day.ByProject) > 0 {
				activeDays++
				for k, p := range day.ByProject {
					projectSet[k] = true
					totalCommits += p.CommitCount
				}
			}
		}
		b.WriteString("\n")
		b.WriteString("\n")
		fmt.Fprintf(&b, "%d projects \u2022 %d commits \u2022 active %d of %d days\n",
			len(projectSet), totalCommits, activeDays, td)
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return noActivity
	}
	return out + "\n"
}

// renderDaySlack renders a single day's projects in Slack bold format.
func renderDaySlack(b *strings.Builder, day model.DaySummary) {
	if len(day.Activities) == 0 {
		return
	}
	projects := sortedProjects(day.ByProject)
	any := false
	for _, p := range projects {
		bullets := bulletsForProject(p)
		if len(bullets) == 0 {
			continue
		}
		if any {
			b.WriteString("\n")
		}
		any = true
		fmt.Fprintf(b, "*%s*\n", p.Name)
		for _, bullet := range bullets {
			fmt.Fprintf(b, "- %s\n", bullet)
		}
		if links := ui.ExtractPRLinks(p.CommitMessages); len(links) > 0 {
			fmt.Fprintf(b, "PRs: %s\n", strings.Join(links, " "))
		}
	}
}

// bulletsForProject returns the standup bullets for a project.
// Session notes (what Claude said it accomplished) are the primary source.
// Signal commits supplement only when they cover work not already in the notes.
// If there are no session notes, signal commits are the fallback.
func bulletsForProject(p model.ProjectSummary) []string {
	var filtered []string
	for _, n := range p.SessionNotes {
		if noteQualityLight(n) {
			filtered = append(filtered, n)
		}
	}
	notes := dedup(filtered)

	if len(notes) > 0 {
		// Supplement with any signal commits not already described in the notes.
		for _, c := range signalOnly(p.CommitMessages) {
			msg := stripPrefix(c)
			if !coveredByNotes(msg, notes) {
				notes = append(notes, msg)
			}
		}
		return notes
	}

	// Fallback: signal commits only (strips noise commits from the list).
	signal := signalOnly(p.CommitMessages)
	if len(signal) > 0 {
		bullets := make([]string, len(signal))
		for i, c := range signal {
			bullets[i] = stripPrefix(c)
		}
		return bullets
	}

	return nil
}

// signalOnly returns only true signal commits — does NOT fall back to all commits
// when nothing qualifies. Used when supplementing session notes.
func signalOnly(messages []string) []string {
	var out []string
	for _, msg := range messages {
		if ui.IsSignalCommit(msg) {
			out = append(out, msg)
		}
	}
	return out
}

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
func SynthesizeSmart(days []model.DaySummary, totalDays int, dateLabel string, slack bool) string {
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
				case "decided":
					b.decided = append(b.decided, note)
				case "investigated":
					b.investigated = append(b.investigated, note)
				case "risk":
					b.risk = append(b.risk, note)
				default:
					b.shipped = append(b.shipped, note)
				}
			}

			// Classify commits; skip if covered by a note
			for _, msg := range p.CommitMessages {
				if commitBucket(msg) == "skip" {
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

	// Render all buckets as flat bullets (Decided → Shipped → Investigated → Risk)
	allItems := append(append(append(b.decided, b.shipped...), b.investigated...), b.risk...)
	for _, item := range allItems {
		fmt.Fprintf(&sb, "%s%s\n", bulletPrefix, item)
	}


	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// commitBucket classifies a commit message into a routing bucket.
// Returns "shipped" to include the commit, "skip" to drop it.
func commitBucket(msg string) string {
	// Detect merge/squash commits
	if strings.HasPrefix(msg, "Merge") || strings.Contains(msg, " (#") {
		return "shipped"
	}

	colonIdx := strings.Index(msg, ":")
	if colonIdx < 0 {
		// No recognized prefix — include by default
		return "shipped"
	}

	prefix := strings.ToLower(msg[:colonIdx])
	if i := strings.Index(prefix, "("); i > 0 {
		prefix = prefix[:i]
	}

	switch prefix {
	case "feat", "fix", "perf", "refactor", "build", "ci":
		return "shipped"
	case "docs":
		if len(stripPrefix(msg)) > 20 {
			return "shipped"
		}
		return "skip"
	case "chore", "test":
		return "skip"
	default:
		return "shipped"
	}
}

// noteQualityLight is a minimal junk filter for the heuristic path (Synthesize/SynthesizeSlack).
// It removes bare-hash notes, meta prefixes, and known noise strings without imposing
// a length minimum so legitimate short notes ("Built the login page") still pass.
func noteQualityLight(note string) bool {
	if len(note) < 10 {
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
	if note == "All the content." {
		return false
	}
	return true
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
	if note == "All the content." {
		return false
	}
	return true
}

// noteBucket classifies a surviving session note into a standup bucket.
func noteBucket(note string) string {
	lower := strings.ToLower(note)
	for _, kw := range []string{"decided", "went with", "chose", "switched to", "picked"} {
		if strings.Contains(lower, kw) {
			return "decided"
		}
	}
	for _, kw := range []string{"found", "discovered", "ruled out", "investigated", "turns out"} {
		if strings.Contains(lower, kw) {
			return "investigated"
		}
	}
	for _, kw := range []string{"risk", "concern", "watch out"} {
		if strings.Contains(lower, kw) {
			return "risk"
		}
	}
	return "shipped"
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
