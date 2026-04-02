package daterange

import (
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

// fixedTime returns a time.Time in UTC for deterministic tests.
func fixedDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// ---- ParseSingleDate -------------------------------------------------------

func TestParseSingleDate_Valid(t *testing.T) {
	tests := []struct {
		input     string
		wantStart time.Time
	}{
		{"2026-03-25", fixedDate(2026, time.March, 25)},
		{"2025-01-01", fixedDate(2025, time.January, 1)},
		{"2024-12-31", fixedDate(2024, time.December, 31)},
	}
	for _, tt := range tests {
		dr, err := ParseSingleDate(tt.input)
		if err != nil {
			t.Errorf("ParseSingleDate(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if !dr.Start.Equal(tt.wantStart) {
			t.Errorf("ParseSingleDate(%q).Start = %v, want %v", tt.input, dr.Start, tt.wantStart)
		}
		wantEnd := tt.wantStart.Add(24 * time.Hour)
		if !dr.End.Equal(wantEnd) {
			t.Errorf("ParseSingleDate(%q).End = %v, want %v", tt.input, dr.End, wantEnd)
		}
	}
}

func TestParseSingleDate_Invalid(t *testing.T) {
	invalids := []string{
		"not-a-date",
		"2026/03/25",
		"25-03-2026",
		"",
		"2026-13-01",
	}
	for _, s := range invalids {
		_, err := ParseSingleDate(s)
		if err == nil {
			t.Errorf("ParseSingleDate(%q) expected error, got nil", s)
		}
	}
}

// ---- ParseFromTo -----------------------------------------------------------

func TestParseFromTo_BothProvided(t *testing.T) {
	dr, err := ParseFromTo("2026-03-01", "2026-03-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := fixedDate(2026, time.March, 1)
	wantEnd := fixedDate(2026, time.March, 8) // to + 24h
	if !dr.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", dr.Start, wantStart)
	}
	if !dr.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", dr.End, wantEnd)
	}
}

func TestParseFromTo_OnlyFrom(t *testing.T) {
	dr, err := ParseFromTo("2026-03-25", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := fixedDate(2026, time.March, 25)
	if !dr.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", dr.Start, wantStart)
	}
	wantEnd := wantStart.Add(24 * time.Hour)
	if !dr.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", dr.End, wantEnd)
	}
}

func TestParseFromTo_OnlyTo(t *testing.T) {
	// from="" means today; we just verify no error and End is To+24h.
	dr, err := ParseFromTo("", "2026-03-25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantEnd := fixedDate(2026, time.March, 26)
	if !dr.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", dr.End, wantEnd)
	}
}

func TestParseFromTo_InvalidFrom(t *testing.T) {
	_, err := ParseFromTo("bad-date", "2026-03-07")
	if err == nil {
		t.Error("expected error for invalid --from, got nil")
	}
}

func TestParseFromTo_InvalidTo(t *testing.T) {
	_, err := ParseFromTo("2026-03-01", "not-a-date")
	if err == nil {
		t.Error("expected error for invalid --to, got nil")
	}
}

// ---- Resolve ---------------------------------------------------------------

func TestResolve_UsesSingleDateWhenSet(t *testing.T) {
	dr, err := Resolve("2026-03-25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := fixedDate(2026, time.March, 25)
	if !dr.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", dr.Start, wantStart)
	}
}

func TestResolve_DefaultsToToday(t *testing.T) {
	dr, err := Resolve("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Now()
	wantDay := now.Format("2006-01-02")
	if dr.Start.Format("2006-01-02") != wantDay {
		t.Errorf("Start day = %q, want %q", dr.Start.Format("2006-01-02"), wantDay)
	}
}

// ---- TodayRange / YesterdayRange -------------------------------------------

func TestTodayRange_SpansToday(t *testing.T) {
	dr := TodayRange()
	now := time.Now()
	if dr.Start.Format("2006-01-02") != now.Format("2006-01-02") {
		t.Errorf("TodayRange start day = %q, want today %q", dr.Start.Format("2006-01-02"), now.Format("2006-01-02"))
	}
	if dr.End.Sub(dr.Start) != 24*time.Hour {
		t.Errorf("TodayRange span = %v, want 24h", dr.End.Sub(dr.Start))
	}
}

func TestYesterdayRange_SpansYesterday(t *testing.T) {
	dr := YesterdayRange()
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	if dr.Start.Format("2006-01-02") != yesterday.Format("2006-01-02") {
		t.Errorf("YesterdayRange start day = %q, want %q", dr.Start.Format("2006-01-02"), yesterday.Format("2006-01-02"))
	}
	if dr.End.Sub(dr.Start) != 24*time.Hour {
		t.Errorf("YesterdayRange span = %v, want 24h", dr.End.Sub(dr.Start))
	}
}

// ---- WeekRange -------------------------------------------------------------

func TestWeekRange_StartIsMonday(t *testing.T) {
	dr := WeekRange()
	if dr.Start.Weekday() != time.Monday {
		t.Errorf("WeekRange start weekday = %v, want Monday", dr.Start.Weekday())
	}
}

func TestWeekRange_EndIsToday(t *testing.T) {
	dr := WeekRange()
	now := time.Now()
	if dr.End.Format("2006-01-02") != now.Format("2006-01-02") {
		t.Errorf("WeekRange end day = %q, want today %q", dr.End.Format("2006-01-02"), now.Format("2006-01-02"))
	}
}

func TestWeekRange_StartNotAfterEnd(t *testing.T) {
	dr := WeekRange()
	if dr.Start.After(dr.End) {
		t.Errorf("WeekRange start %v is after end %v", dr.Start, dr.End)
	}
}

// ---- MonthRange ------------------------------------------------------------

func TestMonthRange_StartsOnFirstOfMonth(t *testing.T) {
	dr := MonthRange()
	if dr.Start.Day() != 1 {
		t.Errorf("MonthRange start day = %d, want 1", dr.Start.Day())
	}
}

func TestMonthRange_EndsToday(t *testing.T) {
	dr := MonthRange()
	now := time.Now()
	if dr.End.Format("2006-01-02") != now.Format("2006-01-02") {
		t.Errorf("MonthRange end day = %q, want today %q", dr.End.Format("2006-01-02"), now.Format("2006-01-02"))
	}
}

func TestMonthRange_SameMonthAsToday(t *testing.T) {
	dr := MonthRange()
	now := time.Now()
	if dr.Start.Month() != now.Month() || dr.Start.Year() != now.Year() {
		t.Errorf("MonthRange start %v not in current month %v/%d", dr.Start, now.Month(), now.Year())
	}
}

// ---- SplitByDay ------------------------------------------------------------

// stubAggregate is a minimal aggregator for testing SplitByDay.
func stubAggregate(activities []model.Activity) model.DaySummary {
	if len(activities) == 0 {
		return model.DaySummary{}
	}
	return model.DaySummary{
		Date:       activities[0].Timestamp,
		Activities: activities,
	}
}

func TestSplitByDay_SingleDay(t *testing.T) {
	day := fixedDate(2026, time.March, 25)
	activities := []model.Activity{
		{Timestamp: day, Project: "proj-a"},
		{Timestamp: day.Add(time.Hour), Project: "proj-b"},
	}
	dr := model.DateRange{Start: day, End: day.Add(24 * time.Hour)}
	summaries := SplitByDay(activities, dr, stubAggregate)
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary for single-day activities, got %d", len(summaries))
	}
}

func TestSplitByDay_MultipleDays(t *testing.T) {
	day1 := fixedDate(2026, time.March, 25)
	day2 := fixedDate(2026, time.March, 26)
	activities := []model.Activity{
		{Timestamp: day1, Project: "proj-a"},
		{Timestamp: day2, Project: "proj-b"},
	}
	dr := model.DateRange{Start: day1, End: day2.Add(24 * time.Hour)}
	summaries := SplitByDay(activities, dr, stubAggregate)
	if len(summaries) != 2 {
		t.Errorf("expected 2 summaries for two-day activities, got %d", len(summaries))
	}
}

func TestSplitByDay_Empty(t *testing.T) {
	dr := model.DateRange{
		Start: fixedDate(2026, time.March, 25),
		End:   fixedDate(2026, time.March, 26),
	}
	summaries := SplitByDay(nil, dr, stubAggregate)
	if len(summaries) != 1 {
		t.Errorf("expected 1 (empty) summary, got %d", len(summaries))
	}
}

func TestSplitByDay_PreservesOrder(t *testing.T) {
	day1 := fixedDate(2026, time.March, 24)
	day2 := fixedDate(2026, time.March, 25)
	day3 := fixedDate(2026, time.March, 26)
	activities := []model.Activity{
		{Timestamp: day3, Project: "proj-c"},
		{Timestamp: day1, Project: "proj-a"},
		{Timestamp: day2, Project: "proj-b"},
	}
	dr := model.DateRange{Start: day1, End: day3.Add(24 * time.Hour)}
	summaries := SplitByDay(activities, dr, stubAggregate)
	if len(summaries) != 3 {
		t.Errorf("expected 3 summaries, got %d", len(summaries))
	}
	// SplitByDay iterates dr.Start forward, so results should be chronological.
	for i := 1; i < len(summaries); i++ {
		if summaries[i].Date.Before(summaries[i-1].Date) {
			t.Errorf("summaries not in chronological order at index %d", i)
		}
	}
}

// ---- WeekRange boundary cases ----------------------------------------------

// TestWeekRange_BoundaryMonday verifies that WeekRange always starts on Monday,
// covering the Monday boundary case explicitly.
func TestWeekRange_BoundaryMonday(t *testing.T) {
	dr := WeekRange()
	// Monday boundary: Start must be a Monday (weekday=1) regardless of today's day.
	if dr.Start.Weekday() != time.Monday {
		t.Errorf("WeekRange start weekday = %v, want Monday (boundary check)", dr.Start.Weekday())
	}
}

// TestWeekRange_SpanIsAtMost7Days verifies that WeekRange never spans more than
// 7 days — catches off-by-one errors in week boundary computation.
func TestWeekRange_SpanIsAtMost7Days(t *testing.T) {
	dr := WeekRange()
	span := dr.End.Sub(dr.Start)
	if span > 7*24*time.Hour {
		t.Errorf("WeekRange span = %v, want at most 7*24h (168h)", span)
	}
}

// TestResolve_EmptyReturnsToday verifies that Resolve("") returns a range
// starting today (the default case when no --date flag is provided).
func TestResolve_EmptyReturnsToday(t *testing.T) {
	dr, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\") unexpected error: %v", err)
	}
	now := time.Now()
	wantDay := now.Format("2006-01-02")
	if dr.Start.Format("2006-01-02") != wantDay {
		t.Errorf("Resolve(\"\").Start day = %q, want today %q", dr.Start.Format("2006-01-02"), wantDay)
	}
}

// TestResolve_ValidDateReturnsRange verifies that Resolve with a specific date
// returns a 24-hour range starting at that date.
func TestResolve_ValidDateReturnsRange(t *testing.T) {
	dr, err := Resolve("2026-06-15")
	if err != nil {
		t.Fatalf("Resolve(\"2026-06-15\") unexpected error: %v", err)
	}
	wantStart := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC)
	if !dr.Start.Equal(wantStart) {
		t.Errorf("Resolve start = %v, want %v", dr.Start, wantStart)
	}
	if !dr.End.Equal(wantEnd) {
		t.Errorf("Resolve end = %v, want %v", dr.End, wantEnd)
	}
}
