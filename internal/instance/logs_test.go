package instance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	"strings"
)

func TestLogManager(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "factctl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create instance directory and log file
	instName := "test-instance"
	instDir := filepath.Join(tmpDir, "instances", instName)
	if err := os.MkdirAll(instDir, 0755); err != nil {
		t.Fatalf("Failed to create instance directory: %v", err)
	}

	logManager := NewLogManager(tmpDir)

	t.Run("log parsing", func(t *testing.T) {
		tests := []struct {
			line     string
			wantLevel LogLevel
			wantMsg   string
		}{
			{
				line:      "2025-10-19 12:34:56 [INFO] Server started",
				wantLevel: LogInfo,
				wantMsg:   "Server started",
			},
			{
				line:      "2025-10-19 12:34:56 [ERROR] Connection failed",
				wantLevel: LogError,
				wantMsg:   "Connection failed",
			},
			{
				line:      "2025-10-19 12:34:56 [WARNING] Low disk space",
				wantLevel: LogWarning,
				wantMsg:   "Low disk space",
			},
			{
				line:      "Invalid log line format",
				wantLevel: LogInfo,
				wantMsg:   "Invalid log line format",
			},
		}

		for _, tt := range tests {
			entry := logManager.parseLine(tt.line)
			if entry.Level != tt.wantLevel {
				t.Errorf("parseLine(%q) got level %v, want %v", tt.line, entry.Level, tt.wantLevel)
			}
			if !strings.Contains(entry.Message, tt.wantMsg) {
				t.Errorf("parseLine(%q) got message %q, want to contain %q", tt.line, entry.Message, tt.wantMsg)
			}
		}
	})

	t.Run("log streaming", func(t *testing.T) {
		// Create a log file with some initial content
		logPath := filepath.Join(instDir, "factorio.log")
		initialLog := "2025-10-19 12:34:56 [INFO] Initial log entry\n"

		// Ensure log file directory exists
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			t.Fatalf("Failed to create log directory: %v", err)
		}

		// Create initial log file
		if err := os.WriteFile(logPath, []byte(initialLog), 0644); err != nil {
			t.Fatalf("Failed to create log file: %v", err)
		}

		// Create channels to receive log entries
		received := make(chan LogEntry, 10)
		errChan := make(chan error, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Subscribe to logs
		handler := func(entry LogEntry) {
			select {
			case received <- entry:
			default:
				// Channel is full, drop the entry
			}
		}
		logManager.Subscribe(instName, handler)
		defer logManager.Unsubscribe(instName, handler)

		// Start streaming in a goroutine
		go func() {
			if err := logManager.StreamLogs(ctx, instName); err != nil {
				errChan <- err
			}
		}()

		// Wait a bit for streaming to start
		time.Sleep(500 * time.Millisecond)

		// Write new log entry
		newLog := "2025-10-19 12:34:57 [INFO] New log entry\n"
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("Failed to open log file: %v", err)
		}

		if _, err := f.WriteString(newLog); err != nil {
			f.Close()
			t.Fatalf("Failed to write log entry: %v", err)
		}
		f.Sync()
		f.Close()

		// Wait for log entry
		select {
		case err := <-errChan:
			t.Errorf("StreamLogs error: %v", err)
		case entry := <-received:
			if !strings.Contains(entry.Raw, "New log entry") {
				t.Errorf("Got unexpected log message: %v", entry.Raw)
			}
		case <-time.After(3 * time.Second):
			t.Error("Timeout waiting for log entry")
		}
	})

	t.Run("log rotation", func(t *testing.T) {
		logManager.SetMaxFileSize(100) // Small size for testing
		logManager.SetMaxFiles(3)      // Keep 3 rotated files

		logPath := filepath.Join(instDir, "factorio.log")
		
		// Create a large log file
		largeLog := make([]byte, 200)
		for i := range largeLog {
			largeLog[i] = 'x'
		}
		if err := os.WriteFile(logPath, largeLog, 0644); err != nil {
			t.Fatalf("Failed to create large log file: %v", err)
		}

		// Rotate logs
		if err := logManager.RotateLogs(instName); err != nil {
			t.Errorf("RotateLogs() error = %v", err)
			return
		}

		// Check rotated files
		for i := 1; i <= 3; i++ {
			rotated := fmt.Sprintf("%s.%d", logPath, i)
			if i == 1 {
				if _, err := os.Stat(rotated); os.IsNotExist(err) {
					t.Errorf("Rotated file %s does not exist", rotated)
				}
			}
		}

		// Check that old log was rotated
		info, err := os.Stat(logPath)
		if err != nil {
			t.Errorf("Failed to stat new log file: %v", err)
			return
		}
		if info.Size() != 0 {
			t.Error("New log file should be empty")
		}
	})

	t.Run("log history", func(t *testing.T) {
		logPath := filepath.Join(instDir, "factorio.log")
		historyLog := `2025-10-19 12:34:56 [INFO] First entry
2025-10-19 12:34:57 [WARNING] Second entry
2025-10-19 12:34:58 [ERROR] Third entry
`
		if err := os.WriteFile(logPath, []byte(historyLog), 0644); err != nil {
			t.Fatalf("Failed to create history log: %v", err)
		}

		entries, err := logManager.GetLogHistory(instName, 2)
		if err != nil {
			t.Errorf("GetLogHistory() error = %v", err)
			return
		}

		if len(entries) != 2 {
			t.Errorf("GetLogHistory() returned %d entries, want 2", len(entries))
			return
		}

		if entries[1].Level != LogError || !strings.Contains(entries[1].Message, "Third entry") {
			t.Error("Last entry should be the error message")
		}
	})
}