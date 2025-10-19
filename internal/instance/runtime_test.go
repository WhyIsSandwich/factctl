package instance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockCmd struct {
	started  bool
	stopped  bool
	exitCode int
}

func TestRuntimeManager(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rm := NewRuntimeManager(tmpDir)

	// Create a test instance
	inst := &Instance{
		Config: &Config{
			Name:     "test-instance",
			Version:  "1.1.87",
			Headless: true,
			Port:     34197,
			Server: &ServerConfig{
				Name:       "Test Server",
				MaxPlayers: 32,
			},
		},
		Dir:   filepath.Join(tmpDir, "instances", "test-instance"),
		State: StateStopped,
	}

	// Create instance directory structure
	dirs := []string{
		filepath.Join(inst.Dir, "saves"),
		filepath.Join(inst.Dir, "mods"),
		filepath.Join(inst.Dir, "config"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create test save file
	saveFile := filepath.Join(inst.Dir, "saves", "test.zip")
	if err := os.WriteFile(saveFile, []byte("test save data"), 0644); err != nil {
		t.Fatalf("Failed to create test save: %v", err)
	}

	// Test argument building
	t.Run("build arguments", func(t *testing.T) {
		args := rm.buildArgs(inst)
		
		expectedArgs := []string{
			"--start-server",
			filepath.Join(inst.Dir, "saves", "default.zip"),
			"--server-settings",
			filepath.Join(inst.Dir, "config", "server-settings.json"),
			"--port",
			"34197",
			"--mod-directory",
			filepath.Join(inst.Dir, "mods"),
		}

		if len(args) != len(expectedArgs) {
			t.Errorf("buildArgs() got %d args, want %d", len(args), len(expectedArgs))
			return
		}

		for i, arg := range args {
			if arg != expectedArgs[i] {
				t.Errorf("buildArgs()[%d] = %s, want %s", i, arg, expectedArgs[i])
			}
		}
	})

	// Test process management
	t.Run("process management", func(t *testing.T) {
		// Skip actual process starting if Factorio is not installed
		if _, err := rm.ensureRuntime(inst.Config.Version); err != nil {
			t.Skip("Skipping process management test: Factorio not installed")
		}

		ctx := context.Background()

		// Start instance
		if err := rm.Start(ctx, inst); err != nil {
			t.Errorf("Start() error = %v", err)
			return
		}

		// Check running state
		if !rm.IsRunning(inst.Config.Name) {
			t.Error("Instance should be marked as running")
		}

		running := rm.ListRunning()
		if len(running) != 1 || running[0] != inst.Config.Name {
			t.Error("ListRunning() returned incorrect list")
		}

		// Stop instance
		if err := rm.Stop(inst.Config.Name); err != nil {
			t.Errorf("Stop() error = %v", err)
			return
		}

		// Give it a moment to fully stop
		time.Sleep(100 * time.Millisecond)

		// Check stopped state
		if rm.IsRunning(inst.Config.Name) {
			t.Error("Instance should be marked as stopped")
		}

		if len(rm.ListRunning()) != 0 {
			t.Error("ListRunning() should return empty list")
		}
	})

	// Test error cases
	t.Run("error cases", func(t *testing.T) {
		// Try to stop non-existent instance
		if err := rm.Stop("nonexistent"); err == nil {
			t.Error("Stop() should fail for non-existent instance")
		}

		// Try to start instance twice
		ctx := context.Background()
		if err := rm.Start(ctx, inst); err != nil {
			t.Skip("Skipping duplicate start test: Factorio not installed")
		}
		if err := rm.Start(ctx, inst); err == nil {
			t.Error("Start() should fail for already running instance")
		}
		rm.Stop(inst.Config.Name)
	})
}