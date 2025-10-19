package instance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid minimal config",
			cfg: Config{
				Name:    "test-instance",
				Version: "1.1.87",
				Mods:    ModsConfig{},
			},
			wantErr: false,
		},
		{
			name: "valid server config",
			cfg: Config{
				Name:     "test-server",
				Version:  "1.1.87",
				Headless: true,
				Port:     34197,
				Mods:     ModsConfig{},
				Server: &ServerConfig{
					Name:             "Test Server",
					MaxPlayers:       32,
					AutoSave:        true,
					AutoSaveInterval: 5,
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cfg: Config{
				Version: "1.1.87",
				Mods:    ModsConfig{},
			},
			wantErr: true,
		},
		{
			name: "missing version",
			cfg: Config{
				Name: "test-instance",
				Mods: ModsConfig{},
			},
			wantErr: true,
		},
		{
			name: "invalid server config",
			cfg: Config{
				Name:    "test-server",
				Version: "1.1.87",
				Mods:    ModsConfig{},
				Server: &ServerConfig{
					MaxPlayers: 0, // Invalid
				},
			},
			wantErr: true,
		},
		{
			name: "mods without sources",
			cfg: Config{
				Name:    "test-instance",
				Version: "1.1.87",
				Mods: ModsConfig{
					Enabled: []string{"mod1"},
					// Missing Sources
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr {
				if err == nil {
					t.Error("validate() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("validate() error = %v", err)
			}
		})
	}
}

func TestConfigLoadSave(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testCfg := &Config{
		Name:     "test-instance",
		Version:  "1.1.87",
		Headless: true,
		Port:     34197,
		SaveFile: "test.zip",
		Mods: ModsConfig{
			Enabled: []string{"base", "mod1"},
			Sources: map[string]string{
				"mod1": "portal:mod1@^1.0.0",
			},
			Settings: map[string]interface{}{
				"mod1.setting": true,
			},
		},
		Server: &ServerConfig{
			Name:             "Test Server",
			MaxPlayers:       32,
			Public:          true,
			AutoSave:        true,
			AutoSaveInterval: 5,
			Settings: map[string]interface{}{
				"visibility": map[string]interface{}{
					"public": true,
				},
			},
		},
	}

	// Test saving config
	configPath := filepath.Join(tmpDir, "test-instance.json")
	if err := testCfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	// Test loading config
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Compare loaded config with original
	if loaded.Name != testCfg.Name {
		t.Errorf("LoadConfig() got Name = %v, want %v", loaded.Name, testCfg.Name)
	}
	if loaded.Version != testCfg.Version {
		t.Errorf("LoadConfig() got Version = %v, want %v", loaded.Version, testCfg.Version)
	}
	if len(loaded.Mods.Enabled) != len(testCfg.Mods.Enabled) {
		t.Errorf("LoadConfig() got %d enabled mods, want %d", len(loaded.Mods.Enabled), len(testCfg.Mods.Enabled))
	}
	if loaded.Server.MaxPlayers != testCfg.Server.MaxPlayers {
		t.Errorf("LoadConfig() got MaxPlayers = %v, want %v", loaded.Server.MaxPlayers, testCfg.Server.MaxPlayers)
	}
}