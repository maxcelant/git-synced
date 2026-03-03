package providers

import "time"

type Entry interface {
	Author() string
	Repo() string
	URL() string
	Title() string
	CreatedAt() string
}

type Provider interface {
	Expand(repos []string) ([]string, error)
	Call(string, string, time.Time) ([]Entry, error)
}
