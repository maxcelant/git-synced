package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/providers"
)

// reportOutput is the top-level structured report for json/yaml output.
type reportOutput struct {
	Date          string         `json:"date"           yaml:"date"`
	LookbackHours int            `json:"lookback_hours" yaml:"lookback_hours"`
	Authors       []authorOutput `json:"authors"        yaml:"authors"`
	TotalMRs      int            `json:"total_mrs"      yaml:"total_mrs"`
}

type authorOutput struct {
	Username string     `json:"username" yaml:"username"`
	MRCount  int        `json:"mr_count" yaml:"mr_count"`
	MRs      []mrOutput `json:"mrs"      yaml:"mrs"`
}

type mrOutput struct {
	Repo      string `json:"repo"       yaml:"repo"`
	Title     string `json:"title"      yaml:"title"`
	URL       string `json:"url"        yaml:"url"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
}


// fetchGroupProjects returns all project path_with_namespace values under a
// GitLab group, including subgroups, by paginating the groups API.
func fetchGroupProjects(p config.ProviderConfig, group string) ([]string, error) {
	encoded := strings.ReplaceAll(group, "/", "%2F")
	baseURL := fmt.Sprintf("%s/api/v4/groups/%s/projects?include_subgroups=true&per_page=100", p.BaseURL, encoded)

	var projects []string
	nextURL := baseURL

	for nextURL != "" {
		req, err := http.NewRequest(http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("building request for group %s: %w", group, err)
		}
		req.Header.Set("Authorization", "Bearer "+p.Token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GET %s: %w", nextURL, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitLab API returned %d for group %s: %s", resp.StatusCode, group, body)
		}

		var page []struct {
			PathWithNamespace string `json:"path_with_namespace"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decoding group projects response: %w", err)
		}
		for _, proj := range page {
			projects = append(projects, proj.PathWithNamespace)
		}

		nextURL = resp.Header.Get("X-Next-Page")
		if nextURL != "" {
			nextURL = fmt.Sprintf("%s/api/v4/groups/%s/projects?include_subgroups=true&per_page=100&page=%s", p.BaseURL, encoded, nextURL)
		}
	}

	return projects, nil
}

// expandRepos resolves wildcard entries (ending in "/*") into concrete repo
// paths by querying the GitLab groups API. Non-wildcard entries pass through.
func expandRepos(p config.ProviderConfig, repos []string) ([]string, error) {
	var expanded []string
	for _, r := range repos {
		if !strings.HasSuffix(r, "/*") {
			expanded = append(expanded, r)
			continue
		}
		group := strings.TrimSuffix(r, "/*")
		projects, err := fetchGroupProjects(p, group)
		if err != nil {
			return nil, fmt.Errorf("expanding wildcard %s: %w", r, err)
		}
		expanded = append(expanded, projects...)
	}
	return expanded, nil
}

// buildReport groups Entry records by author (in config order) and returns a
// structured reportOutput suitable for json/yaml serialization.
func buildReport(entries []providers.Entry, authors []string, lookbackHours int) reportOutput {
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
		mrs := byAuthor[username]
		ao := authorOutput{Username: username, MRCount: len(mrs)}
		for _, e := range mrs {
			ao.MRs = append(ao.MRs, mrOutput{
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
		TotalMRs:      len(entries),
	}
}

// shortRepoName returns the final path component of a "group/repo" string.
func shortRepoName(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

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

func printReport(entries []providers.Entry, authors []string) {
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

	fmt.Printf("# MR Report — %s\n\n", time.Now().Format("2006-01-02"))

	for _, author := range authors {
		mrs := byAuthor[author]
		count := len(mrs)
		if count == 0 {
			fmt.Printf("## %s\n\n_No new MRs._\n\n", author)
			continue
		}
		mrWord := "MRs"
		if count == 1 {
			mrWord = "MR"
		}
		fmt.Printf("## %s (%d new %s)\n\n", author, count, mrWord)
		fmt.Println("| Repo | Title | Created | URL |")
		fmt.Println("|------|-------|---------|-----|")
		for _, e := range mrs {
			t, _ := time.Parse(time.RFC3339, e.CreatedAt())
			fmt.Printf("| %s | %s | %s | %s |\n",
				shortRepoName(e.Repo()), e.Title(), timeAgo(t), e.URL())
		}
		fmt.Println()
	}

	repoSet := make(map[string]struct{})
	for _, e := range entries {
		repoSet[e.Repo()] = struct{}{}
	}

	mrWord := "MRs"
	if len(entries) == 1 {
		mrWord = "MR"
	}
	summary := fmt.Sprintf("%d %s", len(entries), mrWord)
	if len(repoSet) > 0 {
		repoWord := "repos"
		if len(repoSet) == 1 {
			repoWord = "repo"
		}
		summary += fmt.Sprintf(" across %d %s", len(repoSet), repoWord)
	}
	fmt.Printf("---\n\n**Total: %s**\n", summary)
}


func run(cfg config.Config) error {
	var entries []providers.Entry
	var allAuthors []string
	seenAuthors := make(map[string]bool)
	maxLookback := 0

	for i := range cfg.Providers {
		p := &cfg.Providers[i]

		repos, err := expandRepos(*p, p.Repos)
		if err != nil {
			return err
		}
		p.Repos = repos

		createdAfter := time.Now().Add(-time.Duration(p.LookbackHours) * time.Hour)
		if p.LookbackHours > maxLookback {
			maxLookback = p.LookbackHours
		}

		provider := providers.NewGitLabProvider(*p)

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
				allAuthors = append(allAuthors, a)
			}
		}
	}

	switch cfg.Format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(buildReport(entries, allAuthors, maxLookback))
	case "text":
		printReport(entries, allAuthors)
	default: // "yaml"
		return yaml.NewEncoder(os.Stdout).Encode(buildReport(entries, allAuthors, maxLookback))
	}
	return nil
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `git-synced — GitLab MR daily watcher

Usage:
  git-synced [--config <path>]

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Cron example (runs at 9am daily):
  0 9 * * * cd /path/to/git-synced && ./git-synced >> /tmp/mr-report.log 2>&1
`)
	}
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
