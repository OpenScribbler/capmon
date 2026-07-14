package capmon_test

import (
	"testing"

	"github.com/OpenScribbler/capmon"
)

// Landmark snapshots below were extracted from the opencode cache rebuilt
// 2026-07-14 from the anomalyco/opencode (opencode.ai) docs-markdown mirrors,
// immediately after the identity re-onboard. Update when the docs evolve.

// realOpencodeRulesLandmarks — .capmon-cache/opencode/rules.0/extracted.json
// (opencode.ai/docs/rules.md).
var realOpencodeRulesLandmarks = []string{
	"Initialize",
	"Example",
	"Types",
	"Project",
	"Global",
	"Claude Code Compatibility",
	"Precedence",
	"Custom Instructions",
	"Referencing External Files",
	"Using opencode.json",
	"Manual Instructions in AGENTS.md",
}

// realOpencodeAgentsLandmarks — .capmon-cache/opencode/agents.0/extracted.json
// (opencode.ai/docs/agents.md).
var realOpencodeAgentsLandmarks = []string{
	"Types",
	"Primary agents",
	"Subagents",
	"Built-in",
	"Use build",
	"Use plan",
	"Use general",
	"Use explore",
	"Use scout",
	"Use compaction",
	"Use title",
	"Use summary",
	"Usage",
	"Configure",
	"JSON",
	"Markdown",
	"Options",
	"Description",
	"Temperature",
	"Max steps",
	"Disable",
	"Prompt",
	"Model",
	"Tools (deprecated)",
	"Permissions",
	"Mode",
	"Hidden",
	"Task permissions",
	"Color",
	"Top P",
	"Additional",
	"Create agents",
	"Use cases",
	"Examples",
	"Documentation agent",
	"Security auditor",
}

// realOpencodeCommandsLandmarks — .capmon-cache/opencode/commands.0/extracted.json
// (opencode.ai/docs/commands.md).
var realOpencodeCommandsLandmarks = []string{
	"Create command files",
	"Configure",
	"JSON",
	"Markdown",
	"Prompt config",
	"Arguments",
	"Shell output",
	"File references",
	"Options",
	"Template",
	"Description",
	"Agent",
	"Subtask",
	"Model",
	"Built-in",
}

// realOpencodeMcpLandmarks — .capmon-cache/opencode/mcp.0/extracted.json
// (opencode.ai/docs/mcp-servers.md).
var realOpencodeMcpLandmarks = []string{
	"Caveats",
	"Enable",
	"Overriding remote defaults",
	"Local",
	"Options",
	"Remote",
	"OAuth",
	"Automatic",
	"Pre-registered",
	"Authenticating",
	"Disabling OAuth",
	"OAuth Options",
	"Debugging",
	"Manage",
	"Global",
	"Per agent",
	"Glob patterns",
	"Examples",
	"Sentry",
	"Context7",
	"Grep by Vercel",
}

// mergedOpencodeLandmarks mirrors production conditions, where the
// RecognitionContext carries every cached source's landmarks merged.
func mergedOpencodeLandmarks() []string {
	var all []string
	all = append(all, realOpencodeRulesLandmarks...)
	all = append(all, realOpencodeAgentsLandmarks...)
	all = append(all, realOpencodeCommandsLandmarks...)
	all = append(all, realOpencodeMcpLandmarks...)
	return all
}

// TestRecognizeOpencode_MergedRealLandmarks proves the canary path: feeding
// the recognizer the full merged landmark set (rules + agents + commands +
// mcp, as production does) emits every expected capability dot-path at
// confidence "inferred", and emits none of the intentionally-skipped keys.
func TestRecognizeOpencode_MergedRealLandmarks(t *testing.T) {
	result := capmon.RecognizeWithContext("opencode", capmon.RecognitionContext{
		Provider:  "opencode",
		Format:    "markdown",
		Landmarks: mergedOpencodeLandmarks(),
	})

	if result.Status != capmon.StatusRecognized {
		t.Fatalf("status = %q, want %q (missing=%v)", result.Status, capmon.StatusRecognized, result.MissingAnchors)
	}
	caps := result.Capabilities

	expected := map[string][]string{
		"rules": {
			"cross_provider_recognition.agents_md",
			"cross_provider_recognition.claude_md",
			"file_imports",
			"hierarchical_loading",
		},
		"agents": {
			"definition_format",
			"invocation_patterns.at_mention",
			"invocation_patterns.natural_language",
			"model_selection",
			"subagent_spawning",
			"tool_restrictions",
		},
		"commands": {
			"argument_substitution",
			"builtin_commands",
		},
		"mcp": {
			"transport_types",
			"oauth_support",
			"enterprise_management",
			"tool_filtering",
		},
	}
	for ct, keys := range expected {
		if caps[ct+".supported"] != "true" {
			t.Errorf("%s.supported missing", ct)
		}
		for _, c := range keys {
			key := ct + ".capabilities." + c + ".supported"
			if caps[key] != "true" {
				t.Errorf("%s missing", key)
			}
			if got := caps[ct+".capabilities."+c+".confidence"]; got != "inferred" {
				t.Errorf("%s.%s.confidence = %q, want inferred", ct, c, got)
			}
		}
	}

	// Intentionally skipped keys must NOT be emitted: curator-unsupported
	// (rules.activation_mode, rules.auto_memory, agents.per_agent_mcp) and
	// heading-less curator-authoritative (agents.agent_scopes,
	// mcp.env_var_expansion).
	for _, absent := range []string{
		"rules.capabilities.activation_mode.supported",
		"rules.capabilities.auto_memory.supported",
		"agents.capabilities.agent_scopes.supported",
		"agents.capabilities.per_agent_mcp.supported",
		"mcp.capabilities.env_var_expansion.supported",
	} {
		if _, has := caps[absent]; has {
			t.Errorf("%s should NOT be present", absent)
		}
	}
}

// TestRecognizeOpencode_RulesAnchorsMissing proves the negative path:
// stripping a required rules anchor suppresses all rules patterns and
// surfaces the missing anchor name, without disturbing the other content
// types' emissions.
func TestRecognizeOpencode_RulesAnchorsMissing(t *testing.T) {
	var mutated []string
	for _, lm := range mergedOpencodeLandmarks() {
		if lm == "Claude Code Compatibility" {
			continue
		}
		mutated = append(mutated, lm)
	}
	result := capmon.RecognizeWithContext("opencode", capmon.RecognitionContext{
		Provider:  "opencode",
		Format:    "markdown",
		Landmarks: mutated,
	})

	if _, has := result.Capabilities["rules.supported"]; has {
		t.Error("rules.supported should be absent when a required rules anchor is missing")
	}
	found := false
	for _, m := range result.MissingAnchors {
		if m == "Claude Code Compatibility" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("MissingAnchors %v does not include 'Claude Code Compatibility'", result.MissingAnchors)
	}
	// Other content types keep their emissions.
	if result.Capabilities["mcp.supported"] != "true" {
		t.Error("mcp.supported should survive a rules anchor failure")
	}
	if result.Capabilities["agents.supported"] != "true" {
		t.Error("agents.supported should survive a rules anchor failure")
	}
}

// TestRecognizeOpencode_SingleDocContexts proves per-doc isolation: each
// content type's landmarks alone emit only that content type's capabilities —
// the required anchors prevent any cross-content-type bleed.
func TestRecognizeOpencode_SingleDocContexts(t *testing.T) {
	cases := []struct {
		name      string
		landmarks []string
		wantCT    string
		absentCTs []string
	}{
		{"rules only", realOpencodeRulesLandmarks, "rules", []string{"agents", "commands", "mcp"}},
		{"agents only", realOpencodeAgentsLandmarks, "agents", []string{"rules", "commands", "mcp"}},
		{"commands only", realOpencodeCommandsLandmarks, "commands", []string{"rules", "agents", "mcp"}},
		{"mcp only", realOpencodeMcpLandmarks, "mcp", []string{"rules", "agents", "commands"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := capmon.RecognizeWithContext("opencode", capmon.RecognitionContext{
				Provider:  "opencode",
				Format:    "markdown",
				Landmarks: tc.landmarks,
			})
			if result.Capabilities[tc.wantCT+".supported"] != "true" {
				t.Errorf("%s.supported missing in its own doc context", tc.wantCT)
			}
			for _, ct := range tc.absentCTs {
				if _, has := result.Capabilities[ct+".supported"]; has {
					t.Errorf("%s.supported should be absent with only %s landmarks", ct, tc.wantCT)
				}
			}
		})
	}
}
