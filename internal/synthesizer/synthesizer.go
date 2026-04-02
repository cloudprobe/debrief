// Package synthesizer produces standup summaries from locally collected activity data.
// It uses completion statements extracted from Claude session JSONL files (what Claude
// said it did) combined with signal commits — no external API calls required.
package synthesizer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/model"
	"github.com/cloudprobe/debrief/internal/ui"
)

// Synthesize produces a standup summary from one or more day summaries.
// totalDays is the number of calendar days in the requested period (used for
// the period summary line on multi-day views). Pass 0 to default to len(days).
func Synthesize(days []model.DaySummary, totalDays int) string {
	var b strings.Builder

	renderedCount := 0
	for _, day := range days {
		var dayBuf strings.Builder
		renderDay(&dayBuf, day)
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
		return "No activity to report.\n"
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

// bulletsForProject returns the standup bullets for a project.
// Session notes (what Claude said it accomplished) are the primary source.
// Signal commits supplement only when they cover work not already in the notes.
// If there are no session notes, signal commits are the fallback.
func bulletsForProject(p model.ProjectSummary) []string {
	notes := dedup(p.SessionNotes)

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
