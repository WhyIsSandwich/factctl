package resolve

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// saveAPIs saves the current API URLs and returns a function to restore them
func saveAPIs() func() {
	origGitHubURL := githubConfig.baseURL
	origGitLabURL := gitlabConfig.baseURL
	return func() {
		githubConfig.baseURL = origGitHubURL
		gitlabConfig.baseURL = origGitLabURL
	}
}

func TestIntegrationWithRealRepos(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Ensure we use the real APIs for integration tests
	restore := saveAPIs()
	defer restore()
	githubConfig.baseURL = "https://api.github.com"
	gitlabConfig.baseURL = "https://gitlab.com"

	tests := []struct {
		name    string
		src     *Source
		wantErr bool
	}{
		{
			name: "Single mod repository (ScienceCostTweakerM)",
			src: &Source{
				Type:    SourceGitHub,
				Owner:   "mexmer",
				Repo:    "ScienceCostTweakerM",
				Version: "master", // Main branch is called master in this repo
			},
		},
		{
			name: "SeaBlock mod from multi-mod repo",
			src: &Source{
				Type:    SourceGitHub,
				Owner:   "modded-factorio",
				Repo:    "SeaBlock",
				Version: "main",
				SubPath: "SeaBlock",
			},
		},
		{
			name: "SeaBlock PR #343 with correct subpath",
			src: &Source{
				Type:    SourceGitHubPR,
				Owner:   "modded-factorio",
				Repo:    "SeaBlock",
				PR:      343,
				SubPath: "SeaBlock",
			},
		},
		{
			name: "Angels Refining from multi-mod repo",
			src: &Source{
				Type:    SourceGitHub,
				Owner:   "Arch666Angel",
				Repo:    "mods",
				Version: "master", // Angel's mods uses master branch
				SubPath: "angelsrefining",
			},
		},
		{
			name: "SeaBlock without SubPath should fail",
			src: &Source{
				Type:    SourceGitHub,
				Owner:   "modded-factorio",
				Repo:    "SeaBlock",
				Version: "main",
				// Missing SubPath for multi-mod repo should error
			},
			wantErr: true,
		},
		{
			name: "Angels with invalid SubPath",
			src: &Source{
				Type:    SourceGit,
				ID:      "github.com/Arch666Angel/mods",
				Version: "main",
				SubPath: "nonexistent-mod",
			},
			wantErr: true,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fetcher Fetcher
			switch tt.src.Type {
			case SourceGitHub:
				fetcher = NewGitHubFetcher()
			case SourceGitHubPR:
				fetcher = NewGitHubPRFetcher()
			case SourceGit:
				fetcher = NewGitFetcher()
			default:
				t.Fatalf("Unsupported source type: %v", tt.src.Type)
			}

			var buf bytes.Buffer
			hash, err := fetcher.Fetch(ctx, tt.src, &buf)

			if tt.wantErr {
				if err == nil {
					t.Error("Fetch() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Fetch() error = %v", err)
				return
			}

			if len(hash) == 0 {
				t.Error("Fetch() returned empty hash")
			}

			if buf.Len() == 0 {
				t.Error("Fetch() returned no data")
			}

			// Validate the downloaded content
			zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Errorf("Failed to read zip: %v", err)
				return
			}

			// Every Factorio mod must have an info.json
			var foundInfo bool
			modPath := ""
			if tt.src.SubPath != "" {
				modPath = tt.src.SubPath + "/"
			}

			for _, f := range zipReader.File {
				// Get the path after the first directory (which is usually repo-branch/)
				parts := strings.Split(f.Name, "/")
				if len(parts) < 2 {
					continue
				}
				relPath := strings.Join(parts[1:], "/")

				if relPath == modPath+"info.json" {
					foundInfo = true
					
					// Validate info.json content
					rc, err := f.Open()
					if err != nil {
						t.Errorf("Failed to open info.json: %v", err)
						continue
					}
					defer rc.Close()

					var info struct {
						Name    string `json:"name"`
						Version string `json:"version"`
					}
					if err := json.NewDecoder(rc).Decode(&info); err != nil {
						t.Errorf("Failed to parse info.json: %v", err)
						continue
					}

					if info.Name == "" || info.Version == "" {
						t.Error("info.json missing required fields name and/or version")
					}
				}
			}

			if !foundInfo {
				t.Error("No info.json found in downloaded mod")
			}
		})
	}
}