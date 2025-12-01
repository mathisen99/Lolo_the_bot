package splitter

import "fmt"

// SplitError represents an error during message splitting or sending
type SplitError struct {
	Code    string
	Message string
	Details string
}

// Error implements the error interface
func (e *SplitError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Predefined errors
var (
	ErrSplitNotFound = &SplitError{
		Code:    "SPLIT_NOT_FOUND",
		Message: "split operation not found",
	}

	ErrInvalidPartIndex = &SplitError{
		Code:    "INVALID_PART_INDEX",
		Message: "invalid part index for split operation",
	}

	ErrPartialSendFailure = &SplitError{
		Code:    "PARTIAL_SEND_FAILURE",
		Message: "message split partially sent before failure",
	}

	ErrAllPartsFailed = &SplitError{
		Code:    "ALL_PARTS_FAILED",
		Message: "all parts of split message failed to send",
	}
)

// NewPartialSendFailure creates a new partial send failure error
func NewPartialSendFailure(partsSent, totalParts int, lastError string) *SplitError {
	return &SplitError{
		Code:    "PARTIAL_SEND_FAILURE",
		Message: "message split partially sent before failure",
		Details: fmt.Sprintf("sent %d of %d parts, last error: %s", partsSent, totalParts, lastError),
	}
}

// NewAllPartsFailed creates a new all parts failed error
func NewAllPartsFailed(totalParts int, firstError string) *SplitError {
	return &SplitError{
		Code:    "ALL_PARTS_FAILED",
		Message: "all parts of split message failed to send",
		Details: fmt.Sprintf("total parts: %d, first error: %s", totalParts, firstError),
	}
}
