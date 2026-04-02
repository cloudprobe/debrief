// Package daterange provides helpers for resolving named or explicit date ranges
// into model.DateRange values used by the debrief collectors.
package daterange

import (
	"fmt"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

// Resolve returns a DateRange based on the --date CLI flag.
// If dateFlag is non-empty, ParseSingleDate is used.
// Otherwise TodayRange is returned.
func Resolve(dateFlag string) (model.DateRange, error) {
	if dateFlag != "" {
		return ParseSingleDate(dateFlag)
	}
	return TodayRange(), nil
}

// ParseFromTo parses --from / --to flag values (YYYY-MM-DD) into a DateRange.
// Missing from defaults to today; missing to defaults to from + 24 h.
func ParseFromTo(from, to string) (model.DateRange, error) {
	var start, end time.Time
	var err error

	if from != "" {
		start, err = time.Parse("2006-01-02", from)
		if err != nil {
			return model.DateRange{}, fmt.Errorf("invalid --from date %q, expected YYYY-MM-DD: %w", from, err)
		}
	} else {
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	if to != "" {
		end, err = time.Parse("2006-01-02", to)
		if err != nil {
			return model.DateRange{}, fmt.Errorf("invalid --to date %q, expected YYYY-MM-DD: %w", to, err)
		}
		end = end.Add(24 * time.Hour)
	} else {
		end = start.Add(24 * time.Hour)
	}

	return model.DateRange{Start: start, End: end}, nil
}

// ParseSingleDate parses a single YYYY-MM-DD string into a one-day DateRange.
func ParseSingleDate(dateStr string) (model.DateRange, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return model.DateRange{}, fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", dateStr, err)
	}
	return model.DateRange{Start: t, End: t.Add(24 * time.Hour)}, nil
}

// TodayRange returns a DateRange spanning the current calendar day (local time).
func TodayRange() model.DateRange {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return model.DateRange{Start: start, End: start.Add(24 * time.Hour)}
}

// YesterdayRange returns a DateRange spanning the previous calendar day (local time).
func YesterdayRange() model.DateRange {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return model.DateRange{Start: end.Add(-24 * time.Hour), End: end}
}

// WeekRange returns a DateRange from Monday of the current ISO week through end-of-today.
func WeekRange() model.DateRange {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := end.Add(-time.Duration(weekday-1) * 24 * time.Hour)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	return model.DateRange{Start: start, End: end}
}

// MonthRange returns a DateRange from the first of the current month through end-of-today.
func MonthRange() model.DateRange {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	return model.DateRange{Start: start, End: end}
}

// SplitByDay groups activities by calendar day and returns one DaySummary per
// day that has activity. If all activities fall on the same day (or there are
// none), a single aggregated summary is returned via the provided aggregate func.
//
// All timestamps are normalised to dr.Start's location before grouping so that
// git activities (local time) and Claude activities (UTC) land on the same
// calendar day when the user views them.
func SplitByDay(activities []model.Activity, dr model.DateRange, aggregate func([]model.Activity) model.DaySummary) []model.DaySummary {
	loc := dr.Start.Location()

	byDay := make(map[string][]model.Activity)
	for _, a := range activities {
		day := a.Timestamp.In(loc).Format("2006-01-02")
		byDay[day] = append(byDay[day], a)
	}

	if len(byDay) <= 1 {
		return []model.DaySummary{aggregate(activities)}
	}

	var days []model.DaySummary
	for d := dr.Start; d.Before(dr.End); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		if dayActivities, ok := byDay[key]; ok {
			days = append(days, aggregate(dayActivities))
		}
	}
	return days
}
