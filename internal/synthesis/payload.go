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
		if maxBytes <= 12 {
			return "[truncated]"
		}
		payload = payload[:maxBytes-12] + "\n[truncated]"
	}
	return payload
}

type truncLevel int

const maxTruncLevel truncLevel = 5

// sanitizeForPrompt prevents user-controlled strings from injecting LLM instructions.
// It collapses newlines (the primary injection vector) and trims whitespace.
func sanitizeForPrompt(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

// Truncation levels (spec order):
//
//	Level 0: everything included
//	Level 1: drop tools: lines
//	Level 2: cap commit_messages at 5 per project
//	Level 3: cap session_notes at 3 per project
//	Level 4: drop files_created/files_modified
//	Level 5: cap projects to top 1 by score
func renderPayload(days []model.DaySummary, totalDays int, dateLabel string, lvl truncLevel) string {
	var b strings.Builder

	if dateLabel != "" {
		fmt.Fprintf(&b, "period: %s\n", sanitizeForPrompt(dateLabel))
	}
	if totalDays > 1 {
		fmt.Fprintf(&b, "total_days_in_range: %d\n", totalDays)
	}
	fmt.Fprintln(&b)

	for _, day := range days {
		fmt.Fprintf(&b, "=== %s ===\n", day.Date.Format("2006-01-02 (Monday)"))

		// Cost summary for the day — compute percentage share per model.
		var totalCost float64
		modelCosts := make(map[string]float64)
		for mdl, ms := range day.ByModel {
			totalCost += ms.TotalCost
			modelCosts[mdl] = ms.TotalCost
		}
		if totalCost > 0 {
			fmt.Fprintf(&b, "total_cost_usd: %.2f\n", totalCost)
			if lvl < 3 {
				var modelParts []string
				for mdl, cost := range modelCosts {
					pct := cost / totalCost * 100
					modelParts = append(modelParts, fmt.Sprintf("%s %.0f%%", sanitizeForPrompt(mdl), pct))
				}
				sort.Strings(modelParts)
				fmt.Fprintf(&b, "model_mix: %s\n", strings.Join(modelParts, ", "))
			}
		}
		fmt.Fprintln(&b)

		// Sort projects by activity score descending.
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

		// Level 4+: cap projects (4 → top 3, 5 → top 1).
		maxProjects := len(projects)
		if lvl >= 4 && maxProjects > 3 {
			maxProjects = 3
		}
		if lvl >= 5 && maxProjects > 1 {
			maxProjects = 1
		}

		for _, p := range projects[:maxProjects] {
			ps := p.summary
			fmt.Fprintf(&b, "project: %s\n", sanitizeForPrompt(p.name))

			if ps.CommitCount > 0 {
				fmt.Fprintf(&b, "  commits: %d\n", ps.CommitCount)
			}
			if ps.Insertions > 0 || ps.Deletions > 0 {
				fmt.Fprintf(&b, "  changes: +%d -%d lines\n", ps.Insertions, ps.Deletions)
			}
			if ps.Interactions > 0 {
				fmt.Fprintf(&b, "  claude_interactions: %d\n", ps.Interactions)
			}

			// Level 3: cap session_notes at 3 per project.
			notes := ps.SessionNotes
			maxNotes := len(notes)
			if lvl >= 3 && maxNotes > 3 {
				maxNotes = 3
			}
			if len(notes[:maxNotes]) > 0 {
				fmt.Fprintln(&b, "  session_notes:")
				for _, n := range notes[:maxNotes] {
					fmt.Fprintf(&b, "    - %s\n", sanitizeForPrompt(n))
				}
			}

			// Level 2: cap commit_messages at 5 per project.
			msgs := ps.CommitMessages
			maxMsgs := len(msgs)
			if lvl >= 2 && maxMsgs > 5 {
				maxMsgs = 5
			}
			if len(msgs[:maxMsgs]) > 0 {
				fmt.Fprintln(&b, "  commit_messages:")
				for _, m := range msgs[:maxMsgs] {
					fmt.Fprintf(&b, "    - %s\n", sanitizeForPrompt(m))
				}
			}

			// Level 4: drop files_created/files_modified.
			if lvl < 4 {
				if len(ps.FilesCreated) > 0 {
					sanitized := make([]string, len(ps.FilesCreated))
					for i, f := range ps.FilesCreated {
						sanitized[i] = sanitizeForPrompt(f)
					}
					fmt.Fprintf(&b, "  files_created: %s\n", strings.Join(sanitized, ", "))
				}
				if len(ps.FilesModified) > 0 {
					sanitized := make([]string, len(ps.FilesModified))
					for i, f := range ps.FilesModified {
						sanitized[i] = sanitizeForPrompt(f)
					}
					fmt.Fprintf(&b, "  files_modified: %s\n", strings.Join(sanitized, ", "))
				}
			}

			// Level 1: drop tools: lines.
			if lvl < 1 && len(ps.ToolBreakdown) > 0 {
				var tools []string
				for t, c := range ps.ToolBreakdown {
					tools = append(tools, fmt.Sprintf("%s=%d", sanitizeForPrompt(t), c))
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
