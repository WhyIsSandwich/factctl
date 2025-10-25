package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/WhyIsSandwich/factctl/internal/auth"
	"github.com/WhyIsSandwich/factctl/internal/instance"
	"golang.org/x/term"
)

const version = "0.1.0"

func main() {
	// Define root flags
	var (
		showVersion  = flag.Bool("version", false, "Show version information")
		headless     = flag.Bool("headless", false, "Run Factorio in headless mode")
		config       = flag.String("config", "", "Path to instance configuration file")
		baseDir      = flag.String("base-dir", "", "Base directory for instances (default: platform-specific)")
		factorioPath = flag.String("factorio-path", "", "Path to Factorio installation")
		useSymlinks  = flag.Bool("symlinks", false, "Use symlinks instead of copying files for instance overlay")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: factctl <command> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up      Create or update an instance\n")
		fmt.Fprintf(os.Stderr, "  down    Remove an instance\n")
		fmt.Fprintf(os.Stderr, "  run     Launch Factorio with the specified instance\n")
		fmt.Fprintf(os.Stderr, "  logs    Stream instance logs\n")
		fmt.Fprintf(os.Stderr, "  auth    Configure Factorio portal credentials\n")
		fmt.Fprintf(os.Stderr, "  download Download Factorio to runtimes (usage: <build-type> [version])\n\n")
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
	var manager *instance.Manager
	if *factorioPath != "" {
		manager = instance.NewManagerWithFactorio(baseDirPath, *factorioPath)
	} else {
		manager = instance.NewManager(baseDirPath)
	}

	// Configure overlay method
	manager.SetUseSymlinks(*useSymlinks)
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
	case "auth":
		if err := handleAuth(baseDirPath, args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "download":
		if err := handleDownload(baseDirPath, args[1:]); err != nil {
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

		// Override the name from the config file with the command-line argument
		// This allows using the same config file for multiple instances with different names
		cfg.Name = instanceName
	} else {
		// Create default configuration
		cfg = &instance.Config{
			Name:     instanceName,
			Version:  "1.1", // Default to latest stable
			Headless: headless,
			Mods: instance.ModsConfig{
				Enabled: []string{"base"},
				Sources: map[string]string{},
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
		return fmt.Errorf("failed to create instance: %w\nHint: Check that you have write permissions to the instance directory", err)
	}

	// Update player-data.json with service credentials if available
	if err := updatePlayerDataWithCredentials(manager, inst); err != nil {
		fmt.Printf("Warning: Could not update player-data.json with credentials: %v\n", err)
	}

	// Install mods if specified
	if len(cfg.Mods.Enabled) > 0 {
		fmt.Println("Installing mods and dependencies...")
		ctx := context.Background()

		// Use recursive installer to resolve all dependencies
		installedMods, err := modManager.InstallModsRecursively(ctx, inst, cfg.Mods.Enabled)
		if err != nil {
			fmt.Printf("Warning: Some mods failed to install: %v\n", err)
		}

		fmt.Printf("Successfully installed %d mods total\n", len(installedMods))
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

// handleAuth configures Factorio portal credentials
func handleAuth(baseDir string, args []string) error {
	fmt.Println("Configuring Factorio portal credentials...")
	fmt.Println("You'll need your Factorio username and password to authenticate with the Factorio API.")
	fmt.Println()

	// Get username
	fmt.Print("Factorio username: ")
	reader := bufio.NewReader(os.Stdin)
	username, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading username: %w", err)
	}
	username = strings.TrimSpace(username)

	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	// Get password with masking
	fmt.Print("Factorio password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	password := string(passwordBytes)
	fmt.Println() // Add newline after masked input

	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// Authenticate with Factorio API to get token
	fmt.Println("Authenticating with Factorio API...")
	token, err := authenticateWithFactorio(username, password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Create credentials
	creds := &auth.Credentials{
		FactorioUsername: username,
		FactorioToken:    token,
	}

	// Save credentials to config directory
	configDir := filepath.Join(baseDir, "config")
	store := auth.NewStore(configDir)

	if err := store.Save(creds); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	fmt.Printf("Authentication successful! Credentials saved to %s\n", filepath.Join(configDir, "credentials.json"))
	fmt.Println("You can now use 'factctl up' to create instances with mod portal access.")

	return nil
}

// authenticateWithFactorio authenticates with the Factorio API and returns a token
func authenticateWithFactorio(username, password string) (string, error) {
	// Prepare form data for the authentication request
	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)
	data.Set("api_version", "6") // Use latest API version

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://auth.factorio.com/api-login", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "factctl/1.0")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var authResponse struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Error    string `json:"error"`
		Message  string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	// Check for authentication errors
	if authResponse.Error != "" {
		return "", fmt.Errorf("authentication failed: %s - %s", authResponse.Error, authResponse.Message)
	}

	if authResponse.Token == "" {
		return "", fmt.Errorf("no token received from authentication API")
	}

	return authResponse.Token, nil
}

// updatePlayerDataWithCredentials updates the player-data.json with service credentials
func updatePlayerDataWithCredentials(manager *instance.Manager, inst *instance.Instance) error {
	// Try to get credentials from the auth store
	configDir := filepath.Join(manager.BaseDir(), "config")
	store := auth.NewStore(configDir)

	creds, err := store.Load()
	if err != nil {
		// Try default location as fallback
		if defaultPath, err := auth.DefaultLocation(); err == nil {
			defaultStore := auth.NewStore(filepath.Dir(defaultPath))
			creds, err = defaultStore.Load()
		}
	}

	if err != nil || creds.FactorioUsername == "" || creds.FactorioToken == "" {
		// No credentials available, leave player-data.json with empty values
		return nil
	}

	// Update player-data.json with the credentials
	return manager.UpdatePlayerData(inst, creds.FactorioUsername, creds.FactorioToken)
}

// handleDownload downloads a Factorio version
func handleDownload(baseDir string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("build type is required\nUsage: factctl download <build-type> [version] [name] [--allow-experimental]\nBuild types: alpha, headless, expansion, demo\nUse 'latest' for version to get the latest release\nName is optional and will default to smart naming based on version and build type\nUse --allow-experimental to get experimental versions when using 'latest'")
	}

	buildType := args[0]
	version := "latest" // Default to latest
	name := ""          // Default to smart naming
	allowExperimental := false

	// Parse arguments
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--allow-experimental" {
			allowExperimental = true
		} else if version == "latest" && arg != "--allow-experimental" {
			// First non-flag argument is version
			version = arg
		} else if version != "latest" && arg != "--allow-experimental" && name == "" {
			// Second non-flag argument is name
			name = arg
		}
	}

	// Validate build type
	validBuildTypes := []string{"alpha", "headless", "expansion", "demo"}
	valid := false
	for _, bt := range validBuildTypes {
		if buildType == bt {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid build type: %s\nValid build types: %v", buildType, validBuildTypes)
	}

	// Create downloader
	downloader := instance.NewFactorioDownloader(baseDir)

	// If version is "latest", get the latest version
	if version == "latest" {
		ctx := context.Background()
		latestVersion, err := downloader.GetLatestVersion(ctx, buildType, allowExperimental)
		if err != nil {
			return fmt.Errorf("getting latest version: %w", err)
		}
		version = latestVersion
	}

	// Download Factorio
	ctx := context.Background()
	versionDir, err := downloader.DownloadFactorioWithName(ctx, version, buildType, name)
	if err != nil {
		return fmt.Errorf("downloading Factorio %s (%s): %w", version, buildType, err)
	}

	// Show the runtime name that was used
	runtimeName := filepath.Base(versionDir)
	fmt.Printf("Factorio %s (%s) downloaded successfully as runtime '%s' to %s\n", version, buildType, runtimeName, versionDir)
	return nil
}
