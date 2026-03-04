package main

import (
	"os"

	"github.com/spf13/cobra"

	icmd "github.com/maxcelant/git-synced/internal/cmd"
	"github.com/maxcelant/git-synced/internal/config"
)

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
	var authors []string

	rootCmd := &cobra.Command{
		Use:   "gsync",
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
			if cmd.Flags().Changed("authors") {
				for i := range cfg.Providers {
					cfg.Providers[i].Authors = authors
				}
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			return icmd.Run(cfg)
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", defaultConfigPath(), "path to config file")
	rootCmd.Flags().StringVar(&format, "format", "", "output format: text | json | yaml (overrides config)")
	rootCmd.Flags().StringVar(&outputDir, "out", "", "output directory for report file (overrides config)")
	rootCmd.Flags().IntVar(&lookbackHours, "lookback", 0, "hours to look back for MRs (overrides config)")
	rootCmd.Flags().StringSliceVar(&authors, "authors", nil, "comma-separated list of authors to filter by (overrides config)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
