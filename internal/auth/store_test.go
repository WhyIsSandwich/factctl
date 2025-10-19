package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialStore(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	testCreds := &Credentials{
		FactorioUsername: "testuser",
		FactorioToken:    "testtoken",
	}

	// Test saving credentials
	t.Run("save credentials", func(t *testing.T) {
		if err := store.Save(testCreds); err != nil {
			t.Errorf("Save() error = %v", err)
			return
		}

		// Verify file was created with correct permissions
		path := filepath.Join(tmpDir, "credentials.json")
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Failed to stat credentials file: %v", err)
			return
		}

		if info.Mode().Perm() != 0600 {
			t.Errorf("Credentials file has wrong permissions: got %v, want %v", info.Mode().Perm(), 0600)
		}

		// Verify content
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read credentials file: %v", err)
			return
		}

		var savedCreds Credentials
		if err := json.Unmarshal(data, &savedCreds); err != nil {
			t.Errorf("Failed to parse saved credentials: %v", err)
			return
		}

		if savedCreds.FactorioUsername != testCreds.FactorioUsername {
			t.Errorf("Username mismatch: got %v, want %v", savedCreds.FactorioUsername, testCreds.FactorioUsername)
		}
		if savedCreds.FactorioToken != testCreds.FactorioToken {
			t.Errorf("Token mismatch: got %v, want %v", savedCreds.FactorioToken, testCreds.FactorioToken)
		}
	})

	// Test loading credentials
	t.Run("load credentials", func(t *testing.T) {
		loaded, err := store.Load()
		if err != nil {
			t.Errorf("Load() error = %v", err)
			return
		}

		if loaded.FactorioUsername != testCreds.FactorioUsername {
			t.Errorf("Username mismatch: got %v, want %v", loaded.FactorioUsername, testCreds.FactorioUsername)
		}
		if loaded.FactorioToken != testCreds.FactorioToken {
			t.Errorf("Token mismatch: got %v, want %v", loaded.FactorioToken, testCreds.FactorioToken)
		}
	})

	// Test clearing credentials
	t.Run("clear credentials", func(t *testing.T) {
		if err := store.Clear(); err != nil {
			t.Errorf("Clear() error = %v", err)
			return
		}

		// Verify file was deleted
		path := filepath.Join(tmpDir, "credentials.json")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("Credentials file still exists after Clear()")
		}

		// Loading should now return ErrNoCredentials
		_, err := store.Load()
		if err != ErrNoCredentials {
			t.Errorf("Load() after Clear() got error = %v, want %v", err, ErrNoCredentials)
		}
	})
}