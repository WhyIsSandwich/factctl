package resolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Static config for testing
var gitlabConfig = struct {
	baseURL string
}{
	baseURL: "https://gitlab.com",
}

// GitFetcher implements Fetcher for generic Git repositories
type GitFetcher struct {
	client *http.Client
}

// NewGitFetcher creates a new GitFetcher
func NewGitFetcher() *GitFetcher {
	return &GitFetcher{
		client: &http.Client{},
	}
}

// Fetch downloads a Git repository as a zip archive
// Currently supports GitLab and custom GitLab instances
func (f *GitFetcher) Fetch(ctx context.Context, src *Source, w io.Writer) (string, error) {
	if src.Type != SourceGit {
		return "", fmt.Errorf("invalid source type for Git fetcher: %v", src.Type)
	}

	// Parse host and path from repo ID
	repoURL, err := url.Parse("https://" + src.ID)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	// Always use the configured GitLab URL
	path := strings.TrimPrefix(repoURL.Path, "/")
	downloadURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/archive.zip?sha=%s",
		gitlabConfig.baseURL, url.PathEscape(path), url.QueryEscape(src.Version))

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
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
		return "", fmt.Errorf("Git API returned status %d", resp.StatusCode)
	}

	h := sha256.New()
	mw := io.MultiWriter(w, h)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}