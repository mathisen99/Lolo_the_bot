package output

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	// MaxLogSizeMB is the maximum size of the error log file before rotation
	MaxLogSizeMB = 10
	// MaxLogFiles is the maximum number of rotated log files to keep
	MaxLogFiles = 5
)

// ErrorLogger handles file-based error logging with rotation
type ErrorLogger struct {
	logPath  string
	mu       sync.Mutex
	maxSize  int64 // in bytes
	maxFiles int
}

// NewErrorLogger creates a new ErrorLogger
func NewErrorLogger(logPath string) *ErrorLogger {
	return &ErrorLogger{
		logPath:  logPath,
		maxSize:  MaxLogSizeMB * 1024 * 1024, // Convert MB to bytes
		maxFiles: MaxLogFiles,
	}
}

// LogError writes an error to the error log file with timestamp, type, message, and stack trace
func (e *ErrorLogger) LogError(errorType, errorMessage string, originalErr error) error {
	return e.LogErrorWithRequestID(errorType, errorMessage, originalErr, "")
}

// LogErrorWithRequestID writes an error to the error log file with timestamp, type, message, request ID, and stack trace
func (e *ErrorLogger) LogErrorWithRequestID(errorType, errorMessage string, originalErr error, requestID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if rotation is needed
	if err := e.rotateIfNeeded(); err != nil {
		return fmt.Errorf("failed to rotate log: %w", err)
	}

	// Open log file in append mode, create if doesn't exist
	f, err := os.OpenFile(e.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open error log: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Build log entry
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] ERROR: %s\n", timestamp, errorMessage)
	entry += fmt.Sprintf("Type: %s\n", errorType)

	if requestID != "" {
		entry += fmt.Sprintf("Request ID: %s\n", requestID)
	}

	if originalErr != nil {
		entry += fmt.Sprintf("Details: %s\n", originalErr.Error())
	}

	// Add stack trace
	entry += "Stack Trace:\n"
	entry += e.getStackTrace()
	entry += "\n"

	// Write to file
	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to error log: %w", err)
	}

	return nil
}

// rotateIfNeeded checks if the log file exceeds the size limit and rotates if necessary
func (e *ErrorLogger) rotateIfNeeded() error {
	// Check if log file exists and get its size
	info, err := os.Stat(e.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, no rotation needed
			return nil
		}
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	// Check if rotation is needed
	if info.Size() < e.maxSize {
		return nil
	}

	// Perform rotation
	return e.rotate()
}

// rotate performs the actual log rotation
func (e *ErrorLogger) rotate() error {
	// Delete the oldest log file if it exists (error.log.5)
	oldestLog := fmt.Sprintf("%s.%d", e.logPath, e.maxFiles)
	if _, err := os.Stat(oldestLog); err == nil {
		if err := os.Remove(oldestLog); err != nil {
			return fmt.Errorf("failed to remove oldest log: %w", err)
		}
	}

	// Shift existing rotated logs (error.log.4 -> error.log.5, etc.)
	for i := e.maxFiles - 1; i >= 1; i-- {
		oldName := fmt.Sprintf("%s.%d", e.logPath, i)
		newName := fmt.Sprintf("%s.%d", e.logPath, i+1)

		if _, err := os.Stat(oldName); err == nil {
			if err := os.Rename(oldName, newName); err != nil {
				return fmt.Errorf("failed to rotate log %s to %s: %w", oldName, newName, err)
			}
		}
	}

	// Rename current log to error.log.1
	rotatedName := fmt.Sprintf("%s.1", e.logPath)
	if err := os.Rename(e.logPath, rotatedName); err != nil {
		return fmt.Errorf("failed to rotate current log: %w", err)
	}

	return nil
}

// getStackTrace captures the current stack trace
func (e *ErrorLogger) getStackTrace() string {
	const maxStackDepth = 32
	stackBuf := make([]uintptr, maxStackDepth)
	length := runtime.Callers(3, stackBuf) // Skip 3 frames: Callers, getStackTrace, LogError
	stack := stackBuf[:length]

	var trace string
	frames := runtime.CallersFrames(stack)
	for {
		frame, more := frames.Next()
		trace += fmt.Sprintf("  at %s (%s:%d)\n", frame.Function, filepath.Base(frame.File), frame.Line)
		if !more {
			break
		}
	}

	return trace
}

// EnsureLogDirectory creates the log directory if it doesn't exist
func EnsureLogDirectory(logPath string) error {
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	return nil
}
