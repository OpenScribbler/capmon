package capmon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenScribbler/capmon/capyaml"
)

// TestAllBaselinesBuildProviderDocs is a permanent drift guard: every committed
// capability baseline (a top-level docs/provider-capabilities/*.yaml with a
// non-empty slug) must build a provider document against the real canonical-key
// registry. A non-canonical node in canonical-key position fails buildProviderDoc
// with EXPORT_001, which this test surfaces per offending file.
func TestAllBaselinesBuildProviderDocs(t *testing.T) {
	reg, err := loadKeyRegistry(canonicalKeysPath(t))
	if err != nil {
		t.Fatalf("load key registry: %v", err)
	}

	capsDir := filepath.Join(docsRoot(t), "docs", "provider-capabilities")
	entries, err := os.ReadDir(capsDir)
	if err != nil {
		t.Fatalf("read capabilities dir: %v", err)
	}

	realBaselines := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(capsDir, e.Name())
		caps, err := capyaml.LoadCapabilityYAML(path)
		if err != nil {
			t.Errorf("load %s: %v", e.Name(), err)
			continue
		}
		// Per-content-type seed YAMLs carry no top-level slug; only real
		// provider baselines are exported.
		if caps.Slug == "" {
			continue
		}
		realBaselines++
		if _, err := buildProviderDoc(caps, reg); err != nil {
			t.Errorf("buildProviderDoc(%s): %v", e.Name(), err)
		}
	}

	if realBaselines == 0 {
		t.Fatalf("no real baselines found under %s; walk or slug filter is broken", capsDir)
	}
}
