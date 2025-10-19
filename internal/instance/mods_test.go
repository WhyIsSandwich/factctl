package instance

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestModManager(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test instance
	inst := &Instance{
		Config: &Config{
			Name:    "test-instance",
			Version: "1.1.87",
		},
		Dir: filepath.Join(tmpDir, "instances", "test-instance"),
	}

	// Create instance directories
	dirs := []string{
		filepath.Join(inst.Dir, "mods"),
		filepath.Join(inst.Dir, "config"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create initial mod-list.json
	modList := struct {
		Mods []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"mods"`
	}{
		Mods: []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		}{
			{Name: "base", Enabled: true},
		},
	}
	modListPath := filepath.Join(inst.Dir, "config", "mod-list.json")
	if err := SaveJSON(modListPath, &modList); err != nil {
		t.Fatalf("Failed to create mod-list.json: %v", err)
	}

	manager := NewModManager(tmpDir)

	t.Run("mod installation", func(t *testing.T) {
		// Update mod list first
		list := struct {
			Mods []struct {
				Name    string `json:"name"`
				Enabled bool   `json:"enabled"`
			} `json:"mods"`
		}{
			Mods: []struct {
				Name    string `json:"name"`
				Enabled bool   `json:"enabled"`
			}{
				{Name: "base", Enabled: true},
				{Name: "test-mod", Enabled: true},
			},
		}

		listPath := filepath.Join(inst.Dir, "config", "mod-list.json")
		if err := SaveJSON(listPath, &list); err != nil {
			t.Fatalf("Failed to update mod-list.json: %v", err)
		}

		// Create mock mod zip
		modInfo := &ModInfo{
			Name:            "test-mod",
			Version:         "1.0.0",
			Title:           "Test Mod",
			Author:          "Tester",
			Description:     "A test mod",
			Dependencies:    []string{"base >= 1.1"},
			FactorioVersion: "1.1.0",
		}

		// Create mod file directly
		modPath := filepath.Join(inst.Dir, "mods", "test-mod_1.0.0.zip")
		modFile, err := os.Create(modPath)
		if err != nil {
			t.Fatalf("Failed to create mod file: %v", err)
		}

		if err := createTestModZip(modFile, modInfo); err != nil {
			modFile.Close()
			t.Fatalf("Failed to create test mod zip: %v", err)
		}
		if err := modFile.Close(); err != nil {
			t.Fatalf("Failed to close mod file: %v", err)
		}

		// Verify the mod was installed correctly
		mods, err := manager.ListMods(inst)
		if err != nil {
			t.Fatalf("Failed to list mods: %v", err)
		}

		found := false
		for _, mod := range mods {
			if mod.Name == "test-mod" && mod.Version == "1.0.0" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Mod not found in installed mods")
		}

		// Verify mod-list.json
		data, err := os.ReadFile(listPath)
		if err != nil {
			t.Fatalf("Failed to read mod-list.json: %v", err)
		}

		var currentList struct {
			Mods []struct {
				Name    string `json:"name"`
				Enabled bool   `json:"enabled"`
			} `json:"mods"`
		}
		if err := json.Unmarshal(data, &currentList); err != nil {
			t.Fatalf("Failed to parse mod-list.json: %v", err)
		}

		found = false
		for _, mod := range currentList.Mods {
			if mod.Name == "test-mod" && mod.Enabled {
				found = true
				break
			}
		}
		if !found {
			t.Error("Mod not found in mod-list.json or not enabled")
		}
	})

	t.Run("mod uninstallation", func(t *testing.T) {
		if err := manager.UninstallMod(inst, "test-mod"); err != nil {
			t.Errorf("UninstallMod() error = %v", err)
			return
		}

		// Verify mod file is gone
		modPath := filepath.Join(inst.Dir, "mods", "test-mod_1.0.0.zip")
		if _, err := os.Stat(modPath); !os.IsNotExist(err) {
			t.Error("Mod file still exists")
		}

		// Verify mod-list.json
		listPath := filepath.Join(inst.Dir, "config", "mod-list.json")
		var list struct {
			Mods []struct {
				Name    string `json:"name"`
				Enabled bool   `json:"enabled"`
			} `json:"mods"`
		}
		data, err := os.ReadFile(listPath)
		if err != nil {
			t.Errorf("Failed to read mod-list.json: %v", err)
			return
		}
		if err := json.Unmarshal(data, &list); err != nil {
			t.Errorf("Failed to parse mod-list.json: %v", err)
			return
		}

		for _, mod := range list.Mods {
			if mod.Name == "test-mod" && mod.Enabled {
				t.Error("Mod still enabled in mod-list.json")
			}
		}
	})

	t.Run("version compatibility", func(t *testing.T) {
		tests := []struct {
			factorioVersion string
			modVersion      string
			want            bool
		}{
			{"1.1.87", "1.1.0", true},
			{"1.1.87", "1.0.0", false},
			{"1.1.87", "2.0.0", false},
			{"2.0.0", "2.0.1", true},
		}

		for _, tt := range tests {
			got := isVersionCompatible(tt.factorioVersion, tt.modVersion)
			if got != tt.want {
				t.Errorf("isVersionCompatible(%q, %q) = %v, want %v",
					tt.factorioVersion, tt.modVersion, got, tt.want)
			}
		}
	})
}

// createTestModZip creates a mock mod zip file for testing
func createTestModZip(w io.Writer, info *ModInfo) error {
	zw := zip.NewWriter(w)

	// Add info.json
	f, err := zw.Create("info.json")
	if err != nil {
		return err
	}

	if err := json.NewEncoder(f).Encode(info); err != nil {
		return err
	}

	// Important: Close the zip writer to flush the zip footer
	return zw.Close()
}
