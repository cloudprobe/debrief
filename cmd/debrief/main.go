package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudprobe/debrief/internal/aggregator"
	"github.com/cloudprobe/debrief/internal/collector"
	"github.com/cloudprobe/debrief/internal/config"
	"github.com/cloudprobe/debrief/internal/model"
	"github.com/cloudprobe/debrief/internal/ui"
	"github.com/spf13/cobra"
)

var (
	version  = "dev"
	format   string
	date     string
	fromDate string
	toDate   string
	showCost bool
	verbose  bool
	noGit    bool
)

func main() {
	root := &cobra.Command{
		Use:     "debrief",
		Short:   "Know what you actually did today — git commits, AI sessions, one command",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			dr, err := resolveDateRange("", date, fromDate, toDate)
			if err != nil {
				return err
			}
			return run(dr)
		},
	}

	root.PersistentFlags().StringVarP(&format, "format", "o", "", "output format: text, json, standup, markdown")
	root.PersistentFlags().StringVarP(&date, "date", "d", "", "specific date (YYYY-MM-DD)")
	root.PersistentFlags().StringVarP(&fromDate, "from", "f", "", "start date for range (YYYY-MM-DD)")
	root.PersistentFlags().StringVarP(&toDate, "to", "t", "", "end date for range (YYYY-MM-DD)")
	root.PersistentFlags().BoolVarP(&showCost, "cost", "c", false, "show billing view with estimated API costs")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "show debug output on stderr")
	root.PersistentFlags().BoolVar(&noGit, "no-git", false, "skip git commit collection")

	root.AddCommand(yesterdayCmd())
	root.AddCommand(weekCmd())
	root.AddCommand(monthCmd())
	root.AddCommand(standupCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func yesterdayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "yesterday",
		Short: "Show yesterday's activity summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(yesterdayRange())
		},
	}
}

func weekCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "week",
		Short: "Show this week's activity summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(weekRange())
		},
	}
}

func monthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "month",
		Short: "Show this month's activity summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(monthRange())
		},
	}
}

func standupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "standup [week]",
		Short: "Generate copy-paste standup summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			format = "standup"

			// standup week = this week, per-day
			if len(args) > 0 && args[0] == "week" {
				return run(weekRange())
			}

			dr, err := resolveDateRange("", date, fromDate, toDate)
			if err != nil {
				return err
			}
			// Default standup = today (same as base command).
			return run(dr)
		},
	}
	return cmd
}

func run(dr model.DateRange) error {
	cfg := config.Load()

	collectors := buildCollectors(cfg)

	var allActivities []model.Activity
	for _, c := range collectors {
		if !c.Available() {
			if verbose {
				fmt.Fprintf(os.Stderr, "[debrief] skipping %s: not available\n", c.Name())
			}
			continue
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "[debrief] collecting from %s...\n", c.Name())
		}
		activities, err := c.Collect(dr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", c.Name(), err)
			continue
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "[debrief] %s: %d activities found\n", c.Name(), len(activities))
		}
		allActivities = append(allActivities, activities...)
	}

	// For multi-day ranges, split into per-day summaries.
	days := splitByDay(allActivities, dr)

	// Resolve format: flag > config > default.
	outputFormat := format
	if outputFormat == "" {
		outputFormat = cfg.DefaultFormat
	}
	if outputFormat == "" {
		outputFormat = "text"
	}

	opts := ui.RenderOptions{ShowCost: showCost}

	singleDay := len(days) <= 1

	switch outputFormat {
	case "json":
		summary := aggregator.Aggregate(allActivities)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)

	case "standup":
		for i, day := range days {
			if i > 0 {
				fmt.Println()
			}
			fmt.Print(ui.RenderStandup(day, opts))
		}

	case "markdown":
		for i, day := range days {
			if i > 0 {
				fmt.Println()
			}
			fmt.Print(ui.RenderMarkdown(day, opts))
		}

	default:
		if showCost {
			for _, day := range days {
				fmt.Print(ui.RenderCost(day, opts))
			}
			costSummary := buildCostSummary(cfg, allActivities, dr)
			fmt.Print(ui.RenderCostFooter(costSummary))
		} else {
			for i, day := range days {
				if i > 0 {
					fmt.Println()
				}
				renderOpts := opts
				renderOpts.SingleDay = singleDay
				fmt.Print(ui.RenderText(day, renderOpts))
			}
		}
	}

	return nil
}

func buildCollectors(cfg config.Config) []collector.Collector {
	var collectors []collector.Collector
	collectors = append(collectors, collector.NewClaudeCollector(cfg.ClaudeDir, showCost, cfg.Pricing))
	if !noGit {
		collectors = append(collectors, collector.NewGitCollector(cfg.GitRepoPaths, cfg.GitDiscoveryDepth))
	}
	return collectors
}

// buildCostSummary builds cost summary, reusing already-collected activities
// and only fetching wider ranges when the current range doesn't cover them.
func buildCostSummary(cfg config.Config, currentActivities []model.Activity, currentDR model.DateRange) ui.CostSummary {
	currentSummary := aggregator.Aggregate(currentActivities)

	wr := weekRange()
	mr := monthRange()

	// If current range covers week, reuse. Otherwise collect.
	var weekSummary model.DaySummary
	if !currentDR.Start.After(wr.Start) && !currentDR.End.Before(wr.End) {
		weekSummary = currentSummary
	} else {
		weekSummary = aggregator.Aggregate(collectCostActivities(cfg, wr))
	}

	// If current range covers month, reuse. Otherwise collect.
	var monthSummary model.DaySummary
	if !currentDR.Start.After(mr.Start) && !currentDR.End.Before(mr.End) {
		monthSummary = currentSummary
	} else {
		monthSummary = aggregator.Aggregate(collectCostActivities(cfg, mr))
	}

	// Determine period label based on range.
	periodLabel := "Today"
	days := int(currentDR.End.Sub(currentDR.Start).Hours() / 24)
	if days > 20 {
		periodLabel = "This month"
	} else if days > 2 {
		periodLabel = "This week"
	}

	return ui.CostSummary{
		PeriodLabel:  periodLabel,
		PeriodCost:   currentSummary.TotalCost,
		WeekCost:     weekSummary.TotalCost,
		MonthCost:    monthSummary.TotalCost,
		WeekByModel:  weekSummary.ByModel,
		MonthByModel: monthSummary.ByModel,
	}
}

// collectCostActivities runs Claude collector for a date range and returns activities.
func collectCostActivities(cfg config.Config, dr model.DateRange) []model.Activity {
	c := collector.NewClaudeCollector(cfg.ClaudeDir, true, cfg.Pricing)
	if !c.Available() {
		return nil
	}
	activities, _ := c.Collect(dr)
	return activities
}

// splitByDay groups activities into per-day summaries.
func splitByDay(activities []model.Activity, dr model.DateRange) []model.DaySummary {
	// Group by calendar day.
	byDay := make(map[string][]model.Activity)
	for _, a := range activities {
		day := a.Timestamp.Format("2006-01-02")
		byDay[day] = append(byDay[day], a)
	}

	// If only one day or empty, return a single summary.
	if len(byDay) <= 1 {
		return []model.DaySummary{aggregator.Aggregate(activities)}
	}

	// Generate one summary per day in the range.
	var days []model.DaySummary
	for d := dr.Start; d.Before(dr.End); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		if dayActivities, ok := byDay[key]; ok {
			days = append(days, aggregator.Aggregate(dayActivities))
		}
	}
	return days
}

// resolveDateRange resolves flags into a DateRange.
// Priority: --from/--to > --date > default (today).
func resolveDateRange(arg, dateFlag, from, to string) (model.DateRange, error) {
	if from != "" || to != "" {
		return parseFromTo(from, to)
	}
	if dateFlag != "" {
		return parseSingleDate(dateFlag)
	}
	if arg == "week" {
		return weekRange(), nil
	}
	return todayRange(), nil
}

func parseFromTo(from, to string) (model.DateRange, error) {
	var start, end time.Time
	var err error

	if from != "" {
		start, err = time.Parse("2006-01-02", from)
		if err != nil {
			return model.DateRange{}, fmt.Errorf("invalid --from date %q, expected YYYY-MM-DD: %w", from, err)
		}
	} else {
		start = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())
	}

	if to != "" {
		end, err = time.Parse("2006-01-02", to)
		if err != nil {
			return model.DateRange{}, fmt.Errorf("invalid --to date %q, expected YYYY-MM-DD: %w", to, err)
		}
		end = end.Add(24 * time.Hour) // Include the end date fully.
	} else {
		end = start.Add(24 * time.Hour)
	}

	return model.DateRange{Start: start, End: end}, nil
}

func parseSingleDate(dateStr string) (model.DateRange, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return model.DateRange{}, fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", dateStr, err)
	}
	return model.DateRange{
		Start: t,
		End:   t.Add(24 * time.Hour),
	}, nil
}

func todayRange() model.DateRange {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)
	return model.DateRange{Start: start, End: end}
}

func yesterdayRange() model.DateRange {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := end.Add(-24 * time.Hour)
	return model.DateRange{Start: start, End: end}
}

func weekRange() model.DateRange {
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

func monthRange() model.DateRange {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	return model.DateRange{Start: start, End: end}
}
