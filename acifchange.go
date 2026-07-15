package capmon

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	acifChangeRepo   = "holdenhewett/agent-content-interchange-format"
	acifChangeLabel  = "acif-change"
	acifClassBLabel  = "class-b"
	acifClassCLabel  = "class-c"
	acifStaleLabel   = "stale"
	acifStateDir     = "acif-change"
	acifStaleAfter   = 30 * 24 * time.Hour
	acifKeySeparator = "\x00"
)

// UnmappedFormObservation is one observed totality-net hit on real provider content.
type UnmappedFormObservation struct {
	DiagnosticID string // e.g. "acif.rule.activation_mode_unmappable"
	Provider     string // provider slug
	SourceForm   string // the unmapped source-form value, verbatim
	ContentItem  string // stable identifier of the content item it fired on
	ObservedAt   time.Time
}

type unmappedObservationState struct {
	DiagnosticID  string          `json:"diagnostic_id"`
	Provider      string          `json:"provider"`
	SourceForm    string          `json:"source_form"`
	ContentItems  map[string]bool `json:"content_items"`
	FireCount     int             `json:"fire_count"`
	FirstObserved time.Time       `json:"first_observed"`
	LastObserved  time.Time       `json:"last_observed"`
	IssueNumber   int             `json:"issue_number,omitempty"`
	StaleApplied  bool            `json:"stale_applied,omitempty"`
}

// RecordUnmappedForm persists the observation and files or refreshes the
// ACIF class-b issue once the distinct-content-item threshold is met.
//
// This is the exported intake point for the sweep pipeline; the caller that
// detects totality-net diagnostics will be wired in separately.
//
// Returns the issue number when an issue was created or updated this call.
func RecordUnmappedForm(cacheRoot string, obs UnmappedFormObservation) (int, error) {
	provider, err := SanitizeSlug(obs.Provider)
	if err != nil {
		return 0, err
	}
	if obs.DiagnosticID == "" {
		return 0, fmt.Errorf("diagnostic id is required")
	}
	if obs.ContentItem == "" {
		return 0, fmt.Errorf("content item is required")
	}
	observedAt := obs.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	observedAt = observedAt.UTC()

	keyHash := acifUnmappedKeyHash(obs.DiagnosticID, provider, obs.SourceForm)
	path := acifUnmappedStatePath(cacheRoot, provider, keyHash)
	state, err := readUnmappedObservationState(path)
	if err != nil {
		return 0, err
	}
	if state.DiagnosticID == "" {
		state.DiagnosticID = obs.DiagnosticID
		state.Provider = provider
		state.SourceForm = obs.SourceForm
		state.ContentItems = make(map[string]bool)
	} else if state.DiagnosticID != obs.DiagnosticID || state.Provider != provider || state.SourceForm != obs.SourceForm {
		return 0, fmt.Errorf("state key mismatch in %s", path)
	}
	if state.ContentItems == nil {
		state.ContentItems = make(map[string]bool)
	}

	state.FireCount++
	state.ContentItems[obs.ContentItem] = true
	if state.FirstObserved.IsZero() || observedAt.Before(state.FirstObserved) {
		state.FirstObserved = observedAt
	}
	if state.LastObserved.IsZero() || observedAt.After(state.LastObserved) {
		state.LastObserved = observedAt
	}

	if err := writeJSONState(path, &state); err != nil {
		return 0, err
	}
	if len(state.ContentItems) < 2 {
		return 0, nil
	}

	if state.IssueNumber > 0 {
		if err := appendACIFUnmappedComment(state.IssueNumber, state); err != nil {
			return state.IssueNumber, err
		}
		return state.IssueNumber, nil
	}

	issueNum, found, err := findOpenACIFUnmappedIssue(state.DiagnosticID, provider, keyHash)
	if err != nil {
		return 0, err
	}
	if found {
		state.IssueNumber = issueNum
		if err := writeJSONState(path, &state); err != nil {
			return 0, err
		}
		if err := appendACIFUnmappedComment(issueNum, state); err != nil {
			return issueNum, err
		}
		return issueNum, nil
	}

	issueNum, err = createACIFUnmappedIssue(state, keyHash)
	if err != nil {
		return 0, err
	}
	state.IssueNumber = issueNum
	if err := writeJSONState(path, &state); err != nil {
		return 0, err
	}
	return issueNum, nil
}

func acifUnmappedKeyHash(diagnosticID, provider, sourceForm string) string {
	sum := sha256.Sum256([]byte(diagnosticID + acifKeySeparator + provider + acifKeySeparator + sourceForm))
	return fmt.Sprintf("%x", sum[:8])
}

func acifUnmappedStatePath(cacheRoot, provider, keyHash string) string {
	return filepath.Join(cacheRoot, acifStateDir, "unmapped", provider, keyHash+".json")
}

func acifUnmappedAnchor(diagnosticID, provider, keyHash string) string {
	return fmt.Sprintf("<!-- capmon-acif-change: %s/%s/%s -->", diagnosticID, provider, keyHash)
}

func readUnmappedObservationState(path string) (unmappedObservationState, error) {
	var state unmappedObservationState
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read ACIF unmapped state %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse ACIF unmapped state %s: %w", path, err)
	}
	return state, nil
}

func findOpenACIFUnmappedIssue(diagnosticID, provider, keyHash string) (int, bool, error) {
	anchor := acifUnmappedAnchor(diagnosticID, provider, keyHash)
	out, err := ghRunner("issue", "list",
		"--repo", acifChangeRepo,
		"--label", acifChangeLabel,
		"--label", acifClassBLabel,
		"--label", "provider:"+provider,
		"--state", "open",
		"--limit", "100",
		"--json", "number,body",
	)
	if err != nil {
		return 0, false, fmt.Errorf("gh issue list: %w", err)
	}
	var issues []struct {
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return 0, false, fmt.Errorf("parse issue list: %w", err)
	}
	for _, iss := range issues {
		if strings.Contains(iss.Body, anchor) {
			return iss.Number, true, nil
		}
	}
	return 0, false, nil
}

func createACIFUnmappedIssue(state unmappedObservationState, keyHash string) (int, error) {
	title := fmt.Sprintf("acif-change: %s on %s (class-b candidate)", state.DiagnosticID, state.Provider)
	body := buildACIFUnmappedIssueBody(state, keyHash)
	out, err := ghRunner("issue", "create",
		"--repo", acifChangeRepo,
		"--title", title,
		"--label", acifChangeLabel,
		"--label", acifClassBLabel,
		"--label", "provider:"+state.Provider,
		"--body", body,
	)
	if err != nil {
		return 0, fmt.Errorf("gh issue create: %w", err)
	}
	return parseGHIssueNumber(out)
}

func buildACIFUnmappedIssueBody(state unmappedObservationState, keyHash string) string {
	var b strings.Builder
	b.WriteString(acifUnmappedAnchor(state.DiagnosticID, state.Provider, keyHash))
	b.WriteString("\n\nA totality-net diagnostic fired on real provider content with no ACIF Class B mapping row.\n\n")
	fmt.Fprintf(&b, "**Diagnostic ID:** `%s`\n", state.DiagnosticID)
	fmt.Fprintf(&b, "**Provider:** `%s`\n", state.Provider)
	fmt.Fprintf(&b, "**Distinct content items:** %d\n", len(state.ContentItems))
	fmt.Fprintf(&b, "**Fire count:** %d\n", state.FireCount)
	fmt.Fprintf(&b, "**First seen:** `%s`\n", formatACIFTime(state.FirstObserved))
	fmt.Fprintf(&b, "**Last seen:** `%s`\n\n", formatACIFTime(state.LastObserved))
	b.WriteString("**Source form:**\n\n")
	b.WriteString(fencedACIFBlock(state.SourceForm))
	b.WriteString("\n\n**What to do:** Review the ACIF `CHANGE-PROCESS.md` Class B section and decide whether to add a mapping row for this source form.\n")
	b.WriteString("Reference: https://github.com/holdenhewett/agent-content-interchange-format/blob/main/CHANGE-PROCESS.md\n")
	return b.String()
}

func appendACIFUnmappedComment(issueNum int, state unmappedObservationState) error {
	body := fmt.Sprintf(
		"Observed again: fire count `%d`, distinct content items `%d`, last seen `%s`.",
		state.FireCount,
		len(state.ContentItems),
		formatACIFTime(state.LastObserved),
	)
	if _, err := ghRunner("issue", "comment",
		strconv.Itoa(issueNum),
		"--repo", acifChangeRepo,
		"--body", body,
	); err != nil {
		return fmt.Errorf("gh issue comment: %w", err)
	}
	return nil
}

func writeJSONState(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func parseGHIssueNumber(out []byte) (int, error) {
	issueURL := strings.TrimSpace(string(out))
	parts := strings.Split(issueURL, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("unexpected gh issue output: %q", issueURL)
	}
	num, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("parse issue number from %q: %w", issueURL, err)
	}
	return num, nil
}

func formatACIFTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func fencedACIFBlock(value string) string {
	fence := "```"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	return fence + "\n" + value + "\n" + fence
}

// MarkStaleFilings adds the stale label to filed ACIF-change issues whose
// tracked form or extension has not been observed for 30 or more days.
func MarkStaleFilings(cacheRoot string, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	if err := markStaleUnmappedFilings(cacheRoot, now); err != nil {
		return err
	}
	if err := markStaleGraduationFilings(cacheRoot, now); err != nil {
		return err
	}
	return nil
}

func markStaleUnmappedFilings(cacheRoot string, now time.Time) error {
	root := filepath.Join(cacheRoot, acifStateDir, "unmapped")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat ACIF unmapped state root: %w", err)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		state, err := readUnmappedObservationState(path)
		if err != nil {
			return err
		}
		if !shouldMarkACIFStale(state.IssueNumber, state.LastObserved, state.StaleApplied, now) {
			return nil
		}
		if err := addACIFStaleLabel(state.IssueNumber); err != nil {
			return err
		}
		state.StaleApplied = true
		return writeJSONState(path, &state)
	})
}

func shouldMarkACIFStale(issueNumber int, lastObserved time.Time, staleApplied bool, now time.Time) bool {
	if issueNumber == 0 || staleApplied || lastObserved.IsZero() {
		return false
	}
	return !lastObserved.After(now.Add(-acifStaleAfter))
}

func addACIFStaleLabel(issueNumber int) error {
	if _, err := ghRunner("issue", "edit",
		strconv.Itoa(issueNumber),
		"--repo", acifChangeRepo,
		"--add-label", acifStaleLabel,
	); err != nil {
		return fmt.Errorf("gh issue edit stale label: %w", err)
	}
	return nil
}
