package instance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceRemoval(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	manager := NewManager(tmpDir)

	// Create a test instance with complete configuration
	cfg := &Config{
		Name:    "test-instance",
		Version: "1.1.87",
		Mods: ModsConfig{
			Enabled: []string{"base", "mod1"},
			Sources: map[string]string{
				"mod1": "portal:mod1@^1.0.0",
			},
		},
	}

	// Create the instance with all required directories
	inst, err := manager.Create(cfg)
	if err != nil {
		t.Fatalf("Failed to create test instance: %v", err)
	}

	// Create the saves directory explicitly
	savesDir := filepath.Join(inst.Dir, "saves")
	if err := os.MkdirAll(savesDir, 0755); err != nil {
		t.Fatalf("Failed to create saves directory: %v", err)
	}

	// Create some test files
	testFile := filepath.Join(inst.Dir, "saves", "test.zip")
	if err := os.WriteFile(testFile, []byte("test save data"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("Test file was not created: %v", err)
	}

	tests := []struct {
		name     string
		instName string
		backup   bool
		wantErr  bool
	}{
		{
			name:     "remove without backup",
			instName: "test-instance",
			backup:   false,
		},
		{
			name:     "remove with backup",
			instName: "test-instance",
			backup:   true,
		},
		{
			name:     "remove nonexistent",
			instName: "nonexistent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If this isn't the first test, recreate the instance
			if tt.instName == "test-instance" && !tt.wantErr {
				if _, err := manager.Create(cfg); err != nil {
					t.Fatalf("Failed to recreate test instance: %v", err)
				}
			}

			err := manager.Remove(tt.instName, tt.backup)
			if tt.wantErr {
				if err == nil {
					t.Error("Remove() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Remove() error = %v", err)
				return
			}

			// Check that instance directory is gone
			instDir := filepath.Join(tmpDir, "instances", tt.instName)
			if _, err := os.Stat(instDir); !os.IsNotExist(err) {
				t.Error("Instance directory still exists after removal")
			}

			// If backup was requested, check that it exists
			if tt.backup {
				backups, err := manager.ListBackups(tt.instName)
				if err != nil {
					t.Errorf("ListBackups() error = %v", err)
					return
				}
				if len(backups) == 0 {
					t.Error("No backup created")
					return
				}

				// Try restoring the backup
				t.Logf("Attempting to restore backup: %s", backups[0])
				if err := manager.RestoreBackup(backups[0]); err != nil {
					t.Errorf("RestoreBackup() error = %v", err)
					return
				}

				// Check directory structure
				t.Logf("Checking instance directory: %s", instDir)
				entries, err := os.ReadDir(instDir)
				if err != nil {
					t.Errorf("Failed to read instance directory: %v", err)
					return
				}
				for _, entry := range entries {
					t.Logf("Found entry: %s (dir: %v)", entry.Name(), entry.IsDir())
				}

				// Check that test file exists in restored backup
				testFile := filepath.Join(instDir, "saves", "test.zip")
				t.Logf("Checking test file: %s", testFile)
				data, err := os.ReadFile(testFile)
				if err != nil {
					t.Errorf("Failed to read restored test file: %v", err)

					// Check saves directory structure
					savesDir := filepath.Join(instDir, "saves")
					if entries, err := os.ReadDir(savesDir); err != nil {
						t.Logf("Failed to read saves directory: %v", err)
					} else {
						for _, entry := range entries {
							t.Logf("Found entry in saves: %s (dir: %v)", entry.Name(), entry.IsDir())
						}
					}
					return
				}
				if string(data) != "test save data" {
					t.Error("Restored file content does not match original")
				}
			}
		})
	}
}

func TestInstanceCreation(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	manager := NewManager(tmpDir)

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		check   func(*testing.T, string)
	}{
		{
			name: "basic instance",
			cfg: &Config{
				Name:    "test-basic",
				Version: "1.1.87",
				Mods:    ModsConfig{},
			},
			check: func(t *testing.T, dir string) {
				// Check directory structure
				checkDirs := []string{
					"saves",
					"mods",
					"config",
					"scripts",
				}
				for _, d := range checkDirs {
					if _, err := os.Stat(filepath.Join(dir, d)); os.IsNotExist(err) {
						t.Errorf("Directory %s was not created", d)
					}
				}

				// Check mod-list.json
				var modList struct {
					Mods []struct {
						Name    string `json:"name"`
						Enabled bool   `json:"enabled"`
					} `json:"mods"`
				}
				modListPath := filepath.Join(dir, "config", "mod-list.json")
				data, err := os.ReadFile(modListPath)
				if err != nil {
					t.Fatalf("Failed to read mod-list.json: %v", err)
				}
				if err := json.Unmarshal(data, &modList); err != nil {
					t.Fatalf("Failed to parse mod-list.json: %v", err)
				}
				if len(modList.Mods) != 1 || modList.Mods[0].Name != "base" {
					t.Error("Expected only base mod in mod-list.json")
				}
			},
		},
		{
			name: "server instance",
			cfg: &Config{
				Name:     "test-server",
				Version:  "1.1.87",
				Headless: true,
				Port:     34197,
				Mods:     ModsConfig{},
				Server: &ServerConfig{
					Name:             "Test Server",
					MaxPlayers:       32,
					Public:          true,
					AutoSave:        true,
					AutoSaveInterval: 5,
				},
			},
			check: func(t *testing.T, dir string) {
				// Check server-settings.json
				serverConfigPath := filepath.Join(dir, "config", "server-settings.json")
				data, err := os.ReadFile(serverConfigPath)
				if err != nil {
					t.Fatalf("Failed to read server-settings.json: %v", err)
				}

				var serverConfig map[string]interface{}
				if err := json.Unmarshal(data, &serverConfig); err != nil {
					t.Fatalf("Failed to parse server-settings.json: %v", err)
				}

				if serverConfig["name"] != "Test Server" {
					t.Error("Incorrect server name in server-settings.json")
				}
				if serverConfig["max_players"].(float64) != 32 {
					t.Error("Incorrect max_players in server-settings.json")
				}
			},
		},
		{
			name: "instance with mods",
			cfg: &Config{
				Name:    "test-mods",
				Version: "1.1.87",
				Mods: ModsConfig{
					Enabled: []string{"base", "mod1", "mod2"},
					Sources: map[string]string{
						"mod1": "portal:mod1@^1.0.0",
						"mod2": "portal:mod2@^2.0.0",
					},
				},
			},
			check: func(t *testing.T, dir string) {
				// Check mod-list.json
				var modList struct {
					Mods []struct {
						Name    string `json:"name"`
						Enabled bool   `json:"enabled"`
					} `json:"mods"`
				}
				modListPath := filepath.Join(dir, "config", "mod-list.json")
				data, err := os.ReadFile(modListPath)
				if err != nil {
					t.Fatalf("Failed to read mod-list.json: %v", err)
				}
				if err := json.Unmarshal(data, &modList); err != nil {
					t.Fatalf("Failed to parse mod-list.json: %v", err)
				}
				if len(modList.Mods) != 3 {
					t.Errorf("Expected 3 mods in mod-list.json, got %d", len(modList.Mods))
				}
			},
		},
		{
			name: "invalid config",
			cfg: &Config{
				// Missing required fields
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := manager.Create(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("Create() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}

			if inst.State != StateStopped {
				t.Errorf("New instance should be in stopped state, got %v", inst.State)
			}

			if tt.check != nil {
				tt.check(t, inst.Dir)
			}
		})
	}
}