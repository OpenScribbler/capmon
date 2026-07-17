package capmon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Machine verdicts for a status:unmapped candidate. The already-mapped
// direction is machine-authoritative (reject); the genuinely-unmapped
// direction is never machine-closed — it lands unconfirmed and is handed to
// a human via a needs-human issue.
const (
	ConfirmVerdictReject      = "reject"
	ConfirmVerdictUnconfirmed = "unconfirmed"
)

// Stable reason codes recorded in confirm state files and issue bodies.
const (
	// Path a: the token claim is an export member with
	// recognition_requiring:false — mapped-ness is statically decidable.
	ConfirmReasonStaticMember = "static-member"
	// Probe canonicalized the source form cleanly: the mechanism is mapped
	// (for a member token) or the candidate was mis-flagged (for a
	// non-member claim).
	ConfirmReasonProbeClean = "probe-clean"
	// Probe answered acif.hook.platform_mechanism_malformed: the mechanism
	// is mapped and the source form is an authoring error, not a gap.
	ConfirmReasonProbeMalformed = "probe-malformed"
	// Probe answered acif.hook.platform_unmappable for a token the export
	// says is a member: impl/spec skew — a human must reconcile.
	ConfirmReasonProbeSkew = "probe-skew"
	// Path c: no token claim, or the claim is not in the export — a genuine
	// Class B candidate. When a probe also answered platform_unmappable the
	// evidence is strengthened, but the reason code stays the same.
	ConfirmReasonNotInExport = "not-in-export"
	// The token is recognition-requiring but the candidate's scope has no
	// differentially-witnessed probe (adr/0001: no scope's class-B verdict
	// is machine-authoritative until that scope is differentially clean).
	ConfirmReasonScopeUntrusted = "scope-untrusted"
	// Degraded paths (hard invariant: these land unconfirmed, never reject,
	// never silent drop).
	ConfirmReasonExportUnavailable    = "export-unavailable"
	ConfirmReasonProbeDegraded        = "probe-degraded"
	ConfirmReasonSourceFormUnparsable = "source-form-unparsable"
)

const acifNeedsHumanLabel = "needs-human"

// defaultProbeTimeout matches the adapter protocol's per-request timeout.
const defaultProbeTimeout = 30 * time.Second

// ConfirmOptions configures one `capmon acif-change confirm` run.
//
// ExportPath is a local copy of ACIF's conformance/source-mechanisms.yaml;
// the caller fetches it pinned to a git rev and passes that rev as ExportRev,
// which is embedded verbatim in all evidence (mirrors how the drift check
// consumes capability-vocabulary.yaml).
//
// AdapterPath is an executable speaking conformance adapter protocol 2
// (one JSON request per line on stdin, one response per line on stdout,
// "hello" first). It must claim the hook scope. Empty means no probe is
// available: probe-requiring candidates land unconfirmed.
type ConfirmOptions struct {
	FormatDocsDir string
	CacheRoot     string
	ExportPath    string
	ExportRev     string
	AdapterPath   string
	ProbeTimeout  time.Duration // per adapter request; defaults to 30s
	Now           time.Time
}

// ConfirmEvidence pins everything a human needs to audit a machine verdict.
type ConfirmEvidence struct {
	ExportRev       string `json:"export_rev,omitempty"`
	AdapterProtocol int    `json:"adapter_protocol,omitempty"`
	Implementation  string `json:"implementation,omitempty"`
	ImplVersion     string `json:"impl_version,omitempty"`
	AdapterResponse string `json:"adapter_response,omitempty"`
	SourceForm      string `json:"source_form,omitempty"`
}

// ConfirmFinding is the machine verdict for one status:unmapped candidate.
type ConfirmFinding struct {
	Provider       string          `json:"provider"`
	ContentType    string          `json:"content_type"`
	CanonicalKey   string          `json:"canonical_key"`
	MechanismToken string          `json:"mechanism_token,omitempty"`
	Verdict        string          `json:"verdict"`
	Reason         string          `json:"reason"`
	Detail         string          `json:"detail,omitempty"`
	Degraded       bool            `json:"degraded,omitempty"`
	IssueNumber    int             `json:"issue_number,omitempty"`
	Evidence       ConfirmEvidence `json:"evidence"`
}

// ConfirmResult is the outcome of a confirm run. Degraded reports that at
// least one candidate landed unconfirmed through a degraded path (export
// unreadable, adapter down or timed out, unparsable source form) — callers
// should signal it via the exit code so operators notice.
type ConfirmResult struct {
	Findings []ConfirmFinding
	Degraded bool
}

// unmappedCandidate is one status:unmapped canonical-mapping slot found in a
// provider format doc. Identity is (provider, content type, canonical key);
// the token claim and source form are evidence attached to the slot, not
// part of its identity.
type unmappedCandidate struct {
	Provider     string
	ContentType  string
	CanonicalKey string
	Token        string
	SourceForm   string
}

// formatDocScopeForContentType maps format-doc content-type keys (plural) to
// the export's scope keys. Content types absent here have no source-mechanism
// token set, so a token claim under them is never an export member.
var formatDocScopeForContentType = map[string]string{
	"rules": "rule",
	"hooks": "hook",
}

// ConfirmUnmappedCandidates walks provider format docs, machine-checks every
// status:unmapped canonical mapping against the ACIF source-mechanism export
// (plus a hook probe for shape-predicate mechanisms), and acts on the
// verdicts: reject → confirm-report state entry only (curator YAML and the
// event-sourced RecordUnmappedForm counters are never touched); unconfirmed →
// file or refresh a needs-human issue on the ACIF repo with pinned evidence.
//
// HARD INVARIANT (fail-toward-unconfirmed): every degraded path lands the
// candidate at unconfirmed, never reject and never a silent drop. Suppressing
// a true vocabulary gap is invisible; filing a false candidate is a bounded
// ticket.
func ConfirmUnmappedCandidates(opts ConfirmOptions) (*ConfirmResult, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	timeout := opts.ProbeTimeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}

	candidates, err := collectUnmappedCandidates(opts.FormatDocsDir)
	if err != nil {
		return nil, err
	}
	result := &ConfirmResult{}
	if len(candidates) == 0 {
		return result, nil
	}

	export, exportErr := loadSourceMechanismExport(opts.ExportPath)

	// The adapter is spawned lazily on the first probe-requiring candidate
	// and shared across the run (the protocol is one process, requests in
	// order). A dead adapter is not restarted: every later probe degrades.
	var adapter *acifAdapter
	var adapterErr error
	adapterTried := false
	getAdapter := func() (*acifAdapter, error) {
		if !adapterTried {
			adapterTried = true
			if opts.AdapterPath == "" {
				adapterErr = fmt.Errorf("no probe adapter configured (--adapter or CAPMON_ACIF_ADAPTER)")
			} else {
				adapter, adapterErr = startACIFAdapter(opts.AdapterPath, timeout)
			}
		}
		return adapter, adapterErr
	}
	defer func() {
		if adapter != nil {
			adapter.close()
		}
	}()

	var actionErrs []error
	for _, cand := range candidates {
		finding := evaluateUnmappedCandidate(cand, export, exportErr, opts.ExportRev, getAdapter)
		if finding.Verdict == ConfirmVerdictUnconfirmed {
			// Prior-state read is best-effort here: a zero prior issue just
			// means the anchor search runs; persistConfirmState surfaces any
			// real read error below.
			prior, _ := readConfirmState(confirmStatePath(opts.CacheRoot, finding))
			issueNum, err := fileOrRefreshConfirmIssue(finding, prior.IssueNumber)
			if err != nil {
				actionErrs = append(actionErrs, fmt.Errorf("%s/%s/%s: %w", cand.Provider, cand.ContentType, cand.CanonicalKey, err))
			} else {
				finding.IssueNumber = issueNum
			}
		}
		if err := persistConfirmState(opts.CacheRoot, finding, now); err != nil {
			actionErrs = append(actionErrs, err)
		}
		if finding.Degraded {
			result.Degraded = true
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, errors.Join(actionErrs...)
}

// collectUnmappedCandidates walks the format docs dir in deterministic order
// and returns every status:unmapped canonical mapping. An unreadable dir or
// doc is a loud structural error: an unenumerable candidate set cannot be
// failed toward unconfirmed candidate-by-candidate.
func collectUnmappedCandidates(formatDocsDir string) ([]unmappedCandidate, error) {
	entries, err := os.ReadDir(formatDocsDir)
	if err != nil {
		return nil, fmt.Errorf("read format docs dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var candidates []unmappedCandidate
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(formatDocsDir, entry.Name())
		doc, err := LoadFormatDoc(path)
		if err != nil {
			return nil, err
		}
		provider := doc.Provider
		if provider == "" {
			provider = strings.TrimSuffix(entry.Name(), ".yaml")
		}
		provider, err = SanitizeSlug(provider)
		if err != nil {
			return nil, fmt.Errorf("invalid provider in %s: %w", path, err)
		}

		contentTypes := make([]string, 0, len(doc.ContentTypes))
		for contentType := range doc.ContentTypes {
			contentTypes = append(contentTypes, contentType)
		}
		sort.Strings(contentTypes)
		for _, contentType := range contentTypes {
			ctDoc := doc.ContentTypes[contentType]
			keys := make([]string, 0, len(ctDoc.CanonicalMappings))
			for key := range ctDoc.CanonicalMappings {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				mapping := ctDoc.CanonicalMappings[key]
				if mapping.Status != MappingStatusUnmapped {
					continue
				}
				candidates = append(candidates, unmappedCandidate{
					Provider:     provider,
					ContentType:  contentType,
					CanonicalKey: key,
					Token:        mapping.MechanismToken,
					SourceForm:   mapping.SourceForm,
				})
			}
		}
	}
	return candidates, nil
}

// sourceMechanismExport is the parsed membership oracle
// (ACIF conformance/source-mechanisms.yaml).
type sourceMechanismExport struct {
	scopes map[string]sourceMechanismSet
}

type sourceMechanismSet struct {
	// tokens maps each member token to its recognition_requiring flag.
	tokens map[string]bool
	// aliases maps each accepted alternate spelling to its member token.
	aliases map[string]string
}

func loadSourceMechanismExport(path string) (*sourceMechanismExport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source-mechanisms export %s: %w", path, err)
	}
	var file struct {
		SourceMechanisms map[string]struct {
			Tokens []struct {
				Token                string `yaml:"token"`
				RecognitionRequiring bool   `yaml:"recognition_requiring"`
			} `yaml:"tokens"`
			Aliases map[string]string `yaml:"aliases"`
		} `yaml:"source_mechanisms"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse source-mechanisms export %s: %w", path, err)
	}
	if len(file.SourceMechanisms) == 0 {
		return nil, fmt.Errorf("source-mechanisms export %s carries no source_mechanisms table", path)
	}
	export := &sourceMechanismExport{scopes: make(map[string]sourceMechanismSet)}
	for scope, entry := range file.SourceMechanisms {
		set := sourceMechanismSet{tokens: make(map[string]bool), aliases: entry.Aliases}
		for _, tok := range entry.Tokens {
			set.tokens[tok.Token] = tok.RecognitionRequiring
		}
		export.scopes[scope] = set
	}
	return export, nil
}

// lookup resolves a token claim (alias-aware) inside one scope's token set.
func (e *sourceMechanismExport) lookup(scope, token string) (member, recognitionRequiring bool) {
	set, ok := e.scopes[scope]
	if !ok {
		return false, false
	}
	if canonical, ok := set.aliases[token]; ok {
		token = canonical
	}
	rr, ok := set.tokens[token]
	return ok, rr
}

// evaluateUnmappedCandidate applies the confirm decision table (the
// capmon-dgn design field, paths a/b/c) to one candidate.
func evaluateUnmappedCandidate(
	cand unmappedCandidate,
	export *sourceMechanismExport,
	exportErr error,
	exportRev string,
	getAdapter func() (*acifAdapter, error),
) ConfirmFinding {
	finding := ConfirmFinding{
		Provider:       cand.Provider,
		ContentType:    cand.ContentType,
		CanonicalKey:   cand.CanonicalKey,
		MechanismToken: cand.Token,
		Evidence: ConfirmEvidence{
			ExportRev:  exportRev,
			SourceForm: cand.SourceForm,
		},
	}

	if exportErr != nil {
		finding.Verdict = ConfirmVerdictUnconfirmed
		finding.Reason = ConfirmReasonExportUnavailable
		finding.Detail = "membership oracle unavailable: " + exportErr.Error()
		finding.Degraded = true
		return finding
	}

	scope := formatDocScopeForContentType[cand.ContentType]
	member, recognitionRequiring := false, false
	if scope != "" && cand.Token != "" {
		member, recognitionRequiring = export.lookup(scope, cand.Token)
	}
	isHookScope := scope == "hook"

	switch {
	case member && !recognitionRequiring:
		// Path a: statically sound — the mechanism is already mapped.
		finding.Verdict = ConfirmVerdictReject
		finding.Reason = ConfirmReasonStaticMember
		finding.Detail = fmt.Sprintf("token %q is a member of the %s source-mechanism set with recognition_requiring:false — already mapped", cand.Token, scope)

	case member && recognitionRequiring && !isHookScope:
		// Recognition-requiring outside the hook scope: no second-impl
		// witness exists there, so no machine verdict is authoritative.
		finding.Verdict = ConfirmVerdictUnconfirmed
		finding.Reason = ConfirmReasonScopeUntrusted
		finding.Detail = fmt.Sprintf("token %q is recognition-requiring and the %s scope has no differentially-witnessed probe — a human must decide", cand.Token, scope)

	case member && recognitionRequiring && isHookScope:
		// Path b: mapped-ness of this instance is a shape predicate — probe.
		outcome := probeSourceForm(getAdapter, cand.Token, cand.SourceForm, &finding.Evidence)
		switch outcome {
		case probeOutcomeClean:
			finding.Verdict = ConfirmVerdictReject
			finding.Reason = ConfirmReasonProbeClean
			finding.Detail = "conforming adapter canonicalized the source form cleanly — the mechanism instance is mapped"
		case probeOutcomeMalformed:
			finding.Verdict = ConfirmVerdictReject
			finding.Reason = ConfirmReasonProbeMalformed
			finding.Detail = "conforming adapter answered acif.hook.platform_mechanism_malformed — mapped mechanism, malformed value (authoring error, not a vocabulary gap)"
		case probeOutcomeUnmappable:
			finding.Verdict = ConfirmVerdictUnconfirmed
			finding.Reason = ConfirmReasonProbeSkew
			finding.Detail = fmt.Sprintf("adapter answered acif.hook.platform_unmappable for export member token %q — impl/spec skew, a human must reconcile", cand.Token)
		case probeOutcomeUnparsableForm:
			finding.Verdict = ConfirmVerdictUnconfirmed
			finding.Reason = ConfirmReasonSourceFormUnparsable
			finding.Detail = "source_form does not parse as YAML/JSON — no probe request could be built"
			finding.Degraded = true
		default: // probeOutcomeDegraded, probeOutcomeOther
			finding.Verdict = ConfirmVerdictUnconfirmed
			finding.Reason = ConfirmReasonProbeDegraded
			finding.Detail = "probe degraded or answered outside the classified outcome set — fail-toward-unconfirmed"
			finding.Degraded = true
		}

	default:
		// Path c: no token claim, or not in the export — genuine candidate.
		finding.Verdict = ConfirmVerdictUnconfirmed
		finding.Reason = ConfirmReasonNotInExport
		if cand.Token == "" {
			finding.Detail = "no mechanism token claim — genuine Class B candidate, needs a human"
		} else {
			finding.Detail = fmt.Sprintf("token %q is not in the source-mechanism export — genuine Class B candidate, needs a human", cand.Token)
		}
		if isHookScope {
			// The probe can still catch the mis-flag class (a source form a
			// conforming impl canonicalizes cleanly) or strengthen the
			// evidence. Probe trouble here never degrades the run: the
			// static verdict above already stands on its own.
			switch probeSourceForm(getAdapter, cand.Token, cand.SourceForm, &finding.Evidence) {
			case probeOutcomeClean:
				finding.Verdict = ConfirmVerdictReject
				finding.Reason = ConfirmReasonProbeClean
				finding.Detail = "conforming adapter canonicalized the source form cleanly — mis-flagged candidate, already mapped"
			case probeOutcomeUnmappable:
				finding.Detail += " (strengthened: conforming adapter also answered acif.hook.platform_unmappable)"
			}
		}
	}
	return finding
}

// Probe outcome classification. Anything not explicitly classified is
// probeOutcomeOther and fails toward unconfirmed.
type probeOutcome int

const (
	probeOutcomeDegraded probeOutcome = iota
	probeOutcomeClean
	probeOutcomeMalformed
	probeOutcomeUnmappable
	probeOutcomeUnparsableForm
	probeOutcomeOther
)

// probeSourceForm runs one provider_config hook ingest of the source form
// against the conforming adapter and classifies the response. Evidence
// fields (hello identity, verbatim response) are recorded on ev as they
// become available, so even a degraded probe pins what was observed.
func probeSourceForm(getAdapter func() (*acifAdapter, error), token, sourceForm string, ev *ConfirmEvidence) probeOutcome {
	var content any
	if err := yaml.Unmarshal([]byte(sourceForm), &content); err != nil {
		return probeOutcomeUnparsableForm
	}

	adapter, err := getAdapter()
	if err != nil {
		return probeOutcomeDegraded
	}
	ev.AdapterProtocol = adapter.hello.AdapterProtocol
	ev.Implementation = adapter.hello.Implementation
	ev.ImplVersion = adapter.hello.Version

	// A clean canonicalization must be reachable: a conforming impl computes
	// the referenced-file manifest after mapping, and a missing script file
	// would abort the probe there even though the mechanism itself mapped.
	// Stub every plausible path so file existence never masks mapped-ness.
	bodyRoot, err := os.MkdirTemp("", "capmon-confirm-body-")
	if err != nil {
		return probeOutcomeDegraded
	}
	defer os.RemoveAll(bodyRoot)
	if err := materializeProbeBody(bodyRoot, content); err != nil {
		return probeOutcomeDegraded
	}

	providerConfig := map[string]any{"path": "probe", "content": content}
	if token != "" {
		providerConfig["provider"] = token
	}
	request, err := json.Marshal(map[string]any{
		"op": "ingest",
		"input": map[string]any{
			"kind":            "hook",
			"body_root":       bodyRoot,
			"provider_config": providerConfig,
		},
	})
	if err != nil {
		return probeOutcomeDegraded
	}

	response, err := adapter.roundTrip(string(request))
	if err != nil {
		return probeOutcomeDegraded
	}
	ev.AdapterResponse = response
	return classifyProbeResponse(response)
}

// materializeProbeBody writes an executable stub for every top-level string
// value of the parsed source form that could name a script path, under
// bodyRoot. Values that would resolve outside bodyRoot are skipped (the
// probe then degrades toward unconfirmed rather than writing elsewhere).
func materializeProbeBody(bodyRoot string, content any) error {
	obj, ok := content.(map[string]any)
	if !ok {
		return nil
	}
	for _, raw := range obj {
		value, ok := raw.(string)
		if !ok || value == "" || len(value) > 300 || strings.ContainsAny(value, "\x00\n") {
			continue
		}
		abs := filepath.Join(bodyRoot, filepath.FromSlash(value))
		rel, err := filepath.Rel(bodyRoot, abs)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte("#!/bin/sh\n"), 0755); err != nil {
			return err
		}
	}
	return nil
}

// classifyProbeResponse maps one verbatim adapter response line onto the
// confirm decision table. Identifiers are collected from every place the
// protocol lets them appear: the error field, top-level diagnostics, and —
// on ok responses — result.reason and result.diagnostics.
func classifyProbeResponse(line string) probeOutcome {
	var resp struct {
		OK          *bool  `json:"ok"`
		Unsupported bool   `json:"unsupported"`
		Error       string `json:"error"`
		Diagnostics []struct {
			ID string `json:"id"`
		} `json:"diagnostics"`
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return probeOutcomeDegraded
	}

	ids := []string{resp.Error}
	for _, d := range resp.Diagnostics {
		ids = append(ids, d.ID)
	}
	if reason, ok := resp.Result["reason"].(string); ok {
		ids = append(ids, reason)
	}
	if rawDiags, ok := resp.Result["diagnostics"].([]any); ok {
		for _, rawDiag := range rawDiags {
			if diag, ok := rawDiag.(map[string]any); ok {
				if id, ok := diag["id"].(string); ok {
					ids = append(ids, id)
				}
			}
		}
	}

	// platform_unmappable wins over everything: it is the one identifier
	// that must never be machine-closed, whatever else the response says.
	for _, id := range ids {
		if id == "acif.hook.platform_unmappable" {
			return probeOutcomeUnmappable
		}
	}
	for _, id := range ids {
		if id == "acif.hook.platform_mechanism_malformed" {
			return probeOutcomeMalformed
		}
	}
	if resp.OK != nil && *resp.OK {
		if conformant, ok := resp.Result["conformant"].(bool); ok && conformant {
			return probeOutcomeClean
		}
	}
	return probeOutcomeOther
}

// acifAdapter is a running conformance adapter (protocol 2): one JSON
// request per line on stdin, one response per line on stdout, in order.
// Any transport failure or timeout marks it dead for the rest of the run.
type acifAdapter struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	timeout time.Duration
	hello   adapterHello
	dead    bool
}

type adapterHello struct {
	Implementation  string
	Version         string
	AdapterProtocol int
	Scopes          []string
}

func startACIFAdapter(path string, timeout time.Duration) (*acifAdapter, error) {
	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("adapter stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("adapter stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr // protocol: stderr is log passthrough, never parsed
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start adapter %s: %w", path, err)
	}
	adapter := &acifAdapter{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReaderSize(stdout, 64*1024),
		timeout: timeout,
	}

	line, err := adapter.roundTrip(`{"op":"hello","runner_protocol":2}`)
	if err != nil {
		adapter.close()
		return nil, fmt.Errorf("adapter hello: %w", err)
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Implementation  string   `json:"implementation"`
			Version         string   `json:"version"`
			AdapterProtocol int      `json:"adapter_protocol"`
			Scopes          []string `json:"scopes"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil || !resp.OK {
		adapter.close()
		return nil, fmt.Errorf("adapter hello response not ok: %q", line)
	}
	adapter.hello = adapterHello(resp.Result)
	// Protocol 2 is required: under protocol 1, reason identifiers are
	// never asserted, so response classification would be untrustworthy.
	if adapter.hello.AdapterProtocol != 2 {
		adapter.close()
		return nil, fmt.Errorf("adapter declares protocol %d; confirm requires 2", adapter.hello.AdapterProtocol)
	}
	hookScope := false
	for _, scope := range adapter.hello.Scopes {
		if scope == "hook" {
			hookScope = true
			break
		}
	}
	if !hookScope {
		adapter.close()
		return nil, fmt.Errorf("adapter %s (%s) does not claim the hook scope", adapter.hello.Implementation, adapter.hello.Version)
	}
	return adapter, nil
}

func (a *acifAdapter) roundTrip(line string) (string, error) {
	if a.dead {
		return "", fmt.Errorf("adapter is dead")
	}
	if _, err := io.WriteString(a.stdin, line+"\n"); err != nil {
		a.markDead()
		return "", fmt.Errorf("write to adapter: %w", err)
	}
	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		s, err := a.stdout.ReadString('\n')
		ch <- readResult{s, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			a.markDead()
			return "", fmt.Errorf("read adapter response: %w", r.err)
		}
		return strings.TrimRight(r.line, "\n"), nil
	case <-time.After(a.timeout):
		a.markDead()
		return "", fmt.Errorf("adapter response timed out after %s", a.timeout)
	}
}

func (a *acifAdapter) markDead() {
	a.dead = true
	if a.cmd.Process != nil {
		a.cmd.Process.Kill() //nolint:errcheck // best-effort teardown of a dead adapter
	}
}

func (a *acifAdapter) close() {
	a.dead = true
	a.stdin.Close() //nolint:errcheck // best-effort teardown
	if a.cmd.Process != nil {
		a.cmd.Process.Kill() //nolint:errcheck // best-effort teardown
	}
	a.cmd.Wait() //nolint:errcheck // exit status of a killed adapter is irrelevant
}

// confirmCandidateState is the durable confirm-report entry for one
// candidate slot, written under cacheRoot regardless of verdict.
type confirmCandidateState struct {
	ConfirmFinding
	ConfirmedAt time.Time `json:"confirmed_at"`
}

func confirmStateKeyHash(cand ConfirmFinding) string {
	return acifUnmappedKeyHash(cand.ContentType, cand.Provider, cand.CanonicalKey)
}

func confirmStatePath(cacheRoot string, finding ConfirmFinding) string {
	return filepath.Join(cacheRoot, acifStateDir, "confirm", finding.Provider, confirmStateKeyHash(finding)+".json")
}

func readConfirmState(path string) (confirmCandidateState, error) {
	var state confirmCandidateState
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read confirm state %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse confirm state %s: %w", path, err)
	}
	return state, nil
}

func persistConfirmState(cacheRoot string, finding ConfirmFinding, now time.Time) error {
	path := confirmStatePath(cacheRoot, finding)
	prior, err := readConfirmState(path)
	if err != nil {
		return err
	}
	// A slot that was unconfirmed with an open issue and later machine-
	// rejects keeps the issue number for audit; the issue is not touched.
	if finding.IssueNumber == 0 {
		finding.IssueNumber = prior.IssueNumber
	}
	return writeJSONState(path, &confirmCandidateState{ConfirmFinding: finding, ConfirmedAt: now})
}

func acifConfirmAnchor(finding ConfirmFinding) string {
	return fmt.Sprintf("<!-- capmon-acif-change-confirm: %s/%s/%s/%s -->",
		finding.Provider, finding.ContentType, finding.CanonicalKey, confirmStateKeyHash(finding))
}

// fileOrRefreshConfirmIssue is the DIRECT needs-human filing path for
// unconfirmed candidates. It deliberately does not go through
// RecordUnmappedForm: the event-sourced observation counters must never be
// polluted by curator-candidate confirmation traffic. priorIssue, when
// nonzero, is the issue this slot already filed (from confirm state);
// otherwise the dedup anchor is searched across open issues.
func fileOrRefreshConfirmIssue(finding ConfirmFinding, priorIssue int) (int, error) {
	if priorIssue > 0 {
		if err := appendConfirmIssueComment(priorIssue, finding); err != nil {
			return priorIssue, err
		}
		return priorIssue, nil
	}
	labels := []string{acifChangeLabel, acifClassBLabel, acifNeedsHumanLabel}
	anchor := acifConfirmAnchor(finding)
	issueNum, found, err := findOpenIssueByAnchor(acifChangeRepo, labels, anchor)
	if err != nil {
		return 0, err
	}
	if found {
		if err := appendConfirmIssueComment(issueNum, finding); err != nil {
			return issueNum, err
		}
		return issueNum, nil
	}

	title := fmt.Sprintf("acif-change: unconfirmed unmapped candidate %s/%s on %s (needs human)",
		finding.ContentType, finding.CanonicalKey, finding.Provider)
	body := buildConfirmIssueBody(finding)
	out, err := ghRunner("issue", "create",
		"--repo", acifChangeRepo,
		"--title", title,
		"--label", acifChangeLabel,
		"--label", acifClassBLabel,
		"--label", acifNeedsHumanLabel,
		"--body", body,
	)
	if err != nil {
		return 0, fmt.Errorf("gh issue create: %w", err)
	}
	return parseGHIssueNumber(out)
}

func buildConfirmIssueBody(finding ConfirmFinding) string {
	var b strings.Builder
	b.WriteString(acifConfirmAnchor(finding))
	b.WriteString("\n\nA curator-flagged unmapped mechanism could not be machine-confirmed as already-mapped. Per the confirm invariant this lands with a human, never a machine closure.\n\n")
	fmt.Fprintf(&b, "**Provider:** `%s`\n", finding.Provider)
	fmt.Fprintf(&b, "**Content type:** `%s`\n", finding.ContentType)
	fmt.Fprintf(&b, "**Canonical key:** `%s`\n", finding.CanonicalKey)
	token := finding.MechanismToken
	if token == "" {
		token = "(none)"
	}
	fmt.Fprintf(&b, "**Mechanism token claim:** `%s`\n", token)
	fmt.Fprintf(&b, "**Machine verdict:** unconfirmed — `%s`\n", finding.Reason)
	fmt.Fprintf(&b, "**Detail:** %s\n", finding.Detail)
	b.WriteString("\n## Pinned evidence\n\n")
	fmt.Fprintf(&b, "**source-mechanisms.yaml rev:** `%s`\n", finding.Evidence.ExportRev)
	if finding.Evidence.Implementation != "" {
		fmt.Fprintf(&b, "**Probe implementation:** `%s` version `%s` (adapter protocol %d)\n",
			finding.Evidence.Implementation, finding.Evidence.ImplVersion, finding.Evidence.AdapterProtocol)
	}
	if finding.Evidence.AdapterResponse != "" {
		b.WriteString("**Adapter response (verbatim):**\n\n")
		b.WriteString(fencedACIFBlock(finding.Evidence.AdapterResponse))
		b.WriteString("\n")
	}
	if finding.Evidence.SourceForm != "" {
		b.WriteString("**Source form:**\n\n")
		b.WriteString(fencedACIFBlock(finding.Evidence.SourceForm))
		b.WriteString("\n")
	}
	b.WriteString("\n**What to do:** Review the ACIF `CHANGE-PROCESS.md` Class B section and decide whether this mechanism needs a mapping row, new vocabulary, or a curator correction.\n")
	b.WriteString("Reference: https://github.com/OpenScribbler/agent-content-interchange-format/blob/main/CHANGE-PROCESS.md\n")
	return b.String()
}

func appendConfirmIssueComment(issueNum int, finding ConfirmFinding) error {
	body := fmt.Sprintf(
		"Confirm re-run: still unconfirmed (`%s`) against export rev `%s`. %s",
		finding.Reason, finding.Evidence.ExportRev, finding.Detail,
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
