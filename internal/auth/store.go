package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Credentials stores authentication tokens for various services
type Credentials struct {
	FactorioUsername string `json:"factorio_username,omitempty"`
	FactorioToken    string `json:"factorio_token,omitempty"`
}

var (
	ErrNoCredentials = errors.New("no credentials found")
)

// Store manages the storage and retrieval of credentials
type Store struct {
	configDir string
}

// NewStore creates a new credential store using the specified config directory
func NewStore(configDir string) *Store {
	return &Store{configDir: configDir}
}

// DefaultLocation returns the default credentials file location
func DefaultLocation() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "factctl", "credentials.json"), nil
}

// Load reads credentials from disk
func (s *Store) Load() (*Credentials, error) {
	path := filepath.Join(s.configDir, "credentials.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrNoCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return &creds, nil
}

// Save writes credentials to disk
func (s *Store) Save(creds *Credentials) error {
	if err := os.MkdirAll(s.configDir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}

	path := filepath.Join(s.configDir, "credentials.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}

	return nil
}

// Clear removes all stored credentials
func (s *Store) Clear() error {
	path := filepath.Join(s.configDir, "credentials.json")
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}