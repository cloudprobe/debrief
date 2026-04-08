package collector

import (
	"os"
	"path/filepath"
	"time"

	"github.com/cloudprobe/debrief/internal/journal"
	"github.com/cloudprobe/debrief/internal/model"
)

const journalSource = "journal"

// JournalCollector reads debrief log entries for the date range and
// surfaces them as session notes under the journalSource pseudo-project.
type JournalCollector struct {
	cfgDir string
}

// NewJournalCollector creates a JournalCollector reading from cfgDir.
func NewJournalCollector(cfgDir string) *JournalCollector {
	return &JournalCollector{cfgDir: cfgDir}
}

func (j *JournalCollector) Name() string { return journalSource }

func (j *JournalCollector) Available() bool {
	dir := filepath.Join(j.cfgDir, journalSource)
	_, err := os.Stat(dir)
	return err == nil
}

func (j *JournalCollector) Collect(dr model.DateRange) ([]model.Activity, error) {
	var activities []model.Activity
	for d := dr.Start; !d.After(dr.End); d = d.AddDate(0, 0, 1) {
		entries, err := journal.ReadEntries(j.cfgDir, d)
		if err != nil || len(entries) == 0 {
			continue
		}
		notes := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.Message != "" {
				notes = append(notes, e.Message)
			}
		}
		if len(notes) == 0 {
			continue
		}
		activities = append(activities, model.Activity{
			Source:       journalSource,
			Project:      journalSource,
			Timestamp:    time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, d.Location()),
			SessionNotes: notes,
		})
	}
	return activities, nil
}
