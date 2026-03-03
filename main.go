package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the user-provided configuration.
type Config struct {
	GitLabURL     string   `yaml:"gitlab_url"`
	Token         string   `yaml:"token"`
	LookbackHours int      `yaml:"lookback_hours"`
	Authors       []string `yaml:"authors"`
	Repos         []string `yaml:"repos"`
	State         string   `yaml:"state"`
	OutputFormat  string   `yaml:"output_format"`
}

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

// MR represents a GitLab merge request (subset of fields we care about).
type MR struct {
	ID        int    `json:"iid"`
	Title     string `json:"title"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
	Author    struct {
		Username string `json:"username"`
	} `json:"author"`
}

// mrEntry is a flattened record used for display.
type mrEntry struct {
	Author    string
	Repo      string
	Title     string
	URL       string
	CreatedAt time.Time
}

func loadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("opening config: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	// Defaults
	if cfg.GitLabURL == "" {
		cfg.GitLabURL = "https://gitlab.com"
	}
	if cfg.LookbackHours <= 0 {
		cfg.LookbackHours = 24
	}
	if cfg.State == "" {
		cfg.State = "opened"
	}
	if cfg.OutputFormat == "" {
		cfg.OutputFormat = "yaml"
	}

	return cfg, nil
}

func fetchMRs(cfg Config, repo, author string, createdAfter time.Time) ([]MR, error) {
	encoded := strings.ReplaceAll(repo, "/", "%2F")
	base := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", cfg.GitLabURL, encoded)

	params := url.Values{}
	params.Set("author_username", author)
	params.Set("created_after", createdAfter.UTC().Format(time.RFC3339))
	params.Set("state", cfg.State)
	params.Set("per_page", "100")

	reqURL := base + "?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API returned %d for repo=%s author=%s: %s", resp.StatusCode, repo, author, body)
	}

	var mrs []MR
	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return mrs, nil
}

// fetchGroupProjects returns all project path_with_namespace values under a
// GitLab group, including subgroups, by paginating the groups API.
func fetchGroupProjects(cfg Config, group string) ([]string, error) {
	encoded := strings.ReplaceAll(group, "/", "%2F")
	baseURL := fmt.Sprintf("%s/api/v4/groups/%s/projects?include_subgroups=true&per_page=100", cfg.GitLabURL, encoded)

	var projects []string
	nextURL := baseURL

	for nextURL != "" {
		req, err := http.NewRequest(http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("building request for group %s: %w", group, err)
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
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
		for _, p := range page {
			projects = append(projects, p.PathWithNamespace)
		}

		nextURL = resp.Header.Get("X-Next-Page")
		if nextURL != "" {
			nextURL = fmt.Sprintf("%s/api/v4/groups/%s/projects?include_subgroups=true&per_page=100&page=%s", cfg.GitLabURL, encoded, nextURL)
		}
	}

	return projects, nil
}

// expandRepos resolves wildcard entries (ending in "/*") into concrete repo
// paths by querying the GitLab groups API. Non-wildcard entries pass through.
func expandRepos(cfg Config, repos []string) ([]string, error) {
	var expanded []string
	for _, r := range repos {
		if !strings.HasSuffix(r, "/*") {
			expanded = append(expanded, r)
			continue
		}
		group := strings.TrimSuffix(r, "/*")
		projects, err := fetchGroupProjects(cfg, group)
		if err != nil {
			return nil, fmt.Errorf("expanding wildcard %s: %w", r, err)
		}
		expanded = append(expanded, projects...)
	}
	return expanded, nil
}

// buildReport groups mrEntry records by author (in config order) and returns a
// structured reportOutput suitable for json/yaml serialization.
func buildReport(entries []mrEntry, cfg Config) reportOutput {
	byAuthor := make(map[string][]mrEntry)
	for _, e := range entries {
		byAuthor[e.Author] = append(byAuthor[e.Author], e)
	}
	for a := range byAuthor {
		sort.Slice(byAuthor[a], func(i, j int) bool {
			return byAuthor[a][i].CreatedAt.After(byAuthor[a][j].CreatedAt)
		})
	}

	var authors []authorOutput
	for _, username := range cfg.Authors {
		mrs := byAuthor[username]
		ao := authorOutput{Username: username, MRCount: len(mrs)}
		for _, e := range mrs {
			ao.MRs = append(ao.MRs, mrOutput{
				Repo:      e.Repo,
				Title:     e.Title,
				URL:       e.URL,
				CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		authors = append(authors, ao)
	}

	return reportOutput{
		Date:          time.Now().Format("2006-01-02"),
		LookbackHours: cfg.LookbackHours,
		Authors:       authors,
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

func printReport(entries []mrEntry, cfg Config) {
	// Group entries by author, preserving config author order.
	byAuthor := make(map[string][]mrEntry)
	for _, e := range entries {
		byAuthor[e.Author] = append(byAuthor[e.Author], e)
	}

	// Sort each author's MRs by creation time (newest first).
	for a := range byAuthor {
		sort.Slice(byAuthor[a], func(i, j int) bool {
			return byAuthor[a][i].CreatedAt.After(byAuthor[a][j].CreatedAt)
		})
	}

	fmt.Printf("# MR Report — %s\n\n", time.Now().Format("2006-01-02"))

	for _, author := range cfg.Authors {
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
			fmt.Printf("| %s | %s | %s | %s |\n",
				shortRepoName(e.Repo), e.Title, timeAgo(e.CreatedAt), e.URL)
		}
		fmt.Println()
	}

	// Count unique repos that had at least one MR.
	repoSet := make(map[string]struct{})
	for _, e := range entries {
		repoSet[e.Repo] = struct{}{}
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

func run(configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	if cfg.Token == "" {
		return fmt.Errorf("token is required in config")
	}
	if len(cfg.Authors) == 0 {
		return fmt.Errorf("at least one author is required in config")
	}
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("at least one repo is required in config")
	}

	repos, err := expandRepos(cfg, cfg.Repos)
	if err != nil {
		return err
	}
	cfg.Repos = repos

	createdAfter := time.Now().Add(-time.Duration(cfg.LookbackHours) * time.Hour)

	var entries []mrEntry

	for _, repo := range cfg.Repos {
		for _, author := range cfg.Authors {
			mrs, err := fetchMRs(cfg, repo, author, createdAfter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}
			for _, mr := range mrs {
				t, _ := time.Parse(time.RFC3339, mr.CreatedAt)
				entries = append(entries, mrEntry{
					Author:    author,
					Repo:      repo,
					Title:     mr.Title,
					URL:       mr.WebURL,
					CreatedAt: t,
				})
			}
		}
	}

	switch cfg.OutputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(buildReport(entries, cfg))
	case "text": // markdown output
		printReport(entries, cfg)
	default: // "yaml"
		return yaml.NewEncoder(os.Stdout).Encode(buildReport(entries, cfg))
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

	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
