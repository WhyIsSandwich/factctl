package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/WhyIsSandwich/factctl/internal/instance"
)

const version = "0.1.0"

func main() {
	// Define root flags
	var (
		showVersion = flag.Bool("version", false, "Show version information")
		headless    = flag.Bool("headless", false, "Run Factorio in headless mode")
		config      = flag.String("config", "", "Path to instance configuration file")
		baseDir     = flag.String("base-dir", "", "Base directory for instances (default: platform-specific)")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: factctl <command> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up      Create or update an instance\n")
		fmt.Fprintf(os.Stderr, "  down    Remove an instance\n")
		fmt.Fprintf(os.Stderr, "  run     Launch Factorio with the specified instance\n")
		fmt.Fprintf(os.Stderr, "  logs    Stream instance logs\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("factctl version %s\n", version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	// Determine base directory
	var baseDirPath string
	var err error
	if *baseDir != "" {
		baseDirPath = *baseDir
	} else {
		baseDirPath, err = instance.DefaultLocation()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error determining base directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Create managers
	manager := instance.NewManager(baseDirPath)
	runtimeManager := instance.NewRuntimeManager(baseDirPath)
	modManager := instance.NewModManager(baseDirPath)
	logManager := instance.NewLogManager(baseDirPath)

	command := args[0]
	switch command {
	case "up":
		if err := handleUp(manager, modManager, args[1:], *config, *headless); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "down":
		if err := handleDown(manager, args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := handleRun(runtimeManager, manager, args[1:], *headless); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "logs":
		if err := handleLogs(logManager, args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

// handleUp creates or updates an instance
func handleUp(manager *instance.Manager, modManager *instance.ModManager, args []string, configPath string, headless bool) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required\nUsage: factctl up <instance-name> [options]")
	}

	instanceName := args[0]

	// Validate instance name
	if err := validateInstanceName(instanceName); err != nil {
		return fmt.Errorf("invalid instance name: %w", err)
	}

	// Load or create configuration
	var cfg *instance.Config
	var err error

	if configPath != "" {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return fmt.Errorf("configuration file not found: %s", configPath)
		}
		
		cfg, err = instance.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("loading configuration file: %w\nHint: Check that the file is valid JSON/JSONC", err)
		}
	} else {
		// Create default configuration
		cfg = &instance.Config{
			Name:     instanceName,
			Version:  "1.1", // Default to latest stable
			Headless: headless,
			Mods: instance.ModsConfig{
				Enabled: []string{"base"},
				Sources: map[string]string{
					"base": "builtin", // Base mod is built-in
				},
			},
		}
	}

	// Override headless mode if specified
	if headless {
		cfg.Headless = true
	}

	fmt.Printf("Creating/updating instance '%s'...\n", instanceName)

	// Create instance
	inst, err := manager.Create(cfg)
	if err != nil {
		return fmt.Errorf("failed to create instance: %w\nHint: Check that you have write permissions to the base directory", err)
	}

	// Install mods if specified
	if len(cfg.Mods.Sources) > 0 {
		fmt.Println("Installing mods...")
		ctx := context.Background()
		successCount := 0
		for modName, modSource := range cfg.Mods.Sources {
			fmt.Printf("Installing mod '%s' from '%s'...\n", modName, modSource)
			if err := modManager.InstallMod(ctx, inst, modSource); err != nil {
				fmt.Printf("Warning: Failed to install mod '%s': %v\n", modName, err)
			} else {
				successCount++
			}
		}
		fmt.Printf("Successfully installed %d/%d mods\n", successCount, len(cfg.Mods.Sources))
	}

	fmt.Printf("Instance '%s' created successfully!\n", instanceName)
	fmt.Printf("Instance directory: %s\n", inst.Dir)
	return nil
}

// handleDown removes an instance
func handleDown(manager *instance.Manager, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required\nUsage: factctl down <instance-name> [--backup]")
	}

	instanceName := args[0]
	backup := false

	// Validate instance name
	if err := validateInstanceName(instanceName); err != nil {
		return fmt.Errorf("invalid instance name: %w", err)
	}

	// Check for backup flag
	if len(args) > 1 && args[1] == "--backup" {
		backup = true
	}

	// Check if instance exists
	instDir := filepath.Join(manager.BaseDir(), "instances", instanceName)
	if _, err := os.Stat(instDir); os.IsNotExist(err) {
		return fmt.Errorf("instance '%s' does not exist\nHint: Use 'factctl up %s' to create it first", instanceName, instanceName)
	}

	fmt.Printf("Removing instance '%s'...\n", instanceName)

	if err := manager.Remove(instanceName, backup); err != nil {
		return fmt.Errorf("failed to remove instance: %w\nHint: Check that you have write permissions to the instance directory", err)
	}

	if backup {
		fmt.Printf("Instance '%s' removed and backed up successfully!\n", instanceName)
		fmt.Printf("Backup location: %s\n", filepath.Join(manager.BaseDir(), "backups"))
	} else {
		fmt.Printf("Instance '%s' removed successfully!\n", instanceName)
	}

	return nil
}

// handleRun launches a Factorio instance
func handleRun(runtimeManager *instance.RuntimeManager, manager *instance.Manager, args []string, headless bool) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required\nUsage: factctl run <instance-name> [options]")
	}

	instanceName := args[0]

	// Validate instance name
	if err := validateInstanceName(instanceName); err != nil {
		return fmt.Errorf("invalid instance name: %w", err)
	}

	// Check if instance exists
	instDir := filepath.Join(manager.BaseDir(), "instances", instanceName)
	if _, err := os.Stat(instDir); os.IsNotExist(err) {
		return fmt.Errorf("instance '%s' does not exist\nHint: Use 'factctl up %s' to create it first", instanceName, instanceName)
	}

	// Load instance configuration
	configPath := filepath.Join(instDir, "config", "instance.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("instance configuration not found: %s\nHint: The instance may be corrupted, try recreating it", configPath)
	}
	
	cfg, err := instance.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading instance configuration: %w\nHint: Check that the configuration file is valid", err)
	}

	// Override headless mode if specified
	if headless {
		cfg.Headless = true
	}

	// Create instance object
	inst := &instance.Instance{
		Config: cfg,
		Dir:    instDir,
		State:  instance.StateStopped,
	}

	// Check if instance is already running
	if runtimeManager.IsRunning(instanceName) {
		return fmt.Errorf("instance '%s' is already running\nHint: Use 'factctl logs %s' to view logs or stop the existing instance first", instanceName, instanceName)
	}

	fmt.Printf("Launching Factorio instance '%s' (headless=%v)...\n", instanceName, cfg.Headless)

	ctx := context.Background()
	if err := runtimeManager.Start(ctx, inst); err != nil {
		return fmt.Errorf("failed to start instance: %w\nHint: Check that Factorio is installed and accessible", err)
	}

	fmt.Printf("Instance '%s' started successfully!\n", instanceName)
	fmt.Println("Press Ctrl+C to stop the instance")

	// Wait for interrupt
	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleLogs streams logs for an instance
func handleLogs(logManager *instance.LogManager, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required\nUsage: factctl logs <instance-name> [--no-follow]")
	}

	instanceName := args[0]
	follow := true

	// Validate instance name
	if err := validateInstanceName(instanceName); err != nil {
		return fmt.Errorf("invalid instance name: %w", err)
	}

	// Check for --no-follow flag
	if len(args) > 1 && args[1] == "--no-follow" {
		follow = false
	}

	// Check if instance exists
	instDir := filepath.Join(logManager.BaseDir(), "instances", instanceName)
	if _, err := os.Stat(instDir); os.IsNotExist(err) {
		return fmt.Errorf("instance '%s' does not exist\nHint: Use 'factctl up %s' to create it first", instanceName, instanceName)
	}

	if follow {
		fmt.Printf("Streaming logs for instance '%s' (press Ctrl+C to stop)...\n", instanceName)
		
		// Create log handler
		handler := func(entry instance.LogEntry) {
			fmt.Printf("[%s] %s\n", entry.Time.Format("15:04:05"), entry.Message)
		}

		// Subscribe to logs
		logManager.Subscribe(instanceName, handler)
		defer logManager.Unsubscribe(instanceName, handler)

		// Start streaming
		ctx := context.Background()
		if err := logManager.StreamLogs(ctx, instanceName); err != nil {
			return fmt.Errorf("failed to stream logs: %w\nHint: Check that the instance is running and has log files", err)
		}

		// Wait for interrupt
		select {
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		// Show recent logs
		entries, err := logManager.GetLogHistory(instanceName, 50)
		if err != nil {
			return fmt.Errorf("failed to get log history: %w\nHint: Check that the instance has log files", err)
		}

		if len(entries) == 0 {
			fmt.Printf("No logs found for instance '%s'\nHint: The instance may not have been run yet", instanceName)
			return nil
		}

		fmt.Printf("Recent logs for instance '%s' (%d entries):\n", instanceName, len(entries))
		for _, entry := range entries {
			fmt.Printf("[%s] %s\n", entry.Time.Format("15:04:05"), entry.Message)
		}
	}

	return nil
}

// validateInstanceName validates that an instance name is acceptable
func validateInstanceName(name string) error {
	if name == "" {
		return fmt.Errorf("instance name cannot be empty")
	}
	
	if len(name) > 50 {
		return fmt.Errorf("instance name too long (max 50 characters)")
	}
	
	// Check for invalid characters
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || 
			 (char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || 
			 char == '-' || char == '_') {
			return fmt.Errorf("instance name contains invalid character '%c' (only letters, numbers, hyphens, and underscores allowed)", char)
		}
	}
	
	return nil
}