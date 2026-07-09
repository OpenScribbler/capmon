package capmon_test

import (
	"os"
	"path/filepath"
	"testing"
)

// syllagoRoot resolves a syllago checkout for external-package tests that
// compare capmon's recognizers, seeder specs, and source manifests against
// the live syllago repo. Mirror of the internal-package helper in
// syllago_root_test.go — Go test packages cannot share unexported helpers.
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
