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
}

// NewManager creates a new instance manager
func NewManager(baseDir string) *Manager {
	return &Manager{
		baseDir:    baseDir,
		runtimeDir: filepath.Join(baseDir, "runtimes"),
	}
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
	Config *Config
	Dir    string
	State  InstanceState
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

	// Create required subdirectories
	dirs := []string{
		filepath.Join(instDir, "saves"),
		filepath.Join(instDir, "mods"),
		filepath.Join(instDir, "config"),
		filepath.Join(instDir, "scripts"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
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

	return &Instance{
		Config: cfg,
		Dir:    instDir,
		State:  StateStopped,
	}, nil
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
