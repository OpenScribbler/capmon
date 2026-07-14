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
// RESOLVED (2026-07-14, capmon-ctj): the identity divergence is fixed — the
// manifest was re-onboarded to the active anomalyco/opencode (formerly
// sst/opencode; opencode.ai), matching the format doc, and its mcp source is
// now the opencode.ai/docs/mcp-servers.md mirror. The legacy opencode-ai /
// crush cache entries described above no longer exist after refetch. MCP
// recognition is wired in the follow-up recognizer PR against the rebuilt
// docs-markdown cache.

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
// RESOLVED (2026-07-14, capmon-ctj): the identity divergence is fixed — the
// manifest was re-onboarded to the active anomalyco/opencode (formerly
// sst/opencode; opencode.ai), and its agents sources are now the
// opencode.ai/docs/agents.md mirror plus the anomalyco config/agent.ts loader.
// The legacy opencode-ai AgentEvent cache described above no longer exists
// after refetch. Agents recognition is wired in the follow-up recognizer PR
// against the rebuilt cache.

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
// RESOLVED (2026-07-14, capmon-ctj): the identity divergence is fixed — the
// manifest was re-onboarded to the active anomalyco/opencode (formerly
// sst/opencode; opencode.ai), and its commands source is now the
// opencode.ai/docs/commands.md mirror. The legacy opencode-ai TUI cache
// described above no longer exists after refetch. Commands recognition is
// wired in the follow-up recognizer PR against the rebuilt cache.

// recognizeOpencode recognizes skills capabilities for the opencode provider
// (anomalyco/opencode, formerly sst/opencode). opencode has no native skill
// format, so this recognizer uses the cross-provider SKILL.md convention; the
// static scope paths merged below (.opencode/skill/ and
// ~/.config/opencode/skills/) match the skills section of the format doc.
// GoStruct pattern will produce output only if upstream extraction surfaces
// Skill.* fields. MCP, agents, and commands recognition are pending the
// follow-up recognizer PR — see the comment blocks immediately above this
// function.
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
