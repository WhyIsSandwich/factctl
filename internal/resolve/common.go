package resolve

import (
	"net/http"
)

// HTTPClient defines the interface for making HTTP requests
// This allows us to easily mock requests in tests
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// ClientConfig contains configuration for HTTP clients
type ClientConfig struct {
	// BaseURL overrides the default API URL (used in tests)
	BaseURL string
	// Client overrides the default HTTP client (used in tests)
	Client HTTPClient
	// UserAgent sets the User-Agent header for requests
	UserAgent string
}

// DefaultConfig returns the default client configuration
func DefaultConfig() ClientConfig {
	return ClientConfig{
		Client:    http.DefaultClient,
		UserAgent: "factctl",
	}
}

// TestConfig returns a configuration for testing with the given base URL
func TestConfig(baseURL string, client HTTPClient) ClientConfig {
	return ClientConfig{
		BaseURL:   baseURL,
		Client:    client,
		UserAgent: "factctl-test",
	}
}

// knownMultiModRepos is a list of repositories that are known to contain multiple mods
var knownMultiModRepos = map[string]bool{
	"modded-factorio/SeaBlock":   true,
	"Arch666Angel/mods":          true,
	"KiwiHawk/SeaBlock":          true, // Fork that PR 343 is from
}

// isMultiModRepo checks if a repository is known to contain multiple mods
func isMultiModRepo(owner, repo string) bool {
	return knownMultiModRepos[owner+"/"+repo]
}