package resolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

// Static config for testing
var githubConfig = struct {
	baseURL string
}{
	baseURL: "https://api.github.com",
}

// GitHubFetcher implements Fetcher for GitHub repositories
type GitHubFetcher struct {
	client *http.Client
}

// NewGitHubFetcher creates a new GitHubFetcher
func NewGitHubFetcher() *GitHubFetcher {
	return &GitHubFetcher{
		client: &http.Client{},
	}
}

// Fetch downloads a mod from GitHub as a zip archive
func (f *GitHubFetcher) Fetch(ctx context.Context, src *Source, w io.Writer) (string, error) {
	if src.Type != SourceGitHub {
		return "", fmt.Errorf("invalid source type for GitHub fetcher: %v", src.Type)
	}

	// Verify SubPath is provided for multi-mod repos
	if isMultiModRepo(src.Owner, src.Repo) && src.SubPath == "" {
		return "", fmt.Errorf("SubPath is required for multi-mod repository %s/%s", src.Owner, src.Repo)
	}

	// GitHub API provides zip downloads at /repos/{owner}/{repo}/zipball/{ref}
	downloadURL := fmt.Sprintf("%s/repos/%s/%s/zipball/%s",
		githubConfig.baseURL,
		src.Owner, src.Repo, src.Version)

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}

	// GitHub requires a User-Agent
	req.Header.Set("User-Agent", "factctl")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Calculate SHA256 while copying to writer
	h := sha256.New()
	mw := io.MultiWriter(w, h)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}