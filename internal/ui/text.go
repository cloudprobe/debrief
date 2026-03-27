package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/model"
)

// RenderOptions controls what's shown in the output.
type RenderOptions struct {
	ShowCost  bool
	SingleDay bool // true when showing a single day (adds "Your day —" prefix)
	Detail    bool // show per-session detail
}

// CostSummary holds aggregated cost data for the footer.
type CostSummary struct {
	PeriodLabel  string // "Today", "This week", "This month", or custom
	PeriodCost   float64
	WeekCost     float64
	MonthCost    float64
	WeekByModel  map[string]model.ModelSummary
	MonthByModel map[string]model.ModelSummary
}

// RenderText produces a plain text summary — what you actually did.
func RenderText(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No activity found for this period.\n"
	}

	var b strings.Builder

	date := summary.Date.Format("Monday, Jan 2 2006")
	if opts.SingleDay {
		fmt.Fprintf(&b, "\n  Your day — %s\n", date)
	} else {
		fmt.Fprintf(&b, "\n  %s\n", date)
	}
	b.WriteString("  " + strings.Repeat("─", 54) + "\n\n")

	projects := sortedProjects(summary.ByProject)
	totalFiles := 0
	totalCommits := 0
	totalInsertions := 0
	totalDeletions := 0

	for _, p := range projects {
		fmt.Fprintf(&b, "  %s\n", p.Name)

		// Activity description.
		if desc := describeActivity(p); desc != "" {
			fmt.Fprintf(&b, "    %s\n", desc)
		}

		// Session detail (when --detail is set).
		if opts.Detail && len(p.Sessions) > 0 {
			for _, s := range p.Sessions {
				if s.Source != "claude-code" {
					continue
				}
				title := s.SessionTitle
				if title == "" {
					title = "(untitled)"
				}
				modelName := shortModelName(s.Model)
				line := fmt.Sprintf("    \"%s\"  %s  %s  $%.2f",
					title, modelName, plural(s.Interactions, "msg"), s.CostUSD)
				fmt.Fprintf(&b, "%s\n", line)
			}
		}

		// Files line.
		if fileLine := formatFileSummary(p.FilesCreated, p.FilesModified); fileLine != "" {
			fmt.Fprintf(&b, "    %s\n", fileLine)
		}
		totalFiles += len(p.FilesCreated) + len(p.FilesModified)

		// Commit messages with diff stats.
		if p.CommitCount > 0 {
			fmt.Fprintf(&b, "    %s\n", formatCommitMessages(p.CommitMessages, p.CommitCount))
			if p.Insertions > 0 || p.Deletions > 0 {
				fmt.Fprintf(&b, "    +%d -%d lines\n", p.Insertions, p.Deletions)
			}
			totalCommits += p.CommitCount
			totalInsertions += p.Insertions
			totalDeletions += p.Deletions
		}

		b.WriteString("\n")
	}

	// Footer.
	b.WriteString("  " + strings.Repeat("─", 54) + "\n")
	var parts []string
	parts = append(parts, plural(len(projects), "repo"))
	if totalFiles > 0 {
		parts = append(parts, plural(totalFiles, "file")+" changed")
	}
	if totalCommits > 0 {
		commitPart := plural(totalCommits, "commit")
		if totalInsertions > 0 || totalDeletions > 0 {
			commitPart += fmt.Sprintf(" · +%d -%d lines", totalInsertions, totalDeletions)
		}
		parts = append(parts, commitPart)
	}
	if summary.DeepSessions > 0 {
		parts = append(parts, plural(summary.DeepSessions, "deep session"))
	}
	fmt.Fprintf(&b, "  %s\n\n", strings.Join(parts, " · "))

	return b.String()
}

// RenderStandup produces plain text bullet points for your team.
func RenderStandup(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No activity to report.\n"
	}

	var b strings.Builder

	date := summary.Date.Format("Jan 2 2006")
	fmt.Fprintf(&b, "%s:\n", date)

	projects := sortedProjects(summary.ByProject)

	var primary, minor []model.ProjectSummary
	for _, p := range projects {
		if p.Interactions >= 5 || len(p.FilesCreated)+len(p.FilesModified) >= 3 || p.CommitCount >= 2 {
			primary = append(primary, p)
		} else {
			minor = append(minor, p)
		}
	}

	for _, p := range primary {
		fmt.Fprintf(&b, "• %s\n", describeProjectStandup(p))
	}

	if len(minor) > 0 {
		var names []string
		for _, p := range minor {
			names = append(names, p.Name)
		}
		fmt.Fprintf(&b, "• Minor work on %s\n", strings.Join(names, ", "))
	}

	return b.String()
}

// RenderCost produces a billing view — project/model/cost.
func RenderCost(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No cost data for this period.\n"
	}

	var b strings.Builder

	date := summary.Date.Format("Monday, Jan 2 2006")
	fmt.Fprintf(&b, "\n  Cost — %s\n", date)
	b.WriteString("  " + strings.Repeat("─", 54) + "\n\n")

	projects := sortedProjects(summary.ByProject)
	for _, p := range projects {
		if p.TotalCost <= 0 {
			continue
		}
		fmt.Fprintf(&b, "  %-40s $%.2f\n", p.Name, p.TotalCost)

		// Models under each project.
		type modelCost struct {
			name string
			cost float64
		}
		var models []modelCost
		for _, a := range summary.Activities {
			if a.Project == p.Name && a.CostUSD > 0 {
				models = append(models, modelCost{name: shortModelName(a.Model), cost: a.CostUSD})
			}
		}
		// Merge by model name.
		merged := make(map[string]float64)
		for _, m := range models {
			merged[m.name] += m.cost
		}
		var sorted []modelCost
		for name, cost := range merged {
			sorted = append(sorted, modelCost{name: name, cost: cost})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].cost > sorted[j].cost })
		for _, m := range sorted {
			fmt.Fprintf(&b, "    %-26s $%.2f\n", m.name, m.cost)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RenderCostFooter renders the today/week/month cost summary with per-model breakdown.
func RenderCostFooter(cs CostSummary) string {
	var b strings.Builder
	b.WriteString("  " + strings.Repeat("─", 54) + "\n")
	label := cs.PeriodLabel
	if label == "" {
		label = "Today"
	}
	switch label {
	case "This month":
		fmt.Fprintf(&b, "  This month: $%.2f\n", cs.MonthCost)
	case "This week":
		fmt.Fprintf(&b, "  This week: $%.2f · This month: $%.2f\n", cs.WeekCost, cs.MonthCost)
	default:
		fmt.Fprintf(&b, "  %s: $%.2f · This week: $%.2f · This month: $%.2f\n", label, cs.PeriodCost, cs.WeekCost, cs.MonthCost)
	}

	// Week per-model breakdown.
	if len(cs.WeekByModel) > 0 {
		b.WriteString("\n  Week by model:\n")
		renderModelBreakdown(&b, cs.WeekByModel)
	}

	// Month per-model breakdown.
	if len(cs.MonthByModel) > 0 {
		b.WriteString("\n  Month by model:\n")
		renderModelBreakdown(&b, cs.MonthByModel)
	}

	b.WriteString("\n")
	return b.String()
}

func renderModelBreakdown(b *strings.Builder, byModel map[string]model.ModelSummary) {
	type mc struct {
		name string
		cost float64
	}
	var models []mc
	for _, m := range byModel {
		if m.TotalCost > 0 {
			models = append(models, mc{name: shortModelName(m.Name), cost: m.TotalCost})
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].cost > models[j].cost })
	for _, m := range models {
		fmt.Fprintf(b, "    %-26s $%.2f\n", m.name, m.cost)
	}
}

// RenderMarkdown produces markdown output for PRs, wikis, or docs.
func RenderMarkdown(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No activity found for this period.\n"
	}

	var b strings.Builder

	date := summary.Date.Format("January 2, 2006")
	fmt.Fprintf(&b, "## %s\n\n", date)

	projects := sortedProjects(summary.ByProject)

	for _, p := range projects {
		fmt.Fprintf(&b, "### %s\n", p.Name)

		if desc := describeActivity(p); desc != "" {
			fmt.Fprintf(&b, "- %s\n", desc)
		}

		if fileLine := formatFileSummary(p.FilesCreated, p.FilesModified); fileLine != "" {
			fmt.Fprintf(&b, "- %s\n", fileLine)
		}

		if p.CommitCount > 0 {
			fmt.Fprintf(&b, "- %s\n", formatCommitMessages(p.CommitMessages, p.CommitCount))
			if p.Insertions > 0 || p.Deletions > 0 {
				fmt.Fprintf(&b, "- +%d -%d lines\n", p.Insertions, p.Deletions)
			}
		}

		b.WriteString("\n")
	}

	return b.String()
}

// describeActivity generates a conversational one-liner for the default view.
func describeActivity(p model.ProjectSummary) string {
	created := len(p.FilesCreated)
	modified := len(p.FilesModified)
	hasAI := hasSource(p.Sources, "claude-code")
	suffix := ""
	if hasAI {
		suffix = " with Claude"
	}

	switch {
	case created >= 5:
		return fmt.Sprintf("Built out new code%s", suffix)
	case created > 0 && modified > 0:
		return fmt.Sprintf("Worked on code%s", suffix)
	case modified >= 3:
		return fmt.Sprintf("Iterated on existing code%s", suffix)
	case created > 0 || modified > 0:
		return fmt.Sprintf("Made updates%s", suffix)
	case p.CommitCount > 0 && !hasAI:
		// Git-only — no description line, just show commits.
		return ""
	case p.Interactions >= 10:
		return fmt.Sprintf("Researched and planned%s", suffix)
	case hasAI:
		return fmt.Sprintf("Quick exploration%s", suffix)
	default:
		return ""
	}
}

// describeProjectStandup generates a conversational description for standup bullets.
func describeProjectStandup(p model.ProjectSummary) string {
	created := len(p.FilesCreated)
	modified := len(p.FilesModified)

	switch {
	case created >= 5:
		return fmt.Sprintf("Built out %s — %s (%s)",
			p.Name,
			plural(created, "new file"),
			topCodeFileNames(p.FilesCreated, p.FilesModified, 4),
		)

	case created > 0 && modified > 0:
		return fmt.Sprintf("Worked on %s — %s created, %s updated (%s)",
			p.Name,
			plural(created, "file"),
			plural(modified, "file"),
			topCodeFileNames(p.FilesCreated, p.FilesModified, 4),
		)

	case modified >= 3:
		return fmt.Sprintf("Iterated on %s — updated %s (%s)",
			p.Name,
			plural(modified, "file"),
			topCodeFileNames(nil, p.FilesModified, 4),
		)

	case created+modified > 0:
		return fmt.Sprintf("Updated %s — %s (%s)",
			p.Name,
			plural(created+modified, "file"),
			topCodeFileNames(p.FilesCreated, p.FilesModified, 4),
		)

	case p.CommitCount > 0:
		return fmt.Sprintf("Shipped %s in %s",
			plural(p.CommitCount, "commit"),
			p.Name,
		)

	case p.Interactions >= 10:
		return fmt.Sprintf("Researched and planned in %s",
			p.Name,
		)

	default:
		return fmt.Sprintf("Explored %s", p.Name)
	}
}

// formatCommitMessages formats commit messages for display.
func formatCommitMessages(messages []string, count int) string {
	if len(messages) == 0 {
		return plural(count, "commit")
	}

	max := 3
	shown := messages
	if len(shown) > max {
		shown = shown[:max]
	}

	var quoted []string
	for _, m := range shown {
		if len(m) > 60 {
			m = m[:57] + "..."
		}
		quoted = append(quoted, fmt.Sprintf("%q", m))
	}

	result := "Committed: " + strings.Join(quoted, ", ")
	if count > max {
		result += fmt.Sprintf(" +%d more", count-max)
	}
	return result
}

func hasSource(sources []string, name string) bool {
	for _, s := range sources {
		if s == name {
			return true
		}
	}
	return false
}

// topCodeFileNames returns a short string of the most important file names.
func topCodeFileNames(created, modified []string, max int) string {
	files := prioritizeFiles(created, modified)
	if len(files) == 0 {
		return ""
	}
	shown := files
	more := 0
	if len(shown) > max {
		shown = shown[:max]
		more = len(files) - max
	}
	result := strings.Join(shown, ", ")
	if more > 0 {
		result += fmt.Sprintf(" +%d more", more)
	}
	return result
}

// formatFileSummary builds a concise line showing files created and modified.
func formatFileSummary(created, modified []string) string {
	if len(created) == 0 && len(modified) == 0 {
		return ""
	}

	allFiles := prioritizeFiles(created, modified)

	var prefix []string
	if len(created) > 0 {
		prefix = append(prefix, fmt.Sprintf("Created %s", plural(len(created), "file")))
	}
	if len(modified) > 0 {
		prefix = append(prefix, fmt.Sprintf("updated %s", plural(len(modified), "file")))
	}

	line := strings.Join(prefix, ", ")
	if len(allFiles) > 0 {
		shown := allFiles
		more := 0
		if len(shown) > 6 {
			shown = shown[:6]
			more = len(allFiles) - 6
		}
		line += " — " + strings.Join(shown, ", ")
		if more > 0 {
			line += fmt.Sprintf(" +%d more", more)
		}
	}

	return line
}

// prioritizeFiles returns deduplicated base file names, code files first.
func prioritizeFiles(created, modified []string) []string {
	seen := make(map[string]bool)
	var code, other []string

	add := func(paths []string) {
		for _, p := range paths {
			base := filepath.Base(p)
			if seen[base] {
				continue
			}
			seen[base] = true
			if isCodeFile(base) {
				code = append(code, base)
			} else {
				other = append(other, base)
			}
		}
	}

	add(created)
	add(modified)
	return append(code, other...)
}

var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
	".jsx": true, ".rs": true, ".java": true, ".c": true, ".cpp": true,
	".h": true, ".rb": true, ".swift": true, ".kt": true, ".scala": true,
	".sh": true, ".bash": true, ".zsh": true, ".tf": true, ".hcl": true,
	".sql": true, ".proto": true, ".graphql": true, ".css": true,
	".html": true, ".vue": true, ".svelte": true,
}

func isCodeFile(name string) bool {
	return codeExtensions[filepath.Ext(name)]
}

// shortModelName converts "claude-opus-4-6" → "opus 4.6".
func shortModelName(model string) string {
	m := strings.ToLower(model)

	for _, family := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(m, family) {
			parts := strings.Split(m, family+"-")
			if len(parts) == 2 {
				ver := parts[1]
				segments := strings.SplitN(ver, "-", 3)
				if len(segments) >= 2 {
					return family + " " + segments[0] + "." + segments[1]
				}
				if len(segments) == 1 {
					return family + " " + segments[0]
				}
			}
			return family
		}
	}

	return model
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func sortedProjects(m map[string]model.ProjectSummary) []model.ProjectSummary {
	var ps []model.ProjectSummary
	for _, p := range m {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool {
		return ps[i].Interactions > ps[j].Interactions
	})
	return ps
}
