package capmon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const confirmTestExport = `source_mechanisms:
  rule:
    tokens:
      - token: always_on
        recognition_requiring: false
      - token: legacy
        recognition_requiring: true
    aliases: {}
  hook:
    tokens:
      - token: per-os-key-map
        recognition_requiring: true
    aliases:
      per-os-key-map-provider: per-os-key-map
`

const confirmTestHello = `{"ok":true,"result":{"implementation":"fake-impl","version":"9.9.9","adapter_protocol":2,"scopes":["core","hook"]}}`

// Adapter responses covering every classified probe outcome.
const (
	confirmRespClean       = `{"ok":true,"result":{"conformant":true,"installable":true,"canonical":{},"diagnostics":[{"id":"acif.hook.platform_filename_inferred"}]}}`
	confirmRespMalformed   = `{"ok":false,"error":"acif.hook.platform_mechanism_malformed"}`
	confirmRespUnmappable  = `{"ok":false,"error":"acif.hook.platform_unmappable"}`
	confirmRespVerdictSkew = `{"ok":true,"result":{"conformant":false,"reason":"acif.hook.platform_unmappable"}}`
)

func writeConfirmExport(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source-mechanisms.yaml")
	if err := os.WriteFile(path, []byte(confirmTestExport), 0644); err != nil {
		t.Fatalf("write export fixture: %v", err)
	}
	return path
}

func writeConfirmFormatDoc(t *testing.T, dir, provider, contentType, key, mapping string) {
	t.Helper()
	doc := fmt.Sprintf(`provider: %s
content_types:
  %s:
    canonical_mappings:
      %s:
%s`, provider, contentType, key, mapping)
	if err := os.WriteFile(filepath.Join(dir, provider+".yaml"), []byte(doc), 0644); err != nil {
		t.Fatalf("write format doc fixture: %v", err)
	}
}

// unmappedMapping renders a status:unmapped canonical mapping body indented
// for writeConfirmFormatDoc. token may be empty.
func unmappedMapping(token, sourceForm string) string {
	var b strings.Builder
	b.WriteString("        status: unmapped\n")
	if token != "" {
		fmt.Fprintf(&b, "        mechanism_token: %s\n", token)
	}
	b.WriteString("        source_form: |\n")
	for _, line := range strings.Split(strings.TrimRight(sourceForm, "\n"), "\n") {
		fmt.Fprintf(&b, "          %s\n", line)
	}
	b.WriteString("        mechanism: test mechanism\n")
	b.WriteString("        confidence: confirmed\n")
	return b.String()
}

// writeFakeAdapter writes a shell-script adapter that answers the Nth
// request with the Nth canned line, regardless of request content.
func writeFakeAdapter(t *testing.T, responses ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\ni=0\nwhile IFS= read -r _; do\n  i=$((i+1))\n  case $i in\n")
	for idx, resp := range responses {
		quoted := "'" + strings.ReplaceAll(resp, "'", `'\''`) + "'"
		fmt.Fprintf(&b, "  %d) printf '%%s\\n' %s;;\n", idx+1, quoted)
	}
	b.WriteString("  *) exit 0;;\n  esac\ndone\n")
	path := filepath.Join(t.TempDir(), "adapter.sh")
	if err := os.WriteFile(path, []byte(b.String()), 0755); err != nil {
		t.Fatalf("write fake adapter: %v", err)
	}
	return path
}

// stubConfirmGH installs a gh stub that lists no open issues and captures
// creates/comments. Returns the calls slice pointer for assertions.
func stubConfirmGH(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		calls = append(calls, copyArgs(args))
		if isIssueListCall(args) {
			return []byte(`[]`), nil
		}
		if isGH(args, "issue", "create") {
			return []byte("https://github.com/OpenScribbler/agent-content-interchange-format/issues/77\n"), nil
		}
		if isGH(args, "issue", "comment") {
			return nil, nil
		}
		t.Errorf("unexpected gh call: %v", args)
		return nil, fmt.Errorf("unexpected gh call")
	})
	t.Cleanup(func() { SetGHCommandForTest(nil) })
	return &calls
}

func runConfirm(t *testing.T, opts ConfirmOptions) *ConfirmResult {
	t.Helper()
	result, err := ConfirmUnmappedCandidates(opts)
	if err != nil {
		t.Fatalf("ConfirmUnmappedCandidates: %v", err)
	}
	return result
}

func singleFinding(t *testing.T, result *ConfirmResult) ConfirmFinding {
	t.Helper()
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(result.Findings), result.Findings)
	}
	return result.Findings[0]
}

func TestConfirm_NoCandidates(t *testing.T) {
	calls := stubConfirmGH(t)
	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: t.TempDir(),
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
	})
	if len(result.Findings) != 0 || result.Degraded {
		t.Fatalf("want empty non-degraded result, got %+v", result)
	}
	if len(*calls) != 0 {
		t.Fatalf("gh called with no candidates: %v", *calls)
	}
}

func TestConfirm_PathA_StaticMemberReject(t *testing.T) {
	docs := t.TempDir()
	cacheRoot := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "rules", "activation_mode",
		unmappedMapping("always_on", "alwaysApply: true"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     cacheRoot,
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictReject || f.Reason != ConfirmReasonStaticMember {
		t.Fatalf("verdict/reason = %s/%s, want reject/static-member", f.Verdict, f.Reason)
	}
	if result.Degraded {
		t.Fatal("static reject must not degrade the run")
	}
	if len(*calls) != 0 {
		t.Fatalf("reject must never touch gh: %v", *calls)
	}

	// Reject verdicts still leave a confirm-report state entry.
	state, err := readConfirmState(confirmStatePath(cacheRoot, f))
	if err != nil {
		t.Fatalf("read confirm state: %v", err)
	}
	if state.Verdict != ConfirmVerdictReject || state.ConfirmedAt.IsZero() {
		t.Fatalf("state entry = %+v, want persisted reject", state)
	}
}

func TestConfirm_PathB_ProbeClean(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/base.sh\nwindows: hooks/win.cmd"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespClean),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictReject || f.Reason != ConfirmReasonProbeClean {
		t.Fatalf("verdict/reason = %s/%s, want reject/probe-clean", f.Verdict, f.Reason)
	}
	if f.Evidence.Implementation != "fake-impl" || f.Evidence.ImplVersion != "9.9.9" || f.Evidence.AdapterProtocol != 2 {
		t.Fatalf("hello evidence not pinned: %+v", f.Evidence)
	}
	if f.Evidence.AdapterResponse != confirmRespClean {
		t.Fatalf("verbatim response not pinned: %q", f.Evidence.AdapterResponse)
	}
	if result.Degraded || len(*calls) != 0 {
		t.Fatalf("clean probe reject must not degrade or file: degraded=%v calls=%v", result.Degraded, *calls)
	}
}

func TestConfirm_PathB_ProbeMalformed(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: 42"))
	stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespMalformed),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictReject || f.Reason != ConfirmReasonProbeMalformed {
		t.Fatalf("verdict/reason = %s/%s, want reject/probe-malformed", f.Verdict, f.Reason)
	}
	if result.Degraded {
		t.Fatal("malformed classification is a sound reject, not degradation")
	}
}

func TestConfirm_PathB_ProbeSkewFilesIssue(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/base.sh"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespVerdictSkew),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonProbeSkew {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/probe-skew", f.Verdict, f.Reason)
	}
	if f.IssueNumber != 77 {
		t.Fatalf("issue number = %d, want 77", f.IssueNumber)
	}
	if result.Degraded {
		t.Fatal("skew is a principled unconfirmed outcome, not run degradation")
	}

	createCall := findGHCall(*calls, "issue", "create")
	if createCall == nil {
		t.Fatal("missing gh issue create call")
	}
	labels := labelValues(createCall)
	for _, want := range []string{acifChangeLabel, acifClassBLabel, acifNeedsHumanLabel} {
		if !labels[want] {
			t.Errorf("create missing label %q: %v", want, createCall)
		}
	}
	body := argValue(createCall, "--body")
	if !strings.Contains(body, acifConfirmAnchor(f)) {
		t.Errorf("issue body missing dedup anchor:\n%s", body)
	}
	for _, want := range []string{"testrev", "fake-impl", confirmRespVerdictSkew, "command: hooks/base.sh"} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing pinned evidence %q:\n%s", want, body)
		}
	}
}

func TestConfirm_PathB_AliasResolvesToMember(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map-provider", "command: hooks/base.sh"))
	stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespClean),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictReject || f.Reason != ConfirmReasonProbeClean {
		t.Fatalf("alias claim not probed as member: %s/%s", f.Verdict, f.Reason)
	}
}

func TestConfirm_RuleRecognitionRequiring_ScopeUntrusted(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "rules", "activation_mode",
		unmappedMapping("legacy", "legacyRule: true"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		// Deliberately no adapter: the rule scope must never probe.
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonScopeUntrusted {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/scope-untrusted", f.Verdict, f.Reason)
	}
	if result.Degraded {
		t.Fatal("scope-untrusted is principled, not degradation")
	}
	if findGHCall(*calls, "issue", "create") == nil {
		t.Fatal("scope-untrusted candidate must file a needs-human issue")
	}
}

func TestConfirm_PathC_NotInExport_NonHook(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "skills", "activation_type",
		unmappedMapping("", "activation: mystery"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonNotInExport {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/not-in-export", f.Verdict, f.Reason)
	}
	if result.Degraded {
		t.Fatal("a genuine candidate without a probe scope is not a degraded run")
	}
	if findGHCall(*calls, "issue", "create") == nil {
		t.Fatal("genuine candidate must file a needs-human issue")
	}
}

func TestConfirm_PathC_HookProbeClean_MisflagRejected(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("unknown-token", `{"event":"before_tool_execute","handlers":[]}`))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespClean),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictReject || f.Reason != ConfirmReasonProbeClean {
		t.Fatalf("verdict/reason = %s/%s, want reject/probe-clean (mis-flag class)", f.Verdict, f.Reason)
	}
	if len(*calls) != 0 {
		t.Fatalf("mis-flag reject must not file: %v", *calls)
	}
}

func TestConfirm_PathC_HookProbeUnmappable_Strengthened(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("unknown-token", "someKey: someValue"))
	stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespUnmappable),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonNotInExport {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/not-in-export", f.Verdict, f.Reason)
	}
	if !strings.Contains(f.Detail, "strengthened") {
		t.Fatalf("detail not strengthened by probe: %q", f.Detail)
	}
	if result.Degraded {
		t.Fatal("strengthened unconfirmed is not degradation")
	}
}

func TestConfirm_Invariant_AdapterAbsent(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/base.sh"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   filepath.Join(t.TempDir(), "no-such-adapter"),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonProbeDegraded {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/probe-degraded", f.Verdict, f.Reason)
	}
	if !result.Degraded {
		t.Fatal("absent adapter must degrade the run")
	}
	if findGHCall(*calls, "issue", "create") == nil {
		t.Fatal("degraded candidate must still file a needs-human issue")
	}
}

func TestConfirm_Invariant_AdapterTimeout(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/base.sh"))
	stubConfirmGH(t)

	// Adapter answers hello, then never answers the probe.
	script := "#!/bin/sh\nIFS= read -r _\nprintf '%s\\n' '" + confirmTestHello + "'\nIFS= read -r _\nsleep 60\n"
	adapterPath := filepath.Join(t.TempDir(), "slow-adapter.sh")
	if err := os.WriteFile(adapterPath, []byte(script), 0755); err != nil {
		t.Fatalf("write slow adapter: %v", err)
	}

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   adapterPath,
		ProbeTimeout:  200 * time.Millisecond,
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonProbeDegraded {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/probe-degraded", f.Verdict, f.Reason)
	}
	if !result.Degraded {
		t.Fatal("timeout must degrade the run")
	}
}

func TestConfirm_Invariant_AdapterGarbage(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/base.sh"))
	stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, "this is not json"),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonProbeDegraded {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/probe-degraded", f.Verdict, f.Reason)
	}
	if !result.Degraded {
		t.Fatal("garbage response must degrade the run")
	}
}

func TestConfirm_Invariant_AdapterWithoutHookScope(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/base.sh"))
	stubConfirmGH(t)

	coreOnlyHello := `{"ok":true,"result":{"implementation":"fake-impl","version":"9.9.9","adapter_protocol":2,"scopes":["core"]}}`
	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, coreOnlyHello, confirmRespClean),
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonProbeDegraded {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/probe-degraded", f.Verdict, f.Reason)
	}
	if !result.Degraded {
		t.Fatal("hook-less adapter must degrade the run")
	}
}

func TestConfirm_Invariant_ExportMissing(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "rules", "activation_mode",
		unmappedMapping("always_on", "alwaysApply: true"))
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    filepath.Join(t.TempDir(), "no-such-export.yaml"),
		ExportRev:     "testrev",
	})
	f := singleFinding(t, result)
	// Without the membership oracle even a would-be static reject must land
	// unconfirmed: fail-toward-unconfirmed, never toward closure.
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonExportUnavailable {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/export-unavailable", f.Verdict, f.Reason)
	}
	if !result.Degraded {
		t.Fatal("missing export must degrade the run")
	}
	if findGHCall(*calls, "issue", "create") == nil {
		t.Fatal("export-unavailable candidate must still file a needs-human issue")
	}
}

func TestConfirm_Invariant_SourceFormUnparsable(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "[unclosed"))
	stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		// No adapter needed: the parse fails before any probe is attempted.
	})
	f := singleFinding(t, result)
	if f.Verdict != ConfirmVerdictUnconfirmed || f.Reason != ConfirmReasonSourceFormUnparsable {
		t.Fatalf("verdict/reason = %s/%s, want unconfirmed/source-form-unparsable", f.Verdict, f.Reason)
	}
	if !result.Degraded {
		t.Fatal("unparsable source form must degrade the run")
	}
}

func TestConfirm_IssueDedup_PriorStateComments(t *testing.T) {
	docs := t.TempDir()
	cacheRoot := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "skills", "activation_type",
		unmappedMapping("", "activation: mystery"))

	// Seed prior state carrying an already-filed issue for this slot.
	seed := ConfirmFinding{Provider: "testprov", ContentType: "skills", CanonicalKey: "activation_type"}
	seed.Verdict = ConfirmVerdictUnconfirmed
	seed.IssueNumber = 55
	if err := writeJSONState(confirmStatePath(cacheRoot, seed), &confirmCandidateState{ConfirmFinding: seed, ConfirmedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	calls := stubConfirmGH(t)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     cacheRoot,
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
	})
	f := singleFinding(t, result)
	if f.IssueNumber != 55 {
		t.Fatalf("issue number = %d, want prior 55", f.IssueNumber)
	}
	if findGHCall(*calls, "issue", "create") != nil {
		t.Fatalf("must not create a duplicate issue: %v", *calls)
	}
	if findGHCall(*calls, "issue", "comment") == nil {
		t.Fatal("must refresh the prior issue with a comment")
	}
}

func TestConfirm_IssueDedup_AnchorFoundComments(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "skills", "activation_type",
		unmappedMapping("", "activation: mystery"))

	anchor := acifConfirmAnchor(ConfirmFinding{Provider: "testprov", ContentType: "skills", CanonicalKey: "activation_type"})
	var calls [][]string
	SetGHCommandForTest(func(args ...string) ([]byte, error) {
		calls = append(calls, copyArgs(args))
		if isIssueListCall(args) {
			issues := []map[string]any{{"number": 88, "body": "intro\n" + anchor + "\nrest"}}
			out, _ := json.Marshal(issues)
			return out, nil
		}
		if isGH(args, "issue", "comment") {
			return nil, nil
		}
		t.Errorf("unexpected gh call: %v", args)
		return nil, fmt.Errorf("unexpected gh call")
	})
	defer SetGHCommandForTest(nil)

	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
	})
	f := singleFinding(t, result)
	if f.IssueNumber != 88 {
		t.Fatalf("issue number = %d, want anchored 88", f.IssueNumber)
	}
	if findGHCall(calls, "issue", "create") != nil {
		t.Fatalf("must not create when anchor already open: %v", calls)
	}
}

func TestConfirm_MultipleCandidates_SharedAdapter(t *testing.T) {
	docs := t.TempDir()
	writeConfirmFormatDoc(t, docs, "testprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/a.sh"))
	writeConfirmFormatDoc(t, docs, "zprov", "hooks", "timeout_config",
		unmappedMapping("per-os-key-map", "command: hooks/b.sh"))
	stubConfirmGH(t)

	// One hello, then one response per candidate, in deterministic
	// (provider-sorted) order: testprov then zprov.
	result := runConfirm(t, ConfirmOptions{
		FormatDocsDir: docs,
		CacheRoot:     t.TempDir(),
		ExportPath:    writeConfirmExport(t),
		ExportRev:     "testrev",
		AdapterPath:   writeFakeAdapter(t, confirmTestHello, confirmRespClean, confirmRespMalformed),
	})
	if len(result.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(result.Findings))
	}
	if result.Findings[0].Provider != "testprov" || result.Findings[0].Reason != ConfirmReasonProbeClean {
		t.Fatalf("finding[0] = %+v, want testprov probe-clean", result.Findings[0])
	}
	if result.Findings[1].Provider != "zprov" || result.Findings[1].Reason != ConfirmReasonProbeMalformed {
		t.Fatalf("finding[1] = %+v, want zprov probe-malformed", result.Findings[1])
	}
}

func TestMaterializeProbeBody(t *testing.T) {
	bodyRoot := t.TempDir()
	content := map[string]any{
		"command": "hooks/base.sh",
		"windows": "hooks\\win.cmd",
		"escape":  "../outside.sh",
		"empty":   "",
		"number":  42,
	}
	if err := materializeProbeBody(bodyRoot, content); err != nil {
		t.Fatalf("materializeProbeBody: %v", err)
	}
	for _, want := range []string{"hooks/base.sh", "hooks\\win.cmd"} {
		abs := filepath.Join(bodyRoot, filepath.FromSlash(want))
		if info, err := os.Stat(abs); err != nil || !info.Mode().IsRegular() {
			t.Errorf("stub %q not materialized: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(bodyRoot), "outside.sh")); !os.IsNotExist(err) {
		t.Fatal("path escape was materialized outside the body root")
	}
}

func TestClassifyProbeResponse(t *testing.T) {
	cases := []struct {
		name string
		line string
		want probeOutcome
	}{
		{"clean", confirmRespClean, probeOutcomeClean},
		{"malformed error", confirmRespMalformed, probeOutcomeMalformed},
		{"unmappable error", confirmRespUnmappable, probeOutcomeUnmappable},
		{"unmappable verdict reason", confirmRespVerdictSkew, probeOutcomeUnmappable},
		{"unmappable in result diagnostics", `{"ok":true,"result":{"conformant":true,"diagnostics":[{"id":"acif.hook.platform_unmappable"}]}}`, probeOutcomeUnmappable},
		{"unsupported", `{"unsupported":true}`, probeOutcomeOther},
		{"other error", `{"ok":false,"error":"acif.hook.script_file_missing"}`, probeOutcomeOther},
		{"garbage", "not json", probeOutcomeDegraded},
	}
	for _, tc := range cases {
		if got := classifyProbeResponse(tc.line); got != tc.want {
			t.Errorf("%s: classifyProbeResponse = %d, want %d", tc.name, got, tc.want)
		}
	}
}
