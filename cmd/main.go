package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/providers"
	"github.com/maxcelant/git-synced/internal/report"
)

var providerRegistry = map[string]providers.ProviderFunc{
	"gitlab": providers.NewGitLabProvider,
	"github": providers.NewGitHubProvider,
}

func run(cfg config.Config) error {
	var entries []providers.Entry
	var authors []string
	seenAuthors := make(map[string]bool)
	maxLookback := 0

	for i := range cfg.Providers {
		p := &cfg.Providers[i]

		providerFunc, ok := providerRegistry[p.Name]
		if !ok {
			return fmt.Errorf("unsupported provider %q", p.Name)
		}
		provider := providerFunc(*p)

		repos, err := provider.Expand(p.Repos)
		if err != nil {
			return err
		}
		p.Repos = repos

		createdAfter := time.Now().Add(-time.Duration(p.LookbackHours) * time.Hour)
		if p.LookbackHours > maxLookback {
			maxLookback = p.LookbackHours
		}

		for _, repo := range p.Repos {
			for _, author := range p.Authors {
				mrs, err := provider.Call(repo, author, createdAfter)
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

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return home + "/.git-synced/config.yaml"
}

func main() {
	var configPath, format, outputDir string
	var lookbackHours int

	rootCmd := &cobra.Command{
		Use:   "git-synced",
		Short: "GitLab PR daily watcher",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
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
			if err := cfg.Validate(); err != nil {
				return err
			}
			return run(cfg)
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", defaultConfigPath(), "path to config file")
	rootCmd.Flags().StringVar(&format, "format", "", "output format: text | json | yaml (overrides config)")
	rootCmd.Flags().StringVar(&outputDir, "out", "", "output directory for report file (overrides config)")
	rootCmd.Flags().IntVar(&lookbackHours, "lookback", 0, "hours to look back for MRs (overrides config)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
