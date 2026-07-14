package capmon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenScribbler/capmon/internal/output"
)

// --- gate test helpers ------------------------------------------------------

// publishAssetsDir returns the committed publish-asset root (docs/publish),
// the source of the six draft-2020-12 schemas and field-semantics.md that
// writeExportTree copies verbatim into the staged tree. Staging with this as
// PublishAssetsDir is the only configuration validateExportTree can gate: the
// schemas it compiles live under the staged v1/schemas/ dir.
func publishAssetsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(docsRoot(t), "docs", "publish")
}

// stageGatedTree writes the three-baseline fixture plus the committed publish
// assets into a fresh temp tree and returns its root. The resulting tree
// carries v1/schemas/*.json and v1/spec/field-semantics.md, so it is a
// complete, schema-gateable export.
func stageGatedTree(t *testing.T) string {
	t.Helper()
	opts := newExportFixture(t)
	opts.PublishAssetsDir = publishAssetsDir(t)
	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree: %v", err)
	}
	return dst
}

// writeJSONFile re-serializes a mutated document map and overwrites path. The
// bytes need not be canonical — validateExportTree reads whatever is on disk —
// so a plain indented marshal with a trailing newline is sufficient.
func writeJSONFile(t *testing.T, path string, m map[string]any) {
	t.Helper()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// requireStructured asserts err is an output.StructuredError with the given
// code and returns it for further message inspection.
func requireStructured(t *testing.T, err error, code string) output.StructuredError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error, got nil", code)
	}
	var se output.StructuredError
	if !errors.As(err, &se) {
		t.Fatalf("error is not output.StructuredError: %T (%v)", err, err)
	}
	if se.Code != code {
		t.Fatalf("error code = %q, want %q (%v)", se.Code, code, err)
	}
	return se
}

// writeSourceManifests writes a minimal source manifest per slug under a fresh
// temp dir and returns it. Each file carries the slug and status fields — the
// shape sourceManifestStatuses reads to derive the authoritative provider set.
func writeSourceManifests(t *testing.T, slugs ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, slug := range slugs {
		body := "schema_version: \"1\"\nslug: " + slug + "\nstatus: active\n"
		if err := os.WriteFile(filepath.Join(dir, slug+".yaml"), []byte(body), 0644); err != nil {
			t.Fatalf("write source manifest %s: %v", slug, err)
		}
	}
	return dir
}

// --- tests ------------------------------------------------------------------

// TestValidateExportTreePassesFixture stages a valid fixture tree (docs + the
// committed publish assets) and asserts the fail-closed schema gate accepts it:
// all six schemas compile as draft 2020-12 from the staged v1/schemas/ dir and
// every gated document validates against its routed schema.
func TestValidateExportTreePassesFixture(t *testing.T) {
	dst := stageGatedTree(t)
	if err := validateExportTree(dst); err != nil {
		t.Fatalf("validateExportTree on a valid fixture tree: %v", err)
	}
}

// TestValidateExportTreeFailsClosed corrupts one staged per-provider document —
// a content-type node's boolean supported rewritten as a string — and asserts
// the gate rejects the tree with EXPORT_002 naming the offending file. Only the
// on-disk capabilities/alpha.json is corrupted (all.json was written from the
// pre-corruption in-memory maps), so exactly one file violates its schema.
func TestValidateExportTreeFailsClosed(t *testing.T) {
	dst := stageGatedTree(t)

	docPath := filepath.Join(dst, "v1", "capabilities", "alpha.json")
	doc := readJSONMap(t, docPath)
	agents := mustChild(t, mustChild(t, doc, "content_types"), "agents")
	agents["supported"] = "yes" // wrong type: string where the schema requires bool
	writeJSONFile(t, docPath, doc)

	err := validateExportTree(dst)
	se := requireStructured(t, err, "EXPORT_002")
	if !strings.Contains(se.Error(), "alpha.json") {
		t.Errorf("EXPORT_002 error does not name the offending file: %v", se)
	}
}

// TestAssertProviderSetMismatch drives the provider-set gate over temp source
// manifests: an exported set matching the manifests passes; a missing slug and
// an extra slug each fail closed with EXPORT_003 naming the divergent slug.
func TestAssertProviderSetMismatch(t *testing.T) {
	sourcesDir := writeSourceManifests(t, "alpha", "bravo", "charlie")

	// Matching set (order-independent) passes.
	if err := assertProviderSet([]string{"charlie", "alpha", "bravo"}, sourcesDir); err != nil {
		t.Fatalf("assertProviderSet on a matching set: %v", err)
	}

	// Missing slug: exported set omits charlie. The error carries both the
	// divergent slug and the count detail (2 exported vs 3 source manifests).
	err := assertProviderSet([]string{"alpha", "bravo"}, sourcesDir)
	se := requireStructured(t, err, "EXPORT_003")
	if !strings.Contains(se.Error(), "charlie") {
		t.Errorf("EXPORT_003 (missing) does not name the divergent slug: %v", se)
	}
	if !strings.Contains(se.Error(), "2") || !strings.Contains(se.Error(), "3") {
		t.Errorf("EXPORT_003 (missing) does not carry the count detail (want 2 vs 3): %v", se)
	}

	// Extra slug: exported set carries delta, which no manifest declares.
	err = assertProviderSet([]string{"alpha", "bravo", "charlie", "delta"}, sourcesDir)
	se = requireStructured(t, err, "EXPORT_003")
	if !strings.Contains(se.Error(), "delta") {
		t.Errorf("EXPORT_003 (extra) does not name the divergent slug: %v", se)
	}
	if !strings.Contains(se.Error(), "4") || !strings.Contains(se.Error(), "3") {
		t.Errorf("EXPORT_003 (extra) does not carry the count detail (want 4 vs 3): %v", se)
	}
}

// TestFreezeFieldsOptionalFromLaunch stages a valid tree, then adds the freeze
// fields (status "frozen", superseded_by, frozen_at) to a per-provider document
// and to v1/index.json, and re-validates. Both schemas must still accept the
// tree — the freeze fields are pre-provisioned OPTIONAL from initial
// publication, the one part of the freeze contract that cannot be retrofitted.
func TestFreezeFieldsOptionalFromLaunch(t *testing.T) {
	dst := stageGatedTree(t)

	docPath := filepath.Join(dst, "v1", "capabilities", "alpha.json")
	doc := readJSONMap(t, docPath)
	doc["status"] = "frozen"
	doc["superseded_by"] = "/v2/"
	doc["frozen_at"] = "2027-01-01T00:00:00Z"
	writeJSONFile(t, docPath, doc)

	idxPath := filepath.Join(dst, "v1", "index.json")
	idx := readJSONMap(t, idxPath)
	idx["status"] = "frozen"
	idx["superseded_by"] = "/v2/"
	idx["frozen_at"] = "2027-01-01T00:00:00Z"
	writeJSONFile(t, idxPath, idx)

	if err := validateExportTree(dst); err != nil {
		t.Fatalf("validateExportTree with freeze fields present: %v", err)
	}
}

// TestSourceManifestDuplicateSlugFailsClosed: two manifests declaring one slug
// would collapse into a single set member and slip past the count check, so
// the gate must reject the sources dir itself.
func TestSourceManifestDuplicateSlugFailsClosed(t *testing.T) {
	dir := writeSourceManifests(t, "alpha")
	dup := "schema_version: \"1\"\nslug: alpha\nstatus: active\n"
	if err := os.WriteFile(filepath.Join(dir, "alpha-copy.yaml"), []byte(dup), 0644); err != nil {
		t.Fatalf("write duplicate manifest: %v", err)
	}

	err := assertProviderSet([]string{"alpha"}, dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate source manifest slug") {
		t.Fatalf("assertProviderSet with duplicate manifest slugs: want duplicate error, got %v", err)
	}
}

// TestSourceManifestMissingStatusFailsClosed: the manifest schema requires
// status; a manifest that omits it must fail the export rather than silently
// publishing a provider doc without provider_status.
func TestSourceManifestMissingStatusFailsClosed(t *testing.T) {
	dir := t.TempDir()
	body := "schema_version: \"1\"\nslug: alpha\n"
	if err := os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte(body), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := sourceManifestStatuses(dir)
	if err == nil || !strings.Contains(err.Error(), "missing required field 'status'") {
		t.Fatalf("sourceManifestStatuses with status-less manifest: want missing-status error, got %v", err)
	}
}
