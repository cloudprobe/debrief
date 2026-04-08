// Package journal handles per-day journal entries and last-standup state for debrief.
// All functions accept cfgDir as the debrief config directory (e.g. ~/.config/debrief)
// so tests can use t.TempDir() without touching real config.
package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is a single journal entry.
type Entry struct {
	Time    string // "HH:MM"
	Message string
}

// Path returns the journal file path for the given date.
// Returns: <cfgDir>/journal/2006-01-02.md
func Path(cfgDir string, date time.Time) string {
	return filepath.Join(cfgDir, "journal", date.Format("2006-01-02")+".md")
}

// Append adds a timestamped entry to today's journal file.
// Creates the file with header if it doesn't exist.
func Append(cfgDir string, t time.Time, msg string) error {
	dir := filepath.Join(cfgDir, "journal")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating journal directory: %w", err)
	}

	p := Path(cfgDir, t)
	timeStr := t.Format("15:04")
	bullet := fmt.Sprintf("- [%s] %s\n", timeStr, msg)

	if _, err := os.Stat(p); os.IsNotExist(err) {
		header := fmt.Sprintf("# Journal — %s\n\n## Entries\n%s", t.Format("2006-01-02"), bullet)
		return os.WriteFile(p, []byte(header), 0600)
	}

	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening journal file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = f.WriteString(bullet)
	return err
}

// ReadEntries reads journal entries for the given date.
// Returns (nil, nil) if the file doesn't exist.
func ReadEntries(cfgDir string, date time.Time) ([]Entry, error) {
	p := Path(cfgDir, date)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading journal file: %w", err)
	}

	var entries []Entry
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		// Format: - [HH:MM] message
		rest := line[3:] // strip "- ["
		closeBracket := strings.Index(rest, "]")
		if closeBracket < 0 {
			continue
		}
		timeStr := rest[:closeBracket]
		// Validate time looks like HH:MM
		if len(timeStr) != 5 || timeStr[2] != ':' {
			continue
		}
		msgRaw := rest[closeBracket+1:]
		if len(msgRaw) > 0 && msgRaw[0] == ' ' {
			msgRaw = msgRaw[1:]
		}
		entries = append(entries, Entry{Time: timeStr, Message: msgRaw})
	}
	return entries, nil
}

// LastStandupPath returns the path to the last-standup state file.
// Returns: <cfgDir>/state/last-standup.md
func LastStandupPath(cfgDir string) string {
	return filepath.Join(cfgDir, "state", "last-standup.md")
}

// LastStandupMetaPath returns the path to the last-standup metadata file.
// Returns: <cfgDir>/state/last-standup.meta
func LastStandupMetaPath(cfgDir string) string {
	return filepath.Join(cfgDir, "state", "last-standup.meta")
}

// ReadLastStandup reads the most recent standup text and its date.
// Returns ("", time.Time{}, nil) if file doesn't exist.
func ReadLastStandup(cfgDir string) (text string, date time.Time, err error) {
	textPath := LastStandupPath(cfgDir)
	metaPath := LastStandupMetaPath(cfgDir)

	textData, err := os.ReadFile(textPath)
	if os.IsNotExist(err) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("reading last-standup: %w", err)
	}

	metaData, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("reading last-standup meta: %w", err)
	}

	metaStr := strings.TrimSpace(string(metaData))
	const prefix = "date="
	if !strings.HasPrefix(metaStr, prefix) {
		return "", time.Time{}, fmt.Errorf("malformed last-standup meta: %q", metaStr)
	}
	dateStr := metaStr[len(prefix):]
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parsing last-standup date %q: %w", dateStr, err)
	}

	return string(textData), t, nil
}

// WriteLastStandup writes the standup text and records today's date in the meta file.
func WriteLastStandup(cfgDir string, text string, date time.Time) error {
	dir := filepath.Join(cfgDir, "state")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	if err := os.WriteFile(LastStandupPath(cfgDir), []byte(text), 0600); err != nil {
		return fmt.Errorf("writing last-standup: %w", err)
	}

	meta := fmt.Sprintf("date=%s\n", date.Format("2006-01-02"))
	if err := os.WriteFile(LastStandupMetaPath(cfgDir), []byte(meta), 0600); err != nil {
		return fmt.Errorf("writing last-standup meta: %w", err)
	}

	return nil
}
