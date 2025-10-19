package resolve

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitFetcher(t *testing.T) {
	tests := []struct {
		name      string
		src       *Source
		respCode  int
		respBody  []byte
		wantErr   bool
		wantBytes []byte
	}{
		{
			name: "successful gitlab.com fetch",
			src: &Source{
				Type:    SourceGit,
				ID:      "gitlab.com/test/repo",
				Version: "main",
			},
			respCode:  http.StatusOK,
			respBody:  []byte("test gitlab content"),
			wantBytes: []byte("test gitlab content"),
		},
		{
			name: "successful self-hosted gitlab fetch",
			src: &Source{
				Type:    SourceGit,
				ID:      "gitlab.example.com/test/repo",
				Version: "main",
			},
			respCode:  http.StatusOK,
			respBody:  []byte("test self-hosted content"),
			wantBytes: []byte("test self-hosted content"),
		},
		{
			name: "unsupported git host",
			src: &Source{
				Type:    SourceGit,
				ID:      "unsupported.com/test/repo",
				Version: "main",
			},
			wantErr: true,
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
			// Start test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if ua := r.Header.Get("User-Agent"); ua != "factctl" {
					t.Errorf("Expected User-Agent header 'factctl', got %q", ua)
				}

				if strings.Contains(tt.src.ID, "127.0.0.1") {
					// Return success for our test server
					w.WriteHeader(tt.respCode)
					w.Write(tt.respBody)
					return
				}

				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			// Override GitLab URL in tests
			gitlabConfig.baseURL = server.URL
			origID := tt.src.ID
			if !tt.wantErr {
				// Keep the path but use test server host
				path := tt.src.ID[strings.Index(tt.src.ID, "/")+1:]
				tt.src.ID = server.URL[7:] + "/" + path
			}
			
			f := NewGitFetcher()
			var buf bytes.Buffer
			hash, err := f.Fetch(context.Background(), tt.src, &buf)

			// Restore original ID
			tt.src.ID = origID

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