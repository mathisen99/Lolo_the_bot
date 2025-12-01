package errors

import (
	"fmt"

	"github.com/yourusername/lolo/internal/output"
)

// ErrorHandler handles errors by logging them and returning user-friendly messages
type ErrorHandler struct {
	output *output.Output
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(output *output.Output) *ErrorHandler {
	return &ErrorHandler{
		output: output,
	}
}

// Handle processes an error and returns a user-friendly message
// It logs the error to both terminal and file (Requirement 22.5)
func (h *ErrorHandler) Handle(err error) string {
	if err == nil {
		return ""
	}

	// Check if it's a BotError
	if botErr, ok := AsBotError(err); ok {
		return h.handleBotError(botErr)
	}

	// Handle generic errors
	return h.handleGenericError(err)
}

// handleBotError processes a structured BotError
func (h *ErrorHandler) handleBotError(err *BotError) string {
	// Log to file with full details
	h.output.LogErrorToFile(
		string(err.Type),
		err.UserMessage,
		err.InternalError,
	)

	// Return user-friendly message
	return err.UserMessage
}

// handleGenericError processes a generic error
func (h *ErrorHandler) handleGenericError(err error) string {
	// Log to file
	h.output.LogErrorToFile(
		string(ErrorTypeUnexpected),
		"Unexpected error occurred",
		err,
	)

	// Return generic user message
	return "An unexpected error occurred. Please try again later."
}

// HandleWithContext processes an error with additional context
func (h *ErrorHandler) HandleWithContext(err error, context string) string {
	if err == nil {
		return ""
	}

	// Add context to logging
	contextualErr := fmt.Errorf("%s: %w", context, err)

	// Check if it's a BotError
	if botErr, ok := AsBotError(err); ok {
		// Log with context
		h.output.LogErrorToFile(
			string(botErr.Type),
			fmt.Sprintf("%s: %s", context, botErr.UserMessage),
			contextualErr,
		)
		return botErr.UserMessage
	}

	// Handle generic errors with context
	h.output.LogErrorToFile(
		string(ErrorTypeUnexpected),
		fmt.Sprintf("%s: unexpected error", context),
		contextualErr,
	)

	return "An unexpected error occurred. Please try again later."
}

// LogError logs an error without returning a message (for non-critical errors)
func (h *ErrorHandler) LogError(err error, context string) {
	if err == nil {
		return
	}

	contextualErr := fmt.Errorf("%s: %w", context, err)

	if botErr, ok := AsBotError(err); ok {
		h.output.LogErrorToFile(
			string(botErr.Type),
			fmt.Sprintf("%s: %s", context, botErr.UserMessage),
			contextualErr,
		)
	} else {
		h.output.LogErrorToFile(
			string(ErrorTypeUnexpected),
			context,
			contextualErr,
		)
	}
}
