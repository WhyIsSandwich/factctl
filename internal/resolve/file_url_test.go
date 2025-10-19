package resolve

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFileFetcher(t *testing.T) {
	// Create a temp file for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.zip")
	testContent := []byte("test file content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	tests := []struct {
		name      string
		src       *Source
		wantErr   bool
		wantBytes []byte
	}{
		{
			name: "successful fetch",
			src: &Source{
				Type: SourceFile,
				Path: testFile,
			},
			wantBytes: testContent,
		},
		{
			name: "nonexistent file",
			src: &Source{
				Type: SourceFile,
				Path: "/nonexistent/file.zip",
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
			f := NewFileFetcher()
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

func TestURLFetcher(t *testing.T) {
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
				Type: SourceURL,
				URL:  "https://example.com/mod.zip",
			},
			respCode:  http.StatusOK,
			respBody:  []byte("test url content"),
			wantBytes: []byte("test url content"),
		},
		{
			name: "not found",
			src: &Source{
				Type: SourceURL,
				URL:  "https://example.com/nonexistent.zip",
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
				if ua := r.Header.Get("User-Agent"); ua != "factctl" {
					t.Errorf("Expected User-Agent header 'factctl', got %q", ua)
				}

				w.WriteHeader(tt.respCode)
				w.Write(tt.respBody)
			}))
			defer server.Close()

			// Override URL for testing
			if !tt.wantErr || tt.respCode != 0 {
				tt.src.URL = server.URL
			}

			f := NewURLFetcher()
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