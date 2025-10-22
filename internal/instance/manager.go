package instance

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Manager handles instance lifecycle operations
type Manager struct {
	// Base directory for all instances
	baseDir string
	// Base directory for shared Factorio installations
	runtimeDir string
	// Path to Factorio installation (optional, overrides auto-detection)
	factorioPath string
	// Use symlinks instead of copying files (default: false, uses copying)
	useSymlinks bool
}

// NewManager creates a new instance manager
func NewManager(baseDir string) *Manager {
	return &Manager{
		baseDir:    baseDir,
		runtimeDir: filepath.Join(baseDir, "runtimes"),
	}
}

// NewManagerWithFactorio creates a new instance manager with a specific Factorio path
func NewManagerWithFactorio(baseDir, factorioPath string) *Manager {
	return &Manager{
		baseDir:      baseDir,
		runtimeDir:   filepath.Join(baseDir, "runtimes"),
		factorioPath: factorioPath,
		useSymlinks:  false, // Default to copying
	}
}

// SetUseSymlinks configures whether to use symlinks or copy files for the overlay
func (m *Manager) SetUseSymlinks(useSymlinks bool) {
	m.useSymlinks = useSymlinks
}

// BaseDir returns the base directory for instances
func (m *Manager) BaseDir() string {
	return m.baseDir
}

// DefaultLocation returns the default base directory for instances
func DefaultLocation() (string, error) {
	// Use platform-specific default locations
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		return filepath.Join(appData, "factctl"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "factctl"), nil
	default: // Linux and others
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(configDir, "factctl"), nil
	}
}

// InstanceState represents the current state of an instance
type InstanceState string

const (
	StateUnknown  InstanceState = "unknown"
	StateStarting InstanceState = "starting"
	StateRunning  InstanceState = "running"
	StateStopped  InstanceState = "stopped"
	StateError    InstanceState = "error"
)

// Instance represents a Factorio instance
type Instance struct {
	Config  *Config
	Dir     string
	State   InstanceState
	BaseDir string // Path to base Factorio installation
}

// Create creates a new instance with the given configuration
func (m *Manager) Create(cfg *Config) (*Instance, error) {
	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create instance directory
	instDir := filepath.Join(m.baseDir, "instances", cfg.Name)
	if err := os.MkdirAll(instDir, 0755); err != nil {
		return nil, fmt.Errorf("creating instance directory: %w", err)
	}

	// Find base Factorio installation for the specific runtime
	runtimeName := cfg.GetRuntime()
	baseDir, err := m.findBaseFactorioForRuntime(runtimeName)
	if err != nil {
		return nil, fmt.Errorf("finding base Factorio installation for runtime %s: %w", runtimeName, err)
	}

	// Create instance-specific directories (real files)
	instanceDirs := []string{"saves", "mods", "config", "scripts"}
	for _, dir := range instanceDirs {
		if err := os.MkdirAll(filepath.Join(instDir, dir), 0755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Create overlay to base Factorio directories
	if err := m.createOverlay(instDir, baseDir); err != nil {
		return nil, fmt.Errorf("creating overlay: %w", err)
	}

	// Save configuration
	configPath := filepath.Join(instDir, "config", "instance.json")
	if err := cfg.SaveConfig(configPath); err != nil {
		return nil, fmt.Errorf("saving configuration: %w", err)
	}

	// Create mod-list.json
	modList := struct {
		Mods []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"mods"`
	}{
		Mods: []struct {
			Name    string "json:\"name\""
			Enabled bool   "json:\"enabled\""
		}{
			{Name: "base", Enabled: true},
		},
	}

	// Add enabled mods
	for _, mod := range cfg.Mods.Enabled {
		if mod != "base" {
			modList.Mods = append(modList.Mods, struct {
				Name    string "json:\"name\""
				Enabled bool   "json:\"enabled\""
			}{
				Name:    mod,
				Enabled: true,
			})
		}
	}

	modListPath := filepath.Join(instDir, "config", "mod-list.json")
	if err := SaveJSON(modListPath, &modList); err != nil {
		return nil, fmt.Errorf("saving mod list: %w", err)
	}

	// Create server-settings.json if this is a server
	if cfg.Server != nil {
		serverConfig := map[string]interface{}{
			"name":        cfg.Server.Name,
			"description": cfg.Server.Name, // Use name as default description
			"max_players": cfg.Server.MaxPlayers,
			"visibility": map[string]interface{}{
				"public": cfg.Server.Public,
				"lan":    true,
			},
			"username":                  "",
			"password":                  cfg.Server.Password,
			"require_user_verification": cfg.Server.Password != "",
			"admins":                    cfg.Server.Admins,
			"auto_save": map[string]interface{}{
				"enabled":  cfg.Server.AutoSave,
				"interval": cfg.Server.AutoSaveInterval,
				"slots":    5,
			},
		}

		// Add any additional settings
		for k, v := range cfg.Server.Settings {
			serverConfig[k] = v
		}

		serverConfigPath := filepath.Join(instDir, "config", "server-settings.json")
		if err := SaveJSON(serverConfigPath, serverConfig); err != nil {
			return nil, fmt.Errorf("saving server settings: %w", err)
		}
	}

	// Create config-path.cfg in the root directory
	configPathContent := `config-path=__PATH__executable__/../../config

#This value specifies the way the application generates default values for path.read-data and path.write-data
#When set to true, it will use system directories (Users/Name/AppData/Roaming/Factorio on windows), this is set to true
#for the installer versions of Factorio, as people will usually install it in program files, and the application can't write
#to program files by default (without UAC turned off), similar with osx/linux packages.
#When set to false (default value for zip package), it will use application root directory, this is usable to create self-sustainable
#Factorio directory that can be copied anywhere needed (on usb etc), also for people, who don't like to manipulate saves
#in the windows users directory structure (as me, kovarex).
#Note, that once the values in config are generated, this value has no effects (unless you delete config, or the path.read-data/path.write-data values)
use-system-read-write-data-directories=false`
	configPathFile := filepath.Join(instDir, "config-path.cfg")
	if err := os.WriteFile(configPathFile, []byte(configPathContent), 0644); err != nil {
		return nil, fmt.Errorf("creating config-path.cfg: %w", err)
	}

	// Create player-data.json with service credentials
	playerData := map[string]interface{}{
		"service-username": "",
		"service-token":    "",
	}
	playerDataPath := filepath.Join(instDir, "player-data.json")
	if err := SaveJSON(playerDataPath, playerData); err != nil {
		return nil, fmt.Errorf("creating player-data.json: %w", err)
	}

	return &Instance{
		Config:  cfg,
		Dir:     instDir,
		State:   StateStopped,
		BaseDir: baseDir,
	}, nil
}

// UpdatePlayerData updates the player-data.json file with service credentials
func (m *Manager) UpdatePlayerData(inst *Instance, username, token string) error {
	playerData := map[string]interface{}{
		"service-username": username,
		"service-token":    token,
	}
	playerDataPath := filepath.Join(inst.Dir, "player-data.json")
	return SaveJSON(playerDataPath, playerData)
}

// Remove removes an instance and optionally creates a backup
func (m *Manager) Remove(name string, backup bool) error {
	instDir := filepath.Join(m.baseDir, "instances", name)

	// Check if instance exists
	if _, err := os.Stat(instDir); os.IsNotExist(err) {
		return fmt.Errorf("instance %s does not exist", name)
	}

	// Create backup if requested
	if backup {
		if err := m.createBackup(name); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	// Remove instance directory
	if err := os.RemoveAll(instDir); err != nil {
		return fmt.Errorf("removing instance directory: %w", err)
	}

	return nil
}

// findBaseFactorioForRuntime finds the base Factorio installation for a specific runtime
func (m *Manager) findBaseFactorioForRuntime(runtimeName string) (string, error) {
	// If manager has a specific Factorio path, use it (for copying from system)
	if m.factorioPath != "" {
		if _, err := os.Stat(m.factorioPath); err == nil {
			if m.isValidFactorioInstallation(m.factorioPath) {
				// Copy from system installation to runtimes if needed
				return m.ensureRuntimeFromSystem(m.factorioPath, runtimeName)
			}
			return "", fmt.Errorf("specified Factorio path is not a valid installation: %s", m.factorioPath)
		}
		return "", fmt.Errorf("specified Factorio path does not exist: %s", m.factorioPath)
	}

	// Look for the specific runtime in runtimes directory
	runtimePath := filepath.Join(m.runtimeDir, runtimeName)
	if _, err := os.Stat(runtimePath); err == nil {
		if m.isValidFactorioInstallation(runtimePath) {
			return runtimePath, nil
		}
		return "", fmt.Errorf("runtime %s exists but is not a valid Factorio installation", runtimeName)
	}

	return "", fmt.Errorf("runtime %s not found in runtimes directory. Please install Factorio runtime %s in %s or use --factorio-path to copy from system installation", runtimeName, runtimeName, m.runtimeDir)
}

// findBaseFactorio finds the base Factorio installation in the runtimes directory (legacy)
func (m *Manager) findBaseFactorio() (string, error) {
	// If manager has a specific Factorio path, use it (for copying from system)
	if m.factorioPath != "" {
		if _, err := os.Stat(m.factorioPath); err == nil {
			if m.isValidFactorioInstallation(m.factorioPath) {
				// Copy from system installation to runtimes if needed
				return m.ensureRuntimeFromSystem(m.factorioPath, "system")
			}
			return "", fmt.Errorf("specified Factorio path is not a valid installation: %s", m.factorioPath)
		}
		return "", fmt.Errorf("specified Factorio path does not exist: %s", m.factorioPath)
	}

	// Look for Factorio in runtimes directory
	// Check for any version in runtimes directory
	entries, err := os.ReadDir(m.runtimeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("runtimes directory does not exist. Please install Factorio in the runtimes directory or use --factorio-path to copy from system installation")
		}
		return "", fmt.Errorf("reading runtimes directory: %w", err)
	}

	// Find the first valid Factorio installation
	for _, entry := range entries {
		if entry.IsDir() {
			runtimePath := filepath.Join(m.runtimeDir, entry.Name())
			if m.isValidFactorioInstallation(runtimePath) {
				return runtimePath, nil
			}
		}
	}

	return "", fmt.Errorf("no Factorio installation found in runtimes directory. Please install Factorio in %s or use --factorio-path to copy from system installation", m.runtimeDir)
}

// ensureRuntimeFromSystem copies a system Factorio installation to the runtimes directory
func (m *Manager) ensureRuntimeFromSystem(systemPath, runtimeName string) (string, error) {
	// Create runtimes directory if it doesn't exist
	if err := os.MkdirAll(m.runtimeDir, 0755); err != nil {
		return "", fmt.Errorf("creating runtimes directory: %w", err)
	}

	runtimePath := filepath.Join(m.runtimeDir, runtimeName)

	// Check if we already have this copied
	if _, err := os.Stat(runtimePath); err == nil {
		if m.isValidFactorioInstallation(runtimePath) {
			return runtimePath, nil
		}
	}

	fmt.Printf("Copying Factorio from system installation to runtimes directory...\n")

	// Copy the entire Factorio installation
	if err := m.copyDirectory(systemPath, runtimePath); err != nil {
		return "", fmt.Errorf("copying Factorio from system installation: %w", err)
	}

	fmt.Printf("Factorio copied to %s\n", runtimePath)
	return runtimePath, nil
}

// isValidFactorioInstallation checks if a directory contains a valid Factorio installation
func (m *Manager) isValidFactorioInstallation(path string) bool {
	// Check for key Factorio files/directories
	requiredPaths := []string{
		"bin",
		"data",
	}

	for _, reqPath := range requiredPaths {
		if _, err := os.Stat(filepath.Join(path, reqPath)); err != nil {
			return false
		}
	}

	// Check for base directory (can be at root or in data/)
	basePaths := []string{
		filepath.Join(path, "base"),
		filepath.Join(path, "data", "base"),
	}

	baseFound := false
	for _, basePath := range basePaths {
		if _, err := os.Stat(basePath); err == nil {
			baseFound = true
			break
		}
	}

	return baseFound
}

// createOverlay creates either symlinks or copies of base Factorio directories
func (m *Manager) createOverlay(instDir, baseDir string) error {
	if m.useSymlinks {
		return m.createSymlinkOverlay(instDir, baseDir)
	}
	return m.createCopyOverlay(instDir, baseDir)
}

// createSymlinkOverlay creates symlinks to base Factorio directories
func (m *Manager) createSymlinkOverlay(instDir, baseDir string) error {
	// Directories to symlink from base Factorio installation
	baseDirs := []string{
		"bin",      // Factorio executable and libraries
		"data",     // Game data files
		"graphics", // Graphics assets
		"locale",   // Localization files
		"core",     // Core game files
		"base",     // Base game mod
	}

	for _, dir := range baseDirs {
		basePath := filepath.Join(baseDir, dir)
		instancePath := filepath.Join(instDir, dir)

		// Check if base directory exists
		if _, err := os.Stat(basePath); err != nil {
			continue // Skip if base directory doesn't exist
		}

		// Remove existing file/directory if it exists
		if err := os.RemoveAll(instancePath); err != nil {
			return fmt.Errorf("removing existing %s: %w", dir, err)
		}

		// Create symlink to base directory
		if err := os.Symlink(basePath, instancePath); err != nil {
			return fmt.Errorf("creating symlink for %s: %w", dir, err)
		}
	}

	return nil
}

// createCopyOverlay copies base Factorio directories to the instance
func (m *Manager) createCopyOverlay(instDir, baseDir string) error {
	// Directories to copy from base Factorio installation
	baseDirs := []string{
		"bin",      // Factorio executable and libraries
		"data",     // Game data files
		"graphics", // Graphics assets
		"locale",   // Localization files
		"core",     // Core game files
		"base",     // Base game mod
	}

	for _, dir := range baseDirs {
		basePath := filepath.Join(baseDir, dir)
		instancePath := filepath.Join(instDir, dir)

		// Check if base directory exists
		if _, err := os.Stat(basePath); err != nil {
			continue // Skip if base directory doesn't exist
		}

		// Remove existing file/directory if it exists
		if err := os.RemoveAll(instancePath); err != nil {
			return fmt.Errorf("removing existing %s: %w", dir, err)
		}

		// Copy the directory recursively
		if err := m.copyDirectory(basePath, instancePath); err != nil {
			return fmt.Errorf("copying %s: %w", dir, err)
		}
	}

	return nil
}

// copyDirectory recursively copies a directory
func (m *Manager) copyDirectory(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("reading source directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := m.copyDirectory(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying subdirectory %s: %w", entry.Name(), err)
			}
		} else {
			// Copy file
			if err := m.copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// copyFile copies a single file
func (m *Manager) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("copying file content: %w", err)
	}

	// Copy file permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("getting source file info: %w", err)
	}

	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("setting file permissions: %w", err)
	}

	return nil
}

// createBackup creates a backup of an instance
func (m *Manager) createBackup(name string) error {
	instDir := filepath.Join(m.baseDir, "instances", name)
	backupDir := filepath.Join(m.baseDir, "backups")

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("creating backup directory: %w", err)
	}

	// Create timestamp for backup name
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s-%s.tar.gz", name, timestamp)
	backupPath := filepath.Join(backupDir, backupName)

	// Create backup file
	f, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer f.Close()

	// Create gzip writer
	gw := gzip.NewWriter(f)
	defer gw.Close()

	// Create tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk through instance directory and add files to tar
	err = filepath.Walk(instDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if the path is the instance directory itself
		if path == instDir {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(instDir, path)
		if err != nil {
			return fmt.Errorf("getting relative path: %w", err)
		}

		log.Printf("DEBUG: Adding to backup: %s (relative: %s)", path, relPath)

		// Create tar header with link target for symlinks
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("creating tar header: %w", err)
		}
		header.Name = filepath.ToSlash(relPath) // Convert to forward slashes for consistency

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("writing tar header: %w", err)
		}

		// If it's a regular file, write content
		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading file %s: %w", path, err)
			}
			if _, err := tw.Write(data); err != nil {
				return fmt.Errorf("writing file content: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("creating backup archive: %w", err)
	}

	return nil
}

// ListBackups returns a list of available backups for an instance
func (m *Manager) ListBackups(name string) ([]string, error) {
	backupDir := filepath.Join(m.baseDir, "backups")
	pattern := filepath.Join(backupDir, fmt.Sprintf("%s-*.tar.gz", name))

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("listing backups: %w", err)
	}

	// Convert to relative paths
	var backups []string
	for _, match := range matches {
		rel, err := filepath.Rel(backupDir, match)
		if err != nil {
			return nil, fmt.Errorf("getting relative path: %w", err)
		}
		backups = append(backups, rel)
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i] > backups[j]
	})

	return backups, nil
}

// RestoreBackup restores an instance from a backup
func (m *Manager) RestoreBackup(backupName string) error {
	backupPath := filepath.Join(m.baseDir, "backups", backupName)

	// Extract instance name from backup name
	// backupName format: <instance-name>-<timestamp>.tar.gz
	base := strings.TrimSuffix(backupName, ".tar.gz")

	// Extract just the instance name (before the timestamp)
	idx := strings.LastIndex(base, "-")
	if idx == -1 {
		return fmt.Errorf("invalid backup name format")
	}
	instName := base[:idx]

	// Set up target paths
	instDir := filepath.Join(m.baseDir, "instances", instName)
	log.Printf("DEBUG: Restoring backup from %s to %s", backupPath, instDir)

	// Create a temporary directory for extraction
	tmpDir, err := os.MkdirTemp("", "factctl-restore-*")
	if err != nil {
		return fmt.Errorf("creating temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Open backup file
	f, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("opening backup file: %w", err)
	}
	defer f.Close()

	// Create gzip reader
	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	// Create tar reader
	tr := tar.NewReader(gr)

	// Extract files to temporary directory first
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar header: %w", err)
		}

		// Clean the file path and convert to native separators
		cleanPath := filepath.FromSlash(header.Name)
		target := filepath.Join(tmpDir, cleanPath)
		log.Printf("DEBUG: Extracting %s to %s", header.Name, target)

		// Ensure the target path is within the temp directory
		rel, err := filepath.Rel(tmpDir, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("invalid file path in backup: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory with original permissions
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}

		case tar.TypeReg:
			// Create parent directory if it doesn't exist
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", target, err)
			}

			// Create file with original permissions
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("writing to file %s: %w", target, err)
			}
			f.Close()
		}
	}

	// Remove existing instance directory if it exists
	if err := os.RemoveAll(instDir); err != nil {
		return fmt.Errorf("removing existing instance directory: %w", err)
	}

	// Create the parent instances directory if it doesn't exist
	instancesDir := filepath.Dir(instDir)
	if err := os.MkdirAll(instancesDir, 0755); err != nil {
		return fmt.Errorf("creating instances directory: %w", err)
	}

	// Move the entire temporary directory to become the instance directory
	if err := os.Rename(tmpDir, instDir); err != nil {
		return fmt.Errorf("moving restored files to instance directory: %w", err)
	}

	return nil
}

// SaveJSON saves a value as indented JSON to a file
func SaveJSON(path string, v interface{}) error {
	return os.WriteFile(path, []byte(PrettyJSON(v)), 0644)
}

// PrettyJSON returns a prettified JSON string
func PrettyJSON(v interface{}) string {
	// Use 2 spaces for indentation
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
