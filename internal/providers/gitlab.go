package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maxcelant/git-synced/internal/config"
)

type GitLabProvider struct {
	cfg config.ProviderConfig
}

func NewGitLabProvider(cfg config.ProviderConfig) Provider {
	return &GitLabProvider{cfg: cfg}
}

type gitlabEntry struct {
	IID          int    `json:"iid"`
	TitleStr     string `json:"title"`
	WebURL       string `json:"web_url"`
	CreatedAtStr string `json:"created_at"`
	RepoStr      string `json:"-"`
	AuthorInfo   struct {
		Username string `json:"username"`
	} `json:"author"`
}

func (g gitlabEntry) Title() string     { return g.TitleStr }
func (g gitlabEntry) Author() string    { return g.AuthorInfo.Username }
func (g gitlabEntry) Repo() string      { return g.RepoStr }
func (g gitlabEntry) URL() string       { return g.WebURL }
func (g gitlabEntry) CreatedAt() string { return g.CreatedAtStr }

func (gp *GitLabProvider) fetchGroupProjects(group string) ([]string, error) {
	encoded := strings.ReplaceAll(group, "/", "%2F")
	baseURL := fmt.Sprintf("%s/api/v4/groups/%s/projects?include_subgroups=true&per_page=100", gp.cfg.BaseURL, encoded)

	var projects []string
	nextURL := baseURL

	for nextURL != "" {
		req, err := http.NewRequest(http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("building request for group %s: %w", group, err)
		}
		req.Header.Set("Authorization", "Bearer "+gp.cfg.Token)
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
			nextURL = fmt.Sprintf("%s/api/v4/groups/%s/projects?include_subgroups=true&per_page=100&page=%s", gp.cfg.BaseURL, encoded, nextURL)
		}
	}

	return projects, nil
}

func (gp *GitLabProvider) Expand(repos []string) ([]string, error) {
	var expanded []string
	for _, r := range repos {
		if !strings.HasSuffix(r, "/*") {
			expanded = append(expanded, r)
			continue
		}
		group := strings.TrimSuffix(r, "/*")
		projects, err := gp.fetchGroupProjects(group)
		if err != nil {
			return nil, fmt.Errorf("expanding wildcard %s: %w", r, err)
		}
		expanded = append(expanded, projects...)
	}
	return expanded, nil
}

func (gp *GitLabProvider) Call(repo, author string, from, until time.Time) ([]Entry, error) {
	encoded := strings.ReplaceAll(repo, "/", "%2F")
	base := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", gp.cfg.BaseURL, encoded)

	params := url.Values{}
	params.Set("author_username", author)
	params.Set("created_after", from.UTC().Format(time.RFC3339))
	if !until.IsZero() {
		params.Set("created_before", until.UTC().Format(time.RFC3339))
	}
	params.Set("state", gp.cfg.State)
	params.Set("per_page", "100")

	reqURL := base + "?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+gp.cfg.Token)
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

	var prs []gitlabEntry
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	entries := make([]Entry, len(prs))
	for i, pr := range prs {
		pr.RepoStr = repo
		entries[i] = pr
	}
	return entries, nil
}
