package instance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/WhyIsSandwich/factctl/internal/resolve"
	"github.com/blang/semver"
)

// ModInfo represents metadata from a mod's info.json
type ModInfo struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Title           string   `json:"title"`
	Author          string   `json:"author"`
	Contact         string   `json:"contact,omitempty"`
	Homepage        string   `json:"homepage,omitempty"`
	Description     string   `json:"description"`
	Dependencies    []string `json:"dependencies"`
	FactorioVersion string   `json:"factorio_version,omitempty"`
}

// ModManager handles mod installation and management
type ModManager struct {
	baseDir  string
	resolver *resolve.Resolver
	modInfos map[string]*ModInfo
	mu       sync.RWMutex
}

// NewModManager creates a new mod manager
func NewModManager(baseDir string) *ModManager {
	return &ModManager{
		baseDir:  baseDir,
		resolver: resolve.NewResolver(),
		modInfos: make(map[string]*ModInfo),
	}
}

// InstallMod installs a mod for an instance
func (mm *ModManager) InstallMod(ctx context.Context, inst *Instance, modSpec string) error {
	// Prepare mod directory
	modDir := filepath.Join(inst.Dir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		return fmt.Errorf("creating mod directory: %w", err)
	}

	// Create temporary buffer for mod content
	var buf bytes.Buffer

	// TODO: Implement actual mod download using resolver
	// For now, we'll just validate the structure

	// Extract mod info from the buffer
	modInfo, err := mm.extractModInfo(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("extracting mod info: %w", err)
	}

	// Check Factorio version compatibility
	if modInfo.FactorioVersion != "" {
		if !isVersionCompatible(inst.Config.Version, modInfo.FactorioVersion) {
			return fmt.Errorf("mod requires Factorio %s but instance uses %s",
				modInfo.FactorioVersion, inst.Config.Version)
		}
	}

	// Cache mod info
	mm.mu.Lock()
	mm.modInfos[modInfo.Name] = modInfo
	mm.mu.Unlock()

	// Install dependencies first
	if err := mm.installDependencies(ctx, inst, modInfo); err != nil {
		return fmt.Errorf("installing dependencies: %w", err)
	}

	// Write mod file
	modPath := filepath.Join(modDir, fmt.Sprintf("%s_%s.zip", modInfo.Name, modInfo.Version))
	if err := os.WriteFile(modPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing mod file: %w", err)
	}

	// Update mod-list.json
	if err := mm.updateModList(inst, modInfo.Name, true); err != nil {
		return fmt.Errorf("updating mod list: %w", err)
	}

	return nil
}

// UninstallMod removes a mod from an instance
func (mm *ModManager) UninstallMod(inst *Instance, modName string) error {
	// Can't remove base mod
	if modName == "base" {
		return fmt.Errorf("cannot uninstall base mod")
	}

	modDir := filepath.Join(inst.Dir, "mods")
	pattern := filepath.Join(modDir, fmt.Sprintf("%s_*.zip", modName))

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("finding mod files: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("mod %s not found", modName)
	}

	// Remove mod files
	for _, path := range matches {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing mod file: %w", err)
		}
	}

	// Update mod-list.json
	if err := mm.updateModList(inst, modName, false); err != nil {
		return fmt.Errorf("updating mod list: %w", err)
	}

	return nil
}

// ListMods returns information about installed mods
func (mm *ModManager) ListMods(inst *Instance) ([]*ModInfo, error) {
	modDir := filepath.Join(inst.Dir, "mods")
	pattern := filepath.Join(modDir, "*.zip")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("finding mod files: %w", err)
	}

	var mods []*ModInfo
	for _, path := range matches {
		info, err := mm.getModInfo(path)
		if err != nil {
			return nil, fmt.Errorf("reading mod info from %s: %w", filepath.Base(path), err)
		}
		mods = append(mods, info)
	}

	return mods, nil
}

// extractModInfo reads mod info from a zip file
func (mm *ModManager) extractModInfo(r io.Reader) (*ModInfo, error) {
	// Read all data to enable multiple passes
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading mod data: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("reading zip: %w", err)
	}

	// Find and read info.json
	for _, file := range zipReader.File {
		if filepath.Base(file.Name) == "info.json" {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("opening info.json: %w", err)
			}
			defer rc.Close()

			var info ModInfo
			if err := json.NewDecoder(rc).Decode(&info); err != nil {
				return nil, fmt.Errorf("parsing info.json: %w", err)
			}

			return &info, nil
		}
	}

	return nil, fmt.Errorf("info.json not found in mod")
}

// getModInfo reads info from an installed mod file
func (mm *ModManager) getModInfo(path string) (*ModInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return mm.extractModInfo(file)
}

// installDependencies installs required dependencies for a mod
func (mm *ModManager) installDependencies(ctx context.Context, inst *Instance, info *ModInfo) error {
	for _, dep := range info.Dependencies {
		if err := mm.installDependency(ctx, inst, dep); err != nil {
			return fmt.Errorf("installing dependency %s: %w", dep, err)
		}
	}
	return nil
}

// installDependency installs a single dependency
func (mm *ModManager) installDependency(ctx context.Context, inst *Instance, dep string) error {
	// Parse dependency string (e.g., "base >= 1.1")
	parts := strings.Fields(dep)
	if len(parts) == 0 {
		return fmt.Errorf("empty dependency")
	}

	modName := parts[0]
	if modName == "base" {
		return nil // Base mod is always available
	}

	// Check if already installed
	if mm.isModInstalled(inst, modName) {
		return nil
	}

	// Convert dependency to mod specification
	spec := fmt.Sprintf("portal:%s", modName)
	if len(parts) > 2 {
		spec += "@" + parts[2]
	}

	// Install dependency
	return mm.InstallMod(ctx, inst, spec)
}

// isModInstalled checks if a mod is installed
func (mm *ModManager) isModInstalled(inst *Instance, modName string) bool {
	pattern := filepath.Join(inst.Dir, "mods", fmt.Sprintf("%s_*.zip", modName))
	matches, _ := filepath.Glob(pattern)
	return len(matches) > 0
}

// updateModList updates the mod-list.json file
func (mm *ModManager) updateModList(inst *Instance, modName string, enabled bool) error {
	listPath := filepath.Join(inst.Dir, "config", "mod-list.json")

	var list struct {
		Mods []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"mods"`
	}

	// Read existing list
	data, err := os.ReadFile(listPath)
	if err == nil {
		if err := json.Unmarshal(data, &list); err != nil {
			return fmt.Errorf("parsing mod list: %w", err)
		}
	}

	// Update mod entry
	found := false
	for i := range list.Mods {
		if list.Mods[i].Name == modName {
			list.Mods[i].Enabled = enabled
			found = true
			break
		}
	}

	if !found && enabled {
		list.Mods = append(list.Mods, struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		}{
			Name:    modName,
			Enabled: true,
		})
	}

	// Write updated list
	data, err = json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding mod list: %w", err)
	}

	if err := os.WriteFile(listPath, data, 0644); err != nil {
		return fmt.Errorf("writing mod list: %w", err)
	}

	return nil
}

// isVersionCompatible checks if a mod version is compatible with Factorio version
func isVersionCompatible(factorioVersion, modVersion string) bool {
	// Parse versions
	fv, err := semver.Parse(strings.TrimPrefix(factorioVersion, "v"))
	if err != nil {
		return false
	}

	mv, err := semver.Parse(strings.TrimPrefix(modVersion, "v"))
	if err != nil {
		return false
	}

	// Compare major and minor versions
	return fv.Major == mv.Major && fv.Minor == mv.Minor
}
