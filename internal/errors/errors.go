package errors

import (
	"fmt"

	"github.com/yourusername/lolo/internal/database"
)

// ErrorType represents the category of error
type ErrorType string

const (
	// ErrorTypeInvalidSyntax indicates a command syntax error
	ErrorTypeInvalidSyntax ErrorType = "InvalidSyntax"

	// ErrorTypePermission indicates insufficient permissions
	ErrorTypePermission ErrorType = "Permission"

	// ErrorTypeDatabase indicates a database operation failure
	ErrorTypeDatabase ErrorType = "Database"

	// ErrorTypeAPI indicates an API communication failure
	ErrorTypeAPI ErrorType = "API"

	// ErrorTypeUnexpected indicates an unexpected/unknown error
	ErrorTypeUnexpected ErrorType = "Unexpected"

	// ErrorTypeNotFound indicates a resource was not found
	ErrorTypeNotFound ErrorType = "NotFound"

	// ErrorTypeValidation indicates invalid input data
	ErrorTypeValidation ErrorType = "Validation"
)

// BotError represents a structured error with type and user-friendly message
type BotError struct {
	Type           ErrorType
	UserMessage    string // Message to send to the user
	InternalError  error  // Original error for logging
	InternalDetail string // Additional detail for logging
}

// Error implements the error interface
func (e *BotError) Error() string {
	if e.InternalError != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Type, e.UserMessage, e.InternalError)
	}
	if e.InternalDetail != "" {
		return fmt.Sprintf("%s: %s (detail: %s)", e.Type, e.UserMessage, e.InternalDetail)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.UserMessage)
}

// Unwrap returns the underlying error
func (e *BotError) Unwrap() error {
	return e.InternalError
}

// NewInvalidSyntaxError creates an error for invalid command syntax
// Requirement 22.1: Clear error messages for invalid syntax
func NewInvalidSyntaxError(commandName, correctSyntax string) *BotError {
	return &BotError{
		Type:           ErrorTypeInvalidSyntax,
		UserMessage:    fmt.Sprintf("Invalid syntax. Usage: %s", correctSyntax),
		InternalDetail: fmt.Sprintf("command=%s", commandName),
	}
}

// NewPermissionError creates an error for insufficient permissions
// Requirement 22.2: Permission error messages indicating required level
func NewPermissionError(requiredLevel database.PermissionLevel) *BotError {
	levelName := PermissionLevelName(requiredLevel)
	return &BotError{
		Type:           ErrorTypePermission,
		UserMessage:    fmt.Sprintf("Insufficient permissions. This command requires %s level.", levelName),
		InternalDetail: fmt.Sprintf("required_level=%s", levelName),
	}
}

// NewDatabaseError creates an error for database operation failures
// Requirement 22.3: Generic error messages for database failures
func NewDatabaseError(operation string, err error) *BotError {
	return &BotError{
		Type:           ErrorTypeDatabase,
		UserMessage:    "A database error occurred. Please try again later.",
		InternalError:  err,
		InternalDetail: fmt.Sprintf("operation=%s", operation),
	}
}

// NewAPIError creates an error for API communication failures
// Requirement 22.4: API unavailable messages
func NewAPIError(err error) *BotError {
	return &BotError{
		Type:          ErrorTypeAPI,
		UserMessage:   "Command service is temporarily unavailable. Please try again later.",
		InternalError: err,
	}
}

// NewUnexpectedError creates an error for unexpected failures
// Requirement 22.5: User-friendly error messages for unexpected errors
func NewUnexpectedError(err error) *BotError {
	return &BotError{
		Type:          ErrorTypeUnexpected,
		UserMessage:   "An unexpected error occurred. Please try again later.",
		InternalError: err,
	}
}

// NewNotFoundError creates an error for resources that don't exist
func NewNotFoundError(resourceType, resourceName string) *BotError {
	return &BotError{
		Type:           ErrorTypeNotFound,
		UserMessage:    fmt.Sprintf("%s '%s' not found.", resourceType, resourceName),
		InternalDetail: fmt.Sprintf("resource_type=%s, resource_name=%s", resourceType, resourceName),
	}
}

// NewValidationError creates an error for invalid input data
func NewValidationError(message string) *BotError {
	return &BotError{
		Type:        ErrorTypeValidation,
		UserMessage: message,
	}
}

// PermissionLevelName returns a human-readable name for a permission level
func PermissionLevelName(level database.PermissionLevel) string {
	switch level {
	case database.LevelIgnored:
		return "ignored"
	case database.LevelNormal:
		return "normal"
	case database.LevelAdmin:
		return "admin"
	case database.LevelOwner:
		return "owner"
	default:
		return "unknown"
	}
}

// IsBotError checks if an error is a BotError
func IsBotError(err error) bool {
	_, ok := err.(*BotError)
	return ok
}

// AsBotError attempts to convert an error to a BotError
func AsBotError(err error) (*BotError, bool) {
	botErr, ok := err.(*BotError)
	return botErr, ok
}
