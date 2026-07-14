package capmon_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenScribbler/capmon"
	"github.com/OpenScribbler/capmon/capyaml"
)

func TestSeedProviderCapabilities_Idempotent(t *testing.T) {
	capsDir := t.TempDir()
	seedOpts := capmon.SeedOptions{
		CapsDir:  capsDir,
		Provider: "test-provider",
		Extracted: map[string]string{
			"hooks.events.before_tool_execute.native_name": "PreToolUse",
		},
	}
	// First run
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	data1, _ := os.ReadFile(filepath.Join(capsDir, "test-provider.yaml"))
	// Second run
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	data2, _ := os.ReadFile(filepath.Join(capsDir, "test-provider.yaml"))
	if string(data1) != string(data2) {
		t.Error("seed is not idempotent: output changed on second run")
	}
}

func TestSeedProviderCapabilities_PreservesExclusive(t *testing.T) {
	capsDir := t.TempDir()
	// Write initial file with provider_exclusive section
	initial := `schema_version: "1"
slug: test-provider
provider_exclusive:
  hooks.custom_event:
    supported: true
    mechanism: 'CustomEvent: a custom event'
`
	if err := os.WriteFile(filepath.Join(capsDir, "test-provider.yaml"), []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	seedOpts := capmon.SeedOptions{
		CapsDir:  capsDir,
		Provider: "test-provider",
		Extracted: map[string]string{
			"hooks.events.before_tool_execute.native_name": "PreToolUse",
		},
		ForceOverwriteExclusive: false,
	}
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(capsDir, "test-provider.yaml"))
	if !strings.Contains(string(data), "CustomEvent") {
		t.Error("provider_exclusive entry CustomEvent was removed without --force-overwrite-exclusive")
	}
}

// TestSeedProviderCapabilities_RoutesNonCanonicalToExclusive: extracted keys
// not in the canonical-key registry must land under provider_exclusive as
// "<ct>.<key>" nodes — never leak into content_types capabilities (capmon-t8e).
func TestSeedProviderCapabilities_RoutesNonCanonicalToExclusive(t *testing.T) {
	capsDir := t.TempDir()
	seedOpts := capmon.SeedOptions{
		CapsDir:  capsDir,
		Provider: "test-provider",
		Extracted: map[string]string{
			"skills.capabilities.display_name.supported":       "true",
			"skills.capabilities.creation_workflow.supported":  "true",
			"skills.capabilities.creation_workflow.mechanism":  "documented under 'Creating Skills'",
			"skills.capabilities.creation_workflow.confidence": "inferred",
		},
	}
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	caps, err := capyaml.LoadCapabilityYAML(filepath.Join(capsDir, "test-provider.yaml"))
	if err != nil {
		t.Fatalf("load output: %v", err)
	}

	// Canonical key stays in content_types capabilities.
	if _, ok := caps.ContentTypes["skills"].Capabilities["display_name"]; !ok {
		t.Error("canonical display_name missing from skills capabilities")
	}
	// Non-canonical key routed to provider_exclusive, absent from capabilities.
	if _, leaked := caps.ContentTypes["skills"].Capabilities["creation_workflow"]; leaked {
		t.Error("non-canonical creation_workflow leaked into skills capabilities")
	}
	pe, ok := caps.ProviderExclusive["skills.creation_workflow"]
	if !ok {
		t.Fatalf("provider_exclusive missing skills.creation_workflow; got %v", caps.ProviderExclusive)
	}
	if !pe.Supported || pe.Mechanism == "" || pe.Confidence != "inferred" {
		t.Errorf("skills.creation_workflow fields not applied: %+v", pe)
	}
}

// TestSeedProviderCapabilities_ForceOverwriteIsPerEntry: --force-overwrite-exclusive
// must overwrite ONLY the exclusive entries the extraction collides with —
// never clear the section (capmon-t8e repro: it nil'd the whole map).
func TestSeedProviderCapabilities_ForceOverwriteIsPerEntry(t *testing.T) {
	capsDir := t.TempDir()
	initial := `schema_version: "1"
slug: test-provider
provider_exclusive:
  skills.creation_workflow:
    supported: true
    mechanism: 'curated mechanism text'
    confidence: confirmed
  hooks.custom_event:
    supported: true
    mechanism: 'CustomEvent: a custom event'
`
	writeInitial := func(t *testing.T) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(capsDir, "test-provider.yaml"), []byte(initial), 0644); err != nil {
			t.Fatal(err)
		}
	}
	extracted := map[string]string{
		"skills.capabilities.creation_workflow.mechanism": "freshly extracted text",
	}

	// Without force: the colliding extracted key is skipped, curation wins.
	writeInitial(t)
	if err := capmon.SeedProviderCapabilities(capmon.SeedOptions{
		CapsDir: capsDir, Provider: "test-provider", Extracted: extracted,
	}); err != nil {
		t.Fatalf("seed without force: %v", err)
	}
	caps, err := capyaml.LoadCapabilityYAML(filepath.Join(capsDir, "test-provider.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := caps.ProviderExclusive["skills.creation_workflow"].Mechanism; got != "curated mechanism text" {
		t.Errorf("without force: mechanism = %q, want curated text preserved", got)
	}

	// With force: only the colliding entry is overwritten; others survive.
	writeInitial(t)
	if err := capmon.SeedProviderCapabilities(capmon.SeedOptions{
		CapsDir: capsDir, Provider: "test-provider", Extracted: extracted,
		ForceOverwriteExclusive: true,
	}); err != nil {
		t.Fatalf("seed with force: %v", err)
	}
	caps, err = capyaml.LoadCapabilityYAML(filepath.Join(capsDir, "test-provider.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := caps.ProviderExclusive["skills.creation_workflow"].Mechanism; got != "freshly extracted text" {
		t.Errorf("with force: mechanism = %q, want freshly extracted text", got)
	}
	// Overwrite means overwrite: the node is reset before extracted fields
	// land, so the stale curated confidence must NOT survive as a merge.
	if got := caps.ProviderExclusive["skills.creation_workflow"].Confidence; got != "" {
		t.Errorf("with force: stale confidence %q survived — forced collision merged instead of overwriting", got)
	}
	if _, ok := caps.ProviderExclusive["hooks.custom_event"]; !ok {
		t.Error("with force: untouched hooks.custom_event was removed — section cleared wholesale")
	}
}

// TestSeedProviderCapabilities_BareSeedNeedsNoRegistry: stub creation and
// events-only seeds never consult the canonical-key registry, so they must
// work with an unresolvable CanonicalKeysPath (the registry loads lazily).
func TestSeedProviderCapabilities_BareSeedNeedsNoRegistry(t *testing.T) {
	capsDir := t.TempDir()
	err := capmon.SeedProviderCapabilities(capmon.SeedOptions{
		CapsDir:           capsDir,
		Provider:          "test-provider",
		CanonicalKeysPath: filepath.Join(t.TempDir(), "does-not-exist.yaml"),
		Extracted: map[string]string{
			"hooks.supported": "true",
			"hooks.events.before_tool_execute.native_name": "PreToolUse",
		},
	})
	if err != nil {
		t.Fatalf("bare seed without registry: %v", err)
	}
}

func TestSeedProviderCapabilities_WritesConfidence(t *testing.T) {
	capsDir := t.TempDir()
	seedOpts := capmon.SeedOptions{
		CapsDir:  capsDir,
		Provider: "test-provider",
		Extracted: map[string]string{
			"skills.supported":                            "true",
			"skills.capabilities.display_name.supported":  "true",
			"skills.capabilities.display_name.mechanism":  "yaml frontmatter key: name",
			"skills.capabilities.display_name.confidence": "confirmed",
			"skills.capabilities.description.supported":   "true",
			"skills.capabilities.description.mechanism":   "yaml frontmatter key: description",
			"skills.capabilities.description.confidence":  "inferred",
		},
	}
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(capsDir, "test-provider.yaml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "confidence: confirmed") {
		t.Errorf("missing 'confidence: confirmed' in output:\n%s", out)
	}
	if !strings.Contains(out, "confidence: inferred") {
		t.Errorf("missing 'confidence: inferred' in output:\n%s", out)
	}
}

func TestSeedCrushAndRooCodeE2E(t *testing.T) {
	// This test exercises the full pipeline:
	// cache extraction → recognition → capability YAML writing
	// It uses the real .capmon-cache directory relative to the project root.
	cacheRoot := filepath.Join(docsRoot(t), ".capmon-cache")
	if _, err := os.Stat(cacheRoot); os.IsNotExist(err) {
		t.Skip("no .capmon-cache directory — run capmon fetch first")
	}

	for _, provider := range []string{"crush", "roo-code"} {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			dotPaths, err := capmon.LoadAndRecognizeCache(cacheRoot, provider)
			if err != nil {
				t.Fatalf("LoadAndRecognizeCache(%q): %v", provider, err)
			}
			if len(dotPaths) == 0 {
				t.Fatalf("LoadAndRecognizeCache(%q): returned empty map — real recognizer should produce output", provider)
			}
			// Verify key canonical fields are present
			if dotPaths["skills.supported"] != "true" {
				t.Errorf("%s: skills.supported missing or not 'true'", provider)
			}
			if dotPaths["skills.capabilities.display_name.confidence"] != "confirmed" {
				t.Errorf("%s: display_name.confidence not 'confirmed', got %q", provider, dotPaths["skills.capabilities.display_name.confidence"])
			}
			if dotPaths["skills.capabilities.project_scope.supported"] != "true" {
				t.Errorf("%s: project_scope.supported missing", provider)
			}
			if dotPaths["skills.capabilities.canonical_filename.supported"] != "true" {
				t.Errorf("%s: canonical_filename.supported missing", provider)
			}

			// Write to temp output dir — verify YAML is produced with confidence fields
			capsDir := t.TempDir()
			opts := capmon.SeedOptions{
				CapsDir:   capsDir,
				Provider:  provider,
				Extracted: dotPaths,
			}
			if err := capmon.SeedProviderCapabilities(opts); err != nil {
				t.Fatalf("SeedProviderCapabilities(%q): %v", provider, err)
			}
			data, err := os.ReadFile(filepath.Join(capsDir, provider+".yaml"))
			if err != nil {
				t.Fatalf("read output YAML: %v", err)
			}
			out := string(data)
			if !strings.Contains(out, "confidence: confirmed") {
				t.Errorf("%s: output YAML missing 'confidence: confirmed'\n%s", provider, out)
			}
			if !strings.Contains(out, "canonical_filename") {
				t.Errorf("%s: output YAML missing 'canonical_filename'\n%s", provider, out)
			}
		})
	}
}

// TestSeedProviderCapabilities_NestedSubCapabilities verifies that 5-segment
// dot-paths for object-typed canonical keys (activation_mode,
// cross_provider_recognition) round-trip through write and load with their
// nested structure intact. The parent CapabilityEntry's Supported flag must
// be auto-set to true when any sub-capability is added.
func TestSeedProviderCapabilities_NestedSubCapabilities(t *testing.T) {
	capsDir := t.TempDir()
	seedOpts := capmon.SeedOptions{
		CapsDir:  capsDir,
		Provider: "test-provider",
		Extracted: map[string]string{
			"rules.supported": "true",
			"rules.capabilities.activation_mode.always.supported":  "true",
			"rules.capabilities.activation_mode.always.mechanism":  "rules without conditionals load every request",
			"rules.capabilities.activation_mode.always.confidence": "inferred",
			"rules.capabilities.activation_mode.glob.supported":    "true",
			"rules.capabilities.activation_mode.glob.mechanism":    "glob-based path matching",
			"rules.capabilities.activation_mode.glob.confidence":   "inferred",
		},
	}
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	path := filepath.Join(capsDir, "test-provider.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	out := string(data)
	for _, want := range []string{
		"activation_mode:",
		"always:",
		"glob:",
		"glob-based path matching",
		"confidence: inferred",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nFull output:\n%s", want, out)
		}
	}
	// Round-trip through Load: the nested map must rehydrate
	caps, err := capyaml.LoadCapabilityYAML(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rules, ok := caps.ContentTypes["rules"]
	if !ok {
		t.Fatal("rules content type missing after round-trip")
	}
	am, ok := rules.Capabilities["activation_mode"]
	if !ok {
		t.Fatal("activation_mode capability missing after round-trip")
	}
	if !am.Supported {
		t.Error("activation_mode parent.Supported should be auto-set to true")
	}
	if len(am.Capabilities) != 2 {
		t.Fatalf("expected 2 sub-capabilities, got %d: %v", len(am.Capabilities), am.Capabilities)
	}
	always := am.Capabilities["always"]
	if !always.Supported {
		t.Error("always.supported should be true")
	}
	if always.Confidence != "inferred" {
		t.Errorf("always.confidence = %q, want %q", always.Confidence, "inferred")
	}
	if always.Mechanism == "" {
		t.Error("always.mechanism should be populated")
	}
	globs := am.Capabilities["glob"]
	if !globs.Supported {
		t.Error("glob.supported should be true")
	}
}

func TestSeedProviderCapabilities_AppliesDotPaths(t *testing.T) {
	capsDir := t.TempDir()
	seedOpts := capmon.SeedOptions{
		CapsDir:  capsDir,
		Provider: "test-provider",
		Extracted: map[string]string{
			"skills.supported": "true",
			"skills.capabilities.frontmatter_name.supported": "true",
			"skills.capabilities.frontmatter_name.mechanism": "yaml key: name",
			"hooks.events.before_tool_execute.native_name":   "PreToolUse",
			"hooks.events.before_tool_execute.blocking":      "prevent",
		},
	}
	if err := capmon.SeedProviderCapabilities(seedOpts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(capsDir, "test-provider.yaml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	out := string(data)
	checks := []struct {
		want string
		desc string
	}{
		{"skills:", "skills content type entry"},
		{"supported: true", "skills supported flag"},
		{"frontmatter_name:", "frontmatter_name capability"},
		{"yaml key: name", "frontmatter_name mechanism"},
		{"before_tool_execute:", "hook event entry"},
		{"native_name: PreToolUse", "hook native_name"},
		{"blocking: prevent", "hook blocking"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("output missing %s: %q\nFull output:\n%s", c.desc, c.want, out)
		}
	}
}
