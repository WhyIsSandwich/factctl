package instance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"crypto/sha256"
	"encoding/hex"

	"github.com/WhyIsSandwich/factctl/internal/auth"
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

// CacheEntry represents a cached download
type CacheEntry struct {
	URL      string    `json:"url"`
	Hash     string    `json:"hash"`
	FilePath string    `json:"file_path"`
	Size     int64     `json:"size"`
	CachedAt time.Time `json:"cached_at"`
}

// ModManager handles mod installation and management
type ModManager struct {
	baseDir  string
	resolver *resolve.Resolver
	modInfos map[string]*ModInfo
	mu       sync.RWMutex
	cacheDir string
	// Source registry map: modName -> sourceName -> modData
	sourceRegistry map[string]map[string][]byte
}

// NewModManager creates a new mod manager
func NewModManager(baseDir string) *ModManager {
	cacheDir := filepath.Join(baseDir, "cache", "downloads")
	return &ModManager{
		baseDir:        baseDir,
		cacheDir:       cacheDir,
		resolver:       resolve.NewResolver(),
		modInfos:       make(map[string]*ModInfo),
		sourceRegistry: make(map[string]map[string][]byte),
	}
}

// InstallMod installs a mod for an instance
func (mm *ModManager) InstallMod(ctx context.Context, inst *Instance, modSpec string) error {
	// Prepare mod directory
	modDir := filepath.Join(inst.Dir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		return fmt.Errorf("creating mod directory: %w", err)
	}

	// Check if this is a direct source specification (portal:, github:, etc.)
	if strings.Contains(modSpec, ":") && (strings.HasPrefix(modSpec, "portal:") || strings.HasPrefix(modSpec, "github:") || strings.HasPrefix(modSpec, "ghpr:") || strings.HasPrefix(modSpec, "git:")) {
		// This is a direct source specification, download directly
		return mm.installModFromDirectSource(ctx, inst, modSpec)
	}

	// This is a mod name that needs to be resolved from sources
	return mm.installModFromRegistry(ctx, inst, modSpec)
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

// BuildSourceRegistry loads all configured sources and builds a comprehensive map of available mods
func (mm *ModManager) BuildSourceRegistry(ctx context.Context, inst *Instance) error {
	fmt.Println("Building source registry from configured sources...")

	// Clear existing registry
	mm.mu.Lock()
	mm.sourceRegistry = make(map[string]map[string][]byte)
	mm.mu.Unlock()

	// Load each source and build the registry
	for sourceName, sourceURL := range inst.Config.Mods.Sources {
		fmt.Printf("  → Loading source '%s' (%s)...\n", sourceName, sourceURL)

		// Download the source repository
		var repoBuf bytes.Buffer
		if err := mm.downloadMod(ctx, sourceURL, &repoBuf); err != nil {
			fmt.Printf("  → Warning: Failed to download source '%s': %v\n", sourceName, err)
			continue
		}

		// Extract all mods from this source
		mods, err := mm.extractAllModsFromRepository(repoBuf.Bytes())
		if err != nil {
			fmt.Printf("  → Warning: Failed to extract mods from '%s': %v\n", sourceName, err)
			continue
		}

		// Add mods to registry
		mm.mu.Lock()
		for modName, modData := range mods {
			if mm.sourceRegistry[modName] == nil {
				mm.sourceRegistry[modName] = make(map[string][]byte)
			}
			mm.sourceRegistry[modName][sourceName] = modData
		}
		mm.mu.Unlock()

		fmt.Printf("  → Found %d mods in source '%s'\n", len(mods), sourceName)
	}

	// Count total unique mods
	mm.mu.RLock()
	totalMods := len(mm.sourceRegistry)
	mm.mu.RUnlock()

	fmt.Printf("Source registry built with %d unique mods\n", totalMods)
	return nil
}

// InstallModsRecursively installs mods and all their dependencies recursively
func (mm *ModManager) InstallModsRecursively(ctx context.Context, inst *Instance, modNames []string) ([]string, error) {
	// Build source registry upfront to avoid repeated API calls
	if err := mm.BuildSourceRegistry(ctx, inst); err != nil {
		return nil, fmt.Errorf("building source registry: %w", err)
	}

	// Track installed mods to avoid duplicates
	installedMods := make(map[string]bool)
	var installQueue []string
	var errors []error

	// Add initial mods to queue
	for _, modName := range modNames {
		if !installedMods[modName] {
			installQueue = append(installQueue, modName)
		}
	}

	// Process queue until empty
	for len(installQueue) > 0 {
		modName := installQueue[0]
		installQueue = installQueue[1:]

		if installedMods[modName] {
			continue // Already installed
		}

		// Check if mod is already installed
		if mm.isModInstalled(inst, modName) {
			fmt.Printf("Mod '%s' already installed, skipping...\n", modName)
			installedMods[modName] = true
			continue
		}

		// Skip built-in mods (they're always available)
		if isBuiltinMod(modName) {
			fmt.Printf("Mod '%s' is built-in, skipping...\n", modName)
			installedMods[modName] = true
			continue
		}

		fmt.Printf("Installing mod '%s'...\n", modName)

		// Try to install the mod using the optimized registry
		if err := mm.installModFromRegistry(ctx, inst, modName); err != nil {
			fmt.Printf("  → Registry installation failed for '%s': %v\n", modName, err)
			// If installation failed, try to resolve from portal as fallback
			if portalErr := mm.installModFromPortal(ctx, inst, modName); portalErr != nil {
				fmt.Printf("  → Portal fallback also failed for '%s': %v\n", modName, portalErr)
				errors = append(errors, fmt.Errorf("failed to install mod '%s': %w (portal fallback: %v)", modName, err, portalErr))
				continue
			} else {
				fmt.Printf("  → Portal fallback succeeded for '%s'\n", modName)
			}
		}

		// Mark as installed
		installedMods[modName] = true

		// Get dependencies for this mod
		dependencies, err := mm.getModDependencies(ctx, inst, modName)
		if err != nil {
			fmt.Printf("Warning: Could not get dependencies for '%s': %v\n", modName, err)
			continue
		}

		// Add dependencies to queue (avoid circular dependencies)
		for _, dep := range dependencies {
			if !installedMods[dep] && dep != modName {
				fmt.Printf("  → Queueing dependency: %s\n", dep)
				installQueue = append(installQueue, dep)
			} else if dep == modName {
				fmt.Printf("  → Skipping circular dependency: %s\n", dep)
			}
		}
	}

	// Convert map to slice
	var result []string
	for modName := range installedMods {
		result = append(result, modName)
	}

	// Return errors if any
	if len(errors) > 0 {
		fmt.Printf("Warning: Some mods failed to install: ")
		for i, err := range errors {
			if i > 0 {
				fmt.Printf(", ")
			}
			fmt.Printf("%v", err)
		}
		fmt.Printf("\n")
		return result, fmt.Errorf("installation completed with %d errors", len(errors))
	}

	return result, nil
}

// getModDependencies extracts dependencies from a mod's info.json
func (mm *ModManager) getModDependencies(ctx context.Context, inst *Instance, modName string) ([]string, error) {
	// Check if we have mod info cached
	mm.mu.Lock()
	modInfo, exists := mm.modInfos[modName]
	mm.mu.Unlock()

	if !exists {
		// Try to find the mod file and extract info
		modDir := filepath.Join(inst.Dir, "mods")
		pattern := filepath.Join(modDir, fmt.Sprintf("%s_*.zip", modName))
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			return nil, fmt.Errorf("mod file not found: %s", modName)
		}

		// Read and parse the mod file
		modData, err := os.ReadFile(matches[0])
		if err != nil {
			return nil, fmt.Errorf("reading mod file: %w", err)
		}

		modInfo, err = mm.extractModInfo(bytes.NewReader(modData))
		if err != nil {
			return nil, fmt.Errorf("extracting mod info: %w", err)
		}
	}

	// Extract dependencies from mod info
	// Factorio dependency format:
	// - "mod-name" (hard requirement)
	// - "mod-name >= 1.0" (hard requirement with version)
	// - "~mod-name" (load order dependency - doesn't affect load order)
	// - "~mod-name >= 1.0" (load order dependency with version)
	// - "?mod-name" (optional dependency - skip)
	// - "(?)mod-name" (hidden optional dependency - skip)
	// - "!mod-name" (incompatible dependency - skip)
	var dependencies []string
	for _, dep := range modInfo.Dependencies {
		// Skip base mod, optional dependencies, incompatible dependencies, and hidden optional dependencies
		if strings.HasPrefix(dep, "base") ||
			strings.HasPrefix(dep, "?") ||
			strings.HasPrefix(dep, "!") ||
			strings.HasPrefix(dep, "(?)") {
			if strings.HasPrefix(dep, "?") || strings.HasPrefix(dep, "(?)") {
				fmt.Printf("  → Skipping optional dependency: %s\n", dep)
			} else if strings.HasPrefix(dep, "!") {
				fmt.Printf("  → Skipping incompatible dependency: %s\n", dep)
			} else if strings.HasPrefix(dep, "base") {
				fmt.Printf("  → Skipping base mod: %s\n", dep)
			}
			continue
		}

		// Parse dependency string to extract mod name
		// Dependencies can be in formats like:
		// - "mod-name" (hard requirement)
		// - "mod-name >= 1.0" (hard requirement with version)
		// - "~mod-name" (load order dependency)
		// - "~mod-name >= 1.0" (load order dependency with version)
		// - "mod name with spaces" (mod names can contain spaces)
		// - "mod name with spaces >= 1.0" (version constraints)

		// Find the first comparison operator to split mod name from version
		var modName string
		if strings.Contains(dep, " >= ") {
			parts := strings.SplitN(dep, " >= ", 2)
			modName = strings.TrimSpace(parts[0])
		} else if strings.Contains(dep, " <= ") {
			parts := strings.SplitN(dep, " <= ", 2)
			modName = strings.TrimSpace(parts[0])
		} else if strings.Contains(dep, " = ") {
			parts := strings.SplitN(dep, " = ", 2)
			modName = strings.TrimSpace(parts[0])
		} else if strings.Contains(dep, " > ") {
			parts := strings.SplitN(dep, " > ", 2)
			modName = strings.TrimSpace(parts[0])
		} else if strings.Contains(dep, " < ") {
			parts := strings.SplitN(dep, " < ", 2)
			modName = strings.TrimSpace(parts[0])
		} else {
			// No version constraint, the whole string is the mod name
			modName = strings.TrimSpace(dep)
		}

		if modName == "" {
			continue
		}

		// Handle load order dependencies (prefixed with ~)
		if strings.HasPrefix(modName, "~") {
			modName = strings.TrimSpace(strings.TrimPrefix(modName, "~"))
			fmt.Printf("  → Adding load order dependency: %s\n", modName)
		} else {
			modName = strings.TrimSpace(modName)
			fmt.Printf("  → Adding required dependency: %s\n", modName)
		}

		// Skip empty mod names
		if modName == "" {
			fmt.Printf("  → Skipping empty dependency\n")
			continue
		}

		dependencies = append(dependencies, modName)
	}

	return dependencies, nil
}

// isBuiltinMod checks if a mod is built into Factorio
func isBuiltinMod(modName string) bool {
	builtinMods := []string{"base", "elevated-rails", "quality", "space-age"}
	for _, builtin := range builtinMods {
		if modName == builtin {
			return true
		}
	}
	return false
}

// installModFromPortal installs a mod from the Factorio mod portal as fallback
func (mm *ModManager) installModFromPortal(ctx context.Context, inst *Instance, modName string) error {
	fmt.Printf("  → Trying mod portal fallback for '%s'...\n", modName)

	// Create temporary buffer for mod content
	var buf bytes.Buffer

	// Download from portal
	if err := mm.downloadFromPortal(ctx, inst, modName, &buf); err != nil {
		return fmt.Errorf("portal download failed: %w", err)
	}

	// Extract mod info
	modInfo, err := mm.extractModInfo(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("extracting mod info: %w", err)
	}

	// Check version compatibility
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

	// Write mod file
	modDir := filepath.Join(inst.Dir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		return fmt.Errorf("creating mod directory: %w", err)
	}
	modPath := filepath.Join(modDir, fmt.Sprintf("%s_%s.zip", modInfo.Name, modInfo.Version))
	if err := os.WriteFile(modPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing mod file: %w", err)
	}

	// Update mod-list.json
	if err := mm.updateModList(inst, modInfo.Name, true); err != nil {
		return fmt.Errorf("updating mod list: %w", err)
	}

	fmt.Printf("  → Successfully installed '%s' from portal (version %s)\n", modInfo.Name, modInfo.Version)
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

// installModFromDirectSource installs a mod from a direct source specification
func (mm *ModManager) installModFromDirectSource(ctx context.Context, inst *Instance, modSpec string) error {
	// Create temporary buffer for mod content
	var buf bytes.Buffer

	// Download mod using resolver
	if err := mm.downloadMod(ctx, modSpec, &buf); err != nil {
		return fmt.Errorf("downloading mod: %w", err)
	}

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
	modDir := filepath.Join(inst.Dir, "mods")
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

// installModFromRegistry searches the pre-built registry for a mod and installs it
func (mm *ModManager) installModFromRegistry(ctx context.Context, inst *Instance, modName string) error {
	fmt.Printf("  → Searching for mod '%s' in registry...\n", modName)

	// Check if mod exists in the pre-built registry
	mm.mu.RLock()
	modSources, exists := mm.sourceRegistry[modName]
	mm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("mod '%s' not found in registry", modName)
	}

	// Try each source that has this mod
	for sourceName, modData := range modSources {
		fmt.Printf("  → Found mod '%s' in source '%s'\n", modName, sourceName)

		// Extract mod info from the found mod
		modInfo, err := mm.extractModInfo(bytes.NewReader(modData))
		if err != nil {
			fmt.Printf("  → Warning: Failed to extract mod info from source '%s': %v\n", sourceName, err)
			continue
		}

		// Check Factorio version compatibility
		if modInfo.FactorioVersion != "" {
			if !isVersionCompatible(inst.Config.Version, modInfo.FactorioVersion) {
				fmt.Printf("  → Warning: Mod '%s' requires Factorio %s but instance uses %s, trying next source...\n",
					modName, modInfo.FactorioVersion, inst.Config.Version)
				continue
			}
		}

		// Cache mod info
		mm.mu.Lock()
		mm.modInfos[modInfo.Name] = modInfo
		mm.mu.Unlock()

		// Write mod file
		modDir := filepath.Join(inst.Dir, "mods")
		modPath := filepath.Join(modDir, fmt.Sprintf("%s_%s.zip", modInfo.Name, modInfo.Version))
		if err := os.WriteFile(modPath, modData, 0644); err != nil {
			return fmt.Errorf("writing mod file: %w", err)
		}

		// Update mod-list.json
		if err := mm.updateModList(inst, modInfo.Name, true); err != nil {
			return fmt.Errorf("updating mod list: %w", err)
		}

		return nil
	}

	return fmt.Errorf("mod '%s' not found in any compatible source", modName)
}

// installModFromSource downloads a source repository and finds a specific mod
func (mm *ModManager) installModFromSource(ctx context.Context, inst *Instance, sourceURL, modName string) error {
	// Download the entire source repository
	var repoBuf bytes.Buffer
	if err := mm.downloadMod(ctx, sourceURL, &repoBuf); err != nil {
		return fmt.Errorf("downloading source repository: %w", err)
	}

	// Search through the repository to find the mod with the correct name
	modData, err := mm.findModInRepository(repoBuf.Bytes(), modName)
	if err != nil {
		return fmt.Errorf("finding mod in repository: %w", err)
	}

	// Extract mod info from the found mod
	modInfo, err := mm.extractModInfo(bytes.NewReader(modData))
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

	// Write mod file
	modDir := filepath.Join(inst.Dir, "mods")
	modPath := filepath.Join(modDir, fmt.Sprintf("%s_%s.zip", modInfo.Name, modInfo.Version))
	if err := os.WriteFile(modPath, modData, 0644); err != nil {
		return fmt.Errorf("writing mod file: %w", err)
	}

	// Update mod-list.json
	if err := mm.updateModList(inst, modInfo.Name, true); err != nil {
		return fmt.Errorf("updating mod list: %w", err)
	}

	return nil
}

// findModInRepository searches through a repository zip to find a mod with the specified name
func (mm *ModManager) findModInRepository(repoData []byte, modName string) ([]byte, error) {
	// Read the repository zip
	zipReader, err := zip.NewReader(bytes.NewReader(repoData), int64(len(repoData)))
	if err != nil {
		return nil, fmt.Errorf("reading repository zip: %w", err)
	}

	// Search through all folders in the repository
	fmt.Printf("  → Searching for mod '%s' in repository...\n", modName)
	foundMods := make(map[string]string) // mod name -> folder path

	for _, file := range zipReader.File {
		// Look for info.json files
		if filepath.Base(file.Name) == "info.json" {
			// Open and read the info.json
			rc, err := file.Open()
			if err != nil {
				continue
			}

			var info ModInfo
			if err := json.NewDecoder(rc).Decode(&info); err != nil {
				rc.Close()
				continue
			}
			rc.Close()

			// Store found mods for debugging
			folderPath := filepath.Dir(file.Name)
			foundMods[info.Name] = folderPath
			fmt.Printf("    - Found mod '%s' in folder '%s'\n", info.Name, folderPath)

			// Check if this is the mod we're looking for
			if info.Name == modName {
				fmt.Printf("  → Found target mod '%s' in repository\n", modName)

				// Extract the entire folder containing this mod
				return mm.extractModFolderFromZip(repoData, folderPath)
			}
		}
	}

	// If we get here, the mod wasn't found
	fmt.Printf("  → Available mods in repository:\n")
	for modName, folderPath := range foundMods {
		fmt.Printf("    - %s (in %s)\n", modName, folderPath)
	}

	return nil, fmt.Errorf("mod '%s' not found in repository", modName)
}

// extractAllModsFromRepository extracts all mods from a repository zip and returns a map of modName -> modData
func (mm *ModManager) extractAllModsFromRepository(repoData []byte) (map[string][]byte, error) {
	// Read the repository zip
	zipReader, err := zip.NewReader(bytes.NewReader(repoData), int64(len(repoData)))
	if err != nil {
		return nil, fmt.Errorf("reading repository zip: %w", err)
	}

	// Track all mods found in the repository
	mods := make(map[string][]byte)
	modFolders := make(map[string]string) // mod name -> folder path

	// First pass: find all mods and their folder paths
	for _, file := range zipReader.File {
		// Look for info.json files
		if filepath.Base(file.Name) == "info.json" {
			// Open and read the info.json
			rc, err := file.Open()
			if err != nil {
				continue
			}

			var info ModInfo
			if err := json.NewDecoder(rc).Decode(&info); err != nil {
				rc.Close()
				continue
			}
			rc.Close()

			// Store the mod folder path
			folderPath := filepath.Dir(file.Name)
			modFolders[info.Name] = folderPath
		}
	}

	// Second pass: extract each mod
	for modName, folderPath := range modFolders {
		modData, err := mm.extractModFolderFromZip(repoData, folderPath)
		if err != nil {
			fmt.Printf("  → Warning: Failed to extract mod '%s': %v\n", modName, err)
			continue
		}
		mods[modName] = modData
	}

	return mods, nil
}

// extractModFolderFromZip extracts a specific folder from a zip file
func (mm *ModManager) extractModFolderFromZip(zipData []byte, folderPath string) ([]byte, error) {
	// Create a new zip writer for the extracted folder
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Read the original zip
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("reading zip: %w", err)
	}

	// Find files in the folder and copy them to the new zip
	folderPrefix := folderPath + "/"
	found := false

	// Extract the mod name from the folder path for the zip structure
	modName := filepath.Base(folderPath)

	for _, file := range zipReader.File {
		// Check if this file is in the folder
		if strings.HasPrefix(file.Name, folderPrefix) {
			found = true

			// Calculate the relative path within the folder
			relativePath := strings.TrimPrefix(file.Name, folderPrefix)
			if relativePath == "" {
				continue // Skip the folder directory itself
			}

			// Create the file in the new zip with the proper folder structure
			// Factorio expects files to be in a folder named after the mod
			fullPath := modName + "/" + relativePath

			// Create the file in the new zip with original metadata
			zipFile, err := zipWriter.CreateHeader(&zip.FileHeader{
				Name:               fullPath,
				Method:             file.Method,
				CompressedSize64:   file.CompressedSize64,
				UncompressedSize64: file.UncompressedSize64,
				Modified:           file.Modified,
			})
			if err != nil {
				return nil, fmt.Errorf("creating file in zip: %w", err)
			}

			// Copy file contents
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("opening file: %w", err)
			}

			_, err = io.Copy(zipFile, rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("copying file: %w", err)
			}

			// Debug: print info.json content if it's the info.json file
			if relativePath == "info.json" {
				fmt.Printf("  → Debug: Found info.json in extracted mod (path: %s)\n", fullPath)
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("folder '%s' not found in repository", folderPath)
	}

	// Close the zip writer before returning
	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("closing zip writer: %w", err)
	}

	fmt.Printf("  → Extracted mod folder '%s' successfully\n", folderPath)
	return buf.Bytes(), nil
}

// getCachePath returns the path for a cached file based on URL hash
func (mm *ModManager) getCachePath(url string) string {
	hash := sha256.Sum256([]byte(url))
	hashStr := hex.EncodeToString(hash[:])
	return filepath.Join(mm.cacheDir, hashStr+".zip")
}

// loadCacheRegistry loads the cache registry from disk
func (mm *ModManager) loadCacheRegistry() (map[string]*CacheEntry, error) {
	registryPath := filepath.Join(mm.cacheDir, "registry.json")

	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		return make(map[string]*CacheEntry), nil
	}

	data, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("reading cache registry: %w", err)
	}

	var registry map[string]*CacheEntry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parsing cache registry: %w", err)
	}

	return registry, nil
}

// saveCacheRegistry saves the cache registry to disk
func (mm *ModManager) saveCacheRegistry(registry map[string]*CacheEntry) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(mm.cacheDir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	registryPath := filepath.Join(mm.cacheDir, "registry.json")
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache registry: %w", err)
	}

	return os.WriteFile(registryPath, data, 0644)
}

// getCachedDownload checks if a download is cached and returns the cached file
func (mm *ModManager) getCachedDownload(url string) (*CacheEntry, bool) {
	registry, err := mm.loadCacheRegistry()
	if err != nil {
		return nil, false
	}

	entry, exists := registry[url]
	if !exists {
		return nil, false
	}

	// Check if cached file still exists
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		// File was deleted, remove from registry
		delete(registry, url)
		mm.saveCacheRegistry(registry)
		return nil, false
	}

	return entry, true
}

// cacheDownload stores a downloaded file in the cache
func (mm *ModManager) cacheDownload(url string, data []byte) (*CacheEntry, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(mm.cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	// Calculate hash of the data
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Get cache path
	cachePath := mm.getCachePath(url)

	// Write cached file
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return nil, fmt.Errorf("writing cached file: %w", err)
	}

	// Create cache entry
	entry := &CacheEntry{
		URL:      url,
		Hash:     hashStr,
		FilePath: cachePath,
		Size:     int64(len(data)),
		CachedAt: time.Now(),
	}

	// Update registry
	registry, err := mm.loadCacheRegistry()
	if err != nil {
		return nil, fmt.Errorf("loading cache registry: %w", err)
	}

	registry[url] = entry
	if err := mm.saveCacheRegistry(registry); err != nil {
		return nil, fmt.Errorf("saving cache registry: %w", err)
	}

	return entry, nil
}

// getLatestCommitSHA gets the latest commit SHA from GitHub API
func (mm *ModManager) getLatestCommitSHA(ctx context.Context, repoPath string) (string, error) {
	// Use GitHub API to get the latest commit
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/commits", repoPath)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	// Add headers for better API compatibility
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "factctl/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching commit info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	var commits []struct {
		SHA string `json:"sha"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return "", fmt.Errorf("parsing commit response: %w", err)
	}

	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found for repository: %s", repoPath)
	}

	return commits[0].SHA, nil
}

// getCommitSHAForBranch gets the commit SHA for a specific branch or tag
func (mm *ModManager) getCommitSHAForBranch(ctx context.Context, repoPath, branch string) (string, error) {
	// Try to get the branch/tag reference from GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/git/refs/heads/%s", repoPath, branch)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "factctl/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching branch info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// Found as a branch
		var ref struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
			return "", fmt.Errorf("parsing branch response: %w", err)
		}

		return ref.Object.SHA, nil
	}

	// If not found as a branch, try as a tag
	apiURL = fmt.Sprintf("https://api.github.com/repos/%s/git/refs/tags/%s", repoPath, branch)

	req, err = http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating tag request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "factctl/1.0")

	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching tag info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// Found as a tag
		var ref struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
			return "", fmt.Errorf("parsing tag response: %w", err)
		}

		return ref.Object.SHA, nil
	}

	return "", fmt.Errorf("branch or tag '%s' not found in repository %s", branch, repoPath)
}

// extractSubfolderFromZip extracts a specific subfolder from a zip file
func (mm *ModManager) extractSubfolderFromZip(zipData []byte, subfolder string, buf *bytes.Buffer) error {
	// Create a new zip writer for the extracted subfolder
	zipWriter := zip.NewWriter(buf)
	defer zipWriter.Close()

	// Read the original zip
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("reading zip: %w", err)
	}

	// Find files in the subfolder and copy them to the new zip
	subfolderPrefix := subfolder + "/"
	found := false

	// Debug: list available folders
	fmt.Printf("  → Available folders in repository:\n")
	seenFolders := make(map[string]bool)
	for _, file := range zipReader.File {
		parts := strings.Split(file.Name, "/")
		if len(parts) > 1 && parts[0] != "" {
			folder := parts[0]
			if !seenFolders[folder] {
				fmt.Printf("    - %s\n", folder)
				seenFolders[folder] = true
			}
		}
	}

	for _, file := range zipReader.File {
		// Check if this file is in the subfolder
		if strings.HasPrefix(file.Name, subfolderPrefix) {
			found = true

			// Calculate the relative path within the subfolder
			relativePath := strings.TrimPrefix(file.Name, subfolderPrefix)
			if relativePath == "" {
				continue // Skip the subfolder directory itself
			}

			// Create the file in the new zip
			zipFile, err := zipWriter.Create(relativePath)
			if err != nil {
				return fmt.Errorf("creating file in zip: %w", err)
			}

			// Copy file contents
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("opening file: %w", err)
			}

			_, err = io.Copy(zipFile, rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("copying file: %w", err)
			}
		}
	}

	if !found {
		return fmt.Errorf("subfolder '%s' not found in repository", subfolder)
	}

	fmt.Printf("  → Extracted subfolder '%s' successfully\n", subfolder)
	return nil
}

// downloadMod downloads a mod from the specified source
func (mm *ModManager) downloadMod(ctx context.Context, source string, buf *bytes.Buffer) error {
	// Parse the source specification
	// Format: "portal:modname" or "github:user/repo" or "git:url" etc.
	parts := strings.SplitN(source, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid mod source format: %s", source)
	}

	sourceType := parts[0]
	sourcePath := parts[1]

	switch sourceType {
	case "portal":
		// For portal downloads, we need an instance for version compatibility
		// This is a limitation of the current design - portal downloads need instance context
		return fmt.Errorf("portal downloads require instance context - use installModFromPortal instead")
	case "github":
		return mm.downloadFromGitHub(ctx, sourcePath, buf)
	case "ghpr":
		return mm.downloadFromGitHubPR(ctx, sourcePath, buf)
	case "git":
		return mm.downloadFromGit(ctx, sourcePath, buf)
	default:
		return fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// downloadFromPortal downloads a mod from the Factorio mod portal
func (mm *ModManager) downloadFromPortal(ctx context.Context, inst *Instance, modName string, buf *bytes.Buffer) error {
	fmt.Printf("  → Searching mod portal for '%s'...\n", modName)

	// Search for the mod using the API
	searchURL := fmt.Sprintf("https://mods.factorio.com/api/mods/%s", modName)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("searching mod portal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("mod not found on portal: %s", modName)
	}

	var modInfo struct {
		Name     string `json:"name"`
		Releases []struct {
			DownloadURL string `json:"download_url"`
			Version     string `json:"version"`
			InfoJSON    struct {
				FactorioVersion string `json:"factorio_version"`
			} `json:"info_json"`
		} `json:"releases"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modInfo); err != nil {
		return fmt.Errorf("parsing mod info: %w", err)
	}

	if len(modInfo.Releases) == 0 {
		return fmt.Errorf("no releases found for mod: %s", modName)
	}

	// Find the best release for the current Factorio version
	var bestRelease *struct {
		DownloadURL string `json:"download_url"`
		Version     string `json:"version"`
		InfoJSON    struct {
			FactorioVersion string `json:"factorio_version"`
		} `json:"info_json"`
	}

	// Look for a release compatible with the current Factorio version
	for i := len(modInfo.Releases) - 1; i >= 0; i-- {
		release := &modInfo.Releases[i]
		if isVersionCompatible(inst.Config.Version, release.InfoJSON.FactorioVersion) {
			bestRelease = release
			break
		}
	}

	// If no compatible release found, use the latest
	if bestRelease == nil {
		bestRelease = &modInfo.Releases[len(modInfo.Releases)-1]
		fmt.Printf("  → Warning: No release found for Factorio %s, using latest version\n", inst.Config.Version)
	}

	fmt.Printf("  → Found mod '%s' version %s on portal\n", modName, bestRelease.Version)

	// Check for required authentication
	creds, err := mm.getPortalCredentials()
	if err != nil || creds.FactorioUsername == "" || creds.FactorioToken == "" {
		return fmt.Errorf("portal credentials required but not found\nHint: Run 'factctl auth' to authenticate with your Factorio account and set up portal access")
	}

	// Download the mod file with authentication as query parameters
	downloadURL := fmt.Sprintf("https://mods.factorio.com%s?username=%s&token=%s",
		bestRelease.DownloadURL,
		url.QueryEscape(creds.FactorioUsername),
		url.QueryEscape(creds.FactorioToken))

	// Obfuscate credentials in the logged URL
	obfuscatedURL := fmt.Sprintf("https://mods.factorio.com%s?username=***&token=***",
		bestRelease.DownloadURL)
	fmt.Printf("  → Downloading mod '%s' from %s...\n", modName, obfuscatedURL)

	downloadReq, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}

	fmt.Printf("  → Using authentication for download\n")

	downloadResp, err := client.Do(downloadReq)
	if err != nil {
		return fmt.Errorf("downloading mod: %w", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != 200 {
		// This will likely be 302 redirect to login page
		return fmt.Errorf("authentication required for portal downloads (need username/token)")
	}

	// Copy the actual mod file with progress reporting
	if err := mm.copyWithProgress(downloadResp.Body, buf, modName); err != nil {
		return fmt.Errorf("copying mod file: %w", err)
	}

	// Debug: Check if we got a valid ZIP file
	if buf.Len() == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	// Check if it's a valid ZIP by looking at the first few bytes
	firstBytesLen := 4
	if buf.Len() < firstBytesLen {
		firstBytesLen = buf.Len()
	}
	firstBytes := buf.Bytes()[:firstBytesLen]
	if len(firstBytes) >= 4 && firstBytes[0] == 0x50 && firstBytes[1] == 0x4B {
		fmt.Printf("  → Downloaded mod '%s' successfully (%d bytes)\n", modName, buf.Len())
	} else {
		fmt.Printf("  → Warning: Downloaded file doesn't appear to be a ZIP file (first bytes: %v)\n", firstBytes)
		return fmt.Errorf("downloaded file is not a valid ZIP file")
	}

	return nil
}

// getPortalCredentials retrieves stored portal credentials
func (mm *ModManager) getPortalCredentials() (*auth.Credentials, error) {
	// Try to get config directory from base directory
	configDir := filepath.Join(mm.baseDir, "config")
	store := auth.NewStore(configDir)

	creds, err := store.Load()
	if err != nil {
		// Try default location as fallback
		if defaultPath, err := auth.DefaultLocation(); err == nil {
			defaultStore := auth.NewStore(filepath.Dir(defaultPath))
			creds, err = defaultStore.Load()
		}
	}

	return creds, err
}

// downloadFromGitHub downloads a mod from GitHub
func (mm *ModManager) downloadFromGitHub(ctx context.Context, repoPath string, buf *bytes.Buffer) error {
	fmt.Printf("  → Downloading GitHub repository '%s'...\n", repoPath)

	// Parse repository path and branch/tag specification
	// Format: "user/repo@branch" or "user/repo/subfolder@branch"
	var baseRepo, subfolder, branch string

	// Check for branch/tag specification
	if strings.Contains(repoPath, "@") {
		parts := strings.Split(repoPath, "@")
		if len(parts) != 2 {
			return fmt.Errorf("invalid repository format: %s (expected user/repo@branch)", repoPath)
		}
		repoPath = parts[0]
		branch = parts[1]
	}

	// Check if this is a subfolder path (e.g., "user/repo/subfolder")
	parts := strings.Split(repoPath, "/")
	if len(parts) >= 3 {
		// This is a subfolder path
		baseRepo = strings.Join(parts[:2], "/")
		subfolder = parts[2]
		fmt.Printf("  → Detected subfolder '%s' in repository '%s'\n", subfolder, baseRepo)
	} else {
		// This is a regular repository
		baseRepo = repoPath
	}

	// Get the commit SHA for the specified branch/tag or latest
	var commitSHA string
	var err error
	if branch != "" {
		commitSHA, err = mm.getCommitSHAForBranch(ctx, baseRepo, branch)
		if err != nil {
			return fmt.Errorf("getting commit for branch '%s': %w", branch, err)
		}
		fmt.Printf("  → Using branch '%s' (commit %s)\n", branch, commitSHA[:8])
	} else {
		commitSHA, err = mm.getLatestCommitSHA(ctx, baseRepo)
		if err != nil {
			return fmt.Errorf("getting latest commit: %w", err)
		}
		fmt.Printf("  → Using latest commit %s\n", commitSHA[:8])
	}

	// Use commit SHA as cache key
	commitURL := fmt.Sprintf("https://github.com/%s/archive/%s.zip", baseRepo, commitSHA)

	// Check cache first for the base repository
	baseCacheKey := fmt.Sprintf("github:%s:%s", baseRepo, commitSHA)
	fmt.Printf("  → Checking cache for commit %s...\n", commitSHA[:8])
	if entry, cached := mm.getCachedDownload(baseCacheKey); cached {
		fmt.Printf("  → Using cached repository (%.1f MB)\n", float64(entry.Size)/(1024*1024))
		cachedData, err := os.ReadFile(entry.FilePath)
		if err != nil {
			return fmt.Errorf("reading cached file: %w", err)
		}

		// If this is a subfolder, extract only that subfolder
		if subfolder != "" {
			return mm.extractSubfolderFromZip(cachedData, subfolder, buf)
		}

		buf.Write(cachedData)
		return nil
	}

	fmt.Printf("  → No cache found, downloading...\n")

	// Download using commit SHA
	req, err := http.NewRequestWithContext(ctx, "GET", commitURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("repository not found or inaccessible: %s", repoPath)
	}

	// Copy the zip file directly with progress reporting
	if err := mm.copyWithProgress(resp.Body, buf, baseRepo); err != nil {
		return fmt.Errorf("copying zip file: %w", err)
	}

	// Cache the base repository download
	if _, err := mm.cacheDownload(baseCacheKey, buf.Bytes()); err != nil {
		fmt.Printf("  → Warning: Failed to cache download: %v\n", err)
	}

	// If this is a subfolder, extract only that subfolder
	if subfolder != "" {
		return mm.extractSubfolderFromZip(buf.Bytes(), subfolder, buf)
	}

	fmt.Printf("  → Downloaded mod from '%s' successfully\n", repoPath)

	return nil
}

// downloadFromGitHubPR downloads a mod from a GitHub pull request
func (mm *ModManager) downloadFromGitHubPR(ctx context.Context, prSpec string, buf *bytes.Buffer) error {
	fmt.Printf("  → Downloading GitHub PR '%s'...\n", prSpec)

	// Parse the PR specification: "owner/repo#123"
	parts := strings.Split(prSpec, "#")
	if len(parts) != 2 {
		return fmt.Errorf("invalid PR specification format: %s (expected owner/repo#123)", prSpec)
	}

	repoPath := parts[0]
	prNumber := parts[1]

	// Parse owner/repo
	repoParts := strings.Split(repoPath, "/")
	if len(repoParts) != 2 {
		return fmt.Errorf("invalid repository format: %s (expected owner/repo)", repoPath)
	}

	// Check if this is a subfolder path (e.g., "user/repo/subfolder#123")
	var baseRepo, subfolder string
	if len(repoParts) >= 3 {
		// This is a subfolder path
		baseRepo = strings.Join(repoParts[:2], "/")
		subfolder = repoParts[2]
		fmt.Printf("  → Detected subfolder '%s' in repository '%s'\n", subfolder, baseRepo)
	} else {
		// This is a regular repository
		baseRepo = repoPath
	}

	// Get the PR head SHA from GitHub API
	commitSHA, err := mm.getPRHeadSHA(ctx, baseRepo, prNumber)
	if err != nil {
		return fmt.Errorf("getting PR head SHA: %w", err)
	}

	// Use commit SHA as cache key
	commitURL := fmt.Sprintf("https://github.com/%s/archive/%s.zip", baseRepo, commitSHA)

	// Check cache first for the base repository
	baseCacheKey := fmt.Sprintf("githubpr:%s:%s:%s", baseRepo, prNumber, commitSHA)
	fmt.Printf("  → Checking cache for PR #%s (commit %s)...\n", prNumber, commitSHA[:8])
	if entry, cached := mm.getCachedDownload(baseCacheKey); cached {
		fmt.Printf("  → Using cached PR (%.1f MB)\n", float64(entry.Size)/(1024*1024))
		cachedData, err := os.ReadFile(entry.FilePath)
		if err != nil {
			return fmt.Errorf("reading cached file: %w", err)
		}

		// If this is a subfolder, extract only that subfolder
		if subfolder != "" {
			return mm.extractSubfolderFromZip(cachedData, subfolder, buf)
		}

		buf.Write(cachedData)
		return nil
	}

	fmt.Printf("  → No cache found, downloading PR...\n")

	// Download using commit SHA
	req, err := http.NewRequestWithContext(ctx, "GET", commitURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("PR not found or inaccessible: %s", prSpec)
	}

	// Copy the zip file directly with progress reporting
	if err := mm.copyWithProgress(resp.Body, buf, fmt.Sprintf("PR #%s", prNumber)); err != nil {
		return fmt.Errorf("copying zip file: %w", err)
	}

	// Cache the base repository download
	if _, err := mm.cacheDownload(baseCacheKey, buf.Bytes()); err != nil {
		fmt.Printf("  → Warning: Failed to cache download: %v\n", err)
	}

	// If this is a subfolder, extract only that subfolder
	if subfolder != "" {
		return mm.extractSubfolderFromZip(buf.Bytes(), subfolder, buf)
	}

	fmt.Printf("  → Downloaded PR #%s from '%s' successfully\n", prNumber, repoPath)

	return nil
}

// getPRHeadSHA gets the head commit SHA for a GitHub PR
func (mm *ModManager) getPRHeadSHA(ctx context.Context, repoPath, prNumber string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls/%s", repoPath, prNumber)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "factctl")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned status %d for PR #%s", resp.StatusCode, prNumber)
	}

	var prData struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&prData); err != nil {
		return "", fmt.Errorf("parsing PR response: %w", err)
	}

	if prData.Head.SHA == "" {
		return "", fmt.Errorf("PR #%s has no head commit", prNumber)
	}

	return prData.Head.SHA, nil
}

// downloadFromGit downloads a mod from a Git repository
func (mm *ModManager) downloadFromGit(ctx context.Context, gitURL string, buf *bytes.Buffer) error {
	fmt.Printf("  → Downloading Git repository '%s'...\n", gitURL)

	// Try to convert common Git hosting URLs to zip download URLs
	zipURL := mm.convertGitURLToZip(gitURL)
	if zipURL == "" {
		return fmt.Errorf("unsupported Git hosting service: %s", gitURL)
	}

	// Check cache first
	if entry, cached := mm.getCachedDownload(zipURL); cached {
		fmt.Printf("  → Using cached download (%.1f MB)\n", float64(entry.Size)/(1024*1024))
		cachedData, err := os.ReadFile(entry.FilePath)
		if err != nil {
			return fmt.Errorf("reading cached file: %w", err)
		}
		buf.Write(cachedData)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", zipURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading from Git: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("repository not found or inaccessible: %s", gitURL)
	}

	// Copy the zip file directly with progress reporting
	if err := mm.copyWithProgress(resp.Body, buf, gitURL); err != nil {
		return fmt.Errorf("copying zip file: %w", err)
	}

	// Cache the download
	if _, err := mm.cacheDownload(zipURL, buf.Bytes()); err != nil {
		fmt.Printf("  → Warning: Failed to cache download: %v\n", err)
	}

	fmt.Printf("  → Downloaded mod from '%s' successfully\n", gitURL)

	return nil
}

// convertGitURLToZip converts Git URLs to zip download URLs for common hosting services
func (mm *ModManager) convertGitURLToZip(gitURL string) string {
	// GitHub: https://github.com/user/repo -> https://github.com/user/repo/archive/refs/heads/main.zip
	if strings.HasPrefix(gitURL, "https://github.com/") {
		// Remove .git suffix if present
		repoPath := strings.TrimSuffix(gitURL, ".git")
		return fmt.Sprintf("%s/archive/refs/heads/main.zip", repoPath)
	}

	// GitLab: https://gitlab.com/user/repo -> https://gitlab.com/user/repo/-/archive/main/repo-main.zip
	if strings.HasPrefix(gitURL, "https://gitlab.com/") {
		// Remove .git suffix if present
		repoPath := strings.TrimSuffix(gitURL, ".git")
		parts := strings.Split(strings.TrimPrefix(repoPath, "https://gitlab.com/"), "/")
		if len(parts) >= 2 {
			repoName := parts[len(parts)-1]
			return fmt.Sprintf("%s/-/archive/main/%s-main.zip", repoPath, repoName)
		}
	}

	// Bitbucket: https://bitbucket.org/user/repo -> https://bitbucket.org/user/repo/get/main.zip
	if strings.HasPrefix(gitURL, "https://bitbucket.org/") {
		// Remove .git suffix if present
		repoPath := strings.TrimSuffix(gitURL, ".git")
		return fmt.Sprintf("%s/get/main.zip", repoPath)
	}

	// For other Git hosting services, return empty string (unsupported)
	return ""
}

// copyWithProgress copies data with progress reporting for large downloads
func (mm *ModManager) copyWithProgress(src io.Reader, dst io.Writer, name string) error {
	// Create a buffer for reading in chunks
	buf := make([]byte, 32*1024) // 32KB chunks
	totalBytes := int64(0)
	lastReport := time.Now()

	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			totalBytes += int64(n)

			// Report progress every 2 seconds for large downloads
			if time.Since(lastReport) >= 2*time.Second {
				mb := float64(totalBytes) / (1024 * 1024)
				fmt.Printf("  → Downloaded %.1f MB for '%s'...\n", mb, name)
				lastReport = time.Now()
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	// Final progress report
	mb := float64(totalBytes) / (1024 * 1024)
	if mb > 0.1 { // Only report if more than 100KB
		fmt.Printf("  → Downloaded %.1f MB for '%s'\n", mb, name)
	}

	return nil
}

// isVersionCompatible checks if a mod version is compatible with Factorio version
func isVersionCompatible(factorioVersion, modVersion string) bool {
	// Handle Factorio's truncated version format (e.g., "1.1" = "^1.1.0.0")
	// Convert truncated versions to proper semver format
	factorioVersion = normalizeFactorioVersion(factorioVersion)
	modVersion = normalizeFactorioVersion(modVersion)

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

// normalizeFactorioVersion converts Factorio version format to semver
func normalizeFactorioVersion(version string) string {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Split by dots to check format
	parts := strings.Split(version, ".")

	// If only major.minor (e.g., "1.1"), add patch and build
	if len(parts) == 2 {
		return version + ".0"
	}

	// If major.minor.patch (e.g., "1.1.0"), it's already good
	if len(parts) == 3 {
		return version
	}

	// If already has 4 parts, return as is
	return version
}
