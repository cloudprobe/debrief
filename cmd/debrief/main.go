package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudprobe/debrief/internal/aggregator"
	"github.com/cloudprobe/debrief/internal/collector"
	"github.com/cloudprobe/debrief/internal/config"
	"github.com/cloudprobe/debrief/internal/model"
	"github.com/cloudprobe/debrief/internal/synthesizer"
	"github.com/cloudprobe/debrief/internal/ui"
	versioncheck "github.com/cloudprobe/debrief/internal/version"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const argWeek = "week"

var (
	version  = "dev"
	date     string
	fromDate string
	toDate   string
)

func main() {
	root := &cobra.Command{
		Use:   "debrief",
		Short: "Know what you actually did today — git commits, AI sessions, one command",
		Long: `Know what you actually did today — git commits, AI sessions, one command.

Run "debrief <command> -help" for information on a specific command.`,
		// No RunE — shows help when invoked with no subcommand.
	}

	// Suppress the default "completion" command cobra adds automatically.
	root.CompletionOptions.DisableDefaultCmd = true

	root.PersistentFlags().StringVarP(&date, "date", "d", "", "specific date (YYYY-MM-DD)")
	root.PersistentFlags().StringVarP(&fromDate, "from", "f", "", "start date for range (YYYY-MM-DD)")
	root.PersistentFlags().StringVarP(&toDate, "to", "t", "", "end date for range (YYYY-MM-DD)")

	// -version flag as alias for "version" subcommand (terraform-style).
	var showVersion bool
	root.PersistentFlags().BoolVarP(&showVersion, "version", "", false, "An alias for the \"version\" subcommand.")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Printf("debrief %s\n", version)
			os.Exit(0)
		}
		return nil
	}

	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		cacheDir := config.ConfigDir()
		if info, ok := versioncheck.CheckForUpdate(cacheDir, version); ok {
			fmt.Fprintf(os.Stderr, "A new version of debrief is available (%s). Upgrade: brew upgrade debrief\n", info.Latest) //nolint:errcheck
		}
		return nil
	}

	root.AddGroup(&cobra.Group{ID: "main", Title: "Available Commands:"})

	root.AddCommand(initCmd())
	root.AddCommand(standupCmd())
	root.AddCommand(costCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Set up debrief for first use",
		GroupID: "main",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}
	return cmd
}

func standupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "standup [yesterday|week]",
		Short:     "Generate a copy-paste standup summary",
		GroupID:   "main",
		ValidArgs: []string{"yesterday", argWeek},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				switch args[0] {
				case "yesterday":
					return runStandup(yesterdayRange())
				case argWeek:
					return runStandup(weekRange())
				}
			}
			dr, err := resolveDateRange(date, fromDate, toDate)
			if err != nil {
				return err
			}
			return runStandup(dr)
		},
	}
	return cmd
}

func costCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "cost [yesterday|week|month]",
		Short:     "Show estimated API costs",
		GroupID:   "main",
		ValidArgs: []string{"yesterday", argWeek, "month"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				switch args[0] {
				case "yesterday":
					return runCost(yesterdayRange())
				case argWeek:
					return runCost(weekRange())
				case "month":
					return runCost(monthRange())
				}
			}
			dr, err := resolveDateRange(date, fromDate, toDate)
			if err != nil {
				return err
			}
			return runCost(dr)
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show the current debrief version",
		GroupID: "main",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("debrief %s\n", version)
		},
	}
}

func runInit() error {
	existing := config.Load()

	currentPreset := existing.Pricing.Preset
	if currentPreset == "" {
		currentPreset = "direct"
	}

	fmt.Println("How do you access Claude Code?")
	fmt.Println("  1) direct  — Anthropic API (pay per token)")
	fmt.Println("  2) max     — Max or Pro subscription (flat rate)")
	fmt.Println("  3) vertex  — Google Vertex AI")
	fmt.Println("  4) bedrock — AWS Bedrock")
	fmt.Printf("Preset [%s]: ", currentPreset)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		input = currentPreset
	}

	numericPresets := map[string]string{"1": "direct", "2": "max", "3": "vertex", "4": "bedrock"}
	if name, ok := numericPresets[input]; ok {
		input = name
	}

	validPresets := map[string]bool{"direct": true, "max": true, "vertex": true, "bedrock": true}
	if !validPresets[input] {
		return fmt.Errorf("unknown preset %q — choose: direct, max, vertex, bedrock", input)
	}

	existing.Pricing.Preset = input

	dir := config.ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("Configuration saved to %s\n", cfgFile)
	fmt.Println("Setup complete.")
	return nil
}

func runStandup(dr model.DateRange) error {
	cfg := config.Load()
	allActivities := collectActivities(cfg, dr, false)
	days := splitByDay(allActivities, dr)
	fmt.Print(synthesizer.Synthesize(days))
	return nil
}

func runCost(dr model.DateRange) error {
	cfg := config.Load()

	preset := cfg.Pricing.Preset
	if preset == "" {
		fmt.Println("Cost view requires setup. Run `debrief init` to get started.")
		return nil
	}
	if preset == "max" {
		fmt.Println("You're on a Max/Pro subscription — per-token costs don't apply.")
		fmt.Println("Run `debrief standup` to see your activity summary.")
		return nil
	}

	allActivities := collectActivities(cfg, dr, true)
	days := splitByDay(allActivities, dr)
	opts := ui.RenderOptions{ShowCost: true}
	for _, day := range days {
		fmt.Print(ui.RenderCost(day, opts))
	}
	costSummary := buildCostSummary(cfg, allActivities, dr)
	fmt.Print(ui.RenderCostFooter(costSummary))
	return nil
}

func collectActivities(cfg config.Config, dr model.DateRange, costMode bool) []model.Activity {
	collectors := []collector.Collector{
		collector.NewClaudeCollector(cfg.ClaudeDir, costMode, cfg.Pricing, config.ConfigDir()),
		collector.NewGitCollector(cfg.GitRepoPaths, cfg.GitDiscoveryDepth),
	}
	var all []model.Activity
	for _, c := range collectors {
		if !c.Available() {
			continue
		}
		activities, err := c.Collect(dr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", c.Name(), err) //nolint:errcheck
			continue
		}
		all = append(all, activities...)
	}
	return all
}

func buildCostSummary(cfg config.Config, currentActivities []model.Activity, currentDR model.DateRange) ui.CostSummary {
	currentSummary := aggregator.Aggregate(currentActivities)

	wr := weekRange()
	mr := monthRange()

	var weekSummary model.DaySummary
	if !currentDR.Start.After(wr.Start) && !currentDR.End.Before(wr.End) {
		weekSummary = currentSummary
	} else {
		weekSummary = aggregator.Aggregate(collectActivities(cfg, wr, true))
	}

	var monthSummary model.DaySummary
	if !currentDR.Start.After(mr.Start) && !currentDR.End.Before(mr.End) {
		monthSummary = currentSummary
	} else {
		monthSummary = aggregator.Aggregate(collectActivities(cfg, mr, true))
	}

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

func splitByDay(activities []model.Activity, dr model.DateRange) []model.DaySummary {
	byDay := make(map[string][]model.Activity)
	for _, a := range activities {
		day := a.Timestamp.Format("2006-01-02")
		byDay[day] = append(byDay[day], a)
	}

	if len(byDay) <= 1 {
		return []model.DaySummary{aggregator.Aggregate(activities)}
	}

	var days []model.DaySummary
	for d := dr.Start; d.Before(dr.End); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		if dayActivities, ok := byDay[key]; ok {
			days = append(days, aggregator.Aggregate(dayActivities))
		}
	}
	return days
}

func resolveDateRange(dateFlag, from, to string) (model.DateRange, error) {
	if from != "" || to != "" {
		return parseFromTo(from, to)
	}
	if dateFlag != "" {
		return parseSingleDate(dateFlag)
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

func parseSingleDate(dateStr string) (model.DateRange, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return model.DateRange{}, fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", dateStr, err)
	}
	return model.DateRange{Start: t, End: t.Add(24 * time.Hour)}, nil
}

func todayRange() model.DateRange {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return model.DateRange{Start: start, End: start.Add(24 * time.Hour)}
}

func yesterdayRange() model.DateRange {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return model.DateRange{Start: end.Add(-24 * time.Hour), End: end}
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
