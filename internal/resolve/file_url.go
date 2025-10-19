package resolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
)

// FileFetcher implements Fetcher for local files
type FileFetcher struct{}

// NewFileFetcher creates a new FileFetcher
func NewFileFetcher() *FileFetcher {
	return &FileFetcher{}
}

// Fetch copies a local file to the destination
func (f *FileFetcher) Fetch(ctx context.Context, src *Source, w io.Writer) (string, error) {
	if src.Type != SourceFile {
		return "", fmt.Errorf("invalid source type for file fetcher: %v", src.Type)
	}

	file, err := os.Open(src.Path)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	h := sha256.New()
	mw := io.MultiWriter(w, h)

	if _, err := io.Copy(mw, file); err != nil {
		return "", fmt.Errorf("copying file: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// URLFetcher implements Fetcher for direct URL downloads
type URLFetcher struct {
	client *http.Client
}

// NewURLFetcher creates a new URLFetcher
func NewURLFetcher() *URLFetcher {
	return &URLFetcher{
		client: &http.Client{},
	}
}

// Fetch downloads a file from a URL
func (f *URLFetcher) Fetch(ctx context.Context, src *Source, w io.Writer) (string, error) {
	if src.Type != SourceURL {
		return "", fmt.Errorf("invalid source type for URL fetcher: %v", src.Type)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", src.URL, nil)
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
		return "", fmt.Errorf("URL fetch returned status %d", resp.StatusCode)
	}

	h := sha256.New()
	mw := io.MultiWriter(w, h)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}