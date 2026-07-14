package capmon

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// --- committed-fixture helpers ----------------------------------------------

// committedFixtureRoot resolves testdata/fixtures/export/ — the self-contained
// mini-world (two baselines, a six-content-type registry, matching source
// manifests) whose byte-exact export is committed under expected/.
func committedFixtureRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(docsRoot(t), "testdata", "fixtures", "export")
	if _, err := os.Stat(filepath.Join(root, "caps")); err != nil {
		t.Fatalf("export fixture input not found under %s: %v", root, err)
	}
	return root
}

// committedFixtureOpts returns ExportOptions wired to the committed fixture
// input. PublishAssetsDir points at the REAL docs/publish so the gate validates
// generated documents against the versioned contract schemas rather than a
// duplicated fixture copy that could drift. OutDir/GeneratedAt/SourceCommit are
// set by each caller.
func committedFixtureOpts(t *testing.T) ExportOptions {
	t.Helper()
	root := committedFixtureRoot(t)
	return ExportOptions{
		CapsDir:           filepath.Join(root, "caps"),
		CanonicalKeysPath: filepath.Join(root, "registry.yaml"),
		SourcesDir:        filepath.Join(root, "sources"),
		PublishAssetsDir:  filepath.Join(docsRoot(t), "docs", "publish"),
	}
}

// copiedAssetRels returns the OutDir-relative slash paths of every verbatim
// publish asset (schemas + spec/field-semantics.md), i.e. "v1/<rel>" for each
// file under publishDir. These are asserted byte-equal to their docs/publish
// sources, not committed a second time under expected/.
func copiedAssetRels(t *testing.T, publishDir string) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, rel := range walkRelFiles(t, publishDir) {
		set["v1/"+rel] = true
	}
	return set
}

// diffTrees returns the sorted set of OutDir-relative paths that differ between
// two staged trees: present in only one, or present in both with unequal bytes.
func diffTrees(t *testing.T, a, b string) []string {
	t.Helper()
	aFiles := walkRelFiles(t, a)
	bFiles := walkRelFiles(t, b)
	inB := map[string]bool{}
	for _, r := range bFiles {
		inB[r] = true
	}
	inA := map[string]bool{}
	for _, r := range aFiles {
		inA[r] = true
	}

	diff := map[string]bool{}
	for _, r := range aFiles {
		if !inB[r] {
			diff[r] = true
			continue
		}
		if !bytes.Equal(readFileBytes(t, filepath.Join(a, filepath.FromSlash(r))), readFileBytes(t, filepath.Join(b, filepath.FromSlash(r)))) {
			diff[r] = true
		}
	}
	for _, r := range bFiles {
		if !inA[r] {
			diff[r] = true
		}
	}

	out := make([]string, 0, len(diff))
	for r := range diff {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// --- tests ------------------------------------------------------------------

// TestExportFixtureMatchesCommitted runs RunExport over the committed fixture
// with pinned GeneratedAt/SourceCommit, then byte-verifies the staged tree
// against testdata/fixtures/export/expected/. Every generated file must equal
// its committed twin, and no generated file may exist that expected/ omits
// (verbatim schema/spec assets are excluded from expected/ and instead asserted
// byte-equal to their docs/publish sources). It also checks the synthetic
// provider's document exercises the code paths real data cannot reach:
// references trimmed to {url, verified_at}, mirrored events/tools/refs, and
// keyless provider_exclusive nodes.
func TestExportFixtureMatchesCommitted(t *testing.T) {
	opts := committedFixtureOpts(t)
	opts.GeneratedAt = "2026-01-01T00:00:00Z"
	opts.SourceCommit = "0000000000000000000000000000000000000000"
	opts.OutDir = filepath.Join(t.TempDir(), "dist")

	if err := RunExport(opts); err != nil {
		t.Fatalf("RunExport over committed fixture: %v", err)
	}

	expectedRoot := filepath.Join(committedFixtureRoot(t), "expected")
	if _, err := os.Stat(expectedRoot); err != nil {
		t.Fatalf("testdata/fixtures/export/expected/ is absent: %v; run the impl bead's generation step to commit the byte-exact expected tree", err)
	}

	copied := copiedAssetRels(t, opts.PublishAssetsDir)

	// Every generated staged file must equal its committed twin under expected/.
	expectedSet := map[string]bool{}
	for _, rel := range walkRelFiles(t, expectedRoot) {
		expectedSet[rel] = true
		got := filepath.Join(opts.OutDir, filepath.FromSlash(rel))
		if _, err := os.Stat(got); err != nil {
			t.Errorf("expected file %s missing from staged tree: %v", rel, err)
			continue
		}
		if !bytes.Equal(readFileBytes(t, got), readFileBytes(t, filepath.Join(expectedRoot, filepath.FromSlash(rel)))) {
			t.Errorf("staged %s differs from committed expected/%s", rel, rel)
		}
	}

	// No generated staged file may be absent from expected/. Verbatim publish
	// assets are excluded (they are contract artifacts, asserted separately).
	for _, rel := range walkRelFiles(t, opts.OutDir) {
		if copied[rel] {
			continue
		}
		if !expectedSet[rel] {
			t.Errorf("generated staged file %s has no committed expected/%s (regenerate expected/)", rel, rel)
		}
	}

	// Verbatim assets: byte-equal to their docs/publish sources.
	for rel := range copied {
		src := filepath.Join(opts.PublishAssetsDir, filepath.FromSlash(rel[len("v1/"):]))
		staged := filepath.Join(opts.OutDir, filepath.FromSlash(rel))
		if _, err := os.Stat(staged); err != nil {
			t.Errorf("verbatim asset %s missing from staged tree: %v", rel, err)
			continue
		}
		if !bytes.Equal(readFileBytes(t, src), readFileBytes(t, staged)) {
			t.Errorf("staged %s differs from committed source docs/publish/%s", rel, rel[len("v1/"):])
		}
	}

	assertSyntheticProviderDoc(t, opts.OutDir)
}

// assertSyntheticProviderDoc verifies the export-only code paths the synthetic
// baseline exists to cover.
func assertSyntheticProviderDoc(t *testing.T, outDir string) {
	t.Helper()
	docPath := filepath.Join(outDir, "v1", "capabilities", "synthetic.json")
	raw := readFileBytes(t, docPath)

	// Pipeline-internal reference provenance must never leak into published bytes.
	if bytes.Contains(raw, []byte("fetch_method")) {
		t.Error("synthetic.json leaks fetch_method — references were not trimmed")
	}
	if bytes.Contains(raw, []byte("last_content_hash")) {
		t.Error("synthetic.json leaks last_content_hash — references were not trimmed")
	}

	doc := readJSONMap(t, docPath)

	// Empty baseline display_name joins the manifest's; empty last_verified omitted.
	if doc["display_name"] != "Synthetic Provider" {
		t.Errorf("display_name = %v, want manifest \"Synthetic Provider\"", doc["display_name"])
	}
	if _, ok := doc["last_verified"]; ok {
		t.Error("empty last_verified should be omitted from synthetic.json")
	}

	// references trimmed to {url, verified_at}.
	ref := mustChild(t, mustChild(t, doc, "references"), "spec_doc")
	if ref["url"] != "https://example.com/spec" {
		t.Errorf("references.spec_doc.url = %v", ref["url"])
	}
	if ref["verified_at"] != "2026-01-02" {
		t.Errorf("references.spec_doc.verified_at = %v", ref["verified_at"])
	}
	if len(ref) != 2 {
		t.Errorf("references.spec_doc has %d fields, want exactly 2 (url, verified_at)", len(ref))
	}

	cts := mustChild(t, doc, "content_types")

	// events mirrored with refs.
	pre := mustChild(t, mustChild(t, mustChild(t, cts, "hooks"), "events"), "pre_tool")
	if pre["native_name"] != "PreToolUse" {
		t.Errorf("event native_name = %v, want PreToolUse", pre["native_name"])
	}
	if refs := toStrings(t, pre["refs"]); !containsStr(refs, "spec_doc") {
		t.Errorf("event refs = %v, want to contain spec_doc", refs)
	}
	if pre["blocking"] != "deny" {
		t.Errorf("event blocking = %v, want plain string \"deny\"", pre["blocking"])
	}

	// tools mirrored with refs.
	search := mustChild(t, mustChild(t, mustChild(t, cts, "agents"), "tools"), "search")
	if search["native"] != "grep_tool" {
		t.Errorf("tool native = %v, want grep_tool", search["native"])
	}
	if refs := toStrings(t, search["refs"]); !containsStr(refs, "spec_doc") {
		t.Errorf("tool refs = %v, want to contain spec_doc", refs)
	}

	// node-level refs on a registry-backed capability node.
	ip := mustChild(t, mustChild(t, mustChild(t, cts, "agents"), "capabilities"), "invocation_patterns")
	if refs := toStrings(t, ip["refs"]); !containsStr(refs, "spec_doc") {
		t.Errorf("invocation_patterns refs = %v, want to contain spec_doc", refs)
	}

	// provider_exclusive nodes: capability-shaped, keyless, keyed by <ct>.<name>.
	pe := mustChild(t, doc, "provider_exclusive")
	node := mustChild(t, pe, "skills.custom_thing")
	assertSupported(t, node, true)
	assertNoKeyMetadata(t, node)
	nested := mustChild(t, mustChild(t, node, "capabilities"), "nested_detail")
	assertSupported(t, nested, false)
	assertNoKeyMetadata(t, nested)
}

// TestDoubleExportDeterminism runs RunExport twice over the REAL repo inputs
// (via defaults) with identical pinned GeneratedAt/SourceCommit into two temp
// OutDirs and asserts the trees are byte-identical file-for-file, both
// directions. This is the determinism guarantee: identical source data yields
// identical output.
func TestDoubleExportDeterminism(t *testing.T) {
	run := func() string {
		dst := filepath.Join(t.TempDir(), "dist")
		opts := ExportOptions{
			OutDir:       dst,
			GeneratedAt:  "2026-01-01T00:00:00Z",
			SourceCommit: "0000000000000000000000000000000000000000",
		}
		if err := RunExport(opts); err != nil {
			t.Fatalf("RunExport over real docs (defaults): %v", err)
		}
		return dst
	}

	a := run()
	b := run()

	if diff := diffTrees(t, a, b); len(diff) != 0 {
		t.Errorf("two exports of identical source data diverge in %d file(s): %v", len(diff), diff)
	}
}

// TestGeneratedAtConfinement runs two fixture exports differing ONLY in
// GeneratedAt and asserts exactly one file differs: v1/index.json. generated_at
// is the sole run-varying value in the whole tree, confined to the v1 index.
func TestGeneratedAtConfinement(t *testing.T) {
	run := func(generatedAt string) string {
		opts := committedFixtureOpts(t)
		opts.GeneratedAt = generatedAt
		opts.SourceCommit = "0000000000000000000000000000000000000000"
		opts.OutDir = filepath.Join(t.TempDir(), "dist")
		if err := RunExport(opts); err != nil {
			t.Fatalf("RunExport (generated_at %s): %v", generatedAt, err)
		}
		return opts.OutDir
	}

	a := run("2026-01-01T00:00:00Z")
	b := run("2027-06-15T12:30:00Z")

	diff := diffTrees(t, a, b)
	want := []string{"v1/index.json"}
	if len(diff) != 1 || diff[0] != want[0] {
		t.Errorf("exports differing only in generated_at diverge in %v, want exactly %v", diff, want)
	}
}
