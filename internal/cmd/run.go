package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/providers"
	"github.com/maxcelant/git-synced/internal/report"
)

var ProviderRegistry = map[string]providers.ProviderFunc{
	"gitlab": providers.NewGitLabProvider,
	"github": providers.NewGitHubProvider,
}

func Run(cfg config.Config) error {
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
