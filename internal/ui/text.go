package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/model"
)

// RenderOptions controls what's shown in the output.
type RenderOptions struct {
	ShowCost  bool
	SingleDay bool // true when showing a single day (adds "Your day —" prefix)
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
		fmt.Fprintf(&b, "\n  Your day \u2014 %s\n", date)
	} else {
		fmt.Fprintf(&b, "\n  %s\n", date)
	}
	b.WriteString("  " + strings.Repeat("\u2500", 54) + "\n\n")

	projects := sortedProjects(summary.ByProject)
	totalCommits := 0

	for _, p := range projects {
		if p.SummaryLine == "" && p.CommitCount == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %s\n", p.Name)
		if p.SummaryLine != "" {
			fmt.Fprintf(&b, "    %s\n", p.SummaryLine)
		}
		// Commit bullets — signal commits only, falling back to all if none qualify.
		for _, msg := range SignalCommits(p.CommitMessages) {
			fmt.Fprintf(&b, "    \u2022 %s\n", msg)
		}
		totalCommits += p.CommitCount
		b.WriteString("\n")
	}

	// Footer.
	b.WriteString("  " + strings.Repeat("\u2500", 54) + "\n")
	var parts []string
	shown := 0
	for _, p := range projects {
		if p.SummaryLine == "" && p.CommitCount == 0 {
			continue
		}
		shown++
	}
	parts = append(parts, plural(shown, "project"))
	if totalCommits > 0 {
		parts = append(parts, plural(totalCommits, "commit"))
	}
	fmt.Fprintf(&b, "  %s\n\n", strings.Join(parts, " \u00b7 "))

	return b.String()
}

// RenderStandup produces plain text bullet points for your team.
func RenderStandup(summary model.DaySummary, opts RenderOptions) string {
	if len(summary.Activities) == 0 {
		return "No activity to report.\n"
	}

	var b strings.Builder
	date := summary.Date.Format("Jan 2 2006")
	fmt.Fprintf(&b, "%s:\n\n", date)

	projects := sortedProjects(summary.ByProject)

	for _, p := range projects {
		if p.SummaryLine == "" && p.CommitCount == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s\n", p.Name)
		if p.SummaryLine != "" {
			fmt.Fprintf(&b, "  %s\n", p.SummaryLine)
		}
		for _, msg := range SignalCommits(p.CommitMessages) {
			fmt.Fprintf(&b, "  \u2022 %s\n", msg)
		}
		b.WriteString("\n")
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
	fmt.Fprintf(&b, "\n  Cost \u2014 %s\n", date)
	b.WriteString("  " + strings.Repeat("\u2500", 54) + "\n\n")

	projects := sortedProjects(summary.ByProject)
	for _, p := range projects {
		if p.TotalCost <= 0 {
			continue
		}
		fmt.Fprintf(&b, "  %-40s %s\n", p.Name, formatCost(p.TotalCost))

		// Models under each project.
		type modelCost struct {
			name string
			cost float64
		}
		var models []modelCost
		for _, a := range summary.Activities {
			if a.Project == p.Name && a.CostUSD != 0 {
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
			fmt.Fprintf(&b, "    %-26s %s\n", m.name, formatCost(m.cost))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RenderCostFooter renders the today/week/month cost summary with per-model breakdown.
func RenderCostFooter(cs CostSummary) string {
	var b strings.Builder
	b.WriteString("  " + strings.Repeat("\u2500", 54) + "\n")
	label := cs.PeriodLabel
	if label == "" {
		label = "Today"
	}
	switch label {
	case "This month":
		fmt.Fprintf(&b, "  This month: $%.2f\n", cs.MonthCost)
	case "This week":
		fmt.Fprintf(&b, "  This week: $%.2f \u00b7 This month: $%.2f\n", cs.WeekCost, cs.MonthCost)
	default:
		fmt.Fprintf(&b, "  %s: $%.2f \u00b7 This week: $%.2f \u00b7 This month: $%.2f\n", label, cs.PeriodCost, cs.WeekCost, cs.MonthCost)
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
		if p.SummaryLine == "" && p.CommitCount == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", p.Name)
		if p.SummaryLine != "" {
			fmt.Fprintf(&b, "%s\n\n", p.SummaryLine)
		}
		for _, msg := range SignalCommits(p.CommitMessages) {
			fmt.Fprintf(&b, "- %s\n", msg)
		}
		if len(p.CommitMessages) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
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

// formatCost formats a cost value, returning an em-dash for unknown (negative) costs.
func formatCost(c float64) string {
	if c < 0 {
		return "\u2014"
	}
	return fmt.Sprintf("$%.2f", c)
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

// noiseCommitTypes are conventional commit prefixes that carry no standup value.
var noiseCommitTypes = map[string]bool{
	"chore": true, "docs": true, "ci": true, "test": true, "style": true,
}

// noiseScopes are scopes within any prefix that produce noise (e.g. fix(lint)).
var noiseScopes = map[string]bool{
	"lint": true, "nolint": true, "comment": true, "typo": true,
	"spelling": true, "whitespace": true, "format": true,
}

// SignalCommits filters commit messages to those with standup value.
// Falls back to the original slice if nothing survives the filter.
func SignalCommits(messages []string) []string {
	var out []string
	for _, msg := range messages {
		if IsSignalCommit(msg) {
			out = append(out, msg)
		}
	}
	if len(out) == 0 {
		return messages
	}
	return out
}

// IsSignalCommit returns true if the commit message represents real work
// worth surfacing in a standup — not chore, docs, ci, lint fixes, etc.
func IsSignalCommit(msg string) bool {
	lower := strings.ToLower(strings.TrimSpace(msg))

	// Parse "type(scope): ..." or "type: ..."
	colonIdx := strings.Index(lower, ":")
	if colonIdx < 0 {
		return true // no conventional prefix → treat as signal
	}
	prefix := lower[:colonIdx]

	// Extract scope from "type(scope)".
	commitType := prefix
	scope := ""
	if i := strings.Index(prefix, "("); i > 0 && strings.HasSuffix(prefix, ")") {
		commitType = prefix[:i]
		scope = prefix[i+1 : len(prefix)-1]
	}

	if noiseCommitTypes[commitType] {
		return false
	}
	if scope != "" && noiseScopes[scope] {
		return false
	}
	return true
}

func sortedProjects(m map[string]model.ProjectSummary) []model.ProjectSummary {
	var ps []model.ProjectSummary
	for _, p := range m {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].CommitCount != ps[j].CommitCount {
			return ps[i].CommitCount > ps[j].CommitCount
		}
		return ps[i].Interactions > ps[j].Interactions
	})
	return ps
}
