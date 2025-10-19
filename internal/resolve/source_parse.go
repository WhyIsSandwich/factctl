package resolve

import (
	"fmt"
	"strconv"
	"strings"
)

// parsePortalSource parses a portal:<id>@<version|range> specification
func parsePortalSource(spec string) (*Source, error) {
	parts := strings.SplitN(spec, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: missing version in portal spec", ErrInvalidSource)
	}

	return &Source{
		Type:    SourcePortal,
		ID:      parts[0],
		Version: parts[1],
	}, nil
}

// parseGitHubSource parses a gh:<owner>/<repo>@<ref> specification
func parseGitHubSource(spec string) (*Source, error) {
	repoRef := strings.SplitN(spec, "@", 2)
	if len(repoRef) != 2 {
		return nil, fmt.Errorf("%w: missing ref in GitHub spec", ErrInvalidSource)
	}

	ownerRepo := strings.SplitN(repoRef[0], "/", 2)
	if len(ownerRepo) != 2 {
		return nil, fmt.Errorf("%w: invalid GitHub repo format", ErrInvalidSource)
	}

	return &Source{
		Type:    SourceGitHub,
		Owner:   ownerRepo[0],
		Repo:    ownerRepo[1],
		Version: repoRef[1],
	}, nil
}

// parseGitHubPRSource parses a ghpr:<owner>/<repo>#<pr> specification
func parseGitHubPRSource(spec string) (*Source, error) {
	repoPR := strings.SplitN(spec, "#", 2)
	if len(repoPR) != 2 {
		return nil, fmt.Errorf("%w: missing PR number in GitHub PR spec", ErrInvalidSource)
	}

	ownerRepo := strings.SplitN(repoPR[0], "/", 2)
	if len(ownerRepo) != 2 {
		return nil, fmt.Errorf("%w: invalid GitHub repo format", ErrInvalidSource)
	}

	pr, err := strconv.Atoi(repoPR[1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid PR number", ErrInvalidSource)
	}

	return &Source{
		Type:  SourceGitHubPR,
		Owner: ownerRepo[0],
		Repo:  ownerRepo[1],
		PR:    pr,
	}, nil
}

// parseGitSource parses a git:<host>/<repo>@<ref> specification
func parseGitSource(spec string) (*Source, error) {
	repoRef := strings.SplitN(spec, "@", 2)
	if len(repoRef) != 2 {
		return nil, fmt.Errorf("%w: missing ref in Git spec", ErrInvalidSource)
	}

	return &Source{
		Type:    SourceGit,
		ID:      repoRef[0],
		Version: repoRef[1],
	}, nil
}