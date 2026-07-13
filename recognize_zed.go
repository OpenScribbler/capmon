package capmon

func init() {
	RegisterRecognizer("zed", RecognizerKindDoc, recognizeZed)
}

// zedRulesLandmarkOptions returns the landmark patterns for Zed's
// instructions documentation. Anchors derived from the HTML doc at
// zed.dev/docs/ai/instructions — the 2026-07 successor to
// zed.dev/docs/ai/rules (drift issue #17): Zed renamed Rules to
// Instructions and split the old feature in two. Always-on rules became
// AGENTS.md instructions (personal + project); reusable on-demand Library
// rules became Skills — a different content type, so activation_mode.manual
// (previously anchored on 'Slash Commands in Rules') is no longer part of
// zed's rules vocabulary and is intentionally unmapped. rules.0 is zed's
// own .rules instance file (their internal Rust coding guidelines) and
// intentionally NOT used as evidence — instance content is not capability
// vocabulary.
//
// The new page also documents a personal scope (~/.config/zed/AGENTS.md,
// project file wins on conflict). hierarchical_loading stays unmapped: two
// fixed file locations are not directory-level traversal with subdirectory
// overrides.
//
// Required anchors are unique to the instructions doc:
//   - "Instruction File Support" — H2, the per-file compatibility table.
//     Other zed docs use "Agent Settings" (agents.2) or "Agent Panel
//     Usage" (mcp.1).
//   - "Migrating from Rules"     — H2, the rename's own migration heading.
func zedRulesLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Instruction File Support", CaseInsensitive: true},
		{Kind: "substring", Value: "Migrating from Rules", CaseInsensitive: true},
	}
	return RulesLandmarkOptions(
		RulesLandmarkPattern("activation_mode.always", "Project Instructions",
			"instructions are always-on context: personal ~/.config/zed/AGENTS.md applies to every project, and the first-match project-root instruction file applies to the current project, overriding personal on conflict (documented under 'Personal Instructions' / 'Project Instructions')", required),
		RulesLandmarkPattern("cross_provider_recognition.agents_md", "AGENTS.md",
			"AGENTS.md is the primary instruction file (personal + project); AGENT.md also recognized in the project first-match list (documented under 'Project Instructions')", required),
		RulesLandmarkPattern("cross_provider_recognition.claude_md", "CLAUDE.md",
			"CLAUDE.md recognized in the project-root first-match list (documented under 'Project Instructions' / 'Instruction File Support')", required),
		RulesLandmarkPattern("cross_provider_recognition.gemini_md", "GEMINI.md",
			"GEMINI.md recognized in the project-root first-match list (documented under 'Project Instructions')", required),
		RulesLandmarkPattern("cross_provider_recognition.cursorrules", ".cursorrules",
			".cursorrules recognized in the project-root first-match list (documented under 'Project Instructions')", required),
		RulesLandmarkPattern("cross_provider_recognition.windsurfrules", ".windsurfrules",
			".windsurfrules recognized in the project-root first-match list (documented under 'Project Instructions')", required),
		RulesLandmarkPattern("cross_provider_recognition.clinerules", ".clinerules",
			".clinerules recognized in the project-root first-match list (documented under 'Project Instructions')", required),
	)
}

// zedMcpLandmarkOptions returns the landmark patterns for Zed's MCP
// documentation. Anchors derived from .capmon-cache/zed/mcp.1/extracted.json
// (zed.dev/docs/ai/mcp, HTML). mcp.0 is a Rust source file
// (crates/context_server/src/context_server.rs) yielding only 3 struct
// names — typed evidence not aligned to landmark matching.
//
// Zed's MCP doc maps only 2 of 8 canonical MCP keys at the heading level:
// tool_filtering ("Tool Permissions") and marketplace ("As Extensions" —
// Zed's extension catalog is the in-IDE MCP server marketplace).
//
// The other 6 keys are intentionally unmapped here:
//   - transport_types: "As Custom Servers" / "As Extensions" sub-headings
//     describe install methods, not transport types. The Rust struct
//     ContextServerTransport (mcp.0) hints at transport abstraction but the
//     doc heading evidence is too weak.
//   - oauth_support, env_var_expansion, auto_approve, resource_referencing,
//     enterprise_management: no heading evidence in mcp.1.
//
// Required anchors are unique to the MCP doc:
//   - "Model Context Protocol" — H1, MCP-specific
//   - "Installing MCP Servers"  — H2, MCP-specific
//
// Neither appears in zed's rules, commands, or agents docs.
//
// docs/provider-formats/zed.yaml has no curated MCP section — the only
// curated content type is skills (marked unsupported). Recognizer emissions
// land in docs/provider-capabilities/zed.yaml at "inferred" confidence.
func zedMcpLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Model Context Protocol", CaseInsensitive: true},
		{Kind: "substring", Value: "Installing MCP Servers", CaseInsensitive: true},
	}
	return McpLandmarkOptions(
		McpLandmarkPattern("tool_filtering", "Tool Permissions",
			"per-tool permission control documented under 'Tool Permissions' heading", required),
		McpLandmarkPattern("marketplace", "As Extensions",
			"in-IDE MCP server marketplace via Zed's extension catalog documented under 'As Extensions' (vs 'As Custom Servers') sub-heading of 'Installing MCP Servers'", required),
	)
}

// zedAgentsLandmarkOptions returns the landmark patterns for Zed's
// "Agent Settings" doc. Anchors derived from
// .capmon-cache/zed/agents.2/extracted.json
// (zed.dev/docs/ai/agent-settings, HTML).
//
// Zed's agent feature is the "Agent Profile" — a named configuration of tool
// permissions, MCP context-server presets, and an optional model preference.
// Builtin profiles are write/ask/minimal; users may create additional named
// profiles via AgentProfile::create. Profiles are settings.json entries
// (under agent.profiles.<id> per AgentProfileContent in agents.1 Rust
// source), not standalone definition files — so definition_format is
// intentionally unmapped here (the curator may still mark it supported from
// broader knowledge of the AgentProfileSettings struct).
//
// Maps 2 of 7 canonical agents keys at heading-level evidence:
//   - tool_restrictions: per-profile tools toggle map
//     (AgentProfileSettings.tools is IndexMap<tool_name, bool>) with default
//     plus per-tool override and pattern-precedence semantics; documented
//     under "Default Tool Permissions" / "Per-tool Permission Rules" /
//     "Pattern Precedence" headings.
//   - per_agent_mcp: per-profile MCP context server scoping
//     (AgentProfileSettings.context_servers is IndexMap<server_id,
//     ContextServerPreset> with per-tool granularity); documented under
//     "MCP Tool Permissions" heading.
//
// Five keys are intentionally unmapped:
//   - definition_format: profiles are settings.json entries, not separate
//     files. No "Profile File" or "Defining a Profile" heading in agents.2.
//   - invocation_patterns: profiles are switched via the agent panel UI,
//     not slash-command or @-mention. No invocation-mode heading.
//   - agent_scopes: profiles live in global settings.json. No project-scope
//     vs user-scope heading distinction in the doc.
//   - model_selection: AgentProfileSettings has a per-profile default_model
//     field (agents.1 Rust source) but agents.2 doc only documents
//     panel-level "Default Model" / "Feature-specific Models" — those
//     headings describe the global default, not per-profile selection.
//   - subagent_spawning: no chain/spawn/delegate heading; no multi-profile
//     coordination documented.
//
// Required anchors are unique to agents.2:
//   - "Agent Settings"            — H1, agents-specific. Other zed docs use
//     "Agent Panel Usage" (mcp.1) or "Instructions" (rules.1), not "Agent
//     Settings".
//   - "Per-tool Permission Rules" — H3, agents-specific. mcp.1 uses just
//     "Tool Permissions"; this longer phrase appears nowhere else.
func zedAgentsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Agent Settings", CaseInsensitive: true},
		{Kind: "substring", Value: "Per-tool Permission Rules", CaseInsensitive: true},
	}
	return AgentsLandmarkOptions(
		AgentsLandmarkPattern("tool_restrictions", "Per-tool Permission Rules",
			"per-profile tool toggle map (AgentProfileSettings.tools IndexMap<tool_name, bool>) with default + per-tool override semantics, documented under 'Default Tool Permissions' / 'Per-tool Permission Rules' / 'Pattern Precedence' headings", required),
		AgentsLandmarkPattern("per_agent_mcp", "MCP Tool Permissions",
			"per-profile MCP context server scoping (AgentProfileSettings.context_servers IndexMap<server_id, ContextServerPreset> with per-tool granularity), documented under 'MCP Tool Permissions' heading", required),
	)
}

// Commands recognition is intentionally NOT wired for zed.
//
// The cached commands source (.capmon-cache/zed/commands.0/extracted.json,
// fetched from zed.dev's slash-command Rust trait or extension API) yields
// 4 landmarks: "SlashCommand", "SlashCommandOutput",
// "SlashCommandOutputSection", "SlashCommandArgumentCompletion". These are
// Rust trait + struct names from zed's extension API (extension-api crate).
// Type-name landmark matching could theoretically anchor builtin_commands
// (every concrete SlashCommand impl is a built-in), but zed's slash-
// commands surface is provided by the EXTENSION ecosystem rather than a
// closed built-in catalog — there is no canonical "list of built-in
// commands" inside zed itself, so the canonical key would have unclear
// semantics here.
//
// More importantly, docs/provider-formats/zed.yaml has no curated commands
// section at all — the only curated content type is skills (marked
// unsupported). With no curator baseline, the recognizer would be the sole
// source of truth for zed.commands.* dot-paths, and emitting from extension-
// API trait names alone (without a docs page explaining how zed users
// invoke /-commands or whether argument substitution is supported) would
// be a guess rather than evidence.
//
// SlashCommandArgumentCompletion hints that arguments exist, but that
// trait is for IDE auto-complete on argument input, not for in-prompt
// substitution syntax — a different mechanism than {{args}} or
// $ARGUMENTS. Mapping it to canonical argument_substitution would be
// semantically wrong.
//
// Recognizer silence is the right move. Commands recognition can be wired
// once a zed docs page documenting /-command authoring (extension or
// otherwise) and argument substitution semantics is added to the cache.

// recognizeZed recognizes rules + mcp + agents capabilities for the Zed
// provider. Zed does not support Agent Skills, so skills emission is
// intentionally a no-op (confirmed-negative signal). Rules, MCP, and agents
// recognition use landmark matching from zed's HTML docs at
// zed.dev/docs/ai/{rules,mcp,agent-settings}. Commands recognition is
// intentionally absent — see the comment block immediately above this
// function for rationale.
func recognizeZed(ctx RecognitionContext) RecognitionResult {
	rulesResult := recognizeLandmarks(ctx, zedRulesLandmarkOptions())
	mcpResult := recognizeLandmarks(ctx, zedMcpLandmarkOptions())
	agentsResult := recognizeLandmarks(ctx, zedAgentsLandmarkOptions())
	return mergeRecognitionResults(rulesResult, mcpResult, agentsResult)
}
