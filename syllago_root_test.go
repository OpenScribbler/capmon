package capmon

import (
	"os"
	"path/filepath"
	"testing"
)

// syllagoRoot resolves a syllago checkout for tests that compare capmon's
// recognizers, format docs, and seeder specs against the live syllago repo.
//
// Before the extraction these tests lived inside the syllago repo and used
// ../../.. relative paths. Standalone, they need a checkout to compare
// against: set SYLLAGO_ROOT to a syllago working copy (CI checks one out
// next to this repo and sets the variable). Without it these drift guards
// skip — they still run on every CI build via the workflow's syllago
// checkout, so the gates are preserved.
func syllagoRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("SYLLAGO_ROOT")
	if root == "" {
		t.Skip("SYLLAGO_ROOT not set; skipping syllago drift guard (CI sets it via the syllago checkout)")
	}
	if _, err := os.Stat(filepath.Join(root, "docs")); err != nil {
		t.Fatalf("SYLLAGO_ROOT=%q does not look like a syllago checkout: %v", root, err)
	}
	return root
}

// canonicalKeysPath resolves docs/spec/canonical-keys.yaml inside a syllago
// checkout for the canonical-key drift-guard tests.
func canonicalKeysPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(syllagoRoot(t), "docs", "spec", "canonical-keys.yaml")
}
