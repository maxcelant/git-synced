package main

import (
	"os"

	"github.com/spf13/cobra"

	icmd "github.com/maxcelant/git-synced/internal/cmd"
)

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return home + "/.gsync/config.yaml"
}

func main() {
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "gsync",
		Short: "GitLab PR daily watcher",
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath(), "path to config file")

	rootCmd.AddCommand(icmd.NewReportCmd(&configPath))
	rootCmd.AddCommand(icmd.NewConfigCmd(&configPath))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
