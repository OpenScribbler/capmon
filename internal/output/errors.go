package output

// Error code constants. Each code is namespaced by category and numbered.
// Format: CATEGORY_NNN
const (
	// Input: CLI flag and argument validation.
	ErrInputMissing  = "INPUT_001" // required flag or argument not provided
	ErrInputConflict = "INPUT_002" // mutually exclusive flags used together
	ErrInputInvalid  = "INPUT_003" // flag value is invalid (wrong format, out of range)
	ErrInputTerminal = "INPUT_004" // command requires interactive terminal
)

// NewStructuredError creates a StructuredError with Code, Message, and Suggestion.
func NewStructuredError(code, message, suggestion string) StructuredError {
	return StructuredError{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
	}
}

// AllErrorCodes returns all registered error code values for validation.
// Used by tests to assert uniqueness and format correctness.
func AllErrorCodes() []string {
	return []string{
		ErrInputMissing,
		ErrInputConflict,
		ErrInputInvalid,
		ErrInputTerminal,
	}
}
