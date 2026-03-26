package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudprobe/devrecap/internal/aggregator"
	"github.com/cloudprobe/devrecap/internal/collector"
	"github.com/cloudprobe/devrecap/internal/model"
	"github.com/cloudprobe/devrecap/internal/ui"
	"github.com/spf13/cobra"
)

var (
	version  = "dev"
	format   string
	date     string
	showCost bool
)

func main() {
	root := &cobra.Command{
		Use:     "devrecap",
		Short:   "Know what you actually did today, including your AI sessions",
		Version: version,
	}

	root.PersistentFlags().StringVar(&format, "format", "text", "output format: text, json, standup")
	root.PersistentFlags().StringVar(&date, "date", "", "specific date (YYYY-MM-DD)")
	root.PersistentFlags().BoolVar(&showCost, "cost", false, "show estimated API costs (for pay-per-token users)")

	root.AddCommand(todayCmd())
	root.AddCommand(yesterdayCmd())
	root.AddCommand(weekCmd())
	root.AddCommand(standupCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func todayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "Show today's activity summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			dr := todayRange()
			if date != "" {
				var err error
				dr, err = parseDateRange(date)
				if err != nil {
					return err
				}
			}
			return run(dr)
		},
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

func standupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "standup",
		Short: "Generate copy-paste standup summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			format = "standup"
			return run(yesterdayRange())
		},
	}
}

func run(dr model.DateRange) error {
	collectors := []collector.Collector{
		collector.NewClaudeCollector("", showCost),
	}

	var allActivities []model.Activity
	for _, c := range collectors {
		if !c.Available() {
			continue
		}
		activities, err := c.Collect(dr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", c.Name(), err)
			continue
		}
		allActivities = append(allActivities, activities...)
	}

	summary := aggregator.Aggregate(allActivities)
	opts := ui.RenderOptions{ShowCost: showCost}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	case "standup":
		fmt.Print(ui.RenderStandup(summary, opts))
	default:
		fmt.Print(ui.RenderText(summary, opts))
	}

	return nil
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

func parseDateRange(dateStr string) (model.DateRange, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return model.DateRange{}, fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", dateStr, err)
	}
	return model.DateRange{
		Start: t,
		End:   t.Add(24 * time.Hour),
	}, nil
}
