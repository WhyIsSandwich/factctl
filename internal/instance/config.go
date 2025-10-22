package instance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/WhyIsSandwich/factctl/internal/jsonc"
)

// Config represents an instance configuration
type Config struct {
	// Name of the instance
	Name string `json:"name"`

	// Version of Factorio to use
	Version string `json:"version"`

	// Runtime name to use (defaults to version if not specified)
	Runtime string `json:"runtime,omitempty"`

	// Port to run the server on (if running as server)
	Port int `json:"port,omitempty"`

	// Whether to run in headless mode
	Headless bool `json:"headless,omitempty"`

	// Save file to use/create
	SaveFile string `json:"save_file,omitempty"`

	// Mods configuration
	Mods ModsConfig `json:"mods"`

	// Server settings (if running as server)
	Server *ServerConfig `json:"server,omitempty"`
}

// GetRuntime returns the runtime name to use, defaulting to version if not specified
func (c *Config) GetRuntime() string {
	if c.Runtime != "" {
		return c.Runtime
	}
	return c.Version
}

// ModsConfig contains mod-related configuration
type ModsConfig struct {
	// List of mods to enable
	Enabled []string `json:"enabled"`

	// Map of mod names to their versions/sources
	Sources map[string]string `json:"sources"`

	// Additional mod settings
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// ServerConfig contains server-specific settings
type ServerConfig struct {
	// Server name/description
	Name string `json:"name"`

	// Max players allowed
	MaxPlayers int `json:"max_players"`

	// Whether the server is public
	Public bool `json:"public"`

	// Password for connecting (optional)
	Password string `json:"password,omitempty"`

	// Admin users
	Admins []string `json:"admins,omitempty"`

	// Whether to auto-save
	AutoSave bool `json:"auto_save"`

	// Auto-save interval in minutes
	AutoSaveInterval int `json:"auto_save_interval"`

	// Additional server settings
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// LoadConfig loads an instance configuration from a file
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config file: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := jsonc.Parse(f, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves an instance configuration to a file
func (c *Config) SaveConfig(path string) error {
	if err := c.validate(); err != nil {
		return fmt.Errorf("validating config: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	if c.Name == "" {
		return fmt.Errorf("instance name is required")
	}

	if c.Version == "" {
		return fmt.Errorf("factorio version is required")
	}

	// Check if any non-built-in mods require sources
	builtinMods := map[string]bool{
		"base":           true,
		"elevated-rails": true,
		"quality":        true,
		"space-age":      true,
	}

	nonBuiltinMods := []string{}
	for _, mod := range c.Mods.Enabled {
		if !builtinMods[mod] {
			nonBuiltinMods = append(nonBuiltinMods, mod)
		}
	}

	if len(nonBuiltinMods) > 0 && len(c.Mods.Sources) == 0 {
		return fmt.Errorf("mod sources are required for non-built-in mods: %v", nonBuiltinMods)
	}

	if c.Server != nil {
		if err := c.Server.validate(); err != nil {
			return fmt.Errorf("invalid server config: %w", err)
		}
	}

	return nil
}

// validate checks if the server configuration is valid
func (s *ServerConfig) validate() error {
	if s.Name == "" {
		return fmt.Errorf("server name is required")
	}

	if s.MaxPlayers < 1 {
		return fmt.Errorf("max_players must be at least 1")
	}

	if s.AutoSave && s.AutoSaveInterval < 1 {
		return fmt.Errorf("auto_save_interval must be at least 1 minute")
	}

	return nil
}
