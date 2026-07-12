package capmon

import (
	"os"
	"path/filepath"
	"testing"
)

// docsRoot resolves this repo's root for drift-guard tests that compare
// recognizers, format docs, and seeder specs against the capability data
// under docs/. The data was imported into this repo (it previously lived in
// a downstream consumer), so the guards are now self-consistency checks and
// always run. Tests execute with the package directory — the repo root — as
// the working directory.
func docsRoot(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(".", "docs")); err != nil {
		t.Fatalf("docs/ not found in working directory (expected repo root): %v", err)
	}
	return "."
}

// canonicalKeysPath resolves docs/spec/canonical-keys.yaml for the
// canonical-key drift-guard tests.
func canonicalKeysPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(docsRoot(t), "docs", "spec", "canonical-keys.yaml")
}
