package synthesis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/model"
	"github.com/cloudprobe/debrief/internal/ui"
)

const defaultMaxPayload = 50_000

// BuildPayload renders days into a structured text payload for Claude.
// If the rendered payload exceeds maxBytes, it progressively drops
// low-priority data until it fits.
func BuildPayload(days []model.DaySummary, totalDays int, dateLabel string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = defaultMaxPayload
	}
	payload := renderPayload(days, totalDays, dateLabel, truncLevel(0))
	for lvl := truncLevel(1); len(payload) > maxBytes && lvl <= maxTruncLevel; lvl++ {
		payload = renderPayload(days, totalDays, dateLabel, lvl)
	}
	if len(payload) > maxBytes {
		payload = payload[:maxBytes-12] + "\n[truncated]"
	}
	return payload
}

type truncLevel int

const maxTruncLevel truncLevel = 5

func renderPayload(days []model.DaySummary, totalDays int, dateLabel string, lvl truncLevel) string {
	var b strings.Builder

	if dateLabel != "" {
		fmt.Fprintf(&b, "period: %s\n", dateLabel)
	}
	if totalDays > 1 {
		fmt.Fprintf(&b, "total_days_in_range: %d\n", totalDays)
	}
	fmt.Fprintln(&b)

	for _, day := range days {
		fmt.Fprintf(&b, "=== %s ===\n", day.Date.Format("2006-01-02 (Monday)"))

		// Cost summary for the day
		var totalCost float64
		var modelParts []string
		for mdl, ms := range day.ByModel {
			totalCost += ms.TotalCost
			modelParts = append(modelParts, fmt.Sprintf("%s %.0f%%", mdl, ms.TotalCost))
		}
		if totalCost > 0 {
			fmt.Fprintf(&b, "total_cost_usd: %.2f\n", totalCost)
			if lvl < 3 && len(modelParts) > 0 {
				sort.Strings(modelParts)
				fmt.Fprintf(&b, "model_mix: %s\n", strings.Join(modelParts, ", "))
			}
		}
		fmt.Fprintln(&b)

		// Sort projects by activity score descending
		type scored struct {
			name    string
			summary model.ProjectSummary
			score   int
		}
		var projects []scored
		for name, ps := range day.ByProject {
			score := ps.CommitCount*3 + ps.Interactions + len(ps.SessionNotes)*2
			projects = append(projects, scored{name, ps, score})
		}
		sort.Slice(projects, func(i, j int) bool { return projects[i].score > projects[j].score })

		// At higher truncation levels, drop low-activity projects
		maxProjects := len(projects)
		if lvl >= 4 && maxProjects > 3 {
			maxProjects = 3
		} else if lvl >= 5 && maxProjects > 1 {
			maxProjects = 1
		}

		for _, p := range projects[:maxProjects] {
			ps := p.summary
			fmt.Fprintf(&b, "project: %s\n", p.name)

			if ps.CommitCount > 0 {
				fmt.Fprintf(&b, "  commits: %d\n", ps.CommitCount)
			}
			if ps.Insertions > 0 || ps.Deletions > 0 {
				fmt.Fprintf(&b, "  changes: +%d -%d lines\n", ps.Insertions, ps.Deletions)
			}
			if ps.Interactions > 0 {
				fmt.Fprintf(&b, "  claude_interactions: %d\n", ps.Interactions)
			}

			// Session notes — truncate at higher levels
			notes := ps.SessionNotes
			maxNotes := len(notes)
			if lvl >= 2 && maxNotes > 3 {
				maxNotes = 3
			}
			if len(notes[:maxNotes]) > 0 {
				fmt.Fprintln(&b, "  session_notes:")
				for _, n := range notes[:maxNotes] {
					fmt.Fprintf(&b, "    - %s\n", n)
				}
			}

			// Commit messages — truncate at higher levels
			msgs := ps.CommitMessages
			maxMsgs := len(msgs)
			if lvl >= 1 && maxMsgs > 5 {
				maxMsgs = 5
			}
			if lvl >= 3 && maxMsgs > 3 {
				maxMsgs = 3
			}
			if len(msgs[:maxMsgs]) > 0 {
				fmt.Fprintln(&b, "  commit_messages:")
				for _, m := range msgs[:maxMsgs] {
					fmt.Fprintf(&b, "    - %s\n", m)
				}
			}

			// Files — drop at higher levels
			if lvl < 3 {
				if len(ps.FilesCreated) > 0 {
					fmt.Fprintf(&b, "  files_created: %s\n", strings.Join(ps.FilesCreated, ", "))
				}
				if len(ps.FilesModified) > 0 {
					fmt.Fprintf(&b, "  files_modified: %s\n", strings.Join(ps.FilesModified, ", "))
				}
			}

			// Tool breakdown — drop at level 1+
			if lvl < 1 && len(ps.ToolBreakdown) > 0 {
				var tools []string
				for t, c := range ps.ToolBreakdown {
					tools = append(tools, fmt.Sprintf("%s=%d", t, c))
				}
				sort.Strings(tools)
				fmt.Fprintf(&b, "  tools: %s\n", strings.Join(tools, " "))
			}

			// PR links
			if lvl < 4 {
				links := ui.ExtractPRLinks(ps.CommitMessages)
				if len(links) > 0 {
					fmt.Fprintf(&b, "  pr_links: %s\n", strings.Join(links, " "))
				}
			}

			fmt.Fprintln(&b)
		}
	}
	return b.String()
}
