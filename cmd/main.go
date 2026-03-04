package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/providers"
	"github.com/maxcelant/git-synced/internal/report"
)

// timeAgo formats a duration as a human-readable "Xh ago" / "Xm ago" string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func run(cfg config.Config) error {
	var entries []providers.Entry
	var authors []string
	seenAuthors := make(map[string]bool)
	maxLookback := 0

	for i := range cfg.Providers {
		p := &cfg.Providers[i]

		var provider providers.Provider
		switch p.Name {
		case "gitlab":
			provider = providers.NewGitLabProvider(*p)
		default:
			return fmt.Errorf("unsupported provider %q", p.Name)
		}

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
				entries = append(entries, prs...)
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
			if cmd.Flags().Changed("output-dir") {
				cfg.OutputDir = outputDir
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			return run(cfg)
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", defaultConfigPath(), "path to config file")
	rootCmd.Flags().StringVar(&format, "format", "", "output format: text | json | yaml (overrides config)")
	rootCmd.Flags().StringVar(&outputDir, "output-dir", "", "output directory for report file (overrides config)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
