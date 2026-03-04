package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/providers"
	"gopkg.in/yaml.v3"
)

type reportOutput struct {
	Date          string         `json:"date"           yaml:"date"`
	LookbackHours int            `json:"lookback_hours" yaml:"lookback_hours"`
	Authors       []authorOutput `json:"authors"        yaml:"authors"`
	TotalPRs      int            `json:"total_prs"      yaml:"total_prs"`
}

type authorOutput struct {
	Username string     `json:"username" yaml:"username"`
	PRCount  int        `json:"pr_count" yaml:"pr_count"`
	PRs      []prOutput `json:"prs"      yaml:"prs"`
}

type prOutput struct {
	Repo      string `json:"repo"       yaml:"repo"`
	Title     string `json:"title"      yaml:"title"`
	URL       string `json:"url"        yaml:"url"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
}

type Report struct {
	authors     []string
	entries     []providers.Entry
	maxLookBack int
}

func New(authors []string, entries []providers.Entry, maxLookBack int) Report {
	return Report{authors, entries, maxLookBack}
}

func (r Report) Build(cfg config.Config) error {
	out, err := outputWriter(cfg)
	if err != nil {
		return err
	}
	if f, ok := out.(*os.File); ok && f != os.Stdout {
		defer f.Close()
	}
	switch cfg.Format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(r.build())
	case "text":
		r.print(out)
	default: // "yaml"
		return yaml.NewEncoder(out).Encode(r.build())
	}
	return nil
}

func (r Report) build() reportOutput {
	byAuthor := make(map[string][]providers.Entry)
	for _, e := range entries {
		byAuthor[e.Author()] = append(byAuthor[e.Author()], e)
	}
	for a := range byAuthor {
		sort.Slice(byAuthor[a], func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, byAuthor[a][i].CreatedAt())
			tj, _ := time.Parse(time.RFC3339, byAuthor[a][j].CreatedAt())
			return ti.After(tj)
		})
	}

	var authorOutputs []authorOutput
	for _, username := range authors {
		prs := byAuthor[username]
		ao := authorOutput{Username: username, PRCount: len(prs)}
		for _, e := range prs {
			ao.PRs = append(ao.PRs, prOutput{
				Repo:      e.Repo(),
				Title:     e.Title(),
				URL:       e.URL(),
				CreatedAt: e.CreatedAt(),
			})
		}
		authorOutputs = append(authorOutputs, ao)
	}

	return reportOutput{
		Date:          time.Now().Format("2006-01-02"),
		LookbackHours: lookbackHours,
		Authors:       authorOutputs,
		TotalPRs:      len(entries),
	}
}

// shortRepoName returns the final path component of a "group/repo" string.
func shortRepoName(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

func (r *Report) print(w io.Writer) {
	byAuthor := make(map[string][]providers.Entry)
	for _, e := range r.entries {
		byAuthor[e.Author()] = append(byAuthor[e.Author()], e)
	}

	for a := range byAuthor {
		sort.Slice(byAuthor[a], func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, byAuthor[a][i].CreatedAt())
			tj, _ := time.Parse(time.RFC3339, byAuthor[a][j].CreatedAt())
			return ti.After(tj)
		})
	}

	fmt.Fprintf(w, "# PR Report — %s\n\n", time.Now().Format("2006-01-02"))

	for _, author := range r.authors {
		prs := byAuthor[author]
		count := len(prs)
		if count == 0 {
			fmt.Fprintf(w, "## %s\n\n_No new PRs._\n\n", author)
			continue
		}
		prWord := "PRs"
		if count == 1 {
			prWord = "PR"
		}
		fmt.Fprintf(w, "## %s (%d new %s)\n\n", author, count, prWord)
		fmt.Fprintln(w, "| Repo | Title | Created | URL |")
		fmt.Fprintln(w, "|------|-------|---------|-----|")
		for _, e := range prs {
			t, _ := time.Parse(time.RFC3339, e.CreatedAt())
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n",
				shortRepoName(e.Repo()), e.Title(), timeAgo(t), e.URL())
		}
		fmt.Fprintln(w)
	}

	repoSet := make(map[string]struct{})
	for _, e := range r.entries {
		repoSet[e.Repo()] = struct{}{}
	}

	prWord := "PRs"
	if len(r.entries) == 1 {
		prWord = "PR"
	}
	summary := fmt.Sprintf("%d %s", len(r.entries), prWord)
	if len(repoSet) > 0 {
		repoWord := "repos"
		if len(repoSet) == 1 {
			repoWord = "repo"
		}
		summary += fmt.Sprintf(" across %d %s", len(repoSet), repoWord)
	}
	fmt.Fprintf(w, "---\n\n**Total: %s**\n", summary)
}

func outputWriter(cfg config.Config) (io.Writer, error) {
	if cfg.OutputDir == "" {
		return os.Stdout, nil
	}
	ext := map[string]string{"json": "json", "text": "md"}
	e, ok := ext[cfg.Format]
	if !ok {
		e = "yaml"
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}
	name := fmt.Sprintf("pr-report-%s.%s", time.Now().Format("2006-01-02"), e)
	f, err := os.Create(cfg.OutputDir + "/" + name)
	if err != nil {
		return nil, fmt.Errorf("creating output file: %w", err)
	}
	return f, nil
}
