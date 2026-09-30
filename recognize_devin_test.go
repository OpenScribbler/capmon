package capmon_test

import (
	"testing"

	"github.com/OpenScribbler/capmon"
)

// realDevinRulesLandmarks is a snapshot of headings extracted from
// .capmon-cache/devin/rules.0/extracted.json (Memories & Rules doc) and
// .capmon-cache/devin/rules.1/extracted.json (AGENTS.md doc) as of
// 2026-09-30. Update when the docs evolve.
var realDevinRulesLandmarks = []string{
	// rules.0 — Memories & Rules
	"Documentation Index",
	"Memories & Rules",
	"Memories, Rules, Workflows, or Skills?",
	"How to Manage Memories",
	"Memories",
	"Rules",
	"Rules Discovery",
	"Rules Storage Locations",
	"Activation Modes",
	"Best Practices",
	"System-Level Rules (Enterprise)",
	"How System Rules Work",
	// rules.1 — AGENTS.md
	"AGENTS.md",
	"How It Works",
	"Creating an AGENTS.md File",
	"Discovery and Scoping",
	"Automatic Scoping",
	"Comparison with Rules",
}

// TestRecognizeDevin_RealRulesLandmarks proves the canary path: feeding the
// recognizer the real merged rules landmarks (from rules.0 + rules.1)
// produces all expected rules capability dot-paths at confidence "inferred".
func TestRecognizeDevin_RealRulesLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: realDevinRulesLandmarks,
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
		"activation_mode.manual",
		"activation_mode.model_decision",
		"activation_mode.glob",
		"cross_provider_recognition.agents_md",
		"auto_memory",
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
	// file_imports must NOT be emitted — devin docs do not document
	// cross-file import syntax (per seeder spec; XML grouping is in-file).
	if _, has := caps["rules.capabilities.file_imports.supported"]; has {
		t.Error("rules.capabilities.file_imports should NOT be present for devin")
	}
}

// TestRecognizeDevin_AnchorsMissing proves the negative path: stripping a
// required anchor suppresses all rules patterns and surfaces the missing
// anchor name.
func TestRecognizeDevin_AnchorsMissing(t *testing.T) {
	mutated := make([]string, 0, len(realDevinRulesLandmarks))
	for _, lm := range realDevinRulesLandmarks {
		if lm == "Rules Discovery" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: mutated,
	})

	if result.Status != capmon.StatusAnchorsMissing {
		t.Fatalf("status = %q, want %q", result.Status, capmon.StatusAnchorsMissing)
	}
	if _, has := result.Capabilities["rules.supported"]; has {
		t.Error("rules.supported should be absent when required anchor missing")
	}
	found := false
	for _, m := range result.MissingAnchors {
		if m == "Rules Discovery" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("MissingAnchors %v does not include 'Rules Discovery'", result.MissingAnchors)
	}
}

// TestRecognizeDevin_NoLandmarks proves an empty landmark list produces
// "anchors_missing" status with no capabilities.
func TestRecognizeDevin_NoLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider: "devin",
		Format:   "markdown",
	})
	if result.Status != capmon.StatusAnchorsMissing {
		t.Errorf("status = %q, want %q", result.Status, capmon.StatusAnchorsMissing)
	}
	if len(result.Capabilities) != 0 {
		t.Errorf("expected zero capabilities, got %d", len(result.Capabilities))
	}
}

// realDevinHooksLandmarks is a snapshot of the headings extracted from
// .capmon-cache/devin/hooks.0/extracted.json (cli/extensibility/hooks/overview.md)
// and .capmon-cache/devin/hooks.1/extracted.json
// (cli/extensibility/hooks/lifecycle-hooks.md) as of 2026-09-30. Update when
// the docs evolve.
var realDevinHooksLandmarks = []string{
	// hooks.0 — overview
	"Documentation Index",
	"Hooks",
	"What Can Hooks Do?",
	"Quick Example",
	"Hook Events",
	"Hook Format",
	"Command Hooks",
	"Output format",
	"Exit Codes",
	"Where Hooks Live",
	"Project-Level",
	"User-Level (Global)",
	"Verifying Hooks",
	"Next Steps",
	// hooks.1 — lifecycle hooks
	"Lifecycle Hooks",
	"PreToolUse",
	"PostToolUse",
	"PermissionRequest",
	"UserPromptSubmit",
	"Stop",
	"PostCompaction",
	"SessionStart",
	"SessionEnd",
	"Matching Multiple Events",
	"Using the Matcher",
	"Tool names you can match",
}

// TestRecognizeDevin_RealHooksLandmarks proves hooks recognition emits the
// 8 canonical hooks keys the Devin lifecycle hooks docs document, and omits
// async_execution, which neither page documents.
func TestRecognizeDevin_RealHooksLandmarks(t *testing.T) {
	merged := append([]string{}, realDevinRulesLandmarks...)
	merged = append(merged, realDevinHooksLandmarks...)
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["hooks.supported"] != "true" {
		t.Error("hooks.supported missing")
	}
	hooksInferred := []string{
		"handler_types",
		"matcher_patterns",
		"decision_control",
		"input_modification",
		"hook_scopes",
		"json_io_protocol",
		"context_injection",
		"permission_control",
	}
	for _, c := range hooksInferred {
		key := "hooks.capabilities." + c + ".supported"
		if caps[key] != "true" {
			t.Errorf("%s missing", key)
		}
		if got := caps["hooks.capabilities."+c+".confidence"]; got != "inferred" {
			t.Errorf("hooks.%s.confidence = %q, want inferred", c, got)
		}
	}
	if _, has := caps["hooks.capabilities.async_execution.supported"]; has {
		t.Error("hooks.capabilities.async_execution should NOT be present for devin (not documented)")
	}
}

// TestRecognizeDevin_HooksAnchorsMissing proves the required-anchor guard
// suppresses hooks emission when "Where Hooks Live" is absent.
func TestRecognizeDevin_HooksAnchorsMissing(t *testing.T) {
	mutated := []string{}
	for _, lm := range realDevinHooksLandmarks {
		if lm == "Where Hooks Live" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if _, has := result.Capabilities["hooks.supported"]; has {
		t.Error("hooks.supported should NOT be present when 'Where Hooks Live' anchor is missing")
	}
}

// realDevinMcpLandmarks is a snapshot of the headings extracted from
// the MCP doc (.capmon-cache/devin/mcp.0/extracted.json —
// docs.devin.ai/desktop/cascade/mcp.md) as of 2026-09-30.
//
// The MCP doc maps 5 of 8 canonical MCP keys via heading-level
// evidence: transport_types, env_var_expansion, tool_filtering, marketplace,
// enterprise_management. oauth_support, auto_approve, and resource_referencing
// are absent — no heading evidence in the doc.
var realDevinMcpLandmarks = []string{
	"Documentation Index",
	"Cascade MCP Configuration",
	"Adding a new MCP",
	"Configuring MCP tools",
	"mcp\\_config.json",
	"Popular MCP Server Examples",
	"Remote HTTP MCPs",
	"Config Interpolation",
	"Admin Controls (Teams & Enterprises)",
	"MCP Registry",
	"Configuring Custom Registries",
	"MCP Allowlist",
	"How Server Matching Works",
	"Configuration Options",
	"Common Regex Patterns",
	"Notes",
	"Admin Configuration Guidelines",
	"Troubleshooting",
	"General Information",
}

// TestRecognizeDevin_RealMcpLandmarks proves MCP recognition emits 5
// canonical MCP keys at "inferred" confidence: transport_types,
// env_var_expansion, tool_filtering, marketplace, enterprise_management.
// oauth_support, auto_approve, and resource_referencing must NOT be emitted —
// none have heading-level evidence in devin's MCP doc.
//
// Test merges rules + hooks + MCP fixtures to mirror real-world cache merging.
func TestRecognizeDevin_RealMcpLandmarks(t *testing.T) {
	merged := append([]string{}, realDevinRulesLandmarks...)
	merged = append(merged, realDevinHooksLandmarks...)
	merged = append(merged, realDevinMcpLandmarks...)
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
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
	mcpInferred := []string{
		"transport_types",
		"env_var_expansion",
		"tool_filtering",
		"marketplace",
		"enterprise_management",
	}
	for _, c := range mcpInferred {
		key := "mcp.capabilities." + c + ".supported"
		if caps[key] != "true" {
			t.Errorf("%s missing", key)
		}
		if got := caps["mcp.capabilities."+c+".confidence"]; got != "inferred" {
			t.Errorf("mcp.%s.confidence = %q, want inferred", c, got)
		}
	}
	for _, absent := range []string{
		"mcp.capabilities.oauth_support.supported",
		"mcp.capabilities.auto_approve.supported",
		"mcp.capabilities.resource_referencing.supported",
	} {
		if _, has := caps[absent]; has {
			t.Errorf("%s should NOT be present (no heading evidence)", absent)
		}
	}
}

// TestRecognizeDevin_McpAnchorsMissing proves the required-anchor guard
// suppresses MCP emission when "Adding a new MCP" is absent.
func TestRecognizeDevin_McpAnchorsMissing(t *testing.T) {
	mutated := []string{}
	for _, lm := range realDevinMcpLandmarks {
		if lm == "Adding a new MCP" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if _, has := result.Capabilities["mcp.supported"]; has {
		t.Error("mcp.supported should NOT be present when 'Adding a new MCP' anchor is missing")
	}
}

// realDevinCommandsLandmarks is a snapshot of the headings extracted from
// the Workflows doc (.capmon-cache/devin/commands.0/extracted.json —
// docs.devin.ai/desktop/cascade/workflows.md) as of 2026-09-30.
//
// Per docs/provider-formats/devin.yaml, neither canonical commands key
// (argument_substitution, builtin_commands) maps to heading-level evidence
// in this doc:
//   - argument_substitution: curator marks supported=false (no {{args}}
//     placeholder syntax documented for Cascade Workflows)
//   - builtin_commands: curator marks supported=true but evidence is on a
//     different Cascade docs page that is not in this cache
//
// The recognizer therefore emits only commands.supported=true (gated on the
// "How to create a Workflow" + "Workflow Storage Locations" anchors, which
// confirm that devin has a user-authored slash-command surface) without
// any per-key emission. This avoids fabricating signal where heading
// evidence is absent.
var realDevinCommandsLandmarks = []string{
	"Documentation Index",
	"Cascade workflows",
	"How it works",
	"How to create a Workflow",
	"Workflow Discovery",
	"Workflow Storage Locations",
	"Generate a Workflow with Cascade",
	"Example Workflows",
	"System-Level Workflows (Enterprise)",
	"Workflow Precedence",
}

// TestRecognizeDevin_RealCommandsLandmarks proves commands recognition
// emits only the top-level commands.supported=true signal, with no per-key
// emission. The test merges rules + hooks + mcp + commands fixtures to
// mirror real-world cache merging and asserts that no commands.capabilities.*
// keys leak through.
func TestRecognizeDevin_RealCommandsLandmarks(t *testing.T) {
	merged := append([]string{}, realDevinRulesLandmarks...)
	merged = append(merged, realDevinHooksLandmarks...)
	merged = append(merged, realDevinMcpLandmarks...)
	merged = append(merged, realDevinCommandsLandmarks...)
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["commands.supported"] != "true" {
		t.Errorf("commands.supported = %q, want %q", caps["commands.supported"], "true")
	}
	for k := range caps {
		if len(k) > len("commands.capabilities.") && k[:len("commands.capabilities.")] == "commands.capabilities." {
			t.Errorf("commands.capabilities.* should NOT be emitted (no heading evidence): %s = %q", k, caps[k])
		}
	}
}

// TestRecognizeDevin_CommandsAnchorsMissing proves the required-anchor
// guard suppresses commands emission when "How to create a Workflow" is
// absent. This avoids triggering on a docs page that mentions Workflows in
// passing without actually documenting how to create one.
func TestRecognizeDevin_CommandsAnchorsMissing(t *testing.T) {
	mutated := []string{}
	for _, lm := range realDevinCommandsLandmarks {
		if lm == "How to create a Workflow" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if _, has := result.Capabilities["commands.supported"]; has {
		t.Error("commands.supported should NOT be present when 'How to create a Workflow' anchor is missing")
	}
}

// realDevinAgentsLandmarks is a snapshot of the headings extracted from
// .capmon-cache/devin/agents.0/extracted.json (cli/subagents.md) as of
// 2026-09-30. Update when the doc evolves.
var realDevinAgentsLandmarks = []string{
	"Documentation Index",
	"Subagents",
	"How Subagents Work",
	"Subagent Cost",
	"Which Model Does a Subagent Use?",
	"Influencing the Model",
	"Enterprise Controls",
	"Enabling and Disabling Subagents",
	"Subagent Profiles",
	"Tool Permissions",
	"Monitoring Subagents",
	"Subagent Indicator",
	"Subagent Panel",
	"Foreground / Background Switching",
	"Interrupting a Turn",
	"Cancelling Subagents",
	"Resuming Subagents",
	"Nesting Depth",
	"Custom Subagents",
	"Creating a Custom Subagent",
	"Definition File Format",
	"Frontmatter Fields",
	"How Custom Subagents Are Used",
	"Examples",
	"Read-Only Research Agent",
	"Test Runner Agent",
}

// TestRecognizeDevin_RealAgentsLandmarks proves agents recognition emits the
// canonical agents keys the custom-subagents doc documents, and omits
// per_agent_mcp, which it does not.
func TestRecognizeDevin_RealAgentsLandmarks(t *testing.T) {
	merged := append([]string{}, realDevinRulesLandmarks...)
	merged = append(merged, realDevinAgentsLandmarks...)
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: merged,
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["agents.supported"] != "true" {
		t.Error("agents.supported missing")
	}
	for _, c := range []string{
		"definition_format",
		"tool_restrictions",
		"invocation_patterns.automatic_delegation",
		"invocation_patterns.natural_language",
		"invocation_patterns.background",
		"agent_scopes.project",
		"agent_scopes.user",
		"model_selection",
		"subagent_spawning",
	} {
		key := "agents.capabilities." + c + ".supported"
		if caps[key] != "true" {
			t.Errorf("%s missing", key)
		}
		if got := caps["agents.capabilities."+c+".confidence"]; got != "inferred" {
			t.Errorf("agents.%s.confidence = %q, want inferred", c, got)
		}
	}
	if _, has := caps["agents.capabilities.per_agent_mcp.supported"]; has {
		t.Error("agents.capabilities.per_agent_mcp should NOT be present for devin (not documented)")
	}
}

// TestRecognizeDevin_AgentsAnchorsMissing proves the required-anchor guard
// suppresses agents emission when "Creating a Custom Subagent" is absent, so a page
// about built-in subagents alone never claims a user-definable agent format.
func TestRecognizeDevin_AgentsAnchorsMissing(t *testing.T) {
	mutated := []string{}
	for _, lm := range realDevinAgentsLandmarks {
		if lm == "Creating a Custom Subagent" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: mutated,
	})
	if _, has := result.Capabilities["agents.supported"]; has {
		t.Error("agents.supported should NOT be present when 'Creating a Custom Subagent' anchor is missing")
	}
}

// realDevinSkillsLandmarks is a snapshot of headings extracted from
// .capmon-cache/devin/skills.0/extracted.json (cascade/skills.md) as of
// 2026-09-30. Update when the docs evolve.
var realDevinSkillsLandmarks = []string{
	"Documentation Index",
	"Skills",
	"How to Create a Skill",
	"Using the UI (easiest)",
	"Manual Creation",
	"SKILL.md File Format",
	"Example skill",
	"Required Frontmatter Fields",
	"Adding Supporting Resources",
	"Invoking Skills",
	"Automatic Invocation",
	"Manual Invocation",
	"Skill Scopes",
	"System-Level Skills (Enterprise)",
	"Example Use Cases",
	"Deployment Workflow",
	"Code Review Guidelines",
	"Testing Procedures",
	"Best Practices",
	"Skills vs Rules vs Workflows",
	"Related Documentation",
}

// TestRecognizeDevin_RealSkillsLandmarks proves the skills doc alone yields
// every documented skills capability at confidence "inferred".
func TestRecognizeDevin_RealSkillsLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: realDevinSkillsLandmarks,
	})
	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities
	if caps["skills.supported"] != "true" {
		t.Error("skills.supported missing")
	}
	for _, c := range []string{
		"canonical_filename", "display_name", "description", "skill_bundled_resources",
		"auto_invocable", "user_invocable", "project_scope", "global_scope", "shared_scope",
	} {
		key := "skills.capabilities." + c + ".supported"
		if caps[key] != "true" {
			t.Errorf("%s missing", key)
		}
		if got := caps["skills.capabilities."+c+".confidence"]; got != "inferred" {
			t.Errorf("skills.%s.confidence = %q, want inferred", c, got)
		}
	}
}

// TestRecognizeDevin_SkillsAnchorsMissing proves that dropping a required
// skills anchor suppresses every skills pattern.
func TestRecognizeDevin_SkillsAnchorsMissing(t *testing.T) {
	var stripped []string
	for _, l := range realDevinSkillsLandmarks {
		if l != "Skill Scopes" {
			stripped = append(stripped, l)
		}
	}
	result := capmon.RecognizeWithContext("devin", capmon.RecognitionContext{
		Provider:  "devin",
		Format:    "markdown",
		Landmarks: stripped,
	})
	for k := range result.Capabilities {
		if len(k) >= 7 && k[:7] == "skills." {
			t.Errorf("unexpected skills key %s with anchor missing", k)
		}
	}
}
