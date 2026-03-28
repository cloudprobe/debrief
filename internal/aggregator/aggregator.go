package aggregator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudprobe/debrief/internal/model"
)

// Aggregate takes a slice of activities and produces a DaySummary.
func Aggregate(activities []model.Activity) model.DaySummary {
	summary := model.DaySummary{
		ByProject: make(map[string]model.ProjectSummary),
		ByModel:   make(map[string]model.ModelSummary),
	}

	if len(activities) == 0 {
		return summary
	}

	// Sort by timestamp.
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].Timestamp.Before(activities[j].Timestamp)
	})

	summary.Date = activities[0].Timestamp
	summary.Activities = activities

	for _, a := range activities {
		if a.CostUSD >= 0 {
			summary.TotalCost += a.CostUSD
		}
		summary.TotalTokens += a.TokensIn + a.TokensOut
		summary.Interactions += a.Interactions

		// By project.
		p := summary.ByProject[a.Project]
		p.Name = a.Project
		if a.CostUSD >= 0 {
			p.TotalCost += a.CostUSD
		}
		p.TotalTokens += a.TokensIn + a.TokensOut
		p.Interactions += a.Interactions
		p.CommitCount += a.CommitCount
		p.CommitMessages = append(p.CommitMessages, a.CommitMessages...)
		p.Insertions += a.Insertions
		p.Deletions += a.Deletions
		p.Sources = appendUnique(p.Sources, a.Source)
		if a.Model != "" {
			p.Models = appendUnique(p.Models, a.Model)
		}
		p.FilesCreated = appendUniqueSlice(p.FilesCreated, a.FilesCreated)
		p.FilesModified = appendUniqueSlice(p.FilesModified, a.FilesModified)
		p.ToolBreakdown = mergeToolBreakdown(p.ToolBreakdown, a.ToolBreakdown)
		summary.ByProject[a.Project] = p

		// By model.
		if a.Model != "" {
			m := summary.ByModel[a.Model]
			m.Name = a.Model
			m.TokensIn += a.TokensIn
			m.TokensOut += a.TokensOut
			if a.CostUSD >= 0 {
				m.TotalCost += a.CostUSD
			}
			m.CallCount++
			summary.ByModel[a.Model] = m
		}
	}

	// Derive summary lines for each project.
	for name, p := range summary.ByProject {
		p.SummaryLine = deriveProjectSummaryLine(p)
		summary.ByProject[name] = p
	}

	return summary
}

// deriveProjectSummaryLine generates a one-line headline for a project.
// Commits take priority per D-03; file changes are the fallback.
func deriveProjectSummaryLine(p model.ProjectSummary) string {
	if len(p.CommitMessages) > 0 {
		return summarizeCommits(p.CommitMessages)
	}
	if len(p.FilesCreated) > 0 || len(p.FilesModified) > 0 {
		return describeFromFiles(p.FilesCreated, p.FilesModified)
	}
	return ""
}

// conventionalPrefixes are recognized conventional commit type prefixes.
var conventionalPrefixes = map[string]bool{
	"feat": true, "fix": true, "test": true, "chore": true,
	"docs": true, "refactor": true, "perf": true, "build": true, "ci": true,
}

// stripConventionalPrefix removes "feat:", "fix(scope):", etc. from a commit message.
func stripConventionalPrefix(msg string) string {
	parts := strings.SplitN(msg, ":", 2)
	if len(parts) != 2 {
		return msg
	}
	prefix := strings.ToLower(strings.TrimSpace(parts[0]))
	// Strip scope: feat(auth) -> feat
	if i := strings.Index(prefix, "("); i > 0 {
		prefix = prefix[:i]
	}
	if conventionalPrefixes[prefix] {
		return strings.TrimSpace(parts[1])
	}
	return msg
}

// summarizeCommits produces a one-line headline from commit messages.
// Prefers the first signal commit (feat/fix/refactor/perf); falls back to
// the first message of any type. The full list is shown as bullets below.
func summarizeCommits(messages []string) string {
	if len(messages) == 0 {
		return ""
	}
	// Try signal commits first.
	for _, m := range messages {
		if isSignalType(m) {
			if s := stripConventionalPrefix(m); s != "" {
				return strings.ToUpper(s[:1]) + s[1:]
			}
		}
	}
	// Fall back to first commit of any type.
	for _, m := range messages {
		if s := stripConventionalPrefix(m); s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return ""
}

// isSignalType returns true if a commit message type has standup value.
// Mirrors the logic in ui.isSignalCommit but kept local to avoid a package cycle.
func isSignalType(msg string) bool {
	lower := strings.ToLower(strings.TrimSpace(msg))
	colonIdx := strings.Index(lower, ":")
	if colonIdx < 0 {
		return true // free-form message → signal
	}
	prefix := lower[:colonIdx]
	commitType := prefix
	scope := ""
	if i := strings.Index(prefix, "("); i > 0 && strings.HasSuffix(prefix, ")") {
		commitType = prefix[:i]
		scope = prefix[i+1 : len(prefix)-1]
	}
	noiseTypes := map[string]bool{
		"chore": true, "docs": true, "ci": true, "test": true, "style": true,
	}
	noiseScopes := map[string]bool{
		"lint": true, "nolint": true, "comment": true, "typo": true,
		"spelling": true, "whitespace": true, "format": true,
	}
	if noiseTypes[commitType] {
		return false
	}
	if scope != "" && noiseScopes[scope] {
		return false
	}
	return true
}

// describeFromFiles produces a headline from file changes when no commits exist.
func describeFromFiles(created, modified []string) string {
	total := len(created) + len(modified)
	if total == 0 {
		return ""
	}
	if len(created) > 0 && len(modified) > 0 {
		return fmt.Sprintf("Created %d files and modified %d files", len(created), len(modified))
	}
	if len(created) > 0 {
		return fmt.Sprintf("Created %d files", len(created))
	}
	return fmt.Sprintf("Modified %d files", len(modified))
}

func mergeToolBreakdown(dst, src map[string]int) map[string]int {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]int)
	}
	for k, v := range src {
		dst[k] += v
	}
	return dst
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

func appendUniqueSlice(dst, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range src {
		if !seen[s] {
			dst = append(dst, s)
			seen[s] = true
		}
	}
	return dst
}
