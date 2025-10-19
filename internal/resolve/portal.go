package resolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var (
	// factorioModPortalAPI can be overridden for testing
	factorioModPortalAPI = "https://mods.factorio.com/api/mods"
)

// PortalFetcher implements Fetcher for the Factorio mod portal
type PortalFetcher struct {
	client *http.Client
}

// NewPortalFetcher creates a new PortalFetcher
func NewPortalFetcher() *PortalFetcher {
	return &PortalFetcher{
		client: &http.Client{},
	}
}

type modPortalResponse struct {
	Releases []struct {
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		SHA1        string `json:"sha1"`
	} `json:"releases"`
}

// Fetch downloads a mod from the Factorio mod portal
func (f *PortalFetcher) Fetch(ctx context.Context, src *Source, w io.Writer) (string, error) {
	if src.Type != SourcePortal {
		return "", fmt.Errorf("invalid source type for portal fetcher: %v", src.Type)
	}

	// Query mod info from the portal API
	apiURL := fmt.Sprintf("%s/%s", factorioModPortalAPI, url.PathEscape(src.ID))
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mod portal API returned status %d", resp.StatusCode)
	}

	var modInfo modPortalResponse
	if err := json.NewDecoder(resp.Body).Decode(&modInfo); err != nil {
		return "", err
	}

	// TODO: Implement version constraint matching
	if len(modInfo.Releases) == 0 {
		return "", fmt.Errorf("no releases found for mod %s", src.ID)
	}
	latest := modInfo.Releases[len(modInfo.Releases)-1]

	// Download the mod file
	baseURL := strings.TrimSuffix(factorioModPortalAPI, "/api/mods")
	downloadURL := baseURL + latest.DownloadURL
	req, err = http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err = f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mod download failed with status %d", resp.StatusCode)
	}

	// Calculate SHA256 while copying to writer
	h := sha256.New()
	mw := io.MultiWriter(w, h)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}