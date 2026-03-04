package providers

import (
	"time"

	"github.com/maxcelant/git-synced/internal/config"
)

type Entry interface {
	Author() string
	Repo() string
	URL() string
	Title() string
	CreatedAt() string
}

type Provider interface {
	Expand(repos []string) ([]string, error)
	Call(string, string, time.Time, time.Time) ([]Entry, error)
}

type ProviderFunc func(config.ProviderConfig) Provider
