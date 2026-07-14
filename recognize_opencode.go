package capmon

func init() {
	RegisterRecognizer("opencode", RecognizerKindGoStruct, recognizeOpencode)
}

// opencodeRulesLandmarkOptions returns the landmark patterns for opencode's
// rules documentation. Anchors derived from
// .capmon-cache/opencode/rules.0/extracted.json (opencode.ai/docs/rules.md,
// fetched 2026-07-14 after the anomalyco/opencode re-onboard).
//
// Required anchors "Claude Code Compatibility" and "Referencing External
// Files" are unique to the rules doc — verified absent from the agents.0,
// commands.0, and mcp.0 landmark sets, so cross-content-type landmark merging
// cannot fire rules patterns without the rules doc present.
//
// Two curator-unsupported keys are intentionally NOT emitted:
//   - activation_mode: rule files load unconditionally as context.
//   - auto_memory: no runtime command persists notes into rule files.
//
// Verified live: https://opencode.ai/docs/rules.md on 2026-07-14. The page
// documents unconditional context loading, AGENTS.md/CLAUDE.md discovery,
// the instructions array, and precedence — but no conditional-activation
// syntax and no memory-persistence command. Format YAML agrees: both keys
// supported: false (docs/provider-formats/opencode.yaml lines 71–74, 83–86).
func opencodeRulesLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Claude Code Compatibility", CaseInsensitive: true},
		{Kind: "substring", Value: "Referencing External Files", CaseInsensitive: true},
	}
	return RulesLandmarkOptions(
		RulesLandmarkPattern("cross_provider_recognition.agents_md", "Manual Instructions in AGENTS.md",
			"AGENTS.md read from project root and parent directories; global at ~/.config/opencode/AGENTS.md (documented under 'Types' / 'Project' / 'Global' headings)", required),
		RulesLandmarkPattern("cross_provider_recognition.claude_md", "Claude Code Compatibility",
			"CLAUDE.md fallback at project root and ~/.claude/CLAUDE.md globally when no AGENTS.md exists; disable via OPENCODE_DISABLE_CLAUDE_CODE=1 (documented under 'Claude Code Compatibility')", required),
		RulesLandmarkPattern("file_imports", "Referencing External Files",
			"instructions array in opencode.json (or the global ~/.config/opencode/opencode.json) lists additional rule files by relative path, glob pattern, or remote URL — all combined with AGENTS.md into context (documented under 'Referencing External Files' / 'Using opencode.json')", required),
		RulesLandmarkPattern("hierarchical_loading", "Precedence",
			"AGENTS.md (CLAUDE.md fallback) collected walking up from cwd; global ~/.config/opencode/AGENTS.md and ~/.claude/CLAUDE.md appended; all matching files combined (documented under 'Types' / 'Precedence')", required),
	)
}

// opencodeAgentsLandmarkOptions returns the landmark patterns for opencode's
// agents documentation. Anchors derived from
// .capmon-cache/opencode/agents.0/extracted.json (opencode.ai/docs/agents.md,
// 36 landmarks, fetched 2026-07-14). The second agents source (agents.1,
// anomalyco config/agent.ts) is a TypeScript thin loader with no extractable
// landmarks; the docs page is the recognition surface.
//
// Required anchors "Primary agents" and "Subagents" are unique to the agents
// doc — verified absent from the rules.0, commands.0, and mcp.0 landmark sets.
//
// Both invocation_patterns sub-keys gate on the single "Subagents" heading:
// the section documents @-mention manual invocation and description-driven
// automatic delegation together, and neither has its own heading (same
// one-heading-many-subkeys shape as windsurf's activation modes).
//
// The "Model" matcher is Kind exact: the commands doc carries an identical
// 'Model' heading (per-command model override), so a substring match adds
// nothing and exact keeps the intent narrow. Both docs' claims are true and
// curator-confirmed, and Required already gates this pattern on the agents
// doc's presence.
//
// Two keys are intentionally NOT emitted:
//   - agent_scopes: supported and curator-confirmed
//     (docs/provider-formats/opencode.yaml lines 145–148), but the scope
//     paths (.opencode/agents/ project, ~/.config/opencode/agents/ global)
//     appear only in configuration prose — there is no scope heading for a
//     landmark to anchor on. The curated format-YAML value is authoritative.
//     Verified live: https://opencode.ai/docs/agents.md on 2026-07-14 — the
//     page documents both scope paths in the 'Configure' section body.
//   - per_agent_mcp: curator marks supported: false (lines 154–157); the
//     page contains zero mcpServers mentions.
//     Verified live: https://opencode.ai/docs/agents.md on 2026-07-14 —
//     agents inherit workspace MCP config, gated via the permission map.
func opencodeAgentsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Primary agents", CaseInsensitive: true},
		{Kind: "substring", Value: "Subagents", CaseInsensitive: true},
	}
	return AgentsLandmarkOptions(
		AgentsLandmarkPattern("definition_format", "Create agents",
			"markdown with YAML frontmatter in .opencode/agents/*.md (project) and ~/.config/opencode/agents/*.md (global); the loader accepts both agent/ and agents/ directory names and recurses; frontmatter declares description/mode/model/temperature/top_p/steps/permission/color/hidden/disable/variant/prompt and the body becomes the system prompt (documented under 'Configure' / 'Markdown' / 'Create agents')", required),
		AgentsLandmarkPattern("invocation_patterns.at_mention", "Subagents",
			"subagents invoked manually by @-mentioning the subagent name in a message; agent name derives from the filename (documented under 'Subagents')", required),
		AgentsLandmarkPattern("invocation_patterns.natural_language", "Subagents",
			"subagents invoked automatically by primary agents for specialized tasks based on their description field (documented under 'Subagents')", required),
		LandmarkPattern{
			Capability: "model_selection",
			Required:   required,
			Matchers:   []StringMatcher{{Kind: "exact", Value: "Model", CaseInsensitive: true}},
			Mechanism:  "frontmatter model field (provider/model-id format) overrides the session default; subagents default to the invoking primary agent's model (documented under 'Model')",
		},
		AgentsLandmarkPattern("subagent_spawning", "Task permissions",
			"agents with mode: subagent (or all) are invocable from other agents via the Task tool, driven by the callee's description; permission.task on the orchestrator controls which subagents it may invoke (documented under 'Task permissions')", required),
		AgentsLandmarkPattern("tool_restrictions", "Permissions",
			"permission frontmatter map configures ask/allow/deny per tool using named keys or wildcard glob patterns; fine-grained keys accept glob/pattern-to-action objects (documented under 'Permissions')", required),
	)
}

// opencodeCommandsLandmarkOptions returns the landmark patterns for opencode's
// commands documentation. Anchors derived from
// .capmon-cache/opencode/commands.0/extracted.json
// (opencode.ai/docs/commands.md, 15 landmarks, fetched 2026-07-14).
//
// Required anchors "Create command files" and "Prompt config" are unique to
// the commands doc — verified absent from the rules.0, agents.0, and mcp.0
// landmark sets.
//
// The "Built-in" matcher also matches the agents doc's identical 'Built-in'
// heading (built-in agents). Required gates this pattern on the commands
// doc's presence, and the key is curator-confirmed for commands
// (built-in /init, /undo, /redo, /share, /help), so the cross-doc collision
// cannot produce a false claim.
func opencodeCommandsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Create command files", CaseInsensitive: true},
		{Kind: "substring", Value: "Prompt config", CaseInsensitive: true},
	}
	return CommandsLandmarkOptions(
		CommandsLandmarkPattern("argument_substitution", "Arguments",
			"$ARGUMENTS carries all arguments as a single string; positional $1/$2/$3 carry individual arguments — substituted from user input at invocation time (documented under 'Arguments')", required),
		CommandsLandmarkPattern("builtin_commands", "Built-in",
			"built-in /init, /undo, /redo, /share, /help; custom commands override built-ins when given the same name (documented under 'Built-in')", required),
	)
}

// opencodeMcpLandmarkOptions returns the landmark patterns for opencode's MCP
// documentation. Anchors derived from
// .capmon-cache/opencode/mcp.0/extracted.json
// (opencode.ai/docs/mcp-servers.md, 22 landmarks, fetched 2026-07-14).
//
// Required anchors "Overriding remote defaults" and "OAuth Options" are
// unique to the MCP doc — verified absent from the rules.0, agents.0, and
// commands.0 landmark sets.
//
// env_var_expansion is intentionally NOT emitted, but NOT because opencode
// lacks it. The doc documents {env:VAR_NAME} interpolation for remote server
// headers and oauth fields — in configuration prose only, with no dedicated
// heading for a landmark to anchor on; the curated format-YAML value
// (supported: true, confirmed — docs/provider-formats/opencode.yaml lines
// 323–326) is authoritative.
// Verified live: https://opencode.ai/docs/mcp-servers.md on 2026-07-14. The
// page shows {env:MY_MCP_CLIENT_ID}, {env:MY_MCP_CLIENT_SECRET}, and
// {env:MY_API_KEY} examples under the OAuth and headers config blocks.
//
// auto_approve, marketplace, and resource_referencing carry no format-YAML
// mapping and no heading evidence — no claim is made either way.
func opencodeMcpLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Overriding remote defaults", CaseInsensitive: true},
		{Kind: "substring", Value: "OAuth Options", CaseInsensitive: true},
	}
	return McpLandmarkOptions(
		McpLandmarkPattern("transport_types", "Local",
			"two types under the mcp key in opencode.json: local (stdio, launched via command array) and remote (HTTP/SSE streamable, connected via url) (documented under 'Local' / 'Remote' headings)", required),
		McpLandmarkPattern("oauth_support", "OAuth",
			"OAuth 2.0 with Dynamic Client Registration (RFC 7591): 401 auto-detected, tokens stored in ~/.local/share/opencode/mcp-auth.json; oauth object for pre-registered credentials or oauth: false to disable (documented under 'OAuth' / 'Authenticating' / 'OAuth Options')", required),
		McpLandmarkPattern("enterprise_management", "Overriding remote defaults",
			"organizations publish default MCP server configs via a .well-known/opencode endpoint; local opencode.json values override remote defaults (documented under 'Enable' / 'Overriding remote defaults')", required),
		McpLandmarkPattern("tool_filtering", "Glob patterns",
			"global tools map keyed by server-name prefix with glob patterns (e.g. mymcp_*); per-agent access via the agent permission map (documented under 'Manage' / 'Global' / 'Per agent' / 'Glob patterns')", required),
	)
}

// recognizeOpencode recognizes skills + rules + agents + commands + mcp
// capabilities for the opencode provider (anomalyco/opencode, formerly
// sst/opencode; opencode.ai). Skills use the GoStruct strategy plus static
// cross-provider SKILL.md scope paths — opencode has no native skill format,
// so GoStruct produces output only if upstream extraction surfaces Skill.*
// typed fields. Rules, agents, commands, and MCP are landmark-based against
// the rebuilt opencode.ai docs-markdown caches (rules.0, agents.0,
// commands.0, mcp.0) — wired 2026-07-14 after the anomalyco/opencode
// re-onboard resolved the opencode-ai identity divergence and refetched the
// cache from the project the format doc actually tracks.
func recognizeOpencode(ctx RecognitionContext) RecognitionResult {
	skillsCaps := recognizeGoStruct(ctx.Fields, SkillsGoStructOptions())
	if len(skillsCaps) > 0 {
		mergeInto(skillsCaps, capabilityDotPaths("skills", "project_scope", "cross-provider SKILL.md convention at .opencode/skill/<name>/SKILL.md", "confirmed"))
		mergeInto(skillsCaps, capabilityDotPaths("skills", "global_scope", "cross-provider convention at ~/.config/opencode/skills/<name>/SKILL.md", "confirmed"))
		mergeInto(skillsCaps, capabilityDotPaths("skills", "canonical_filename", "SKILL.md (Agent Skills spec)", "confirmed"))
	}
	skillsResult := wrapCapabilities(skillsCaps)

	rulesResult := recognizeLandmarks(ctx, opencodeRulesLandmarkOptions())
	agentsResult := recognizeLandmarks(ctx, opencodeAgentsLandmarkOptions())
	commandsResult := recognizeLandmarks(ctx, opencodeCommandsLandmarkOptions())
	mcpResult := recognizeLandmarks(ctx, opencodeMcpLandmarkOptions())

	return mergeRecognitionResults(skillsResult, rulesResult, agentsResult, commandsResult, mcpResult)
}
