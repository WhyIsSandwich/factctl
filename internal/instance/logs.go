package instance

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarning
	LogError
)

// LogEntry represents a single log message
type LogEntry struct {
	Time    time.Time
	Level   LogLevel
	Message string
	Raw     string
}

// LogHandler is a function that processes log entries
type LogHandler func(LogEntry)

// LogManager handles log file operations and streaming
type LogManager struct {
	baseDir     string
	handlers    map[string][]LogHandler
	mu          sync.RWMutex
	maxFileSize int64
	maxFiles    int
}

// NewLogManager creates a new log manager
func NewLogManager(baseDir string) *LogManager {
	return &LogManager{
		baseDir:     baseDir,
		handlers:    make(map[string][]LogHandler),
		maxFileSize: 10 * 1024 * 1024, // 10MB default
		maxFiles:    5,                // Keep 5 rotated files by default
	}
}

// BaseDir returns the base directory for logs
func (lm *LogManager) BaseDir() string {
	return lm.baseDir
}

// Subscribe adds a log handler for an instance
func (lm *LogManager) Subscribe(instanceName string, handler LogHandler) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.handlers[instanceName] = append(lm.handlers[instanceName], handler)
}

// Unsubscribe removes a log handler for an instance
func (lm *LogManager) Unsubscribe(instanceName string, handler LogHandler) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	handlers := lm.handlers[instanceName]
	for i, h := range handlers {
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			lm.handlers[instanceName] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}

	if len(lm.handlers[instanceName]) == 0 {
		delete(lm.handlers, instanceName)
	}
}

// StreamLogs starts streaming logs for an instance
func (lm *LogManager) StreamLogs(ctx context.Context, instanceName string) error {
	logPath := filepath.Join(lm.baseDir, "instances", instanceName, "factorio.log")

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	// Seek to end for live streaming
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		file.Close()
		return fmt.Errorf("seeking log file: %w", err)
	}

	// Create scanner for reading lines
	scanner := bufio.NewScanner(file)

	// Monitor file in background
	go func() {
		defer file.Close()

		// Keep track of current position
		currentPos, _ := file.Seek(0, io.SeekCurrent)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				if scanner.Scan() {
					line := scanner.Text()
					entry := lm.parseLine(line)

					// Update position
					currentPos += int64(len(line) + 1) // +1 for newline

					// Notify handlers
					lm.mu.RLock()
					handlers := lm.handlers[instanceName]
					lm.mu.RUnlock()

					for _, handler := range handlers {
						handler(entry)
					}
				} else if err := scanner.Err(); err != nil {
					fmt.Printf("Scanner error: %v\n", err)
					return
				} else {
					// At EOF, check for new content
					stat, err := file.Stat()
					if err != nil {
						fmt.Printf("Error getting file info: %v\n", err)
						return
					}

					if stat.Size() > currentPos {
						// New content available
						if _, err := file.Seek(currentPos, io.SeekStart); err != nil {
							fmt.Printf("Error seeking to position: %v\n", err)
							return
						}
						scanner = bufio.NewScanner(file)
					} else {
						// Wait briefly before checking again
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
		}
	}()

	return nil
}

// checkRotation verifies if the log file has been rotated and updates the scanner
func (lm *LogManager) checkRotation(path string, oldFile *os.File, scanner *bufio.Scanner) error {
	// Get current file info
	oldInfo, err := oldFile.Stat()
	if err != nil {
		return err
	}

	// Get fresh file info
	newInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Check if file has been rotated
	if os.SameFile(oldInfo, newInfo) {
		return nil
	}

	// File has been rotated, open new file
	newFile, err := os.Open(path)
	if err != nil {
		return err
	}

	// Update scanner to use new file
	oldFile.Close()
	*scanner = *bufio.NewScanner(newFile)
	return nil
}

// RotateLogs performs log rotation for an instance
func (lm *LogManager) RotateLogs(instanceName string) error {
	logDir := filepath.Join(lm.baseDir, "instances", instanceName)
	logPath := filepath.Join(logDir, "factorio.log")

	// Check if rotation is needed
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No log file yet
		}
		return err
	}

	if info.Size() < lm.maxFileSize {
		return nil // File is still small enough
	}

	// Remove oldest log if we have too many
	for i := lm.maxFiles; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", logPath, i)
		if i == lm.maxFiles {
			os.Remove(oldPath) // Ignore error - file might not exist
		} else {
			newPath := fmt.Sprintf("%s.%d", logPath, i+1)
			os.Rename(oldPath, newPath) // Ignore error - file might not exist
		}
	}

	// Rotate current log
	if err := os.Rename(logPath, logPath+".1"); err != nil {
		return fmt.Errorf("rotating log file: %w", err)
	}

	// Create new empty log file
	if err := os.WriteFile(logPath, nil, 0644); err != nil {
		return fmt.Errorf("creating new log file: %w", err)
	}

	return nil
}

// parseLine converts a log line into a structured LogEntry
func (lm *LogManager) parseLine(line string) LogEntry {
	entry := LogEntry{
		Time:  time.Now(),
		Level: LogInfo,
		Raw:   line,
	}

	// Handle empty lines
	if line == "" {
		entry.Message = ""
		return entry
	}

	// Example Factorio log line format:
	// 2025-10-19 12:34:56 [INFO] Game started
	parts := strings.SplitN(line, " ", 4)
	if len(parts) >= 4 && len(parts[0]) == 10 && len(parts[1]) == 8 {
		// Try to parse timestamp
		if ts, err := time.Parse("2006-01-02 15:04:05", parts[0]+" "+parts[1]); err == nil {
			entry.Time = ts

			// Parse log level
			level := strings.Trim(parts[2], "[]")
			switch strings.ToUpper(level) {
			case "DEBUG":
				entry.Level = LogDebug
			case "INFO":
				entry.Level = LogInfo
			case "WARNING", "WARN":
				entry.Level = LogWarning
			case "ERROR":
				entry.Level = LogError
			}

			entry.Message = parts[3]
			return entry
		}
	}

	// For unparseable lines, return the full line as message
	entry.Message = line
	return entry
}

// SetMaxFileSize sets the maximum size for log files before rotation
func (lm *LogManager) SetMaxFileSize(size int64) {
	lm.maxFileSize = size
}

// SetMaxFiles sets the maximum number of rotated log files to keep
func (lm *LogManager) SetMaxFiles(count int) {
	lm.maxFiles = count
}

// GetLogHistory returns recent log entries for an instance
func (lm *LogManager) GetLogHistory(instanceName string, maxLines int) ([]LogEntry, error) {
	logPath := filepath.Join(lm.baseDir, "instances", instanceName, "factorio.log")

	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	defer file.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(file)

	// Read all lines first (we'll trim later)
	for scanner.Scan() {
		entries = append(entries, lm.parseLine(scanner.Text()))
	}

	// Return only the requested number of most recent lines
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}

	return entries, nil
}
