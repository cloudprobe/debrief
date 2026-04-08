package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/cloudprobe/debrief/internal/aggregator"
	"github.com/cloudprobe/debrief/internal/clipboard"
	"github.com/cloudprobe/debrief/internal/collector"
	"github.com/cloudprobe/debrief/internal/config"
	"github.com/cloudprobe/debrief/internal/daterange"
	"github.com/cloudprobe/debrief/internal/journal"
	"github.com/cloudprobe/debrief/internal/model"
	"github.com/cloudprobe/debrief/internal/synthesizer"
	"github.com/cloudprobe/debrief/internal/ui"
	versioncheck "github.com/cloudprobe/debrief/internal/version"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const argWeek = "week"

var (
	version = "dev"
	date    string
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
	// Suppress the "help" subcommand — keep only the --help flag.
	root.SetHelpCommand(&cobra.Command{Hidden: true})

	root.PersistentFlags().StringVarP(&date, "date", "d", "", "specific date (YYYY-MM-DD)")

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
	root.AddCommand(logCmd())

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
	var projectFilter string
	var format string
	var copyOut bool
	cmd := &cobra.Command{
		Use:       "standup [today|yesterday|week|month]",
		Short:     "Generate a copy-paste standup summary",
		GroupID:   "main",
		ValidArgs: []string{"today", "yesterday", argWeek, "month"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "slack" {
				return fmt.Errorf("invalid --format %q (allowed: text, slack)", format)
			}
			if len(args) > 0 {
				switch args[0] {
				case "today":
					return runStandup(daterange.TodayRange(), "", projectFilter, format, copyOut)
				case "yesterday":
					return runStandup(daterange.YesterdayRange(), "", projectFilter, format, copyOut)
				case argWeek:
					dr := daterange.WeekRange()
					sun := dr.Start.AddDate(0, 0, 6)
					return runStandup(dr, fmt.Sprintf("Week of %s \u2013 %s", dr.Start.Format("Jan 2"), sun.Format("Jan 2, 2006")), projectFilter, format, copyOut)
				case "month":
					dr := daterange.MonthRange()
					return runStandup(dr, dr.Start.Format("January 2006"), projectFilter, format, copyOut)
				default:
					return fmt.Errorf("unknown argument %q (allowed: today, yesterday, week, month)", args[0])
				}
			}
			dr, err := daterange.Resolve(date)
			if err != nil {
				return err
			}
			return runStandup(dr, "", projectFilter, format, copyOut)
		},
	}
	cmd.Flags().StringVarP(&projectFilter, "project", "p", "", "filter to projects matching name")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text or slack")
	cmd.Flags().BoolVar(&copyOut, "copy", false, "copy output to clipboard")
	return cmd
}

func costCmd() *cobra.Command {
	var projectFilter string
	var copyOut bool
	cmd := &cobra.Command{
		Use:       "cost [today|yesterday|week|month]",
		Short:     "Show estimated API costs",
		GroupID:   "main",
		ValidArgs: []string{"today", "yesterday", argWeek, "month"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				switch args[0] {
				case "today":
					return runCost(daterange.TodayRange(), projectFilter, copyOut)
				case "yesterday":
					return runCost(daterange.YesterdayRange(), projectFilter, copyOut)
				case argWeek:
					return runCost(daterange.WeekRange(), projectFilter, copyOut)
				case "month":
					return runCost(daterange.MonthRange(), projectFilter, copyOut)
				default:
					return fmt.Errorf("unknown argument %q (allowed: today, yesterday, week, month)", args[0])
				}
			}
			dr, err := daterange.Resolve(date)
			if err != nil {
				return err
			}
			return runCost(dr, projectFilter, copyOut)
		},
	}
	cmd.Flags().StringVarP(&projectFilter, "project", "p", "", "filter to projects matching name")
	cmd.Flags().BoolVar(&copyOut, "copy", false, "copy output to clipboard")
	return cmd
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

	fmt.Println("Welcome to debrief!")
	fmt.Println("Track your Claude Code sessions and git commits.")
	fmt.Println("Costs are estimated from API token usage \u2014 set your pricing preset to get accurate numbers.")
	fmt.Println()

	fmt.Println("How do you access Claude Code?")
	fmt.Println("  1) direct  \u2014 Anthropic API (pay per token, standard rates)")
	fmt.Println("  2) max     \u2014 Max or Pro subscription (flat rate, cost view disabled)")
	fmt.Println("  3) vertex  \u2014 Google Vertex AI (different per-token rates)")
	fmt.Println("  4) bedrock \u2014 AWS Bedrock (different per-token rates)")
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

	fmt.Printf("Saved to %s\n", cfgFile)
	fmt.Println("Run debrief standup today to see your activity.")
	return nil
}

func runStandup(dr model.DateRange, header string, projectFilter string, format string, copyOut bool) error {
	cfg := config.Load()
	allActivities := collectActivities(cfg, dr, false)
	days := daterange.SplitByDay(allActivities, dr, aggregator.Aggregate)
	if projectFilter != "" {
		days = filterDays(days, projectFilter)
		if len(days) == 0 {
			fmt.Printf("No activity for project matching %q.\n", projectFilter)
			return nil
		}
		fmt.Printf("Showing: projects matching %q\n\n", projectFilter)
	}
	body := synthesizer.SynthesizeSmart(days, header, format == "slack")
	if body != synthesizer.NoActivity {
		if werr := journal.WriteLastStandup(config.ConfigDir(), body, time.Now()); werr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save standup state: %v\n", werr)
		}
	}

	fmt.Print(body)
	if copyOut {
		if tool, ok, err := clipboard.Copy(body); ok {
			fmt.Fprintln(os.Stderr, "[copied to clipboard]")
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "warning: clipboard copy failed (%s): %v\n", tool, err)
		}
	}
	return nil
}

func runCost(dr model.DateRange, projectFilter string, copyOut bool) error {
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
	days := daterange.SplitByDay(allActivities, dr, aggregator.Aggregate)
	if projectFilter != "" {
		days = filterDays(days, projectFilter)
		if len(days) == 0 {
			fmt.Printf("No activity for project matching %q.\n", projectFilter)
			return nil
		}
		fmt.Printf("Showing: projects matching %q\n\n", projectFilter)
	}
	output := ui.RenderCostTable(days)
	fmt.Print(output)
	if copyOut {
		if tool, ok, err := clipboard.Copy(output); ok {
			fmt.Fprintln(os.Stderr, "[copied to clipboard]")
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "warning: clipboard copy failed (%s): %v\n", tool, err)
		}
	}
	return nil
}

// filterDays returns a copy of days containing only projects matching filter
// (case-insensitive substring). TotalCost is recomputed from retained projects.
func filterDays(days []model.DaySummary, filter string) []model.DaySummary {
	lower := strings.ToLower(filter)
	var result []model.DaySummary
	for _, day := range days {
		filtered := make(map[string]model.ProjectSummary)
		var total float64
		for k, p := range day.ByProject {
			if strings.Contains(strings.ToLower(p.Name), lower) {
				filtered[k] = p
				total += p.TotalCost
			}
		}
		if len(filtered) > 0 {
			day.ByProject = filtered
			day.TotalCost = total
			result = append(result, day)
		}
	}
	return result
}

func logCmd() *cobra.Command {
	var list bool
	cmd := &cobra.Command{
		Use:     `log "message"`,
		Short:   "Record a journal entry for today (decisions, blockers, notes)",
		GroupID: "main",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			if list {
				return runLogList(cfg)
			}
			if len(args) == 0 {
				return errors.New(`usage: debrief log "your message"  (or: debrief log --list)`)
			}
			return runLogAppend(cfg, strings.Join(args, " "))
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "show today's journal entries")
	return cmd
}

func runLogAppend(_ config.Config, msg string) error {
	now := time.Now()
	if err := journal.Append(config.ConfigDir(), now, msg); err != nil {
		return fmt.Errorf("journal write failed: %w", err)
	}
	fmt.Printf("logged: [%s] %s\n", now.Format("15:04"), msg)
	return nil
}

func runLogList(_ config.Config) error {
	entries, err := journal.ReadEntries(config.ConfigDir(), time.Now())
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No journal entries for today.")
		return nil
	}
	for _, e := range entries {
		fmt.Printf("[%s] %s\n", e.Time, e.Message)
	}
	return nil
}

func collectActivities(cfg config.Config, dr model.DateRange, costMode bool) []model.Activity {
	collectors := []collector.Collector{
		collector.NewClaudeCollector(cfg.ClaudeDir, costMode, cfg.Pricing),
		collector.NewGitCollector(cfg.GitRepoPaths, cfg.GitDiscoveryDepth),
		collector.NewJournalCollector(config.ConfigDir()),
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
