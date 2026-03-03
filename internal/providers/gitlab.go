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

func NewGitLabProvider(cfg config.ProviderConfig) *GitLabProvider {
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

func (gp *GitLabProvider) Call(repo, author string, from time.Time) ([]Entry, error) {
	encoded := strings.ReplaceAll(repo, "/", "%2F")
	base := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", gp.cfg.BaseURL, encoded)

	params := url.Values{}
	params.Set("author_username", author)
	params.Set("created_after", from.UTC().Format(time.RFC3339))
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

	var mrs []gitlabEntry
	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	entries := make([]Entry, len(mrs))
	for i, mr := range mrs {
		mr.RepoStr = repo
		entries[i] = mr
	}
	return entries, nil
}
