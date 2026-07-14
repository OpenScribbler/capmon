package capmon

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corruptBaselineNonCanonical is a valid provider baseline (Slug != "") whose
// skills content type carries a direct capabilities child not registered in the
// fixture registry, so buildProviderDoc fails closed with EXPORT_001 during
// staging — before anything could reach OutDir.
const corruptBaselineNonCanonical = `schema_version: "1"
slug: alpha
display_name: Alpha
content_types:
  skills:
    supported: true
    capabilities:
      bogus_unregistered_key:
        supported: true
`

// snapshotTree reads every file under root into a rel→bytes map for exact
// before/after comparison.
func snapshotTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, rel := range walkRelFiles(t, root) {
		out[rel] = readFileBytes(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return out
}

// sentinelOutDir creates an OutDir pre-populated with a sentinel "good site"
// and returns its path plus a before-snapshot.
func sentinelOutDir(t *testing.T) (string, map[string][]byte) {
	t.Helper()
	outDir := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(filepath.Join(outDir, "v1"), 0755); err != nil {
		t.Fatalf("mkdir sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.json"), []byte("SENTINEL-ROOT\n"), 0644); err != nil {
		t.Fatalf("write sentinel root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "v1", "index.json"), []byte("SENTINEL-V1\n"), 0644); err != nil {
		t.Fatalf("write sentinel v1: %v", err)
	}
	return outDir, snapshotTree(t, outDir)
}

// assertSentinelUntouched compares OutDir against its before-snapshot: a failed
// export must leave every sentinel byte exactly as it was.
func assertSentinelUntouched(t *testing.T, outDir string, before map[string][]byte) {
	t.Helper()
	after := snapshotTree(t, outDir)
	if len(after) != len(before) {
		t.Fatalf("OutDir file count changed after failed export: before %d, after %d", len(before), len(after))
	}
	for rel, want := range before {
		got, ok := after[rel]
		if !ok {
			t.Errorf("sentinel file %s removed by a failed export", rel)
			continue
		}
		if !bytes.Equal(want, got) {
			t.Errorf("sentinel file %s mutated by a failed export", rel)
		}
	}
}

// TestRunExportFailClosedPreservesOut proves the atomic-replace contract from
// both ends. The early case fails before the first document is built
// (EXPORT_001). The late case is the discriminating one: the whole tree stages
// and schema-validates successfully, and only then the provider-set assert
// fails (EXPORT_003) — a non-atomic implementation writing directly into
// OutDir would already have replaced the sentinel by that point.
func TestRunExportFailClosedPreservesOut(t *testing.T) {
	base := committedFixtureOpts(t)

	t.Run("early failure before staging writes", func(t *testing.T) {
		corruptCaps := t.TempDir()
		if err := os.WriteFile(filepath.Join(corruptCaps, "alpha.yaml"), []byte(corruptBaselineNonCanonical), 0644); err != nil {
			t.Fatalf("write corrupt baseline: %v", err)
		}
		outDir, before := sentinelOutDir(t)

		opts := ExportOptions{
			CapsDir:           corruptCaps,
			CanonicalKeysPath: base.CanonicalKeysPath,
			SourcesDir:        base.SourcesDir,
			PublishAssetsDir:  base.PublishAssetsDir,
			OutDir:            outDir,
			GeneratedAt:       "2026-01-01T00:00:00Z",
		}
		err := RunExport(opts)
		requireStructured(t, err, "EXPORT_001")
		assertSentinelUntouched(t, outDir, before)
	})

	t.Run("late failure after full staging", func(t *testing.T) {
		// Valid caps, but a sources dir declaring only alpha: the fixture's
		// synthetic provider makes the exported set diverge, so EXPORT_003
		// fires after every document has been staged and validated.
		partialSources := t.TempDir()
		src := readFileBytes(t, filepath.Join(base.SourcesDir, "alpha.yaml"))
		if err := os.WriteFile(filepath.Join(partialSources, "alpha.yaml"), src, 0644); err != nil {
			t.Fatalf("write partial sources: %v", err)
		}
		outDir, before := sentinelOutDir(t)

		opts := ExportOptions{
			CapsDir:           base.CapsDir,
			CanonicalKeysPath: base.CanonicalKeysPath,
			SourcesDir:        partialSources,
			PublishAssetsDir:  base.PublishAssetsDir,
			OutDir:            outDir,
			GeneratedAt:       "2026-01-01T00:00:00Z",
		}
		err := RunExport(opts)
		requireStructured(t, err, "EXPORT_003")
		assertSentinelUntouched(t, outDir, before)
	})

	t.Run("slug-permanence failure after full staging", func(t *testing.T) {
		// Both source manifests present (EXPORT_003 passes) but the lock omits
		// synthetic: the slug-permanence gate must fire EXPORT_005 after full
		// staging, and the pre-existing OutDir must survive untouched.
		lockedSources := t.TempDir()
		for _, name := range []string{"alpha.yaml", "synthetic.yaml"} {
			src := readFileBytes(t, filepath.Join(base.SourcesDir, name))
			if err := os.WriteFile(filepath.Join(lockedSources, name), src, 0644); err != nil {
				t.Fatalf("copy source manifest %s: %v", name, err)
			}
		}
		if err := os.WriteFile(filepath.Join(lockedSources, publishedSlugsLockName), []byte("alpha\n"), 0644); err != nil {
			t.Fatalf("write partial published-slugs lock: %v", err)
		}
		outDir, before := sentinelOutDir(t)

		opts := ExportOptions{
			CapsDir:           base.CapsDir,
			CanonicalKeysPath: base.CanonicalKeysPath,
			SourcesDir:        lockedSources,
			PublishAssetsDir:  base.PublishAssetsDir,
			OutDir:            outDir,
			GeneratedAt:       "2026-01-01T00:00:00Z",
		}
		err := RunExport(opts)
		se := requireStructured(t, err, "EXPORT_005")
		if !strings.Contains(se.Error(), "synthetic") {
			t.Errorf("EXPORT_005 does not name the unlocked exported slug: %v", se)
		}
		assertSentinelUntouched(t, outDir, before)
	})
}

// TestRunExportTrailingSeparatorOutDir pins the OutDir normalization: with a
// trailing separator, filepath.Dir would otherwise resolve the parent to
// OutDir itself, staging the temp dir inside the live tree and making the
// final rename target its own child.
func TestRunExportTrailingSeparatorOutDir(t *testing.T) {
	outDir, _ := sentinelOutDir(t)

	opts := committedFixtureOpts(t)
	opts.OutDir = outDir + string(os.PathSeparator)
	opts.GeneratedAt = "2026-01-01T00:00:00Z"
	if err := RunExport(opts); err != nil {
		t.Fatalf("RunExport with trailing-separator OutDir: %v", err)
	}

	got := readFileBytes(t, filepath.Join(outDir, "v1", "index.json"))
	if bytes.Contains(got, []byte("SENTINEL")) {
		t.Error("OutDir still holds the sentinel site after a successful export")
	}
}

// TestReplaceOutDirRestoresOnRenameFailure covers the one window where the
// atomic-replace contract could break: the old tree has been renamed aside and
// the stage→OutDir rename then fails. The old tree must come back.
func TestReplaceOutDirRestoresOnRenameFailure(t *testing.T) {
	outDir, before := sentinelOutDir(t)
	parent := filepath.Dir(outDir)

	err := replaceOutDir(filepath.Join(parent, "no-such-stage"), outDir, parent)
	if err == nil {
		t.Fatal("replaceOutDir with a missing staging dir: want error, got nil")
	}
	assertSentinelUntouched(t, outDir, before)
}

// escapingSlugBaseline is a loadable baseline whose slug is a path-traversal
// value; staging must fail closed before any byte is written outside the
// staging tree.
const escapingSlugBaseline = `schema_version: "1"
slug: ../../../../escape
display_name: Escape
`

// TestRunExportEscapingSlugFailsClosed proves a traversal slug can never write
// outside the staging tree or touch a pre-existing OutDir.
func TestRunExportEscapingSlugFailsClosed(t *testing.T) {
	base := committedFixtureOpts(t)

	corruptCaps := t.TempDir()
	if err := os.WriteFile(filepath.Join(corruptCaps, "escape.yaml"), []byte(escapingSlugBaseline), 0644); err != nil {
		t.Fatalf("write escaping baseline: %v", err)
	}
	outDir, before := sentinelOutDir(t)

	opts := ExportOptions{
		CapsDir:           corruptCaps,
		CanonicalKeysPath: base.CanonicalKeysPath,
		SourcesDir:        base.SourcesDir,
		PublishAssetsDir:  base.PublishAssetsDir,
		OutDir:            outDir,
		GeneratedAt:       "2026-01-01T00:00:00Z",
	}
	err := RunExport(opts)
	if err == nil || !strings.Contains(err.Error(), "escapes the export tree") {
		t.Fatalf("RunExport with escaping slug: want staging-escape error, got %v", err)
	}
	assertSentinelUntouched(t, outDir, before)
}
