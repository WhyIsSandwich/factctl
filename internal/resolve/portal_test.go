package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPortalFetcher(t *testing.T) {
	tests := []struct {
		name      string
		src       *Source
		apiResp   interface{}
		modData   string
		wantErr   bool
		wantBytes []byte
	}{
		{
			name: "successful fetch",
			src: &Source{
				Type:    SourcePortal,
				ID:      "test-mod",
				Version: "1.0.0",
			},
			apiResp: modPortalResponse{
				Releases: []struct {
					Version     string `json:"version"`
					DownloadURL string `json:"download_url"`
					SHA1        string `json:"sha1"`
				}{
					{
						Version:     "1.0.0",
						DownloadURL: "/download/test-mod/1.0.0",
						SHA1:        "test-sha1",
					},
				},
			},
			modData:   "test mod content",
			wantBytes: []byte("test mod content"),
		},
		{
			name: "no releases",
			src: &Source{
				Type:    SourcePortal,
				ID:      "empty-mod",
				Version: "1.0.0",
			},
			apiResp: modPortalResponse{
				Releases: []struct {
					Version     string `json:"version"`
					DownloadURL string `json:"download_url"`
					SHA1        string `json:"sha1"`
				}{},
			},
			wantErr: true,
		},
		{
			name: "wrong source type",
			src: &Source{
				Type: SourceGitHub,
				ID:   "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that handles both API and download requests
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/mods/"):
					json.NewEncoder(w).Encode(tt.apiResp)
				case strings.HasPrefix(r.URL.Path, "/download/"):
					w.Write([]byte(tt.modData))
				default:
					t.Logf("Unexpected request to %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			// Override the portal API URL for testing
			originalAPI := factorioModPortalAPI
			factorioModPortalAPI = server.URL + "/api/mods"
			defer func() { factorioModPortalAPI = originalAPI }()

			f := NewPortalFetcher()
			var buf strings.Builder
			hash, err := f.Fetch(context.Background(), tt.src, &buf)

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

			if !bytes.Equal([]byte(buf.String()), tt.wantBytes) {
				t.Errorf("Fetch() got content = %v, want %v", buf.String(), string(tt.wantBytes))
			}
		})
	}
}