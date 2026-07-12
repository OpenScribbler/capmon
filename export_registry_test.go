package capmon

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureRegistry mirrors the docs/spec/canonical-keys.yaml shape:
// content_types.<content_type>.<key>.{description,type,spec_ref}.
const fixtureRegistry = `content_types:
  hooks:
    handler_types:
      description: Types of hook handlers a provider recognizes.
      type: string
      spec_ref: ACIF-HOOK §1.1 (DERIVABLE)
    async_execution:
      description: Whether hooks may run asynchronously.
      type: bool
      spec_ref: ACIF-HOOK §1.2 (DERIVABLE)
  mcp:
    transport_types:
      description: Supported MCP transport mechanisms.
      type: object
      spec_ref: ACIF-MCP §3.1 (DERIVABLE)
`

func writeFixtureRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "canonical-keys.yaml")
	if err := os.WriteFile(path, []byte(fixtureRegistry), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadKeyRegistryPreservesMetadata(t *testing.T) {
	reg, err := loadKeyRegistry(writeFixtureRegistry(t))
	if err != nil {
		t.Fatalf("loadKeyRegistry: %v", err)
	}

	if len(reg) != 2 {
		t.Errorf("content type count = %d, want 2", len(reg))
	}

	tests := []struct {
		ct, key      string
		wantType     string
		wantDescHead string
		wantSpecRef  string
	}{
		{"hooks", "handler_types", "string", "Types of hook handlers", "ACIF-HOOK §1.1 (DERIVABLE)"},
		{"hooks", "async_execution", "bool", "Whether hooks may run", "ACIF-HOOK §1.2 (DERIVABLE)"},
		{"mcp", "transport_types", "object", "Supported MCP transport", "ACIF-MCP §3.1 (DERIVABLE)"},
	}
	for _, tt := range tests {
		t.Run(tt.ct+"."+tt.key, func(t *testing.T) {
			meta, ok := reg[tt.ct][tt.key]
			if !ok {
				t.Fatalf("registry missing %s.%s", tt.ct, tt.key)
			}
			if meta.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", meta.Type, tt.wantType)
			}
			if meta.SpecRef != tt.wantSpecRef {
				t.Errorf("SpecRef = %q, want %q", meta.SpecRef, tt.wantSpecRef)
			}
			if len(meta.Description) < len(tt.wantDescHead) || meta.Description[:len(tt.wantDescHead)] != tt.wantDescHead {
				t.Errorf("Description = %q, want prefix %q", meta.Description, tt.wantDescHead)
			}
		})
	}

	// Absence: an unknown content type and an unknown key both report not-present
	// via the comma-ok form rather than surfacing a populated zero value.
	if _, ok := reg["does_not_exist"]; ok {
		t.Error("unknown content type reported present")
	}
	if _, ok := reg["hooks"]["does_not_exist"]; ok {
		t.Error("unknown key reported present")
	}
}

func TestLoadKeyRegistryRealFile(t *testing.T) {
	reg, err := loadKeyRegistry(canonicalKeysPath(t))
	if err != nil {
		t.Fatalf("loadKeyRegistry: %v", err)
	}

	if len(reg) != 6 {
		t.Errorf("content type count = %d, want 6", len(reg))
	}

	validType := map[string]bool{"string": true, "bool": true, "object": true}
	total := 0
	for ct, keys := range reg {
		for key, meta := range keys {
			total++
			if meta.Description == "" {
				t.Errorf("%s.%s: empty Description", ct, key)
			}
			if !validType[meta.Type] {
				t.Errorf("%s.%s: Type = %q, want one of string|bool|object", ct, key, meta.Type)
			}
			if meta.SpecRef == "" {
				t.Errorf("%s.%s: empty SpecRef", ct, key)
			}
		}
	}
	if total != 46 {
		t.Errorf("total key count = %d, want 46", total)
	}
}
