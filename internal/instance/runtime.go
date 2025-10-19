package instance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
	"sync"
	"syscall"
)

// RuntimeManager handles the execution of Factorio instances
type RuntimeManager struct {
	baseDir    string
	runtimeDir string
	processes  map[string]*InstanceProcess
	mu         sync.RWMutex
}

// InstanceProcess represents a running Factorio instance
type InstanceProcess struct {
	Instance *Instance
	Cmd      *exec.Cmd
	Done     chan struct{}
}

// NewRuntimeManager creates a new runtime manager
func NewRuntimeManager(baseDir string) *RuntimeManager {
	return &RuntimeManager{
		baseDir:    baseDir,
		runtimeDir: filepath.Join(baseDir, "runtimes"),
		processes:  make(map[string]*InstanceProcess),
	}
}

// Start launches a Factorio instance
func (rm *RuntimeManager) Start(ctx context.Context, inst *Instance) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if instance is already running
	if _, exists := rm.processes[inst.Config.Name]; exists {
		return fmt.Errorf("instance %s is already running", inst.Config.Name)
	}

	// Ensure runtime is available
	runtimePath, err := rm.ensureRuntime(inst.Config.Version)
	if err != nil {
		return fmt.Errorf("ensuring runtime: %w", err)
	}

	// Build command line arguments
	args := rm.buildArgs(inst)

	// Create command
	cmd := exec.CommandContext(ctx, runtimePath, args...)

	// Set working directory to instance directory
	cmd.Dir = inst.Dir

	// Setup environment
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("FACTORIO_HOME=%s", inst.Dir),
	)

	// Setup output handling
	logFile, err := os.OpenFile(
		filepath.Join(inst.Dir, "factorio.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting process: %w", err)
	}

	// Create process tracker
	proc := &InstanceProcess{
		Instance: inst,
		Cmd:      cmd,
		Done:     make(chan struct{}),
	}

	// Store process
	rm.processes[inst.Config.Name] = proc

	// Update instance state
	inst.State = StateRunning

	// Monitor process in background
	go func() {
		defer close(proc.Done)
		defer logFile.Close()
		
		err := cmd.Wait()
		
		rm.mu.Lock()
		delete(rm.processes, inst.Config.Name)
		rm.mu.Unlock()

		if err != nil {
			inst.State = StateError
		} else {
			inst.State = StateStopped
		}
	}()

	return nil
}

// Stop stops a running Factorio instance
func (rm *RuntimeManager) Stop(name string) error {
	rm.mu.Lock()
	proc, exists := rm.processes[name]
	rm.mu.Unlock()

	if !exists {
		return fmt.Errorf("instance %s is not running", name)
	}

	// Try graceful shutdown first
	if err := rm.gracefulStop(proc); err != nil {
		// If graceful shutdown fails, force kill
		if err := proc.Cmd.Process.Kill(); err != nil {
			return fmt.Errorf("killing process: %w", err)
		}
	}

	// Wait for process to finish
	<-proc.Done

	return nil
}

// gracefulStop attempts to gracefully stop the Factorio server
func (rm *RuntimeManager) gracefulStop(proc *InstanceProcess) error {
	if proc.Instance.Config.Headless {
		// For headless servers, try SIGTERM first
		if err := proc.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return err
		}
	} else {
		// For GUI instances, try SIGINT first (Ctrl+C)
		if err := proc.Cmd.Process.Signal(syscall.SIGINT); err != nil {
			return err
		}
	}

	// Give it a few seconds to shut down
	select {
	case <-proc.Done:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("graceful shutdown timed out")
	}
}

// ensureRuntime ensures the specified Factorio version is available
// and returns the path to the executable
func (rm *RuntimeManager) ensureRuntime(version string) (string, error) {
	// TODO: Implement runtime download/installation
	// For now, assume Factorio is installed in a standard location
	var paths []string
	switch runtime.GOOS {
	case "windows":
		paths = []string{
			"C:\\Program Files\\Factorio\\bin\\x64\\factorio.exe",
			"C:\\Program Files (x86)\\Factorio\\bin\\x64\\factorio.exe",
		}
	case "darwin":
		paths = []string{
			"/Applications/factorio.app/Contents/MacOS/factorio",
		}
	default: // Linux and others
		paths = []string{
			"/usr/bin/factorio",
			"/usr/local/bin/factorio",
		}
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("factorio %s not found", version)
}

// buildArgs constructs the command line arguments for launching Factorio
func (rm *RuntimeManager) buildArgs(inst *Instance) []string {
	var args []string

	if inst.Config.Headless {
		args = append(args, "--start-server")
		if inst.Config.SaveFile != "" {
			args = append(args, filepath.Join(inst.Dir, "saves", inst.Config.SaveFile))
		} else {
			args = append(args, filepath.Join(inst.Dir, "saves", "default.zip"))
		}
	}

	// Server settings
	if inst.Config.Server != nil {
		args = append(args,
			"--server-settings", filepath.Join(inst.Dir, "config", "server-settings.json"),
		)
	}

	// Set port if specified
	if inst.Config.Port > 0 {
		args = append(args, "--port", fmt.Sprintf("%d", inst.Config.Port))
	}

	// Add mod directory
	args = append(args,
		"--mod-directory", filepath.Join(inst.Dir, "mods"),
	)

	return args
}

// ListRunning returns a list of running instances
func (rm *RuntimeManager) ListRunning() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var running []string
	for name := range rm.processes {
		running = append(running, name)
	}
	return running
}

// IsRunning checks if an instance is running
func (rm *RuntimeManager) IsRunning(name string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	_, exists := rm.processes[name]
	return exists
}

// WaitFor waits for an instance to stop
func (rm *RuntimeManager) WaitFor(name string) error {
	rm.mu.RLock()
	proc, exists := rm.processes[name]
	rm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("instance %s is not running", name)
	}

	<-proc.Done
	return nil
}