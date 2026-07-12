package capyaml_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenScribbler/capmon/capyaml"
)

// fixtureProviderExclusive is a capability YAML whose provider_exclusive section
// carries CapabilityEntry-shaped nodes (supported/mechanism/confidence plus a
// nested capabilities map). Once ProviderExclusive is retyped to
// map[string]CapabilityEntry, these nodes load as ordinary recursive entries.
const fixtureProviderExclusive = `schema_version: "1"
slug: test-provider
display_name: Test Provider
content_types:
  skills:
    supported: true
provider_exclusive:
  frontmatter:
    supported: true
    mechanism: YAML frontmatter parsing
    confidence: confirmed
    capabilities:
      nested_field:
        supported: true
        mechanism: nested mechanism string
        confidence: inferred
  base_dir:
    supported: false
    confidence: unknown
`

func TestProviderExclusiveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test-provider.yaml")
	if err := os.WriteFile(src, []byte(fixtureProviderExclusive), 0644); err != nil {
		t.Fatal(err)
	}

	caps, err := capyaml.LoadCapabilityYAML(src)
	if err != nil {
		t.Fatalf("LoadCapabilityYAML: %v", err)
	}

	// Compile-time proof that ProviderExclusive is the retyped map. Fails to
	// compile against the old map[string]interface{} shape.
	var _ map[string]capyaml.CapabilityEntry = caps.ProviderExclusive

	fm, ok := caps.ProviderExclusive["frontmatter"]
	if !ok {
		t.Fatal("provider_exclusive.frontmatter missing")
	}
	if !fm.Supported {
		t.Error("frontmatter.supported: got false, want true")
	}
	if fm.Mechanism != "YAML frontmatter parsing" {
		t.Errorf("frontmatter.mechanism = %q", fm.Mechanism)
	}
	if fm.Confidence != "confirmed" {
		t.Errorf("frontmatter.confidence = %q, want confirmed", fm.Confidence)
	}

	nested, ok := fm.Capabilities["nested_field"]
	if !ok {
		t.Fatal("provider_exclusive.frontmatter.capabilities.nested_field missing")
	}
	if !nested.Supported {
		t.Error("nested_field.supported: got false, want true")
	}
	if nested.Confidence != "inferred" {
		t.Errorf("nested_field.confidence = %q, want inferred", nested.Confidence)
	}

	base, ok := caps.ProviderExclusive["base_dir"]
	if !ok {
		t.Fatal("provider_exclusive.base_dir missing")
	}
	if base.Supported {
		t.Error("base_dir.supported: got true, want false")
	}

	// Byte-stable round-trip: the writer must reach a fixpoint. Write once,
	// reload the written bytes, write again, and require the two serializations
	// to be byte-identical.
	var first bytes.Buffer
	if err := capyaml.WriteCapabilityYAML(&first, caps); err != nil {
		t.Fatalf("WriteCapabilityYAML (first): %v", err)
	}

	reloadPath := filepath.Join(dir, "reload.yaml")
	if err := os.WriteFile(reloadPath, first.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	caps2, err := capyaml.LoadCapabilityYAML(reloadPath)
	if err != nil {
		t.Fatalf("LoadCapabilityYAML (reload): %v", err)
	}

	var second bytes.Buffer
	if err := capyaml.WriteCapabilityYAML(&second, caps2); err != nil {
		t.Fatalf("WriteCapabilityYAML (second): %v", err)
	}

	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Errorf("round-trip not byte-stable:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}

	// The retyped nodes must survive the round-trip with fields intact.
	if got := caps2.ProviderExclusive["frontmatter"].Capabilities["nested_field"].Mechanism; got != "nested mechanism string" {
		t.Errorf("after round-trip nested_field.mechanism = %q", got)
	}
}
