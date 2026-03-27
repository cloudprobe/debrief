package aggregator

import (
	"sort"
	"time"

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
		summary.TotalCost += a.CostUSD
		summary.TotalTokens += a.TokensIn + a.TokensOut
		summary.Interactions += a.Interactions

		// By project.
		p := summary.ByProject[a.Project]
		p.Name = a.Project
		p.TotalCost += a.CostUSD
		p.TotalTokens += a.TokensIn + a.TokensOut
		p.Interactions += a.Interactions
		p.CommitCount += a.CommitCount
		p.CommitMessages = append(p.CommitMessages, a.CommitMessages...)
		p.Insertions += a.Insertions
		p.Deletions += a.Deletions
		p.SessionCount++
		p.Sessions = append(p.Sessions, a)
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
			m.TotalCost += a.CostUSD
			m.CallCount++
			summary.ByModel[a.Model] = m
		}
	}

	// Calculate deep sessions (sustained work blocks >30 min on one project).
	summary.DeepSessions = countDeepSessions(activities)

	return summary
}

// countDeepSessions counts sustained work blocks where >30 min was spent
// on a single project without switching to another project.
func countDeepSessions(activities []model.Activity) int {
	if len(activities) == 0 {
		return 0
	}

	deep := 0
	currentProject := activities[0].Project
	blockStart := activities[0].Timestamp

	for i := 1; i < len(activities); i++ {
		a := activities[i]
		if a.Project != currentProject {
			// Project switch — check if the block we just finished was deep.
			blockEnd := activities[i-1].EndTime
			if !blockEnd.IsZero() && blockEnd.Sub(blockStart) >= 30*time.Minute {
				deep++
			}
			currentProject = a.Project
			blockStart = a.Timestamp
		}
	}

	// Check the last block.
	last := activities[len(activities)-1]
	blockEnd := last.EndTime
	if blockEnd.IsZero() {
		blockEnd = last.Timestamp
	}
	if blockEnd.Sub(blockStart) >= 30*time.Minute {
		deep++
	}

	return deep
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
