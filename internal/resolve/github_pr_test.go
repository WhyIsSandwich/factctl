package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubPRFetcher(t *testing.T) {
	tests := []struct {
		name      string
		src       *Source
		prResp    *prResponse
		zipData   []byte
		wantErr   bool
		wantBytes []byte
	}{
		{
			name: "successful fetch",
			src: &Source{
				Type:  SourceGitHubPR,
				Owner: "test",
				Repo:  "repo",
				PR:    123,
			},
			prResp: &prResponse{
				Head: struct {
					SHA string "json:\"sha\""
				}{
					SHA: "abc123",
				},
			},
			zipData:   []byte("test pr zip content"),
			wantBytes: []byte("test pr zip content"),
		},
		{
			name: "wrong source type",
			src: &Source{
				Type: SourcePortal,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify User-Agent
				if ua := r.Header.Get("User-Agent"); ua != "factctl" {
					t.Errorf("Expected User-Agent header 'factctl', got %q", ua)
				}

				t.Logf("Test server received request: %s %s", r.Method, r.URL.Path)
				
				// First request should be for PR info
				if r.URL.Path == "/repos/test/repo/pulls/123" {
					json.NewEncoder(w).Encode(tt.prResp)
					return
				}
				
				// Second request should be for the zip file
				if r.URL.Path == "/repos/test/repo/zipball/abc123" {
					w.Write(tt.zipData)
					return
				}

				t.Logf("Unexpected path: %s, expected /repos/test/repo/pulls/123 or /repos/test/repo/zipball/abc123", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			// Override GitHub API URL for testing
			origURL := githubConfig.baseURL
			githubConfig.baseURL = server.URL
			defer func() { githubConfig.baseURL = origURL }()

			f := NewGitHubPRFetcher()
			var buf bytes.Buffer
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

			if !bytes.Equal(buf.Bytes(), tt.wantBytes) {
				t.Errorf("Fetch() got content = %v, want %v", buf.String(), string(tt.wantBytes))
			}
		})
	}
}