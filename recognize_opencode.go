package capmon

func init() {
	RegisterRecognizer("opencode", RecognizerKindGoStruct, recognizeOpencode)
}

// MCP recognition is intentionally NOT wired for opencode.
//
// The cached MCP sources are unusable for landmark recognition:
//   - mcp.0 (.capmon-cache/opencode/mcp.0/extracted.json) points at
//     https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json
//     — this is the Crush JSON Schema, not opencode's. The seeder spec for
//     opencode has the wrong source URL. Even if it were the right schema,
//     JSON-Schema fields land in Fields not Landmarks (the listed landmarks
//     are crush struct names like Permissions, Tools, LSPConfig, MCPConfig).
//   - mcp.1 (https://raw.githubusercontent.com/opencode-ai/opencode/main/internal/llm/agent/mcp-tools.go)
//     yields a single landmark "MCPClient" — a Go struct name, not heading
//     evidence and not aligned to any of the 8 canonical MCP keys.
//
// NOTE (2026-07-14): docs/provider-formats/opencode.yaml DOES now carry a fully
// curated mcp section — but for a different project. The format doc was
// re-curated to track sst/opencode (opencode.ai, active), whereas the cached
// sources above and this manifest's content_types still point at the legacy,
// archived opencode-ai/opencode Go tree (and the crush schema). The two describe
// different same-named projects — see the identity-divergence flag in
// docs/provider-sources/opencode.yaml.
//
// Recognizer silence is still the right move against these caches — emitting any
// canonical MCP key from the legacy opencode-ai / crush landmarks would either be
// a false positive (crush schema fields attributed to opencode) or unanchored (a
// struct name without semantic meaning). MCP recognition can be wired once the
// cache is rebuilt from the project the format doc actually tracks (sst/opencode).

// Agents recognition is intentionally NOT wired for opencode.
//
// The cached agents source (.capmon-cache/opencode/agents.0/extracted.json,
// fetched from opencode-ai/opencode/main/internal/llm/agent/agent.go) is a
// Go runtime-event implementation file. Landmarks are AgentEvent,
// AgentEventType, Service — runtime types, not user-facing capability
// vocabulary. Fields are AgentEvent.Done / .Error / .Message / .Progress /
// .SessionID / .Type and AgentEventTypeError / .Response / .Summarize —
// event constants, not agent-definition or scope or tool-restriction
// vocabulary.
//
// None of the 7 canonical agents keys (definition_format, tool_restrictions,
// invocation_patterns, agent_scopes, model_selection, per_agent_mcp,
// subagent_spawning) can be anchored on these landmarks or fields. The
// source describes how the runtime emits events about ONE agent's
// processing, not how multiple custom agents are defined or invoked.
//
// NOTE (2026-07-14): docs/provider-formats/opencode.yaml DOES now carry a fully
// curated agents section — but for sst/opencode (opencode.ai, active), which the
// format doc was re-curated to track. The cached agents source above is still the
// legacy, archived opencode-ai/opencode Go runtime file — a different same-named
// project. See the identity-divergence flag in docs/provider-sources/opencode.yaml.
//
// Recognizer silence is still the right move against this cache — emitting any
// canonical agents key from the legacy AgentEvent runtime types would conflate
// "agent runtime exists" with "user-defined custom agents are supported". Agents
// recognition can be wired once the cache is rebuilt from the project the format
// doc actually tracks (sst/opencode), whose agent format IS documented.

// Commands recognition is intentionally NOT wired for opencode.
//
// The cached commands source (.capmon-cache/opencode/commands.0/extracted.json,
// fetched from opencode-ai/opencode/main/internal/tui/components/dialog/
// commands.go) yields exactly one landmark: "CommandRunCustomMsg". This is
// a Go BubbleTea message struct used internally to dispatch slash-command
// runs through the TUI event loop — a runtime type, not a user-facing
// capability vocabulary. Neither canonical commands key
// (argument_substitution, builtin_commands) can be anchored on a single
// message-struct name.
//
// NOTE (2026-07-14): docs/provider-formats/opencode.yaml DOES now carry a fully
// curated commands section — but for sst/opencode (opencode.ai, active), which
// the format doc was re-curated to track. The cached commands source above is
// still the legacy, archived opencode-ai/opencode Go TUI file — a different
// same-named project. See the identity-divergence flag in
// docs/provider-sources/opencode.yaml.
//
// Recognizer silence is still the right move against this cache — emitting any
// commands key from the legacy CommandRunCustomMsg TUI dispatcher would conflate
// "TUI dispatcher exists" with "slash-command authoring is supported". Commands
// recognition can be wired once the cache is rebuilt from the project the format
// doc actually tracks (sst/opencode), whose command format IS documented.

// recognizeOpencode recognizes skills capabilities for the OpenCode provider.
// OpenCode has no native skill format, so this recognizer uses the cross-provider
// SKILL.md convention; the static scope paths merged below (.opencode/skill/ and
// ~/.config/opencode/skills/) match the skills section of the format doc, which
// now tracks the active sst/opencode (opencode.ai). GoStruct pattern will produce
// output only if upstream extraction surfaces Skill.* fields. MCP, agents, and
// commands recognition are intentionally absent — see the comment blocks
// immediately above this function for rationale, including the sst/opencode vs
// legacy opencode-ai/opencode source-identity divergence.
func recognizeOpencode(ctx RecognitionContext) RecognitionResult {
	result := recognizeGoStruct(ctx.Fields, SkillsGoStructOptions())
	if len(result) == 0 {
		return wrapCapabilities(result)
	}
	mergeInto(result, capabilityDotPaths("skills", "project_scope", "cross-provider SKILL.md convention at .opencode/skill/<name>/SKILL.md", "confirmed"))
	mergeInto(result, capabilityDotPaths("skills", "global_scope", "cross-provider convention at ~/.config/opencode/skills/<name>/SKILL.md", "confirmed"))
	mergeInto(result, capabilityDotPaths("skills", "canonical_filename", "SKILL.md (Agent Skills spec)", "confirmed"))
	return wrapCapabilities(result)
}
