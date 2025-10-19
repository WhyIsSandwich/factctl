package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

// mockFetcher implements Fetcher for testing
type mockFetcher struct {
	content string
	hash    string
	err     error
}

func (m *mockFetcher) Fetch(_ context.Context, _ *Source, w io.Writer) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	io.WriteString(w, m.content)
	return m.hash, nil
}

func TestResolver(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		fetcher   *mockFetcher
		wantHash  string
		wantErr   bool
		wantBytes []byte
	}{
		{
			name: "successful fetch",
			spec: "portal:test-mod@1.0.0",
			fetcher: &mockFetcher{
				content: "test content",
				hash:    "testhash123",
			},
			wantHash:  "testhash123",
			wantBytes: []byte("test content"),
		},
		{
			name: "fetcher error",
			spec: "portal:bad-mod@1.0.0",
			fetcher: &mockFetcher{
				err: fmt.Errorf("fetch failed"),
			},
			wantErr: true,
		},
		{
			name:    "invalid source",
			spec:    "invalid:spec",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResolver()
			if tt.fetcher != nil {
				src, _ := ParseSource(tt.spec)
				r.RegisterFetcher(src.Type, tt.fetcher)
			}

			var buf strings.Builder
			info, err := r.Resolve(context.Background(), tt.spec, &buf)

			if tt.wantErr {
				if err == nil {
					t.Error("Resolve() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Resolve() error = %v", err)
				return
			}

			if info.Hash != tt.wantHash {
				t.Errorf("Resolve() got hash = %v, want %v", info.Hash, tt.wantHash)
			}

			if !bytes.Equal([]byte(buf.String()), tt.wantBytes) {
				t.Errorf("Resolve() got content = %v, want %v", buf.String(), string(tt.wantBytes))
			}
		})
	}
}