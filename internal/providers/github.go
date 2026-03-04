package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/maxcelant/git-synced/internal/config"
)

type GitHubProvider struct {
	cfg config.ProviderConfig
}

func NewGitHubProvider(cfg config.ProviderConfig) Provider {
	return &GitHubProvider{cfg: cfg}
}

type githubEntry struct {
	TitleStr     string `json:"title"`
	HTMLURL      string `json:"html_url"`
	CreatedAtStr string `json:"created_at"`
	RepoStr      string `json:"-"`
	UserInfo     struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (g githubEntry) Title() string     { return g.TitleStr }
func (g githubEntry) Author() string    { return g.UserInfo.Login }
func (g githubEntry) Repo() string      { return g.RepoStr }
func (g githubEntry) URL() string       { return g.HTMLURL }
func (g githubEntry) CreatedAt() string { return g.CreatedAtStr }

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func (gp *GitHubProvider) nextLink(header string) string {
	m := linkNextRe.FindStringSubmatch(header)
	if m != nil {
		return m[1]
	}
	return ""
}

func (gp *GitHubProvider) baseURL() string {
	if gp.cfg.BaseURL != "" && gp.cfg.BaseURL != "https://github.com" {
		return strings.TrimRight(gp.cfg.BaseURL, "/")
	}
	return "https://api.github.com"
}

func (gp *GitHubProvider) do(reqURL string) ([]byte, *http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+gp.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp, nil
}

func (gp *GitHubProvider) fetchOrgRepos(org string) ([]string, error) {
	nextURL := fmt.Sprintf("%s/orgs/%s/repos?per_page=100", gp.baseURL(), org)
	var repos []string

	for nextURL != "" {
		body, resp, err := gp.do(nextURL)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d for org %s: %s", resp.StatusCode, org, body)
		}

		var page []struct {
			FullName string `json:"full_name"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decoding org repos response: %w", err)
		}
		for _, r := range page {
			repos = append(repos, r.FullName)
		}

		nextURL = gp.nextLink(resp.Header.Get("Link"))
	}

	return repos, nil
}

func (gp *GitHubProvider) Expand(repos []string) ([]string, error) {
	var expanded []string
	for _, r := range repos {
		if !strings.HasSuffix(r, "/*") {
			expanded = append(expanded, r)
			continue
		}
		org := strings.TrimSuffix(r, "/*")
		orgRepos, err := gp.fetchOrgRepos(org)
		if err != nil {
			return nil, fmt.Errorf("expanding wildcard %s: %w", r, err)
		}
		expanded = append(expanded, orgRepos...)
	}
	return expanded, nil
}

func githubStateQualifier(state string) string {
	switch state {
	case "opened":
		return "is:open"
	case "merged":
		return "is:merged"
	case "closed":
		return "is:closed"
	default: // "all" or anything else
		return ""
	}
}

func (gp *GitHubProvider) Call(repo, author string, from, until time.Time) ([]Entry, error) {
	var q string
	if !until.IsZero() {
		q = fmt.Sprintf("type:pr author:%s repo:%s created:%s..%s",
			author, repo, from.UTC().Format("2006-01-02"), until.UTC().Format("2006-01-02"))
	} else {
		q = fmt.Sprintf("type:pr author:%s repo:%s created:>%s",
			author, repo, from.UTC().Format(time.RFC3339))
	}
	if sq := githubStateQualifier(gp.cfg.State); sq != "" {
		q += " " + sq
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("per_page", "100")

	nextURL := fmt.Sprintf("%s/search/issues?%s", gp.baseURL(), params.Encode())
	var entries []Entry

	for nextURL != "" {
		body, resp, err := gp.do(nextURL)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d for repo=%s author=%s: %s", resp.StatusCode, repo, author, body)
		}

		var result struct {
			Items []githubEntry `json:"items"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		for _, item := range result.Items {
			item.RepoStr = repo
			entries = append(entries, item)
		}

		nextURL = gp.nextLink(resp.Header.Get("Link"))
	}

	return entries, nil
}
