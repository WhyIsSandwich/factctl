package resolve

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubFetcher(t *testing.T) {
	tests := []struct {
		name      string
		src       *Source
		respCode  int
		respBody  []byte
		wantErr   bool
		wantBytes []byte
	}{
		{
			name: "successful fetch",
			src: &Source{
				Type:    SourceGitHub,
				Owner:   "test",
				Repo:    "repo",
				Version: "main",
			},
			respCode:  http.StatusOK,
			respBody:  []byte("test zip content"),
			wantBytes: []byte("test zip content"),
		},
		{
			name: "not found",
			src: &Source{
				Type:    SourceGitHub,
				Owner:   "test",
				Repo:    "nonexistent",
				Version: "main",
			},
			respCode: http.StatusNotFound,
			wantErr:  true,
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
				// Verify User-Agent is set
				if ua := r.Header.Get("User-Agent"); ua != "factctl" {
					t.Errorf("Expected User-Agent header 'factctl', got %q", ua)
				}

				w.WriteHeader(tt.respCode)
				w.Write(tt.respBody)
			}))
			defer server.Close()

			// Override GitHub API URL in request
			originalURL := githubConfig.baseURL
			githubConfig.baseURL = server.URL

			f := NewGitHubFetcher()
			var buf bytes.Buffer
			hash, err := f.Fetch(context.Background(), tt.src, &buf)

			// Restore original URL
			tt.src.Version = originalURL

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