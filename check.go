package capmon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CapmonCheckOptions configures a RunCapmonCheck run.
type CapmonCheckOptions struct {
	// FormatsDir is the directory containing provider format doc YAML files
	// (default: "docs/provider-formats").
	FormatsDir string
	// SourcesDir is the directory containing provider source manifests
	// (default: "docs/provider-sources").
	SourcesDir string
	// CacheRoot is the root of the capmon cache (default: ".capmon-cache").
	CacheRoot string
	// CanonicalKeysPath is the path to canonical-keys.yaml
	// (default: "docs/spec/canonical-keys.yaml").
	CanonicalKeysPath string
	// ProviderFilter limits the run to a single provider slug. Empty means all.
	ProviderFilter string
	// DryRun logs actions but makes no GitHub API calls.
	DryRun bool
}

// sourceChange records a single content-hash change event for accumulation.
type sourceChange struct {
	contentType string
	sourceURI   string
	oldHash     string
	newHash     string
}

// fetchErrorEntry records a single fetch/validity failure for accumulation.
type fetchErrorEntry struct {
	contentType string
	sourceURI   string
	reason      string
}

// providerBatch accumulates all change events and fetch errors for one provider
// across the full content-type/source loop. The flush phase reads it once
// after the inner loops complete.
type providerBatch struct {
	changes     []sourceChange
	fetchErrors []fetchErrorEntry
}

func (b *providerBatch) isEmpty() bool {
	return len(b.changes) == 0 && len(b.fetchErrors) == 0
}

// buildProviderIssueBody assembles the multi-section issue body from a provider
// batch. Returns an empty string when the batch is empty.
func buildProviderIssueBody(batch *providerBatch) string {
	if batch.isEmpty() {
		return ""
	}
	var sb strings.Builder

	// Group changes by content type for deterministic section ordering.
	byType := make(map[string][]sourceChange, len(batch.changes))
	for _, c := range batch.changes {
		byType[c.contentType] = append(byType[c.contentType], c)
	}
	cts := make([]string, 0, len(byType))
	for ct := range byType {
		cts = append(cts, ct)
	}
	sort.Strings(cts)
	for _, ct := range cts {
		fmt.Fprintf(&sb, "## %s\n\n", ct)
		for _, c := range byType[ct] {
			fmt.Fprintf(&sb, "- %s\n  Old hash: %s\n  New hash: %s\n\n", c.sourceURI, c.oldHash, c.newHash)
		}
	}

	if len(batch.fetchErrors) > 0 {
		fmt.Fprintf(&sb, "## Fetch Errors\n\n")
		for _, fe := range batch.fetchErrors {
			fmt.Fprintf(&sb, "- %s (%s): %s\n", fe.sourceURI, fe.contentType, fe.reason)
		}
	}
	return sb.String()
}

// flushProviderBatch writes at most one GitHub issue for the accumulated batch.
// If an open issue already exists for the provider, this is a silent no-op.
// Does nothing when the batch is empty. DryRun logs a summary to stderr and skips
// all GitHub calls.
func flushProviderBatch(ctx context.Context, opts CapmonCheckOptions, provider string, batch *providerBatch) error {
	if batch.isEmpty() {
		return nil
	}
	if opts.DryRun {
		fmt.Fprintf(os.Stderr, "dry-run: would create issue for %s (%d changes, %d fetch errors)\n",
			provider, len(batch.changes), len(batch.fetchErrors))
		return nil
	}
	// Two concurrent runs can still produce a duplicate if both call
	// FindOpenCapmonProviderIssue before either creates an issue. The window is
	// narrow (one race opportunity per provider per run, not per content type) and
	// duplicates are dedup-detectable by anchor. See ADR-0009.
	_, found, err := FindOpenCapmonProviderIssue(provider)
	if err != nil {
		return fmt.Errorf("find provider issue for %s: %w", provider, err)
	}
	if found {
		return nil // open issue already exists — silent skip (ADR-0010)
	}
	body := buildProviderIssueBody(batch)
	title := fmt.Sprintf("capmon: changes detected for %s", provider)
	_, err = CreateCapmonProviderIssue(ctx, provider, title, body)
	return err
}

// RunCapmonCheck runs the capmon check pipeline over all (or one filtered) provider
// format docs. It validates infrastructure, detects content drift, and creates or
// updates GitHub issues for each changed source.
//
// Pipeline:
//  1. ValidateSources for each provider (blocking)
//  2. ValidateFormatDoc for each format doc (blocking)
//  3. Fetch each source, compare hash, create/append issue on change
func RunCapmonCheck(ctx context.Context, opts CapmonCheckOptions) error {
	// Apply defaults.
	if opts.FormatsDir == "" {
		opts.FormatsDir = "docs/provider-formats"
	}
	if opts.SourcesDir == "" {
		opts.SourcesDir = "docs/provider-sources"
	}
	if opts.CacheRoot == "" {
		opts.CacheRoot = ".capmon-cache"
	}
	if opts.CanonicalKeysPath == "" {
		opts.CanonicalKeysPath = "docs/spec/canonical-keys.yaml"
	}

	// Enumerate format doc files.
	entries, err := os.ReadDir(opts.FormatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // empty dir is valid
		}
		return fmt.Errorf("read formats dir: %w", err)
	}

	var providers []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".yaml")
		if opts.ProviderFilter != "" && slug != opts.ProviderFilter {
			continue
		}
		providers = append(providers, slug)
	}

	ciMode := os.Getenv("GITHUB_TOKEN") != ""

	for _, provider := range providers {
		// Step 1: Validate source manifest (blocking).
		if err := ValidateSources(opts.SourcesDir, provider); err != nil {
			return fmt.Errorf("capmon check: %w", err)
		}

		// Step 2: Validate format doc (blocking errors + non-blocking warnings).
		// In CI (GITHUB_TOKEN set): route warnings to GitHub issues with dedup.
		// Locally: log to stderr.
		warnings, err := ValidateFormatDocWithWarnings(opts.FormatsDir, opts.CanonicalKeysPath, provider)
		if err != nil {
			return fmt.Errorf("capmon check: validate format doc for %s: %w", provider, err)
		}

		seenKeys := make(map[string]bool, len(warnings))
		for _, w := range warnings {
			seenKeys[w.DeduplicationKey()] = true
			fmt.Fprintf(os.Stderr, "warning: format doc for %q: [%s] %s: %s\n",
				provider, w.DeduplicationKey(), w.Field, w.Message)

			if ciMode && !opts.DryRun {
				issueNum, found, findErr := FindOpenCapmonWarningIssue(provider, w)
				if findErr != nil {
					fmt.Fprintf(os.Stderr, "warning: find warning issue for %s: %v\n", provider, findErr)
					continue
				}
				if found {
					_ = AppendCapmonChangeEvent(ctx, issueNum,
						fmt.Sprintf("Still present: %s = `%s`", w.Field, w.Value))
				} else {
					_, createErr := CreateCapmonWarningIssue(ctx, provider, w)
					if createErr != nil {
						fmt.Fprintf(os.Stderr, "warning: create warning issue for %s: %v\n", provider, createErr)
					}
				}
			}
		}

		// Auto-close resolved warning issues when in CI.
		if ciMode && !opts.DryRun {
			if closeErr := CloseResolvedWarningIssues(ctx, provider, seenKeys); closeErr != nil {
				fmt.Fprintf(os.Stderr, "warning: close resolved issues for %s: %v\n", provider, closeErr)
			}
		}

		// Step 3: Fetch and compare each source URI, accumulating changes and
		// fetch errors into a per-provider batch for deferred issue creation.
		doc, err := LoadFormatDoc(FormatDocPath(opts.FormatsDir, provider))
		if err != nil {
			return fmt.Errorf("capmon check: load format doc for %s: %w", provider, err)
		}

		batch := &providerBatch{}
		for ct, ctDoc := range doc.ContentTypes {
			for _, src := range ctDoc.Sources {
				if err := runSourceCheck(ctx, ct, src, batch); err != nil {
					return err
				}
			}
		}
		if err := flushProviderBatch(ctx, opts, provider, batch); err != nil {
			return fmt.Errorf("capmon check: flush batch for %s: %w", provider, err)
		}
	}

	return nil
}

// runSourceCheck fetches one source URI, validates the response, compares the
// hash against the stored value in the format doc, and records the result into
// batch for deferred issue creation. All GitHub API calls are deferred to
// flushProviderBatch, which fires once after the full provider loop completes.
func runSourceCheck(ctx context.Context, contentType string, src SourceRef, batch *providerBatch) error {
	// local_file sources live in the repo; drift is detected by git, not HTTP.
	if src.FetchMethod == "local_file" {
		return nil
	}

	// Fetch content.
	body, respContentType, finalURL, fetchErr := fetchForCheck(ctx, src.URI)
	if fetchErr != nil {
		logOrCreateFetchErrorIssue(contentType, src.URI,
			fmt.Sprintf("fetch error: %v", fetchErr), batch)
		return nil
	}

	// Validate content response.
	if err := ValidateContentResponse(body, respContentType, src.URI, finalURL); err != nil {
		logOrCreateFetchErrorIssue(contentType, src.URI,
			fmt.Sprintf("content invalid: %v", err), batch)
		return nil
	}

	// Compare hash.
	newHash := SHA256Hex(body)
	if src.ContentHash != "" && src.ContentHash == newHash {
		return nil // no change
	}

	// Content changed (or first fetch — empty hash). Accumulate into batch.
	batch.changes = append(batch.changes, sourceChange{
		contentType: contentType,
		sourceURI:   src.URI,
		oldHash:     src.ContentHash,
		newHash:     newHash,
	})
	return nil
}

// HashURLResult carries what `capmon hash-url` reports for one fetch.
type HashURLResult struct {
	Hash        string
	ContentType string
	FinalURL    string
	Size        int
}

// HashURL fetches rawURL exactly as the drift check does (fetchForCheck) and
// returns the canonical content hash with fetch metadata. It exists so
// curation can compute valid baselines without replicating the check path's
// header behavior by hand — curl is NOT byte-equivalent on every host.
func HashURL(ctx context.Context, rawURL string) (HashURLResult, error) {
	body, ct, fu, err := fetchForCheck(ctx, rawURL)
	if err != nil {
		return HashURLResult{}, err
	}
	return HashURLResult{Hash: SHA256Hex(body), ContentType: ct, FinalURL: fu, Size: len(body)}, nil
}

// logOrCreateFetchErrorIssue records a fetch/validity failure into the provider
// batch for deferred issue creation. DryRun is handled by flushProviderBatch.
func logOrCreateFetchErrorIssue(contentType, sourceURI, reason string, batch *providerBatch) {
	batch.fetchErrors = append(batch.fetchErrors, fetchErrorEntry{
		contentType: contentType,
		sourceURI:   sourceURI,
		reason:      reason,
	})
}

// HTTPDoer is the minimal client surface CheckFetch needs; callers pass their
// own (test-overridable) client.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// CheckFetch performs the canonical baseline fetch: a direct HTTP GET with
// pinned content negotiation, returning the body, Content-Type header, final
// URL (after redirects), and any error. EVERY code path that computes or
// compares a content_hash baseline (capmon check, capmon backfill, capmon
// hash-url, provmon's source-hash checker) MUST fetch through this function —
// the pinned headers ARE the hash contract:
//   - User-Agent: capmon/1.0
//   - Accept: text/markdown for *.md paths (code.claude.com intermittently
//     serves rendered HTML without it), */* otherwise (curl's default;
//     Mintlify hosts vary bytes on its absence).
//   - Accept-Encoding: identity, so the transport's implicit gzip never
//     shapes the hashed bytes. A response that ignores the request and
//     compresses anyway fails loudly instead of hashing wire bytes.
//
// The canonical hash recipe is therefore:
//
//	curl -H 'User-Agent: capmon/1.0' -H 'Accept: <as above>' \
//	     -H 'Accept-Encoding: identity' -L <url> | sha256sum
//
// or, without remembering any of that, `capmon hash-url <url>`.
func CheckFetch(ctx context.Context, doer HTTPDoer, rawURL string) (body []byte, contentType, finalURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "capmon/1.0")
	if strings.HasSuffix(req.URL.Path, ".md") {
		req.Header.Set("Accept", "text/markdown")
	} else {
		req.Header.Set("Accept", "*/*")
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := doer.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "" && !strings.EqualFold(ce, "identity") {
		return nil, "", "", fmt.Errorf("unexpected Content-Encoding %q (requested identity)", ce)
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("read body: %w", err)
	}
	ct := resp.Header.Get("Content-Type")
	// Final URL: use the request URL from the response (set by http.Client after redirects).
	fu := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		fu = resp.Request.URL.String()
	}
	return body, ct, fu, nil
}

// fetchForCheck is CheckFetch over the package httpDoer (overridable in tests
// via SetHTTPClientForTest).
func fetchForCheck(ctx context.Context, rawURL string) (body []byte, contentType, finalURL string, err error) {
	return CheckFetch(ctx, httpDoer, rawURL)
}
