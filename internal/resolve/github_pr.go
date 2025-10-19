package resolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GitHubPRFetcher implements Fetcher for GitHub pull requests
type GitHubPRFetcher struct {
	client *http.Client
}

// NewGitHubPRFetcher creates a new GitHubPRFetcher
func NewGitHubPRFetcher() *GitHubPRFetcher {
	return &GitHubPRFetcher{
		client: &http.Client{},
	}
}

type prResponse struct {
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

// Fetch downloads a mod from a GitHub PR as a zip archive
func (f *GitHubPRFetcher) Fetch(ctx context.Context, src *Source, w io.Writer) (string, error) {
	if src.Type != SourceGitHubPR {
		return "", fmt.Errorf("invalid source type for GitHub PR fetcher: %v", src.Type)
	}

	// Verify SubPath is provided for multi-mod repos
	if isMultiModRepo(src.Owner, src.Repo) && src.SubPath == "" {
		return "", fmt.Errorf("SubPath is required for multi-mod repository %s/%s", src.Owner, src.Repo)
	}

	// First get the PR to find its head SHA
	prURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d",
		githubConfig.baseURL, // imported from github.go
		src.Owner, src.Repo, src.PR)

	req, err := http.NewRequestWithContext(ctx, "GET", prURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "factctl")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d for PR info", resp.StatusCode)
	}

	var pr prResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("parsing PR response: %w", err)
	}

	// Now download the zip using the head SHA
	downloadURL := fmt.Sprintf("%s/repos/%s/%s/zipball/%s",
		githubConfig.baseURL, src.Owner, src.Repo, pr.Head.SHA)

	req, err = http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "factctl")

	resp, err = f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d for zip download", resp.StatusCode)
	}

	// Calculate SHA256 while copying to writer
	h := sha256.New()
	mw := io.MultiWriter(w, h)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}