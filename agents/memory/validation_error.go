package memory

import stdErrors "errors"

// ValidationErrorCode is a stable error code for input validation failures.
type ValidationErrorCode string

const (
	// ValidationErrorCodeProjectRequired indicates missing project value.
	ValidationErrorCodeProjectRequired ValidationErrorCode = "PROJECT_REQUIRED"
	// ValidationErrorCodeSessionIDRequired indicates missing session ID value.
	ValidationErrorCodeSessionIDRequired ValidationErrorCode = "SESSION_ID_REQUIRED"
	// ValidationErrorCodeSessionIDInvalid indicates a malformed session ID value
	// that is unsafe for use as a single path segment (e.g. contains path
	// separators, control characters, or is a traversal token like "." / "..").
	ValidationErrorCodeSessionIDInvalid ValidationErrorCode = "SESSION_ID_INVALID"
	// ValidationErrorCodeTurnIDRequired indicates missing turn ID value.
	ValidationErrorCodeTurnIDRequired ValidationErrorCode = "TURN_ID_REQUIRED"
	// ValidationErrorCodeCurrentInputRequired indicates missing current input items.
	ValidationErrorCodeCurrentInputRequired ValidationErrorCode = "CURRENT_INPUT_REQUIRED"
)

// ValidationError represents one deterministic input validation failure.
type ValidationError struct {
	Code    ValidationErrorCode
	Field   string
	Message string
}

// Error returns a deterministic string with stable code and message.
func (err *ValidationError) Error() string {
	if err == nil {
		return ""
	}

	if err.Message == "" {
		return string(err.Code)
	}

	if err.Code == "" {
		return err.Message
	}

	return string(err.Code) + ": " + err.Message
}

// newValidationError creates one validation error from code, field, and message.
func newValidationError(code ValidationErrorCode, field, message string) *ValidationError {
	return &ValidationError{Code: code, Field: field, Message: message}
}

// IsValidationError reports whether err or its wrapped chain contains a validation error.
func IsValidationError(err error) bool {
	var validationErr *ValidationError

	return stdErrors.As(err, &validationErr)
}

// ValidationErrorCodeFromError extracts the first validation error code from err chain.
func ValidationErrorCodeFromError(err error) (ValidationErrorCode, bool) {
	var validationErr *ValidationError
	if !stdErrors.As(err, &validationErr) || validationErr == nil || validationErr.Code == "" {
		return "", false
	}

	return validationErr.Code, true
}
