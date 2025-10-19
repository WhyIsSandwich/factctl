package resolve

import "testing"

func TestParseSource(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    *Source
		wantErr bool
	}{
		{
			name: "portal source",
			spec: "portal:flib@^0.12",
			want: &Source{
				Type:    SourcePortal,
				ID:      "flib",
				Version: "^0.12",
			},
		},
		{
			name: "github source",
			spec: "gh:Earendel/SpaceExploration@v0.6.151",
			want: &Source{
				Type:    SourceGitHub,
				Owner:   "Earendel",
				Repo:    "SpaceExploration",
				Version: "v0.6.151",
			},
		},
		{
			name: "github PR source",
			spec: "ghpr:org/SpaceExploration#123",
			want: &Source{
				Type:  SourceGitHubPR,
				Owner: "org",
				Repo:  "SpaceExploration",
				PR:    123,
			},
		},
		{
			name: "git source",
			spec: "git:gitlab.com/user/repo@main",
			want: &Source{
				Type:    SourceGit,
				ID:      "gitlab.com/user/repo",
				Version: "main",
			},
		},
		{
			name: "file source",
			spec: "file:/path/to/mod.zip",
			want: &Source{
				Type: SourceFile,
				Path: "/path/to/mod.zip",
			},
		},
		{
			name: "url source",
			spec: "url:https://example.com/mod.zip",
			want: &Source{
				Type: SourceURL,
				URL:  "https://example.com/mod.zip",
			},
		},
		{
			name:    "invalid source type",
			spec:    "invalid:test",
			wantErr: true,
		},
		{
			name:    "missing source type",
			spec:    "test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSource(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseSource() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSource() error = %v", err)
				return
			}
			if got.Type != tt.want.Type {
				t.Errorf("ParseSource() got Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.ID != tt.want.ID {
				t.Errorf("ParseSource() got ID = %v, want %v", got.ID, tt.want.ID)
			}
			if got.Version != tt.want.Version {
				t.Errorf("ParseSource() got Version = %v, want %v", got.Version, tt.want.Version)
			}
			if got.Owner != tt.want.Owner {
				t.Errorf("ParseSource() got Owner = %v, want %v", got.Owner, tt.want.Owner)
			}
			if got.Repo != tt.want.Repo {
				t.Errorf("ParseSource() got Repo = %v, want %v", got.Repo, tt.want.Repo)
			}
			if got.PR != tt.want.PR {
				t.Errorf("ParseSource() got PR = %v, want %v", got.PR, tt.want.PR)
			}
			if got.Path != tt.want.Path {
				t.Errorf("ParseSource() got Path = %v, want %v", got.Path, tt.want.Path)
			}
			if got.URL != tt.want.URL {
				t.Errorf("ParseSource() got URL = %v, want %v", got.URL, tt.want.URL)
			}
		})
	}
}