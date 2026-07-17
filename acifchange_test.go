package capmon

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecordUnmappedForm_ThresholdCreatesIssue(t *testing.T) {
	cacheRoot := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	base := UnmappedFormObservation{
		DiagnosticID: "acif.rule.activation_mode_unmappable",
		Provider:     "claude-code",
		SourceForm:   "background-agent",
		ObservedAt:   now,
	}

	var calls [][]string
	var createBody string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		calls = append(calls, copyArgs(args))
		if isIssueListCall(args) {
			return []byte(`[]`), nil
		}
		if isGH(args, "api", "-X") {
			return nil, nil // provider label ensure
		}
		if isGH(args, "issue", "create") {
			createBody = argValue(args, "--body")
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/123\n"), nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	first := base
	first.ContentItem = "doc-a"
	issueNum, err := RecordUnmappedForm(cacheRoot, first)
	if err != nil {
		t.Fatalf("RecordUnmappedForm first: %v", err)
	}
	if issueNum != 0 {
		t.Fatalf("issueNum after one content item = %d, want 0", issueNum)
	}
	if len(calls) != 0 {
		t.Fatalf("gh called below threshold: %v", calls)
	}

	second := base
	second.ContentItem = "doc-b"
	issueNum, err = RecordUnmappedForm(cacheRoot, second)
	if err != nil {
		t.Fatalf("RecordUnmappedForm second: %v", err)
	}
	if issueNum != 123 {
		t.Fatalf("issueNum = %d, want 123", issueNum)
	}

	createCall := findGHCall(calls, "issue", "create")
	if createCall == nil {
		t.Fatal("missing gh issue create call")
	}
	if !hasArgPair(createCall, "--repo", acifChangeRepo) {
		t.Fatalf("create missing --repo %s: %v", acifChangeRepo, createCall)
	}
	labels := labelValues(createCall)
	for _, want := range []string{acifChangeLabel, acifClassBLabel, "provider:claude-code"} {
		if !labels[want] {
			t.Errorf("create missing label %q in %v", want, createCall)
		}
	}

	keyHash := acifUnmappedKeyHash(base.DiagnosticID, "claude-code", base.SourceForm)
	anchor := acifUnmappedAnchor(base.DiagnosticID, "claude-code", keyHash)
	if !strings.Contains(createBody, anchor) {
		t.Fatalf("issue body missing anchor %q\n\n%s", anchor, createBody)
	}
	if !strings.Contains(createBody, "```\nbackground-agent\n```") {
		t.Fatalf("issue body missing fenced source form:\n%s", createBody)
	}

	state, err := readUnmappedObservationState(acifUnmappedStatePath(cacheRoot, "claude-code", keyHash))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.IssueNumber != 123 || state.FireCount != 2 || len(state.ContentItems) != 2 {
		t.Fatalf("state = %+v, want issue 123, fire count 2, two content items", state)
	}
}

func TestRecordUnmappedForm_DedupCommentsExistingIssue(t *testing.T) {
	cacheRoot := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	base := UnmappedFormObservation{
		DiagnosticID: "acif.rule.activation_mode_unmappable",
		Provider:     "claude-code",
		SourceForm:   "background-agent",
		ObservedAt:   now,
	}

	var creates int
	var comments int
	var commentedIssue string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isIssueListCall(args) {
			return []byte(`[]`), nil
		}
		if isGH(args, "api", "-X") {
			return nil, nil // provider label ensure
		}
		if isGH(args, "issue", "create") {
			creates++
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/77\n"), nil
		}
		if isGH(args, "issue", "comment") {
			comments++
			commentedIssue = args[2]
			if !hasArgPair(args, "--repo", acifChangeRepo) {
				t.Fatalf("comment missing --repo %s: %v", acifChangeRepo, args)
			}
			return nil, nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	for _, item := range []string{"doc-a", "doc-b", "doc-c"} {
		obs := base
		obs.ContentItem = item
		if _, err := RecordUnmappedForm(cacheRoot, obs); err != nil {
			t.Fatalf("RecordUnmappedForm %s: %v", item, err)
		}
	}
	if creates != 1 {
		t.Fatalf("creates = %d, want 1", creates)
	}
	if comments != 1 || commentedIssue != "77" {
		t.Fatalf("comments = %d on issue %q, want one on 77", comments, commentedIssue)
	}
}

func TestRecordUnmappedForm_StateLostAnchorFoundComments(t *testing.T) {
	cacheRoot := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	base := UnmappedFormObservation{
		DiagnosticID: "acif.rule.activation_mode_uninferable",
		Provider:     "claude-code",
		SourceForm:   "mystery-mode",
		ObservedAt:   now,
	}
	keyHash := acifUnmappedKeyHash(base.DiagnosticID, "claude-code", base.SourceForm)
	anchor := acifUnmappedAnchor(base.DiagnosticID, "claude-code", keyHash)

	var creates int
	var comments int
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isIssueListCall(args) {
			if !hasArg(args, "repos/"+acifChangeRepo+"/issues") {
				t.Fatalf("list missing repos/%s/issues path: %v", acifChangeRepo, args)
			}
			return []byte(`[{"number":88,"body":"` + anchor + `"}]`), nil
		}
		if isGH(args, "issue", "comment") {
			comments++
			if args[2] != "88" {
				t.Fatalf("comment issue = %q, want 88", args[2])
			}
			return nil, nil
		}
		if isGH(args, "api", "-X") {
			return nil, nil // provider label ensure
		}
		if isGH(args, "issue", "create") {
			creates++
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/99\n"), nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	first := base
	first.ContentItem = "doc-a"
	if issueNum, err := RecordUnmappedForm(cacheRoot, first); err != nil {
		t.Fatalf("RecordUnmappedForm first: %v", err)
	} else if issueNum != 0 {
		t.Fatalf("issueNum after one content item = %d, want 0", issueNum)
	}

	second := base
	second.ContentItem = "doc-b"
	issueNum, err := RecordUnmappedForm(cacheRoot, second)
	if err != nil {
		t.Fatalf("RecordUnmappedForm second: %v", err)
	}
	if issueNum != 88 {
		t.Fatalf("issueNum = %d, want 88", issueNum)
	}
	if creates != 0 {
		t.Fatalf("create called %d times, want 0", creates)
	}
	if comments != 1 {
		t.Fatalf("comments = %d, want 1", comments)
	}

	state, err := readUnmappedObservationState(acifUnmappedStatePath(cacheRoot, "claude-code", keyHash))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.IssueNumber != 88 {
		t.Fatalf("state issue = %d, want 88", state.IssueNumber)
	}
}

func TestMarkStaleFilings_UnmappedAddsStaleOnce(t *testing.T) {
	cacheRoot := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	keyHash := acifUnmappedKeyHash("acif.rule.activation_mode_unmappable", "claude-code", "old-mode")
	state := unmappedObservationState{
		DiagnosticID:  "acif.rule.activation_mode_unmappable",
		Provider:      "claude-code",
		SourceForm:    "old-mode",
		ContentItems:  map[string]bool{"doc-a": true, "doc-b": true},
		FireCount:     2,
		FirstObserved: now.Add(-40 * 24 * time.Hour),
		LastObserved:  now.Add(-31 * 24 * time.Hour),
		IssueNumber:   55,
	}
	statePath := acifUnmappedStatePath(cacheRoot, "claude-code", keyHash)
	if err := writeJSONState(statePath, &state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	var edits int
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isGH(args, "issue", "edit") {
			edits++
			if args[2] != "55" {
				t.Fatalf("edit issue = %q, want 55", args[2])
			}
			if !hasArgPair(args, "--repo", acifChangeRepo) || !hasArgPair(args, "--add-label", acifStaleLabel) {
				t.Fatalf("edit missing repo or stale label: %v", args)
			}
			return nil, nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	if err := MarkStaleFilings(cacheRoot, now); err != nil {
		t.Fatalf("MarkStaleFilings first: %v", err)
	}
	if err := MarkStaleFilings(cacheRoot, now); err != nil {
		t.Fatalf("MarkStaleFilings second: %v", err)
	}
	if edits != 1 {
		t.Fatalf("edits = %d, want 1", edits)
	}
	roundTrip, err := readUnmappedObservationState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !roundTrip.StaleApplied {
		t.Fatal("stale_applied was not persisted")
	}
}

func TestRecordUnmappedForm_StateRoundTripThroughJSON(t *testing.T) {
	cacheRoot := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	obs := UnmappedFormObservation{
		DiagnosticID: "acif.rule.kind_unmappable",
		Provider:     "claude-code",
		SourceForm:   "one-shot",
		ContentItem:  "doc-a",
		ObservedAt:   now,
	}
	if issueNum, err := RecordUnmappedForm(cacheRoot, obs); err != nil {
		t.Fatalf("RecordUnmappedForm first: %v", err)
	} else if issueNum != 0 {
		t.Fatalf("issueNum = %d, want 0", issueNum)
	}

	keyHash := acifUnmappedKeyHash(obs.DiagnosticID, "claude-code", obs.SourceForm)
	statePath := acifUnmappedStatePath(cacheRoot, "claude-code", keyHash)
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state was not written: %v", err)
	}
	if !strings.Contains(string(data), `"doc-a": true`) {
		t.Fatalf("state file missing doc-a content item:\n%s", data)
	}

	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isIssueListCall(args) {
			return []byte(`[]`), nil
		}
		if isGH(args, "api", "-X") {
			return nil, nil // provider label ensure
		}
		if isGH(args, "issue", "create") {
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/66\n"), nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	obs.ContentItem = "doc-b"
	if issueNum, err := RecordUnmappedForm(cacheRoot, obs); err != nil {
		t.Fatalf("RecordUnmappedForm second: %v", err)
	} else if issueNum != 66 {
		t.Fatalf("issueNum = %d, want 66", issueNum)
	}
	state, err := readUnmappedObservationState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(state.ContentItems) != 2 || state.FireCount != 2 || state.IssueNumber != 66 {
		t.Fatalf("state = %+v, want two content items, fire count 2, issue 66", state)
	}
}

func copyArgs(args []string) []string {
	cp := make([]string, len(args))
	copy(cp, args)
	return cp
}

func isGH(args []string, first, second string) bool {
	return len(args) >= 2 && args[0] == first && args[1] == second
}

// isIssueListCall reports whether args is the exhaustive open-issue listing
// call (`gh api --paginate ... /issues`) that replaced `gh issue list` for
// dedup anchor lookups.
func isIssueListCall(args []string) bool {
	return isGH(args, "api", "--paginate")
}

func findGHCall(calls [][]string, first, second string) []string {
	for _, call := range calls {
		if isGH(call, first, second) {
			return call
		}
	}
	return nil
}

func argValue(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func hasArg(args []string, val string) bool {
	for _, a := range args {
		if a == val {
			return true
		}
	}
	return false
}

func hasArgPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

func labelValues(args []string) map[string]bool {
	labels := make(map[string]bool)
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--label" {
			labels[args[i+1]] = true
		}
	}
	return labels
}

func TestCreateACIFUnmappedIssue_EnsuresProviderLabelFirst(t *testing.T) {
	var calls [][]string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		calls = append(calls, copyArgs(args))
		if isGH(args, "api", "-X") {
			return nil, nil
		}
		if isGH(args, "issue", "create") {
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/44\n"), nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	state := unmappedObservationState{
		DiagnosticID: "acif.rule.activation_mode_unmappable",
		Provider:     "claude-code",
		SourceForm:   "background-agent",
		ContentItems: map[string]bool{"a": true, "b": true},
	}
	if _, err := createACIFUnmappedIssue(state, "deadbeef"); err != nil {
		t.Fatalf("createACIFUnmappedIssue: %v", err)
	}
	if len(calls) != 2 || !isGH(calls[0], "api", "-X") || !isGH(calls[1], "issue", "create") {
		t.Fatalf("want label ensure then create, got: %v", calls)
	}
	if !hasArg(calls[0], "name=provider:claude-code") {
		t.Fatalf("label ensure missing provider label name: %v", calls[0])
	}
}

func TestEnsureACIFLabel_TreatsAlreadyExistsAsSuccess(t *testing.T) {
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1: gh: Validation Failed (HTTP 422) already_exists")
	})
	defer SetGHCommandForTest(nil)

	if err := ensureACIFLabel("provider:claude-code", "ededed", "test"); err != nil {
		t.Fatalf("ensureACIFLabel on already_exists: %v", err)
	}
}

func TestEnsureACIFLabel_SurfacesRealErrors(t *testing.T) {
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1: gh: Not Found (HTTP 404)")
	})
	defer SetGHCommandForTest(nil)

	if err := ensureACIFLabel("provider:claude-code", "ededed", "test"); err == nil {
		t.Fatal("expected real gh error to surface")
	}
}
