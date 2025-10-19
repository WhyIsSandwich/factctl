package resolve

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Source represents a mod source specification
type Source struct {
	Type    SourceType
	ID      string   // Mod ID for portal, repo for git
	Version string   // Version constraint or git ref
	Owner   string   // For GitHub sources
	Repo    string   // For GitHub sources
	PR      int      // For GitHub PR sources
	Path    string   // For file sources
	URL     string   // For URL sources
	SubPath string   // Directory within repo containing the mod (for multi-mod repos)
}

// SourceType identifies the type of mod source
type SourceType int

const (
	SourceUnknown SourceType = iota
	SourcePortal            // portal:<id>@<version|range>
	SourceGitHub           // gh:<owner>/<repo>@<ref>
	SourceGitHubPR        // ghpr:<owner>/<repo>#<pr>
	SourceGit             // git:<host>/<repo>@<ref>
	SourceFile            // file:...
	SourceURL             // url:...
)

var (
	ErrInvalidSource = errors.New("invalid source specification")
)

// ParseSource parses a mod source specification string into a Source struct
func ParseSource(spec string) (*Source, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: missing source type", ErrInvalidSource)
	}

	srcType := parts[0]
	srcSpec := parts[1]

	switch srcType {
	case "portal":
		return parsePortalSource(srcSpec)
	case "gh":
		return parseGitHubSource(srcSpec)
	case "ghpr":
		return parseGitHubPRSource(srcSpec)
	case "git":
		return parseGitSource(srcSpec)
	case "file":
		return &Source{
			Type: SourceFile,
			Path: srcSpec,
		}, nil
	case "url":
		if _, err := url.Parse(srcSpec); err != nil {
			return nil, fmt.Errorf("%w: invalid URL", ErrInvalidSource)
		}
		return &Source{
			Type: SourceURL,
			URL:  srcSpec,
		}, nil
	default:
		return nil, fmt.Errorf("%w: unknown source type %q", ErrInvalidSource, srcType)
	}
}