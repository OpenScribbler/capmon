package output

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestNewStructuredError_Fields(t *testing.T) {
	se := NewStructuredError(ErrInputInvalid, "invalid --provider: bad slug", "use a lowercase kebab-case slug")
	if se.Code != ErrInputInvalid {
		t.Errorf("Code = %q, want %q", se.Code, ErrInputInvalid)
	}
	if se.Message != "invalid --provider: bad slug" {
		t.Errorf("Message = %q", se.Message)
	}
	if se.Suggestion != "use a lowercase kebab-case slug" {
		t.Errorf("Suggestion = %q", se.Suggestion)
	}
	if se.Details != "" {
		t.Errorf("Details should be empty, got %q", se.Details)
	}
}

func TestStructuredError_ErrorInterface(t *testing.T) {
	se := NewStructuredError(ErrInputInvalid, "invalid --provider: bad slug", "")
	got := se.Error()
	if !strings.Contains(got, "INPUT_003") {
		t.Errorf("Error() = %q, want code INPUT_003 in output", got)
	}
	if !strings.Contains(got, "invalid --provider: bad slug") {
		t.Errorf("Error() = %q, want message in output", got)
	}
}

func TestPrintStructuredError_PlainText(t *testing.T) {
	_, stderr := SetForTest(t)

	se := NewStructuredError(ErrInputMissing, "missing required flag", "pass --provider")
	PrintStructuredError(se)

	out := stderr.String()
	if !strings.Contains(out, "INPUT_001") {
		t.Errorf("plain text output missing code, got:\n%s", out)
	}
	if !strings.Contains(out, "missing required flag") {
		t.Errorf("plain text output missing message, got:\n%s", out)
	}
	if !strings.Contains(out, "pass --provider") {
		t.Errorf("plain text output missing suggestion, got:\n%s", out)
	}
}

func TestPrintStructuredError_JSON(t *testing.T) {
	_, stderr := SetForTest(t)
	JSON = true

	se := NewStructuredError(ErrInputConflict, "flags conflict", "pass only one")
	PrintStructuredError(se)

	out := stderr.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if result["code"] != ErrInputConflict {
		t.Errorf("JSON code = %v, want %q", result["code"], ErrInputConflict)
	}
	if result["message"] != "flags conflict" {
		t.Errorf("JSON message = %v", result["message"])
	}
	if result["suggestion"] != "pass only one" {
		t.Errorf("JSON suggestion = %v", result["suggestion"])
	}
}

func TestPrintStructuredError_NoSuggestion(t *testing.T) {
	_, stderr := SetForTest(t)

	se := NewStructuredError(ErrInputTerminal, "requires interactive terminal", "")
	PrintStructuredError(se)

	out := stderr.String()
	if strings.Contains(out, "Suggestion:") {
		t.Errorf("output should omit Suggestion line when empty, got:\n%s", out)
	}
	if !strings.Contains(out, "INPUT_004") {
		t.Errorf("output missing code, got:\n%s", out)
	}
}

func TestPrintStructuredError_Details(t *testing.T) {
	_, stderr := SetForTest(t)

	se := StructuredError{
		Code:    ErrInputInvalid,
		Message: "invalid value",
		Details: "line one\nline two",
	}
	PrintStructuredError(se)

	out := stderr.String()
	if !strings.Contains(out, "  line one\n") || !strings.Contains(out, "  line two\n") {
		t.Errorf("output should indent each detail line, got:\n%s", out)
	}
}

func TestAllErrorCodes_UniqueValues(t *testing.T) {
	seen := make(map[string]bool)
	for _, val := range AllErrorCodes() {
		if seen[val] {
			t.Errorf("duplicate error code value %q", val)
		}
		seen[val] = true
	}
}

func TestAllErrorCodes_Format(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Z]+_\d{3}$`)
	for _, val := range AllErrorCodes() {
		if !pattern.MatchString(val) {
			t.Errorf("error code %q does not match CATEGORY_NNN pattern", val)
		}
	}
}
