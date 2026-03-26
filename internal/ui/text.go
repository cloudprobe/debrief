package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudprobe/devrecap/internal/model"
)

// RenderOptions controls what's shown in the output.
type RenderOptions struct {
	ShowCost bool
}

// RenderText produces a plain text summary for terminal output.
func RenderText(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No activity found for this period.\n"
	}

	var b strings.Builder

	date := summary.Date.Format("Monday, Jan 2 2006")
	b.WriteString(fmt.Sprintf("\n  devrecap — %s\n", date))
	b.WriteString("  " + strings.Repeat("─", 54) + "\n\n")

	for _, p := range sortedProjects(summary.ByProject) {
		b.WriteString(fmt.Sprintf("  %s\n", p.Name))

		// Stats line.
		var stats []string
		if p.Interactions > 0 {
			stats = append(stats, plural(p.Interactions, "interaction"))
		}
		if p.TotalTokens > 0 {
			stats = append(stats, fmt.Sprintf("%s tokens", formatTokens(p.TotalTokens)))
		}
		if opts.ShowCost && p.TotalCost > 0 {
			stats = append(stats, fmt.Sprintf("$%.2f", p.TotalCost))
		}
		if len(p.Models) > 0 {
			stats = append(stats, strings.Join(shortModelNames(p.Models), ", "))
		}
		if len(stats) > 0 {
			b.WriteString(fmt.Sprintf("    %s\n", strings.Join(stats, " · ")))
		}

		// Files line — the actual work product.
		if fileLine := formatFileSummary(p.FilesCreated, p.FilesModified); fileLine != "" {
			b.WriteString(fmt.Sprintf("    %s\n", fileLine))
		}

		// Commits.
		if p.CommitCount > 0 {
			b.WriteString(fmt.Sprintf("    %s\n", plural(p.CommitCount, "commit")))
		}

		b.WriteString("\n")
	}

	// Model token breakdown.
	if len(summary.ByModel) > 0 {
		b.WriteString("  Models\n")
		for _, m := range sortedModels(summary.ByModel) {
			tokens := m.TokensIn + m.TokensOut
			line := fmt.Sprintf("    %-24s %s tokens", shortModelName(m.Name), formatTokens(tokens))
			if opts.ShowCost && m.TotalCost > 0 {
				line += fmt.Sprintf("  $%.2f", m.TotalCost)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	// Footer.
	b.WriteString("  " + strings.Repeat("─", 54) + "\n")
	totalLine := fmt.Sprintf("  %s · %s tokens",
		plural(summary.Interactions, "interaction"),
		formatTokens(summary.TotalTokens),
	)
	if opts.ShowCost && summary.TotalCost > 0 {
		totalLine += fmt.Sprintf(" · $%.2f", summary.TotalCost)
	}
	b.WriteString(totalLine + "\n\n")

	return b.String()
}

// RenderStandup produces a conversational, copy-paste ready standup summary.
func RenderStandup(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No activity to report.\n"
	}

	var b strings.Builder

	date := summary.Date.Format("Jan 2")
	b.WriteString(fmt.Sprintf("**%s:**\n", date))

	projects := sortedProjects(summary.ByProject)

	// Split into primary work (significant) and minor touches.
	var primary, minor []model.ProjectSummary
	for _, p := range projects {
		if p.Interactions >= 5 || len(p.FilesCreated)+len(p.FilesModified) >= 3 || p.CommitCount >= 2 {
			primary = append(primary, p)
		} else {
			minor = append(minor, p)
		}
	}

	for _, p := range primary {
		b.WriteString(fmt.Sprintf("• %s\n", describeProject(p)))
	}

	if len(minor) > 0 {
		var names []string
		for _, p := range minor {
			names = append(names, p.Name)
		}
		b.WriteString(fmt.Sprintf("• Minor work on %s\n", strings.Join(names, ", ")))
	}

	return b.String()
}

// describeProject generates a conversational description of work on a project.
func describeProject(p model.ProjectSummary) string {
	created := len(p.FilesCreated)
	modified := len(p.FilesModified)
	totalFiles := created + modified

	// Determine the activity type from what actually happened.
	switch {
	case created >= 5:
		// Significant new code — scaffolding/building.
		return fmt.Sprintf("Built out **%s** — created %s (%s)",
			p.Name,
			plural(created, "new file"),
			topCodeFileNames(p.FilesCreated, p.FilesModified, 4),
		)

	case created > 0 && modified > 0:
		// Mix of new and updated files.
		return fmt.Sprintf("Worked on **%s** — %s created, %s updated (%s)",
			p.Name,
			plural(created, "file"),
			plural(modified, "file"),
			topCodeFileNames(p.FilesCreated, p.FilesModified, 4),
		)

	case modified >= 3:
		// Mostly editing existing code.
		return fmt.Sprintf("Iterated on **%s** — updated %s (%s)",
			p.Name,
			plural(modified, "file"),
			topCodeFileNames(nil, p.FilesModified, 4),
		)

	case totalFiles > 0:
		// Small file changes.
		return fmt.Sprintf("Updated **%s** — %s (%s)",
			p.Name,
			plural(totalFiles, "file"),
			topCodeFileNames(p.FilesCreated, p.FilesModified, 4),
		)

	case p.CommitCount > 0:
		// Git commits but no AI file tracking.
		return fmt.Sprintf("Shipped %s in **%s**",
			plural(p.CommitCount, "commit"),
			p.Name,
		)

	case p.Interactions >= 10:
		// Lots of interaction but no files — research/planning/debugging.
		return fmt.Sprintf("Researched and planned in **%s** (%s)",
			p.Name,
			plural(p.Interactions, "AI interaction"),
		)

	default:
		// Light interaction.
		return fmt.Sprintf("Explored **%s** (%s)",
			p.Name,
			plural(p.Interactions, "interaction"),
		)
	}
}

// topCodeFileNames returns a short string of the most important file names
// from both created and modified lists.
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

	// Merge all file names for a combined display, prioritizing code files.
	allFiles := prioritizeFiles(created, modified)

	var prefix []string
	if len(created) > 0 {
		prefix = append(prefix, plural(len(created), "file")+" created")
	}
	if len(modified) > 0 {
		prefix = append(prefix, plural(len(modified), "file")+" modified")
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

	// Anthropic models.
	for _, family := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(m, family) {
			// Extract version: claude-opus-4-6 → 4.6, claude-haiku-4-5-20251001 → 4.5
			parts := strings.Split(m, family+"-")
			if len(parts) == 2 {
				ver := parts[1]
				// Take first two numeric segments: "4-6" → "4.6", "4-5-20251001" → "4.5"
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

	// OpenAI models — pass through.
	// Google models — pass through.
	return model
}

func shortModelNames(models []string) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = shortModelName(m)
	}
	return out
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

func sortedModels(m map[string]model.ModelSummary) []model.ModelSummary {
	var ms []model.ModelSummary
	for _, v := range m {
		ms = append(ms, v)
	}
	sort.Slice(ms, func(i, j int) bool {
		return (ms[i].TokensIn + ms[i].TokensOut) > (ms[j].TokensIn + ms[j].TokensOut)
	})
	return ms
}
