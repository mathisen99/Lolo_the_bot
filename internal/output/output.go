package output

import (
	"fmt"
)

// Output combines colored terminal logging with file-based error logging
type Output struct {
	Logger      Logger
	ErrorLogger *ErrorLogger
}

// NewOutput creates a new Output with both terminal and file logging
func NewOutput(errorLogPath string) (*Output, error) {
	// Ensure log directory exists
	if err := EnsureLogDirectory(errorLogPath); err != nil {
		return nil, fmt.Errorf("failed to ensure log directory: %w", err)
	}

	return &Output{
		Logger:      NewColorLogger(),
		ErrorLogger: NewErrorLogger(errorLogPath),
	}, nil
}

// LogErrorToFile logs an error to the file-based error log
// This is a convenience method that also prints to terminal
func (o *Output) LogErrorToFile(errorType, errorMessage string, err error) {
	// Log to terminal
	if err != nil {
		o.Logger.Error("%s: %s - %v", errorType, errorMessage, err)
	} else {
		o.Logger.Error("%s: %s", errorType, errorMessage)
	}

	// Log to file
	if logErr := o.ErrorLogger.LogError(errorType, errorMessage, err); logErr != nil {
		// If we can't log to file, at least print to terminal
		o.Logger.Error("Failed to write to error log: %v", logErr)
	}
}
