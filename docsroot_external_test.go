package capmon_test

import (
	"os"
	"path/filepath"
	"testing"
)

// docsRoot resolves this repo's root for external-package drift-guard tests
// that compare recognizers, seeder specs, and source manifests against the
// capability data under docs/. Mirror of the internal-package helper in
// docsroot_test.go — Go test packages cannot share unexported helpers.
func docsRoot(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(".", "docs")); err != nil {
		t.Fatalf("docs/ not found in working directory (expected repo root): %v", err)
	}
	return "."
}
