package capmon

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// --- fixture data -----------------------------------------------------------

// fixtureExportRegistry is a trimmed canonical-key registry carrying all six
// real content types (skills, rules, hooks, mcp, agents, commands) so the tree
// always emits six by-content-type pivots regardless of which types the
// baselines declare.
const fixtureExportRegistry = `content_types:
  skills:
    display_name:
      description: Human-readable display name for the skill.
      type: string
      spec_ref: ACIF-SKILL §10.2 (OUT-OF-SCOPE-AT-L1)
  rules:
    file_imports:
      description: Whether rules files may import other files.
      type: bool
      spec_ref: ACIF-RULE §2.1 (DERIVABLE)
  hooks:
    handler_types:
      description: Types of hook handlers a provider recognizes.
      type: string
      spec_ref: ACIF-HOOK §1.1 (DERIVABLE)
  mcp:
    transport_types:
      description: Supported MCP transport mechanisms.
      type: object
      spec_ref: ACIF-MCP §3.1 (DERIVABLE)
  agents:
    invocation_patterns:
      description: How subagents are invoked.
      type: object
      spec_ref: ACIF-AGENT §9.1 (DERIVABLE)
    model_selection:
      description: Per-agent model override.
      type: bool
      spec_ref: ACIF-AGENT §9.2 (DERIVABLE)
  commands:
    argument_hint:
      description: Hint text for command arguments.
      type: string
      spec_ref: ACIF-COMMAND §4.1 (DERIVABLE)
`

// fixtureAlpha declares agents (registry-backed branch + leaf, with a
// vocabulary-member child) and hooks (an event, no capabilities); it carries a
// last_verified so the index-entry omission logic is exercised.
const fixtureAlpha = `schema_version: "1"
slug: alpha
display_name: Alpha
last_verified: "2026-07-10"
content_types:
  agents:
    supported: true
    capabilities:
      invocation_patterns:
        supported: true
        capabilities:
          at_mention:
            supported: true
            mechanism: "@-mention syntax"
            confidence: inferred
      model_selection:
        supported: false
        mechanism: per-subagent override
        confidence: inferred
  hooks:
    supported: true
    events:
      pre_tool:
        native_name: PreToolUse
        blocking: deny
`

// fixtureBravo declares skills and mcp; no last_verified.
const fixtureBravo = `schema_version: "1"
slug: bravo
display_name: Bravo
content_types:
  skills:
    supported: true
    capabilities:
      display_name:
        supported: true
        mechanism: frontmatter name field
        confidence: confirmed
  mcp:
    supported: true
    capabilities:
      transport_types:
        supported: true
        capabilities:
          stdio:
            supported: true
`

// fixtureCharlie is minimal: one registry-backed leaf under rules; no
// display_name (so the slug fallback applies) and no last_verified.
const fixtureCharlie = `schema_version: "1"
slug: charlie
content_types:
  rules:
    supported: true
    capabilities:
      file_imports:
        supported: true
        confidence: confirmed
`

// --- shared fixture + helpers -----------------------------------------------

// newExportFixture writes three minimal baselines, the six-content-type
// registry, and empty source/asset dirs to temp dirs, returning ExportOptions
// wired to them. An empty PublishAssetsDir means the staged tree is exactly the
// generated document set. GeneratedAt is pinned; SourceCommit is empty by
// default and set by the caller when a source_commit is under test.
func newExportFixture(t *testing.T) ExportOptions {
	t.Helper()

	capsDir := t.TempDir()
	for name, body := range map[string]string{
		"alpha.yaml":   fixtureAlpha,
		"bravo.yaml":   fixtureBravo,
		"charlie.yaml": fixtureCharlie,
	} {
		if err := os.WriteFile(filepath.Join(capsDir, name), []byte(body), 0644); err != nil {
			t.Fatalf("write baseline %s: %v", name, err)
		}
	}

	regPath := filepath.Join(t.TempDir(), "canonical-keys.yaml")
	if err := os.WriteFile(regPath, []byte(fixtureExportRegistry), 0644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	return ExportOptions{
		CapsDir:           capsDir,
		CanonicalKeysPath: regPath,
		SourcesDir:        t.TempDir(), // empty — provider-set gate lands in slice 4
		PublishAssetsDir:  t.TempDir(), // empty — exact generated layout
		GeneratedAt:       "2026-07-12T09:00:00Z",
	}
}

// readJSONMap parses a staged JSON document as a map[string]any. Numbers parse
// to float64, so structural comparisons via reflect.DeepEqual are apples-to-
// apples across two parsed documents.
func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return m
}

// walkRelFiles returns every file under root as a sorted slash-separated path
// relative to root.
func walkRelFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

// assertSingleTrailingLF fails unless the file ends with exactly one LF — a
// canonicalization-profile basic every staged JSON document must satisfy.
func assertSingleTrailingLF(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Errorf("%s is empty", path)
		return
	}
	if data[len(data)-1] != '\n' {
		t.Errorf("%s does not end with a trailing LF", path)
	}
	if len(data) >= 2 && data[len(data)-2] == '\n' {
		t.Errorf("%s ends with more than one trailing LF", path)
	}
}

// --- tests ------------------------------------------------------------------

func TestWriteExportTreeLayout(t *testing.T) {
	opts := newExportFixture(t)
	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree: %v", err)
	}

	want := []string{
		"index.json",
		"v1/advisories.json",
		"v1/by-content-type/agents.json",
		"v1/by-content-type/commands.json",
		"v1/by-content-type/hooks.json",
		"v1/by-content-type/mcp.json",
		"v1/by-content-type/rules.json",
		"v1/by-content-type/skills.json",
		"v1/capabilities/all.json",
		"v1/capabilities/alpha.json",
		"v1/capabilities/bravo.json",
		"v1/capabilities/charlie.json",
		"v1/index.json",
		"v1/spec/canonical-keys.json",
	}
	sort.Strings(want)

	got := walkRelFiles(t, dst)
	if !reflect.DeepEqual(got, want) {
		wantSet := map[string]bool{}
		for _, w := range want {
			wantSet[w] = true
		}
		gotSet := map[string]bool{}
		for _, g := range got {
			gotSet[g] = true
			if !wantSet[g] {
				t.Errorf("unexpected staged path: %q", g)
			}
		}
		for _, w := range want {
			if !gotSet[w] {
				t.Errorf("missing staged path: %q", w)
			}
		}
		t.Errorf("staged layout mismatch\n got: %v\nwant: %v", got, want)
	}

	// Profile basic: every staged JSON file ends with exactly one trailing LF.
	for _, rel := range got {
		if strings.HasSuffix(rel, ".json") {
			assertSingleTrailingLF(t, filepath.Join(dst, filepath.FromSlash(rel)))
		}
	}
}

func TestBuildAllAndPivotsShareNodeShape(t *testing.T) {
	opts := newExportFixture(t)
	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree: %v", err)
	}

	providerDoc := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", "alpha.json"))

	// all.json reuses the exact per-provider document object under providers.<slug>.
	allDoc := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", "all.json"))
	if allDoc["schema_version"] != "1" {
		t.Errorf("all.json schema_version = %v, want \"1\"", allDoc["schema_version"])
	}
	alphaInAll := mustChild(t, mustChild(t, allDoc, "providers"), "alpha")
	if !reflect.DeepEqual(alphaInAll, providerDoc) {
		t.Errorf("all.json providers.alpha differs from capabilities/alpha.json\n all: %v\n doc: %v", alphaInAll, providerDoc)
	}

	// The agents pivot reuses the exact content-type node from the provider doc.
	pivot := readJSONMap(t, filepath.Join(dst, "v1", "by-content-type", "agents.json"))
	if pivot["content_type"] != "agents" {
		t.Errorf("agents pivot content_type = %v, want \"agents\"", pivot["content_type"])
	}
	if pivot["schema_version"] != "1" {
		t.Errorf("agents pivot schema_version = %v, want \"1\"", pivot["schema_version"])
	}
	alphaNode := mustChild(t, mustChild(t, pivot, "providers"), "alpha")
	agentsNode := mustChild(t, mustChild(t, providerDoc, "content_types"), "agents")
	if !reflect.DeepEqual(alphaNode, agentsNode) {
		t.Errorf("agents pivot providers.alpha differs from provider doc content_types.agents\n pivot: %v\n doc:   %v", alphaNode, agentsNode)
	}
}

// TestProviderStatusJoin verifies the source-manifest lifecycle status lands
// in each provider doc as provider_status, and that a baseline with no
// manifest omits the field (absent = unknown; the EXPORT_003 gate catches the
// set mismatch separately).
func TestProviderStatusJoin(t *testing.T) {
	opts := newExportFixture(t)
	for name, body := range map[string]string{
		"alpha.yaml": "schema_version: \"1\"\nslug: alpha\nstatus: active\ndisplay_name: Manifest Alpha\n",
		"bravo.yaml": "schema_version: \"1\"\nslug: bravo\nstatus: archived\n",
		// charlie deliberately has no manifest.
	} {
		if err := os.WriteFile(filepath.Join(opts.SourcesDir, name), []byte(body), 0644); err != nil {
			t.Fatalf("write manifest %s: %v", name, err)
		}
	}

	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree: %v", err)
	}

	for slug, want := range map[string]string{"alpha": "active", "bravo": "archived"} {
		doc := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", slug+".json"))
		if doc["provider_status"] != want {
			t.Errorf("%s provider_status = %v, want %q", slug, doc["provider_status"], want)
		}
		// The document-lifecycle status stays live regardless of provider health.
		if doc["status"] != "live" {
			t.Errorf("%s status = %v, want \"live\"", slug, doc["status"])
		}
	}

	charlie := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", "charlie.json"))
	if v, ok := charlie["provider_status"]; ok {
		t.Errorf("charlie provider_status = %v, want field absent", v)
	}

	// display_name precedence: a non-empty baseline value beats the manifest;
	// a provider with no manifest keeps the slug fallback.
	alpha := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", "alpha.json"))
	if alpha["display_name"] != "Alpha" {
		t.Errorf("alpha display_name = %v, want baseline \"Alpha\" (manifest must not win over a set baseline)", alpha["display_name"])
	}
	if charlie["display_name"] != "charlie" {
		t.Errorf("charlie display_name = %v, want slug fallback \"charlie\"", charlie["display_name"])
	}
}

// TestExportTreeRealData runs the exporter over the committed docs/ tree. The
// slice-2 relocation of non-canonical nodes has landed, so writeExportTree
// builds all 15 provider docs cleanly.
func TestExportTreeRealData(t *testing.T) {
	root := docsRoot(t)
	opts := ExportOptions{
		CapsDir:           filepath.Join(root, "docs", "provider-capabilities"),
		CanonicalKeysPath: canonicalKeysPath(t),
		SourcesDir:        filepath.Join(root, "docs", "provider-sources"),
		PublishAssetsDir:  t.TempDir(), // empty
		GeneratedAt:       "2026-07-12T09:00:00Z",
	}
	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree over real docs: %v", err)
	}

	docs, err := filepath.Glob(filepath.Join(dst, "v1", "capabilities", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	providerCount := 0
	for _, p := range docs {
		if filepath.Base(p) != "all.json" {
			providerCount++
		}
	}
	if providerCount != 15 {
		t.Errorf("staged provider doc count = %d, want 15", providerCount)
	}

	idx := readJSONMap(t, filepath.Join(dst, "v1", "index.json"))
	provs, ok := idx["providers"].([]any)
	if !ok {
		t.Fatalf("v1/index.json providers is not an array: %T", idx["providers"])
	}
	if len(provs) != 15 {
		t.Errorf("v1/index.json lists %d providers, want 15", len(provs))
	}
	for _, pv := range provs {
		pm := mustMap(t, pv)
		if pm["status"] != "tracked" {
			t.Errorf("provider %v status = %v, want \"tracked\"", pm["slug"], pm["status"])
		}
	}

	// Every committed baseline has a source manifest, so every real provider
	// doc carries provider_status; the sunset providers surface as archived.
	for _, p := range docs {
		if filepath.Base(p) == "all.json" {
			continue
		}
		doc := readJSONMap(t, p)
		if s, _ := doc["provider_status"].(string); s == "" {
			t.Errorf("%s: provider_status missing or empty", filepath.Base(p))
		}
	}
	for slug, want := range map[string]string{"roo-code": "archived", "claude-code": "active"} {
		doc := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", slug+".json"))
		if doc["provider_status"] != want {
			t.Errorf("%s provider_status = %v, want %q", slug, doc["provider_status"], want)
		}
	}

	// Every committed baseline leaves display_name empty, so the manifest
	// display name joins in — the published value must not be the slug.
	rooCode := readJSONMap(t, filepath.Join(dst, "v1", "capabilities", "roo-code.json"))
	if rooCode["display_name"] != "Roo Code" {
		t.Errorf("roo-code display_name = %v, want manifest \"Roo Code\"", rooCode["display_name"])
	}
}

// TestSpecArtifactsCopiedVerbatim stages the fixture tree with the committed
// publish assets (docs/publish) and asserts every asset — each schema under
// v1/schemas/ and v1/spec/field-semantics.md — lands byte-identical to its
// committed source. The published schemas and spec are contract artifacts:
// export copies them verbatim, it never regenerates them.
func TestSpecArtifactsCopiedVerbatim(t *testing.T) {
	assetsDir := filepath.Join(docsRoot(t), "docs", "publish")
	opts := newExportFixture(t)
	opts.PublishAssetsDir = assetsDir
	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree: %v", err)
	}

	seen := 0
	found := make(map[string]bool)
	err := filepath.WalkDir(assetsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(assetsDir, p)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		staged, err := os.ReadFile(filepath.Join(dst, "v1", rel))
		if err != nil {
			t.Errorf("staged asset v1/%s missing: %v", filepath.ToSlash(rel), err)
			return nil
		}
		if !bytes.Equal(src, staged) {
			t.Errorf("staged v1/%s differs from committed docs/publish/%s", filepath.ToSlash(rel), filepath.ToSlash(rel))
		}
		seen++
		found[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk publish assets: %v", err)
	}
	if seen == 0 {
		t.Fatalf("no publish assets found under %s", assetsDir)
	}
	// The acceptance-named artifacts must exist by their exact relative paths;
	// without this the walk passes vacuously against an incomplete docs/publish.
	for _, rel := range []string{
		"schemas/provider-capabilities.json",
		"schemas/all-providers.json",
		"schemas/by-content-type.json",
		"schemas/index.json",
		"schemas/advisories.json",
		"schemas/canonical-keys.json",
		"spec/field-semantics.md",
	} {
		if !found[rel] {
			t.Errorf("committed publish asset missing: docs/publish/%s", rel)
		}
	}
}
