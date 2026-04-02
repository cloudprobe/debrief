package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cloudprobe/debrief/internal/model"
)

const noCostData = "No cost data for this period.\n"

// RenderCostTable produces a box-drawing table with Date | Model | Cost (USD) columns.
// Each day group shows per-model rows sorted alphabetically, a subtotal row, and a grand total.
// Days with empty ByModel are skipped entirely (git-only days).
func RenderCostTable(days []model.DaySummary) string {
	if len(days) == 0 {
		return noCostData
	}

	type modelRow struct {
		model string // shortModelName output
		cost  float64
	}
	type dayGroup struct {
		date     string
		rows     []modelRow // sorted alphabetically by model name
		subtotal float64
	}

	var groups []dayGroup
	var grandTotal float64

	for _, day := range days {
		if len(day.ByModel) == 0 {
			continue
		}

		// Collect and sort model keys alphabetically.
		keys := make([]string, 0, len(day.ByModel))
		for k := range day.ByModel {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var mrows []modelRow
		var subtotal float64
		for _, k := range keys {
			ms := day.ByModel[k]
			mrows = append(mrows, modelRow{
				model: shortModelName(k),
				cost:  ms.TotalCost,
			})
			subtotal += ms.TotalCost
		}

		grandTotal += subtotal
		groups = append(groups, dayGroup{
			date:     day.Date.Format("2006-01-02"),
			rows:     mrows,
			subtotal: subtotal,
		})
	}

	if len(groups) == 0 {
		return noCostData
	}

	// Compute column widths.
	dateW := len("Date")
	if len("Total") > dateW {
		dateW = len("Total")
	}
	// subtotal label: "─ subtotal" = 10 rune display width
	const subtotalLabel = "\u2500 subtotal"
	modelW := len("Model")
	subtotalLabelW := utf8.RuneCountInString(subtotalLabel)
	if subtotalLabelW > modelW {
		modelW = subtotalLabelW
	}
	const costHeader = "Cost (USD)"
	const costW = len(costHeader)

	for _, dg := range groups {
		if len(dg.date) > dateW {
			dateW = len(dg.date)
		}
		for _, mr := range dg.rows {
			w := len(mr.model)
			if w > modelW {
				modelW = w
			}
		}
	}

	// Box-drawing helpers.
	var b strings.Builder

	topBorder := func() {
		fmt.Fprintf(&b, "\u250c%s\u252c%s\u252c%s\u2510\n",
			strings.Repeat("\u2500", dateW+2),
			strings.Repeat("\u2500", modelW+2),
			strings.Repeat("\u2500", costW+2))
	}
	midBorder := func() {
		fmt.Fprintf(&b, "\u251c%s\u253c%s\u253c%s\u2524\n",
			strings.Repeat("\u2500", dateW+2),
			strings.Repeat("\u2500", modelW+2),
			strings.Repeat("\u2500", costW+2))
	}
	botBorder := func() {
		fmt.Fprintf(&b, "\u2514%s\u2534%s\u2534%s\u2518\n",
			strings.Repeat("\u2500", dateW+2),
			strings.Repeat("\u2500", modelW+2),
			strings.Repeat("\u2500", costW+2))
	}
	cell := func(content string, width int) string {
		return fmt.Sprintf(" %-*s ", width, content)
	}
	// cellRune pads by rune count — needed for multi-byte box-drawing chars in subtotal label.
	cellRune := func(content string, width int) string {
		pad := width - utf8.RuneCountInString(content)
		if pad < 0 {
			pad = 0
		}
		return " " + content + strings.Repeat(" ", pad) + " "
	}
	costCell := func(val string) string {
		return fmt.Sprintf(" %*s ", costW, val)
	}

	// Header.
	topBorder()
	fmt.Fprintf(&b, "\u2502%s\u2502%s\u2502%s\u2502\n",
		cell("Date", dateW),
		cell("Model", modelW),
		costCell(costHeader))
	midBorder()

	// Day groups.
	for i, dg := range groups {
		if i > 0 {
			midBorder()
		}
		for j, mr := range dg.rows {
			dateCol := ""
			if j == 0 {
				dateCol = dg.date
			}
			fmt.Fprintf(&b, "\u2502%s\u2502%s\u2502%s\u2502\n",
				cell(dateCol, dateW),
				cell(mr.model, modelW),
				costCell(fmt.Sprintf("$%.2f", mr.cost)))
		}
		// Subtotal row.
		fmt.Fprintf(&b, "\u2502%s\u2502%s\u2502%s\u2502\n",
			cell("", dateW),
			cellRune(subtotalLabel, modelW),
			costCell(fmt.Sprintf("$%.2f", dg.subtotal)))
	}

	// Grand total row.
	midBorder()
	fmt.Fprintf(&b, "\u2502%s\u2502%s\u2502%s\u2502\n",
		cell("Total", dateW),
		cell("", modelW),
		costCell(fmt.Sprintf("$%.2f", grandTotal)))
	botBorder()

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

var (
	reGitHubPR = regexp.MustCompile(`https://github\.com/[^\s/]+/[^\s/]+/pull/\d+`)
	reGitLabMR = regexp.MustCompile(`https://gitlab\.com/[^\s/]+/[^\s/]+/-/merge_requests/\d+`)
	reBareRef  = regexp.MustCompile(`^#\d+$`)

	prIntentWords = map[string]bool{
		"closes": true, "close": true, "closed": true,
		"fixes": true, "fix": true, "fixed": true,
		"resolves": true, "resolve": true, "resolved": true,
		"pr": true, "merge": true, "merged": true,
	}
)

// ExtractPRLinks extracts PR/MR links from commit messages.
// Returns a deduplicated, ordered slice: full URLs first, bare #N after.
// Bare #N references are only included when a PR-intent word appears
// within ~5 words in the same sentence.
func ExtractPRLinks(commitMessages []string) []string {
	seenURLs := make(map[string]bool)
	seenRefs := make(map[string]bool)
	var urls, refs []string

	for _, msg := range commitMessages {
		// Extract full GitHub URLs unconditionally.
		for _, u := range reGitHubPR.FindAllString(msg, -1) {
			if !seenURLs[u] {
				seenURLs[u] = true
				urls = append(urls, u)
			}
		}
		// Extract full GitLab URLs unconditionally.
		for _, u := range reGitLabMR.FindAllString(msg, -1) {
			if !seenURLs[u] {
				seenURLs[u] = true
				urls = append(urls, u)
			}
		}
		// Extract bare #N only with adjacent PR-intent word.
		words := strings.Fields(strings.ToLower(msg))
		for i, w := range words {
			ref := strings.TrimRight(w, ".,;:!?)")
			if !reBareRef.MatchString(ref) {
				continue
			}
			// Check window of 5 words before/after for a PR-intent word.
			lo := i - 5
			if lo < 0 {
				lo = 0
			}
			hi := i + 5
			if hi >= len(words) {
				hi = len(words) - 1
			}
			for _, nearby := range words[lo : hi+1] {
				nearby = strings.Trim(nearby, ".,;:!?()")
				if prIntentWords[nearby] {
					if !seenRefs[ref] {
						seenRefs[ref] = true
						refs = append(refs, ref)
					}
					break
				}
			}
		}
	}

	sort.Strings(urls)
	sort.Strings(refs)
	return append(urls, refs...)
}
