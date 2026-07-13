package capmon_test

import (
	"testing"

	"github.com/OpenScribbler/capmon"
)

// realClineLandmarks is a snapshot of the headings extracted from cline's skills
// doc (.capmon-cache/cline/skills.0/extracted.json) as of 2026-04-16.
var realClineLandmarks = []string{
	"Documentation Index",
	"Skills",
	"How Skills Work",
	"Skill Structure",
	"Creating a Skill",
	"Toggling Skills",
	"Writing Your SKILL.md",
	"Naming Conventions",
	"Writing Effective Descriptions",
	"Keeping Skills Focused",
	"Where Skills Live",
	"Bundling Supporting Files",
	"docs/",
	"templates/",
	"scripts/",
	"Referencing Bundled Files",
	"Example: Data Analysis Skill",
}

// realClineNonSkillsLandmarks is a sample drawn from cline's other content-type
// docs (mcp, commands). The required anchors must NOT match any of these
// — proves the false-positive guardrail works under multi-source merge.
// Rules and hooks anchors are excluded here because they are now legitimately
// recognized by their content types; see realClineRulesLandmarks /
// realClineHooksLandmarks for those cases. We do include partial hooks
// landmarks (Hook Types, Hook Lifecycle) to verify the hooks required-anchor
// guard suppresses cleanly when "Hook Locations" is absent.
var realClineNonSkillsLandmarks = []string{
	"Documentation Index",
	"Hooks", "What You Can Build", "Hook Types", "Hook Lifecycle",
	"Adding & Configuring Servers", "Finding MCP Servers", "Managing Servers",
	"Using Commands", "Slash Commands",
}

// realClineHooksLandmarks is a snapshot of the hooks-doc headings from
// .capmon-cache/cline/hooks.0/extracted.json (docs.cline.bot/customization/hooks)
// as of 2026-07-13. The page is now a two-line stub redirecting to the SDK
// plugin docs; only the "Hooks" heading survives. See the attestation on
// clineHooksLandmarkOptions in recognize_cline.go — the hooks fragment now
// suppresses cleanly because no capability heading remains on the tracked source.
var realClineHooksLandmarks = []string{
	"Documentation Index",
	"Hooks",
}

// realClineRulesLandmarks is a snapshot of the rules-doc headings from
// .capmon-cache/cline/rules.0/extracted.json (cline-rules.md) as of 2026-04-16.
var realClineRulesLandmarks = []string{
	"Documentation Index",
	"Rules",
	"Supported Rule Types",
	"Where Rules Live",
	"Global Rules Directory",
	"Creating Rules",
	"Toggling Rules",
	"Writing Effective Rules",
	"Conditional Rules",
	"How It Works",
	"Writing Conditional Rules",
	"The paths Conditional",
	"Behavior Details",
	"Practical Examples",
	"Frontend vs Backend Rules",
	"Test File Rules",
	"Documentation Rules",
	"Combining with Rule Toggles",
	"Tips for Effective Conditional Rules",
	"Troubleshooting Conditional Rules",
}

func TestRecognizeCline_RealLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: realClineLandmarks,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q", result.Status, capmon.StatusRecognized)
	}
	caps := result.Capabilities
	if caps["skills.supported"] != "true" {
		t.Error("skills.supported missing")
	}
	inferred := []string{
		"directory_structure",
		"creation_workflow",
		"toggling",
		"frontmatter",
		"naming_conventions",
		"description_guidance",
		"bundled_files",
		"file_references",
	}
	for _, c := range inferred {
		if caps["skills.capabilities."+c+".supported"] != "true" {
			t.Errorf("%s.supported missing", c)
		}
		if got := caps["skills.capabilities."+c+".confidence"]; got != "inferred" {
			t.Errorf("%s.confidence = %q, want inferred", c, got)
		}
	}
	for _, c := range []string{"project_scope", "global_scope", "canonical_filename"} {
		if caps["skills.capabilities."+c+".confidence"] != "confirmed" {
			t.Errorf("%s.confidence = %q, want confirmed", c, caps["skills.capabilities."+c+".confidence"])
		}
	}
}

// TestRecognizeCline_NonSkillsLandmarks proves the false-positive guardrail:
// when cline's other content-type doc landmarks are present (rules, hooks, mcp,
// commands) but the skills-specific anchors are NOT, the recognizer suppresses.
// This is the realistic multi-source case — every cline run merges all sources.
func TestRecognizeCline_NonSkillsLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: realClineNonSkillsLandmarks,
	})
	if result.Status != capmon.StatusAnchorsMissing {
		t.Errorf("status = %q, want %q", result.Status, capmon.StatusAnchorsMissing)
	}
	if len(result.Capabilities) != 0 {
		t.Errorf("expected zero capabilities from non-skills landmarks, got %d: %v",
			len(result.Capabilities), result.Capabilities)
	}
}

func TestRecognizeCline_AnchorsMissing(t *testing.T) {
	mutated := []string{}
	for _, lm := range realClineLandmarks {
		if lm == "Where Skills Live" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if result.Status != capmon.StatusAnchorsMissing {
		t.Errorf("status = %q, want %q", result.Status, capmon.StatusAnchorsMissing)
	}
	if len(result.Capabilities) != 0 {
		t.Errorf("expected zero capabilities, got %d", len(result.Capabilities))
	}
}

func TestRecognizeCline_NoLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{Provider: "cline", Format: "markdown"})
	if result.Status != capmon.StatusAnchorsMissing {
		t.Errorf("status = %q, want %q", result.Status, capmon.StatusAnchorsMissing)
	}
	if len(result.Capabilities) != 0 {
		t.Errorf("expected zero capabilities, got %d", len(result.Capabilities))
	}
}

// TestRecognizeCline_RealRulesLandmarks proves rules recognition on the merged
// skills+rules landmarks. Per the seeder spec, cline supports a smaller
// activation_mode vocabulary (only always_on + frontmatter_globs) than
// cursor/kiro/windsurf. file_imports, cross_provider_recognition, and
// auto_memory are intentionally absent.
func TestRecognizeCline_RealRulesLandmarks(t *testing.T) {
	merged := append([]string{}, realClineLandmarks...)
	merged = append(merged, realClineRulesLandmarks...)
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["rules.supported"] != "true" {
		t.Error("rules.supported missing")
	}
	rulesInferred := []string{
		"activation_mode.always",
		"activation_mode.glob",
		"hierarchical_loading",
	}
	for _, c := range rulesInferred {
		key := "rules.capabilities." + c + ".supported"
		if caps[key] != "true" {
			t.Errorf("%s missing", key)
		}
		if got := caps["rules.capabilities."+c+".confidence"]; got != "inferred" {
			t.Errorf("rules.%s.confidence = %q, want inferred", c, got)
		}
	}
	for _, absent := range []string{
		"rules.capabilities.file_imports.supported",
		"rules.capabilities.cross_provider_recognition.agents_md.supported",
		"rules.capabilities.auto_memory.supported",
		"rules.capabilities.activation_mode.manual.supported",
		"rules.capabilities.activation_mode.model_decision.supported",
	} {
		if _, has := caps[absent]; has {
			t.Errorf("%s should NOT be present for cline", absent)
		}
	}
}

// realClineMcpLandmarks is a snapshot of the MCP-doc headings from the
// consolidated mcp-overview page (.capmon-cache/cline/mcp.0 and mcp.1 both now
// resolve to docs.cline.bot/mcp/mcp-overview) as of 2026-07-13 (issue #5). The
// prior split pages (configuring-mcp-servers, mcp-transport-mechanisms) redirect
// here; the old H1s and the "Finding MCP Servers" marketplace section are gone.
var realClineMcpLandmarks = []string{
	"Documentation Index",
	"MCP",
	"What MCP gives you",
	"Quick start",
	"Add servers",
	"Manual config",
	"CLI MCP wizard",
	"Configuration examples",
	"Local server (STDIO)",
	"Remote server (Streamable HTTP)",
	"Transport types",
	"Managing servers",
	"Security basics",
	"Troubleshooting",
	"CLI",
}

// TestRecognizeCline_RealMcpLandmarks proves MCP recognition emits the single
// canonical MCP key backed by heading-level evidence on the consolidated
// overview page: transport_types. marketplace was removed from the docs
// (2026-07-13) and must NOT be emitted. tool_filtering / auto_approve remain
// curated as supported but live in body text / config-example JSON (autoApprove
// field), not headings, so the recognizer stays silent. Test merges the other
// content-type fixtures to verify cross-content-type robustness.
func TestRecognizeCline_RealMcpLandmarks(t *testing.T) {
	merged := append([]string{}, realClineLandmarks...)
	merged = append(merged, realClineRulesLandmarks...)
	merged = append(merged, realClineHooksLandmarks...)
	merged = append(merged, realClineMcpLandmarks...)
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["mcp.supported"] != "true" {
		t.Error("mcp.supported missing")
	}
	if caps["mcp.capabilities.transport_types.supported"] != "true" {
		t.Error("mcp.capabilities.transport_types.supported missing")
	}
	if got := caps["mcp.capabilities.transport_types.confidence"]; got != "inferred" {
		t.Errorf("mcp.transport_types.confidence = %q, want inferred", got)
	}
	for _, absent := range []string{
		"mcp.capabilities.marketplace.supported",
		"mcp.capabilities.oauth_support.supported",
		"mcp.capabilities.env_var_expansion.supported",
		"mcp.capabilities.tool_filtering.supported",
		"mcp.capabilities.auto_approve.supported",
		"mcp.capabilities.resource_referencing.supported",
		"mcp.capabilities.enterprise_management.supported",
	} {
		if _, has := caps[absent]; has {
			t.Errorf("%s should NOT be present for cline (no heading evidence or removed from docs)", absent)
		}
	}
}

// TestRecognizeCline_McpAnchorsMissing proves the required-anchor guard
// suppresses MCP emission when "Transport types" is absent.
func TestRecognizeCline_McpAnchorsMissing(t *testing.T) {
	mutated := make([]string, 0, len(realClineMcpLandmarks))
	for _, lm := range realClineMcpLandmarks {
		if lm == "Transport types" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if _, has := result.Capabilities["mcp.supported"]; has {
		t.Error("mcp.supported should NOT be present when 'MCP Transport Mechanisms' anchor is missing")
	}
}

// TestRecognizeCline_HooksStubSuppresses proves the hooks fragment suppresses
// cleanly now that the tracked hooks doc (docs.cline.bot/customization/hooks) is
// a stub redirecting to the SDK plugin docs (2026-07-13, issue #5). With only
// the "Hooks" heading present, the required anchors "Hook Lifecycle" / "Hook
// Locations" are absent, so NO hooks capabilities may be emitted. skills+rules
// still recognize, so the overall status stays Recognized.
func TestRecognizeCline_HooksStubSuppresses(t *testing.T) {
	merged := append([]string{}, realClineLandmarks...)
	merged = append(merged, realClineRulesLandmarks...)
	merged = append(merged, realClineHooksLandmarks...)
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if _, has := caps["hooks.supported"]; has {
		t.Error("hooks.supported should NOT be present — hooks doc is now a stub with no capability headings")
	}
	for _, absent := range []string{
		"hooks.capabilities.handler_types.supported",
		"hooks.capabilities.hook_scopes.supported",
		"hooks.capabilities.json_io_protocol.supported",
		"hooks.capabilities.context_injection.supported",
	} {
		if _, has := caps[absent]; has {
			t.Errorf("%s should NOT be present — hooks doc is a stub (moved to SDK plugins)", absent)
		}
	}
}

// realClineCommandsLandmarks is a snapshot of the headings from Cline's
// slash-commands doc (docs.cline.bot/core-workflows/using-commands) as of
// 2026-07-13. Five built-in slash commands appear as individual H3 landmarks —
// the strongest heading-level evidence for builtin_commands. Changes since the
// 2026-04 snapshot (issue #5): /explain-changes was removed from the table, and
// the "Custom Workflows" section is gone (the dedicated Workflows doc was
// removed; skills-via-slash-command now fills that role — see "Skills via Slash
// Commands").
var realClineCommandsLandmarks = []string{
	"Documentation Index",
	"Using Commands",
	"Slash Commands",
	"/newtask",
	"/smol",
	"/newrule",
	"/deep-planning",
	"/reportbug",
	"Skills via Slash Commands",
}

// TestRecognizeCline_RealCommandsLandmarks proves commands recognition fires
// on the merged skills+rules+hooks+commands fixture, emits builtin_commands
// at "inferred" confidence, and does NOT emit argument_substitution
// (intentionally unmapped — no documented substitution syntax in cline's
// custom workflows per the curator).
func TestRecognizeCline_RealCommandsLandmarks(t *testing.T) {
	merged := append([]string{}, realClineLandmarks...)
	merged = append(merged, realClineRulesLandmarks...)
	merged = append(merged, realClineHooksLandmarks...)
	merged = append(merged, realClineCommandsLandmarks...)
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["commands.supported"] != "true" {
		t.Error("commands.supported missing")
	}
	if caps["commands.capabilities.builtin_commands.supported"] != "true" {
		t.Error("commands.capabilities.builtin_commands.supported missing")
	}
	if got := caps["commands.capabilities.builtin_commands.confidence"]; got != "inferred" {
		t.Errorf("commands.builtin_commands.confidence = %q, want inferred", got)
	}
	if _, has := caps["commands.capabilities.argument_substitution.supported"]; has {
		t.Error("commands.capabilities.argument_substitution.supported should NOT be present for cline (no documented substitution syntax)")
	}
}

// TestRecognizeCline_CommandsAnchorsMissing proves the required-anchor guard
// suppresses commands emission when the unique "/newtask" anchor is absent —
// preventing commands patterns from firing on contexts where only the
// generic "Slash Commands" header appears.
func TestRecognizeCline_CommandsAnchorsMissing(t *testing.T) {
	mutated := make([]string, 0, len(realClineCommandsLandmarks))
	for _, lm := range realClineCommandsLandmarks {
		if lm == "/newtask" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("cline", capmon.RecognitionContext{
		Provider:  "cline",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if _, has := result.Capabilities["commands.supported"]; has {
		t.Error("commands.supported should NOT be present when '/newtask' anchor is missing")
	}
}
