package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/providers"
	"github.com/maxcelant/git-synced/internal/report"
	"github.com/spf13/cobra"
)

var ProviderRegistry = map[string]providers.ProviderFunc{
	"gitlab": providers.NewGitLabProvider,
	"github": providers.NewGitHubProvider,
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func NewReportCmd(configPath *string) *cobra.Command {
	var format, outputDir, since, until, state string
	var lookbackHours int
	var authors []string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a merge request report",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("format") {
				cfg.Format = format
			}
			if cmd.Flags().Changed("out") {
				cfg.OutputDir = outputDir
			}
			if cmd.Flags().Changed("lookback") {
				for i := range cfg.Providers {
					cfg.Providers[i].LookbackHours = lookbackHours
				}
			}
			if cmd.Flags().Changed("authors") {
				for i := range cfg.Providers {
					cfg.Providers[i].Authors = authors
				}
			}
			if cmd.Flags().Changed("state") {
				for i := range cfg.Providers {
					cfg.Providers[i].State = state
				}
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			var from time.Time
			if cmd.Flags().Changed("since") {
				from, err = parseDate(since)
				if err != nil {
					return fmt.Errorf("invalid --since date %q: %w", since, err)
				}
			}

			var untilTime time.Time
			if cmd.Flags().Changed("until") {
				untilTime, err = parseDate(until)
				if err != nil {
					return fmt.Errorf("invalid --until date %q: %w", until, err)
				}
			}

			return run(cfg, from, untilTime)
		},
	}

	cmd.Flags().StringVar(&format, "format", "", "output format: text | json | yaml (overrides config)")
	cmd.Flags().StringVar(&outputDir, "out", "", "output directory for report file (overrides config)")
	cmd.Flags().IntVar(&lookbackHours, "lookback", 0, "hours to look back for MRs (overrides config)")
	cmd.Flags().StringSliceVar(&authors, "authors", nil, "comma-separated list of authors to filter by (overrides config)")
	cmd.Flags().StringVar(&since, "since", "", "start date YYYY-MM-DD (overrides --lookback)")
	cmd.Flags().StringVar(&until, "until", "", "end date YYYY-MM-DD (default: now)")
	cmd.Flags().StringVar(&state, "state", "", "MR state: opened | closed | merged | all (overrides config)")

	return cmd
}

func run(cfg config.Config, from, until time.Time) error {
	var entries []providers.Entry
	var authors []string
	seenAuthors := make(map[string]bool)
	maxLookback := 0

	for i := range cfg.Providers {
		p := &cfg.Providers[i]

		providerFunc, ok := ProviderRegistry[p.Name]
		if !ok {
			return fmt.Errorf("unsupported provider %q", p.Name)
		}
		provider := providerFunc(*p)

		repos, err := provider.Expand(p.Repos)
		if err != nil {
			return err
		}
		p.Repos = repos

		createdAfter := from
		if createdAfter.IsZero() {
			createdAfter = time.Now().Add(-time.Duration(p.LookbackHours) * time.Hour)
		}
		if p.LookbackHours > maxLookback {
			maxLookback = p.LookbackHours
		}

		for _, repo := range p.Repos {
			for _, author := range p.Authors {
				mrs, err := provider.Call(repo, author, createdAfter, until)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", err)
					continue
				}
				entries = append(entries, mrs...)
			}
		}

		for _, a := range p.Authors {
			if !seenAuthors[a] {
				seenAuthors[a] = true
				authors = append(authors, a)
			}
		}
	}

	return report.New(authors, entries, maxLookback).Build(cfg)
}
