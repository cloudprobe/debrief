package synthesis

import (
	"fmt"
	"strings"

	"github.com/cloudprobe/debrief/internal/journal"
)

const maxPreviousStandupChars = 1500

// renderExtras builds the journal + previous-standup prefix block for the payload.
// Returns "" if both are empty/nil.
func renderExtras(journalEntries []journal.Entry, previousStandup, previousDate string) string {
	var sb strings.Builder

	if len(journalEntries) > 0 {
		fmt.Fprintln(&sb, "journal_entries (highest priority — user explicitly recorded):")
		for _, e := range journalEntries {
			fmt.Fprintf(&sb, "  - [%s] %s\n", e.Time, sanitizeForPrompt(e.Message))
		}
		fmt.Fprintln(&sb)
	}

	if previousStandup != "" {
		fmt.Fprintln(&sb, "previously_reported (omit anything already covered here):")
		fmt.Fprintf(&sb, "  date: %s\n", previousDate)

		// Sanitize line-by-line and cap at 1500 chars total.
		lines := strings.Split(previousStandup, "\n")
		var sanitized []string
		for _, line := range lines {
			sanitized = append(sanitized, "  "+sanitizeForPrompt(line))
		}
		combined := strings.Join(sanitized, "\n")
		if len(combined) > maxPreviousStandupChars {
			combined = combined[:maxPreviousStandupChars] + "...[truncated]"
		}
		fmt.Fprintln(&sb, combined)
		fmt.Fprintln(&sb)
	}

	if sb.Len() == 0 {
		return ""
	}

	fmt.Fprintln(&sb, "---")
	return sb.String()
}
