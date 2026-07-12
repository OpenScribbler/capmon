package capmon

import (
	"errors"
	"strings"
	"testing"

	"github.com/OpenScribbler/capmon/capyaml"
	"github.com/OpenScribbler/capmon/internal/output"
)

// --- map navigation helpers -------------------------------------------------

func mustMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("value is not map[string]any: %T (%v)", v, v)
	}
	return m
}

func mustChild(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q in %v", key, m)
	}
	return mustMap(t, v)
}

func assertSupported(t *testing.T, node map[string]any, want bool) {
	t.Helper()
	v, ok := node["supported"]
	if !ok {
		t.Errorf("node missing \"supported\": %v", node)
		return
	}
	b, ok := v.(bool)
	if !ok {
		t.Errorf("\"supported\" not bool: %T", v)
		return
	}
	if b != want {
		t.Errorf("supported = %v, want %v", b, want)
	}
}

func assertNoKeyMetadata(t *testing.T, node map[string]any) {
	t.Helper()
	if _, ok := node["key"]; ok {
		t.Errorf("vocabulary member unexpectedly carries \"key\": %v", node)
	}
	if _, ok := node["key_path"]; ok {
		t.Errorf("vocabulary member unexpectedly carries \"key_path\": %v", node)
	}
}

func toStrings(t *testing.T, v any) []string {
	t.Helper()
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, len(s))
		for i, e := range s {
			str, ok := e.(string)
			if !ok {
				t.Fatalf("ref element not a string: %T", e)
			}
			out[i] = str
		}
		return out
	default:
		t.Fatalf("value is not a string slice: %T", v)
		return nil
	}
}

func containsStr(hay []string, want string) bool {
	for _, s := range hay {
		if s == want {
			return true
		}
	}
	return false
}

func assertExport001(t *testing.T, err error, wantParts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var se output.StructuredError
	if !errors.As(err, &se) {
		t.Fatalf("error is not output.StructuredError: %T (%v)", err, err)
	}
	if se.Code != "EXPORT_001" {
		t.Errorf("error code = %q, want EXPORT_001", se.Code)
	}
	hay := err.Error() + "|" + se.Message + "|" + se.Details + "|" + se.Suggestion
	for _, p := range wantParts {
		if !strings.Contains(hay, p) {
			t.Errorf("error %q does not mention %q", hay, p)
		}
	}
}

// --- tests ------------------------------------------------------------------

func TestBuildProviderDocKeyMetadata(t *testing.T) {
	reg := keyRegistry{
		"agents": {
			"invocation_patterns": keyMeta{
				Description: "How subagents are invoked.",
				Type:        "object",
				SpecRef:     "ACIF-AGENT §9.1 (DERIVABLE)",
			},
			"model_selection": keyMeta{
				Description: "Per-agent model override.",
				Type:        "bool",
				SpecRef:     "ACIF-AGENT §9.2 (DERIVABLE)",
			},
		},
	}

	caps := &capyaml.ProviderCapabilities{
		SchemaVersion: "1",
		Slug:          "demo",
		DisplayName:   "Demo",
		LastVerified:  "2026-07-10",
		ContentTypes: map[string]capyaml.ContentTypeEntry{
			"agents": {
				Supported: true,
				Capabilities: map[string]capyaml.CapabilityEntry{
					"invocation_patterns": {
						Supported: true,
						Capabilities: map[string]capyaml.CapabilityEntry{
							"at_mention": {Supported: true, Mechanism: "@-mention syntax", Confidence: "inferred"},
						},
					},
					"model_selection": {Supported: false, Mechanism: "per-subagent override", Confidence: "inferred"},
				},
			},
		},
	}

	doc, err := buildProviderDoc(caps, reg)
	if err != nil {
		t.Fatalf("buildProviderDoc: %v", err)
	}

	if doc["schema_version"] != "1" {
		t.Errorf("schema_version = %v, want \"1\"", doc["schema_version"])
	}
	if doc["status"] != "live" {
		t.Errorf("status = %v, want \"live\"", doc["status"])
	}
	if doc["slug"] != "demo" {
		t.Errorf("slug = %v, want \"demo\"", doc["slug"])
	}

	capsM := mustChild(t, mustChild(t, mustChild(t, doc, "content_types"), "agents"), "capabilities")

	// Registry-backed BRANCH node: key_path + key metadata present.
	branch := mustChild(t, capsM, "invocation_patterns")
	assertSupported(t, branch, true)
	if branch["key_path"] != "agents.invocation_patterns" {
		t.Errorf("branch key_path = %v, want agents.invocation_patterns", branch["key_path"])
	}
	branchKey := mustChild(t, branch, "key")
	if branchKey["description"] != "How subagents are invoked." {
		t.Errorf("key.description = %v", branchKey["description"])
	}
	if branchKey["type"] != "object" {
		t.Errorf("key.type = %v, want object", branchKey["type"])
	}
	if branchKey["spec_ref"] != "ACIF-AGENT §9.1 (DERIVABLE)" {
		t.Errorf("key.spec_ref = %v", branchKey["spec_ref"])
	}

	// Its children are vocabulary members: supported present, no key metadata.
	atMention := mustChild(t, mustChild(t, branch, "capabilities"), "at_mention")
	assertSupported(t, atMention, true)
	assertNoKeyMetadata(t, atMention)

	// Registry-backed LEAF node: gets both key_path and key; supported=false
	// is still emitted.
	leaf := mustChild(t, capsM, "model_selection")
	assertSupported(t, leaf, false)
	if leaf["key_path"] != "agents.model_selection" {
		t.Errorf("leaf key_path = %v, want agents.model_selection", leaf["key_path"])
	}
	leafKey := mustChild(t, leaf, "key")
	if leafKey["type"] != "bool" {
		t.Errorf("leaf key.type = %v, want bool", leafKey["type"])
	}
}

func TestBuildProviderDocNonCanonicalNodeFailsClosed(t *testing.T) {
	t.Run("unregistered direct child fails closed", func(t *testing.T) {
		reg := keyRegistry{
			"skills": {
				"description": keyMeta{Description: "d", Type: "string", SpecRef: "ACIF-SKILL §1"},
			},
		}
		caps := &capyaml.ProviderCapabilities{
			SchemaVersion: "1",
			Slug:          "badprov",
			ContentTypes: map[string]capyaml.ContentTypeEntry{
				"skills": {
					Supported: true,
					Capabilities: map[string]capyaml.CapabilityEntry{
						"frontmatter": {Supported: true},
					},
				},
			},
		}
		_, err := buildProviderDoc(caps, reg)
		assertExport001(t, err, "badprov", "frontmatter")
	})

	t.Run("descendants of a registry-backed key are never name-checked", func(t *testing.T) {
		reg := keyRegistry{
			"skills": {
				"compatibility": keyMeta{Description: "compat", Type: "object", SpecRef: "ACIF-SKILL §2"},
			},
		}
		caps := &capyaml.ProviderCapabilities{
			SchemaVersion: "1",
			Slug:          "goodprov",
			ContentTypes: map[string]capyaml.ContentTypeEntry{
				"skills": {
					Supported: true,
					Capabilities: map[string]capyaml.CapabilityEntry{
						"compatibility": {
							Supported: true,
							Capabilities: map[string]capyaml.CapabilityEntry{
								// Arbitrary non-registry name — must NOT error.
								"totally_made_up_child": {Supported: true},
							},
						},
					},
				},
			},
		}
		doc, err := buildProviderDoc(caps, reg)
		if err != nil {
			t.Fatalf("buildProviderDoc unexpectedly failed on a vocabulary member: %v", err)
		}
		capsM := mustChild(t, mustChild(t, mustChild(t, doc, "content_types"), "skills"), "capabilities")
		compat := mustChild(t, capsM, "compatibility")
		child := mustChild(t, mustChild(t, compat, "capabilities"), "totally_made_up_child")
		assertSupported(t, child, true)
		assertNoKeyMetadata(t, child)
	})
}

func TestBuildProviderDocFallbacksAndTrimming(t *testing.T) {
	t.Run("display_name fallback, last_verified and empty maps omitted", func(t *testing.T) {
		caps := &capyaml.ProviderCapabilities{
			SchemaVersion: "1",
			Slug:          "slugonly",
			DisplayName:   "",
			LastVerified:  "",
		}
		doc, err := buildProviderDoc(caps, keyRegistry{})
		if err != nil {
			t.Fatalf("buildProviderDoc: %v", err)
		}
		if doc["display_name"] != "slugonly" {
			t.Errorf("display_name = %v, want slug fallback \"slugonly\"", doc["display_name"])
		}
		if _, ok := doc["last_verified"]; ok {
			t.Error("empty last_verified should be omitted from the doc")
		}
		if _, ok := doc["content_types"]; ok {
			t.Error("empty content_types should be omitted from the doc")
		}
		if _, ok := doc["references"]; ok {
			t.Error("absent references should be omitted from the doc")
		}
		if _, ok := doc["provider_exclusive"]; ok {
			t.Error("absent provider_exclusive should be omitted from the doc")
		}
	})

	t.Run("references trimmed to url and verified_at", func(t *testing.T) {
		caps := &capyaml.ProviderCapabilities{
			SchemaVersion: "1",
			Slug:          "refprov",
			References: map[string]capyaml.ReferenceEntry{
				"doc1": {
					URL:             "https://example.com/a",
					FetchMethod:     "http",
					VerifiedAt:      "2026-01-01",
					LastContentHash: "deadbeef",
				},
			},
		}
		doc, err := buildProviderDoc(caps, keyRegistry{})
		if err != nil {
			t.Fatalf("buildProviderDoc: %v", err)
		}
		d1 := mustChild(t, mustChild(t, doc, "references"), "doc1")
		if d1["url"] != "https://example.com/a" {
			t.Errorf("references.doc1.url = %v", d1["url"])
		}
		if d1["verified_at"] != "2026-01-01" {
			t.Errorf("references.doc1.verified_at = %v", d1["verified_at"])
		}
		if _, ok := d1["fetch_method"]; ok {
			t.Error("references entry should not carry fetch_method")
		}
		if _, ok := d1["last_content_hash"]; ok {
			t.Error("references entry should not carry last_content_hash")
		}
		if len(d1) != 2 {
			t.Errorf("references entry has %d fields, want exactly 2 (url, verified_at)", len(d1))
		}
	})

	t.Run("events and tools mirrored, empty maps omitted", func(t *testing.T) {
		caps := &capyaml.ProviderCapabilities{
			SchemaVersion: "1",
			Slug:          "etprov",
			ContentTypes: map[string]capyaml.ContentTypeEntry{
				"hooks": {
					Supported: true,
					Events: map[string]capyaml.EventEntry{
						"pre_tool": {NativeName: "PreToolUse", Blocking: "deny", Refs: []string{"ref_a"}},
					},
				},
				"agents": {
					Supported: true,
					Tools: map[string]capyaml.ToolEntry{
						"search": {Native: "grep_tool", Refs: []string{"ref_b"}},
					},
				},
			},
		}
		doc, err := buildProviderDoc(caps, keyRegistry{})
		if err != nil {
			t.Fatalf("buildProviderDoc: %v", err)
		}

		hooks := mustChild(t, mustChild(t, doc, "content_types"), "hooks")
		pre := mustChild(t, mustChild(t, hooks, "events"), "pre_tool")
		if pre["native_name"] != "PreToolUse" {
			t.Errorf("event native_name = %v, want PreToolUse", pre["native_name"])
		}
		if pre["blocking"] != "deny" {
			t.Errorf("event blocking = %v, want deny", pre["blocking"])
		}
		if refs := toStrings(t, pre["refs"]); !containsStr(refs, "ref_a") {
			t.Errorf("event refs = %v, want to contain ref_a", refs)
		}
		if _, ok := hooks["capabilities"]; ok {
			t.Error("hooks node should omit empty capabilities map")
		}
		if _, ok := hooks["tools"]; ok {
			t.Error("hooks node should omit empty tools map")
		}

		agents := mustChild(t, mustChild(t, doc, "content_types"), "agents")
		search := mustChild(t, mustChild(t, agents, "tools"), "search")
		if search["native"] != "grep_tool" {
			t.Errorf("tool native = %v, want grep_tool", search["native"])
		}
		if refs := toStrings(t, search["refs"]); !containsStr(refs, "ref_b") {
			t.Errorf("tool refs = %v, want to contain ref_b", refs)
		}
		if _, ok := agents["events"]; ok {
			t.Error("agents node should omit empty events map")
		}
	})

	t.Run("provider_exclusive nodes are capability-shaped without key metadata", func(t *testing.T) {
		caps := &capyaml.ProviderCapabilities{
			SchemaVersion: "1",
			Slug:          "peprov",
			ProviderExclusive: map[string]capyaml.CapabilityEntry{
				"skills.frontmatter": {
					Supported:  true,
					Mechanism:  "frontmatter parsing",
					Confidence: "confirmed",
					Capabilities: map[string]capyaml.CapabilityEntry{
						"nested": {Supported: false},
					},
				},
			},
		}
		doc, err := buildProviderDoc(caps, keyRegistry{})
		if err != nil {
			t.Fatalf("buildProviderDoc: %v", err)
		}
		pe := mustChild(t, doc, "provider_exclusive")
		node := mustChild(t, pe, "skills.frontmatter")
		assertSupported(t, node, true)
		assertNoKeyMetadata(t, node)
		if node["mechanism"] != "frontmatter parsing" {
			t.Errorf("provider_exclusive node mechanism = %v", node["mechanism"])
		}
		// Nested descendants are equally keyless, and supported=false still emits.
		nested := mustChild(t, mustChild(t, node, "capabilities"), "nested")
		assertSupported(t, nested, false)
		assertNoKeyMetadata(t, nested)
	})
}
