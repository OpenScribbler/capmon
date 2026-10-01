package capmon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanGraduationCandidates_DebouncedThresholdCreatesIssue(t *testing.T) {
	cacheRoot := t.TempDir()
	formatsDir := t.TempDir()
	canonicalKeysPath := writeGraduationCanonicalKeys(t)
	day1 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

	writeGraduationFormatDoc(t, formatsDir, "alpha", "shared_extension", "Alpha Shared", "Alpha summary")

	var creates int
	var comments int
	var createCall []string
	var createBody string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isIssueListCall(args) {
			if !hasArg(args, "repos/"+acifChangeRepo+"/issues") {
				t.Fatalf("list missing repos/%s/issues path: %v", acifChangeRepo, args)
			}
			return []byte(`[]`), nil
		}
		if isGH(args, "issue", "create") {
			creates++
			createCall = copyArgs(args)
			createBody = argValue(args, "--body")
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/321\n"), nil
		}
		if isGH(args, "issue", "comment") {
			comments++
			return nil, nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	issues, err := ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1)
	if err != nil {
		t.Fatalf("ScanGraduationCandidates day1: %v", err)
	}
	if len(issues) != 0 || creates != 0 {
		t.Fatalf("day1 issues=%v creates=%d, want none", issues, creates)
	}

	issues, err = ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ScanGraduationCandidates day2: %v", err)
	}
	if len(issues) != 0 || creates != 0 {
		t.Fatalf("one provider over two dates issues=%v creates=%d, want none", issues, creates)
	}

	writeGraduationFormatDoc(t, formatsDir, "beta", "shared_extension", "Beta Shared", "Beta summary")
	issues, err = ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, 2))
	if err != nil {
		t.Fatalf("ScanGraduationCandidates day3: %v", err)
	}
	if len(issues) != 0 || creates != 0 {
		t.Fatalf("beta has one scan date issues=%v creates=%d, want none", issues, creates)
	}

	issues, err = ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("ScanGraduationCandidates day4: %v", err)
	}
	if len(issues) != 1 || issues[0] != 321 {
		t.Fatalf("issues = %v, want [321]", issues)
	}
	if creates != 1 {
		t.Fatalf("creates = %d, want 1", creates)
	}
	if !hasArgPair(createCall, "--repo", acifChangeRepo) {
		t.Fatalf("create missing --repo %s: %v", acifChangeRepo, createCall)
	}
	labels := labelValues(createCall)
	for _, want := range []string{acifChangeLabel, acifClassCLabel} {
		if !labels[want] {
			t.Errorf("create missing label %q in %v", want, createCall)
		}
	}
	for label := range labels {
		if strings.HasPrefix(label, "provider:") {
			t.Fatalf("class-c issue should not have provider label: %v", createCall)
		}
	}
	for _, want := range []string{
		acifGraduationAnchor(sharedSkillKey),
		"`alpha`",
		"`beta`",
		"Alpha Shared",
		"Beta summary",
	} {
		if !strings.Contains(createBody, want) {
			t.Fatalf("body missing %q\n\n%s", want, createBody)
		}
	}

	state, err := readGraduationState(acifGraduationStatePath(cacheRoot, sharedSkillKey))
	if err != nil {
		t.Fatalf("read graduation state: %v", err)
	}
	if state.IssueNumber != 321 {
		t.Fatalf("state issue = %d, want 321", state.IssueNumber)
	}
	if len(state.Providers["alpha"].ScanDates) != 4 || len(state.Providers["beta"].ScanDates) != 2 {
		t.Fatalf("scan dates not persisted as expected: %+v", state.Providers)
	}

	issues, err = ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, 4))
	if err != nil {
		t.Fatalf("ScanGraduationCandidates day5: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("same qualifying set returned issues %v, want none", issues)
	}
	if creates != 1 {
		t.Fatalf("creates after re-scan = %d, want 1", creates)
	}
	if comments != 0 {
		t.Fatalf("comments after same qualifying set = %d, want 0", comments)
	}
}

func TestScanGraduationCandidates_CommentsWhenQualifyingSetChanges(t *testing.T) {
	cacheRoot := t.TempDir()
	formatsDir := t.TempDir()
	canonicalKeysPath := writeGraduationCanonicalKeys(t)
	day1 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	writeGraduationFormatDoc(t, formatsDir, "alpha", "shared_extension", "Alpha Shared", "Alpha summary")
	writeGraduationFormatDoc(t, formatsDir, "beta", "shared_extension", "Beta Shared", "Beta summary")

	var creates int
	var comments int
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isIssueListCall(args) {
			return []byte(`[]`), nil
		}
		if isGH(args, "issue", "create") {
			creates++
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/321\n"), nil
		}
		if isGH(args, "issue", "comment") {
			comments++
			if args[2] != "321" {
				t.Fatalf("comment issue = %q, want 321", args[2])
			}
			if !hasArgPair(args, "--repo", acifChangeRepo) {
				t.Fatalf("comment missing --repo %s: %v", acifChangeRepo, args)
			}
			return nil, nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	for _, day := range []time.Time{day1, day1.AddDate(0, 0, 1)} {
		if _, err := ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day); err != nil {
			t.Fatalf("ScanGraduationCandidates: %v", err)
		}
	}
	if creates != 1 {
		t.Fatalf("creates = %d, want 1", creates)
	}

	writeGraduationFormatDoc(t, formatsDir, "gamma", "shared_extension", "Gamma Shared", "Gamma summary")
	if issues, err := ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, 2)); err != nil {
		t.Fatalf("ScanGraduationCandidates gamma day1: %v", err)
	} else if len(issues) != 0 {
		t.Fatalf("gamma one scan date issues = %v, want none", issues)
	}
	if issues, err := ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, 3)); err != nil {
		t.Fatalf("ScanGraduationCandidates gamma day2: %v", err)
	} else if len(issues) != 1 || issues[0] != 321 {
		t.Fatalf("issues = %v, want [321]", issues)
	}
	if comments != 1 {
		t.Fatalf("comments = %d, want 1", comments)
	}
}

func TestMarkStaleFilings_GraduationAddsStaleOnce(t *testing.T) {
	cacheRoot := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	state := graduationState{
		ContentType:  "skills",
		ExtensionID:  "shared_extension",
		IssueNumber:  222,
		LastObserved: now.Add(-31 * 24 * time.Hour),
		Providers: map[string]*graduationProviderState{
			"alpha": {ScanDates: map[string]bool{"2026-06-01": true, "2026-06-02": true}},
			"beta":  {ScanDates: map[string]bool{"2026-06-01": true, "2026-06-02": true}},
		},
		LastQualifyingProviders: []string{"alpha", "beta"},
	}
	statePath := acifGraduationStatePath(cacheRoot, sharedSkillKey)
	if err := writeJSONState(statePath, &state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	var edits int
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isGH(args, "issue", "edit") {
			edits++
			if args[2] != "222" {
				t.Fatalf("edit issue = %q, want 222", args[2])
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
	roundTrip, err := readGraduationState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !roundTrip.StaleApplied {
		t.Fatal("stale_applied was not persisted")
	}
}

var sharedSkillKey = graduationKey{ContentType: "skills", ExtensionID: "shared_extension"}

// runGraduationScanDays scans on consecutive days and fails the test on any
// gh issue create, comment, or open-issue lookup: callers expect nothing to
// qualify.
func runGraduationScanDays(t *testing.T, cacheRoot, formatsDir, canonicalKeysPath string, days int) {
	t.Helper()
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)
	day1 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < days; i++ {
		issues, err := ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, i))
		if err != nil {
			t.Fatalf("ScanGraduationCandidates day %d: %v", i+1, err)
		}
		if len(issues) != 0 {
			t.Fatalf("day %d issues = %v, want none", i+1, issues)
		}
	}
}

func TestScanGraduationCandidates_SharedIDAcrossContentTypesDoesNotMerge(t *testing.T) {
	cacheRoot := t.TempDir()
	formatsDir := t.TempDir()
	canonicalKeysPath := writeGraduationCanonicalKeys(t)
	writeGraduationFormatDocYAML(t, formatsDir, "alpha", `
  skills:
    provider_extensions:
`+graduationExtensionYAML("when_to_use"))
	writeGraduationFormatDocYAML(t, formatsDir, "beta", `
  agents:
    provider_extensions:
`+graduationExtensionYAML("when_to_use"))

	runGraduationScanDays(t, cacheRoot, formatsDir, canonicalKeysPath, 3)

	for _, tc := range []struct {
		key      graduationKey
		provider string
	}{
		{graduationKey{"skills", "when_to_use"}, "alpha"},
		{graduationKey{"agents", "when_to_use"}, "beta"},
	} {
		state, err := readGraduationState(acifGraduationStatePath(cacheRoot, tc.key))
		if err != nil {
			t.Fatalf("read state %s: %v", tc.key, err)
		}
		if state.ContentType != tc.key.ContentType || len(state.Providers) != 1 || state.Providers[tc.provider] == nil {
			t.Fatalf("state %s = %+v, want only provider %s", tc.key, state, tc.provider)
		}
	}
}

func TestScanGraduationCandidates_CanonicalExtensionsDoNotFile(t *testing.T) {
	cacheRoot := t.TempDir()
	formatsDir := t.TempDir()
	canonicalKeysPath := writeGraduationCanonicalKeys(t)
	// supporting_files is linked to a canonical key; auto_invocable is a
	// canonical key itself. Both appear in two providers of one content type.
	for _, provider := range []string{"alpha", "beta"} {
		writeGraduationFormatDocYAML(t, formatsDir, provider, `
  skills:
    canonical_mappings:
      skill_bundled_resources:
        status: mapped
        extension_id: supporting_files
        mechanism: "co-located files"
        confidence: confirmed
    provider_extensions:
`+graduationExtensionYAML("supporting_files")+graduationExtensionYAML("auto_invocable"))
	}

	runGraduationScanDays(t, cacheRoot, formatsDir, canonicalKeysPath, 3)

	for _, id := range []string{"supporting_files", "auto_invocable"} {
		path := acifGraduationStatePath(cacheRoot, graduationKey{"skills", id})
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("canonical extension %s left state at %s (err=%v)", id, path, err)
		}
	}
}

func TestScanGraduationCandidates_NewConceptInOneContentTypeFiles(t *testing.T) {
	cacheRoot := t.TempDir()
	formatsDir := t.TempDir()
	canonicalKeysPath := writeGraduationCanonicalKeys(t)
	// A link to a key that is canonical only for another content type does
	// not make the extension canonical here.
	for _, provider := range []string{"alpha", "beta"} {
		writeGraduationFormatDocYAML(t, formatsDir, provider, `
  skills:
    canonical_mappings:
      tool_restrictions:
        status: mapped
        extension_id: fresh_concept
        mechanism: "wrong content type"
        confidence: confirmed
    provider_extensions:
`+graduationExtensionYAML("fresh_concept"))
	}

	var titles, bodies []string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		if isIssueListCall(args) {
			return []byte(`[]`), nil
		}
		if isGH(args, "issue", "create") {
			titles = append(titles, argValue(args, "--title"))
			bodies = append(bodies, argValue(args, "--body"))
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/400\n"), nil
		}
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})
	defer SetGHCommandForTest(nil)

	day1 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if _, err := ScanGraduationCandidates(cacheRoot, formatsDir, canonicalKeysPath, day1.AddDate(0, 0, i)); err != nil {
			t.Fatalf("ScanGraduationCandidates: %v", err)
		}
	}
	if len(titles) != 1 {
		t.Fatalf("creates = %d, want 1", len(titles))
	}
	if !strings.Contains(titles[0], `skills extension "fresh_concept"`) {
		t.Fatalf("title %q missing content type", titles[0])
	}
	for _, want := range []string{acifGraduationAnchor(graduationKey{"skills", "fresh_concept"}), "**Content type:** `skills`"} {
		if !strings.Contains(bodies[0], want) {
			t.Fatalf("body missing %q\n\n%s", want, bodies[0])
		}
	}
}

func writeGraduationCanonicalKeys(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "canonical-keys.yaml")
	content := `content_types:
  skills:
    auto_invocable:
      type: bool
    skill_bundled_resources:
      type: bool
  agents:
    tool_restrictions:
      type: object
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write canonical keys: %v", err)
	}
	return path
}

func graduationExtensionYAML(id string) string {
	return fmt.Sprintf(`      - id: %s
        name: %q
        summary: "summary"
        source_ref: "https://example.com/%s"
        graduation_candidate: true
        conversion: embedded
`, id, id, id)
}

func writeGraduationFormatDocYAML(t *testing.T, dir, provider, contentTypes string) {
	t.Helper()
	content := "provider: " + provider + "\ncontent_types:" + contentTypes
	if err := os.WriteFile(filepath.Join(dir, provider+".yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write format doc %s: %v", provider, err)
	}
}

func writeGraduationFormatDoc(t *testing.T, dir, provider, extensionID, name, summary string) {
	t.Helper()
	content := fmt.Sprintf(`provider: %s
content_types:
  skills:
    provider_extensions:
      - id: %s
        name: %q
        summary: %q
        source_ref: "https://example.com/%s"
        graduation_candidate: true
        conversion: embedded
`, provider, extensionID, name, summary, provider)
	path := filepath.Join(dir, provider+".yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write format doc %s: %v", provider, err)
	}
}
