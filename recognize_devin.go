package capmon

func init() {
	RegisterRecognizer("devin", RecognizerKindDoc, recognizeDevin)
}

// devinRulesLandmarkOptions returns the landmark patterns for Devin
// Desktop's rules documentation. Anchors derived from
// .capmon-cache/devin/rules.0/extracted.json (Memories & Rules doc) and
// .capmon-cache/devin/rules.1/extracted.json (AGENTS.md doc).
//
// Required anchors "Rules Discovery" and "Rules Storage Locations" are unique
// to the rules.0 doc — they prevent rules patterns from firing on sources that
// only mention rules in passing (e.g. the AGENTS.md doc alone, or any other
// content type's landmarks merged into the recognition context).
//
// Activation modes: the rules.0 doc has a single "Activation Modes" heading
// followed by a table whose rows name the four trigger values
// (always_on, manual, model_decision, glob). Table cells are not extracted as
// landmarks, so all four sub-keys gate on the same parent heading. This is the
// strongest available signal that devin supports the full activation_mode
// vocabulary — the seeder spec singles out this provider as the most explicit
// among all 14 providers.
func devinRulesLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Rules Discovery", CaseInsensitive: true},
		{Kind: "substring", Value: "Rules Storage Locations", CaseInsensitive: true},
	}
	return RulesLandmarkOptions(
		RulesLandmarkPattern("activation_mode.always", "Activation Modes",
			"always_on trigger value documented in 'Activation Modes' table — full rule content included in system prompt every message", required),
		RulesLandmarkPattern("activation_mode.manual", "Activation Modes",
			"manual trigger value documented in 'Activation Modes' table — activated via @-mention", required),
		RulesLandmarkPattern("activation_mode.model_decision", "Activation Modes",
			"model_decision trigger value documented in 'Activation Modes' table — Cascade decides based on context", required),
		RulesLandmarkPattern("activation_mode.glob", "Activation Modes",
			"glob trigger value documented in 'Activation Modes' table — activated when files matching glob pattern are in context", required),
		RulesLandmarkPattern("cross_provider_recognition.agents_md", "AGENTS.md",
			"AGENTS.md fallback documented in dedicated rules.1 doc with 'Comparison with Rules' section", required),
		RulesLandmarkPattern("auto_memory", "How to Manage Memories",
			"Cascade-managed Memories layer auto-writes context based on conversation (documented under 'Memories & Rules' / 'How to Manage Memories')", required),
		RulesLandmarkPattern("hierarchical_loading", "Rules Storage Locations",
			".devin/rules/ (preferred; .windsurf/rules/ legacy fallback) scanned in current workspace, sub-directories, and parent dirs up to git root (documented under 'Rules Discovery' / 'Rules Storage Locations')", required),
	)
}

// devinHooksLandmarkOptions returns the landmark patterns for the Devin
// lifecycle hooks docs that Devin Local runs on. Anchors derived from
// .capmon-cache/devin/hooks.0/extracted.json (cli/extensibility/hooks/overview.md)
// and .capmon-cache/devin/hooks.1/extracted.json
// (cli/extensibility/hooks/lifecycle-hooks.md).
//
// Required anchors are unique to the hooks overview:
//   - "Where Hooks Live" — H2 naming the project and user config files
//   - "Hook Events" — H2 listing the eight PascalCase lifecycle events
//
// async_execution is not emitted.
// Verified live: https://docs.devin.ai/cli/extensibility/hooks/overview.md on 2026-09-30.
// The page and lifecycle-hooks.md document command/prompt handlers, matchers,
// decisions, and context injection but no async or background handler option.
// Format YAML status: hooks.async_execution unsupported (docs/provider-formats/devin.yaml).
//
// The legacy Cascade hooks doc (desktop/cascade/hooks.md, snake_case events)
// is no longer a tracked source; it applies only to the Cascade agent.
func devinHooksLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Where Hooks Live", CaseInsensitive: true},
		{Kind: "substring", Value: "Hook Events", CaseInsensitive: true},
	}
	return HooksLandmarkOptions(
		HooksLandmarkPattern("handler_types", "Hook Format",
			"each hook has a type of \"command\" (shell command) or \"prompt\" (LLM prompt evaluation), documented under 'Hook Format'", required),
		HooksLandmarkPattern("matcher_patterns", "Using the Matcher",
			"matcher is a regex against the event's tool_name; empty or omitted matches all tools (documented under 'Using the Matcher' / 'Tool names you can match')", required),
		HooksLandmarkPattern("decision_control", "Output format",
			"top-level decision \"approve\" or \"block\" with optional reason; exit code 2 blocks (documented under 'Output format' / 'Exit Codes')", required),
		HooksLandmarkPattern("input_modification", "PreToolUse",
			"PreToolUse hooks rewrite tool input via hookSpecificOutput.updatedInput, merged into the tool's arguments before execution", required),
		HooksLandmarkPattern("hook_scopes", "Where Hooks Live",
			"project (.devin/hooks.v1.json, .devin/config.json, .devin/config.local.json) and user (~/.config/devin/config.json) scopes documented under 'Project-Level' / 'User-Level (Global)'", required),
		HooksLandmarkPattern("json_io_protocol", "Command Hooks",
			"event data passed as JSON on stdin; JSON output on stdout, documented under 'Command Hooks'", required),
		HooksLandmarkPattern("context_injection", "UserPromptSubmit",
			"hookSpecificOutput.additionalContext injects text into the agent's context (UserPromptSubmit and SessionStart examples)", required),
		HooksLandmarkPattern("permission_control", "PermissionRequest",
			"PermissionRequest event fires when a permission decision is needed and can approve or block it", required),
	)
}

// devinMcpLandmarkOptions returns the landmark patterns for Devin Desktop's
// MCP documentation. Anchors derived from
// .capmon-cache/devin/mcp.0/extracted.json (cascade/mcp.md).
//
// The MCP doc is the richest of the 14 providers — 19 landmarks
// covering transport, interpolation, marketplace, enterprise admin controls,
// and registry customization. 5 of 8 canonical MCP keys map to heading-level
// evidence: transport_types, env_var_expansion, tool_filtering, marketplace,
// enterprise_management.
//
// Three keys are not emitted as landmarks here:
//   - oauth_support: NOT emitted by the recognizer, but NOT because devin
//     lacks OAuth. The MCP doc DOES document it — "Devin Desktop also supports
//     OAuth for each transport type" — and docs/provider-formats/devin.yaml
//     curates oauth_support: supported: true / confidence: confirmed accordingly.
//     It is unemitted here only because that single prose sentence carries no
//     dedicated OAuth heading for a substring landmark to anchor on; the curated
//     format-YAML value is authoritative.
//     Verified live: https://docs.devin.ai/desktop/cascade/mcp.md on 2026-07-14.
//     The page documents OAuth for each transport type (stdio, Streamable HTTP,
//     SSE) in prose, but with no OAuth heading to gate a landmark emission.
//   - auto_approve: no auto-approve heading; curator confirms devin does
//     not support pre-configured auto-approval.
//   - resource_referencing: no @-mention or resources heading; curator
//     confirms devin does not document MCP resource access.
//
// Required anchors are unique to the MCP doc:
//   - "Cascade MCP Configuration" — H1, MCP-specific
//   - "Adding a new MCP" — H2, MCP-specific
//
// Neither appears in devin's skills, rules, hooks, agents, or commands
// docs.
//
// Per docs/provider-formats/devin.yaml, the curator marks 4 keys as
// supported confirmed (transport_types, env_var_expansion, marketplace,
// enterprise_management) and tool_filtering as unsupported. The recognizer
// additionally emits tool_filtering as "inferred" because heading-level
// evidence exists ("Configuring MCP tools" and "MCP Allowlist") — the
// curator interprets these headings as the 100-tool cap and admin-side
// regex matching rather than per-server tool allowlist. The two YAML files
// are independent: provider-capabilities/ tracks recognizer emissions,
// provider-formats/ tracks curator judgments.
func devinMcpLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Cascade MCP Configuration", CaseInsensitive: true},
		{Kind: "substring", Value: "Adding a new MCP", CaseInsensitive: true},
	}
	return McpLandmarkOptions(
		McpLandmarkPattern("transport_types", "Remote HTTP MCPs",
			"transport types (stdio for local, Streamable HTTP and SSE for remote) documented under 'Remote HTTP MCPs' heading; remote servers use serverUrl/url instead of command/args", required),
		McpLandmarkPattern("env_var_expansion", "Config Interpolation",
			"${env:VAR_NAME} and ${file:/path} interpolation documented under 'Config Interpolation' heading", required),
		McpLandmarkPattern("tool_filtering", "MCP Allowlist",
			"per-server tool filtering documented under 'Configuring MCP tools' (UI tool toggles, 100-tool cap) and 'MCP Allowlist' (admin-side regex matching) headings", required),
		McpLandmarkPattern("marketplace", "MCP Registry",
			"in-IDE MCP marketplace documented under 'MCP Registry' / 'Configuring Custom Registries' headings", required),
		McpLandmarkPattern("enterprise_management", "Admin Controls (Teams & Enterprises)",
			"organization-level MCP admin controls (allowlist, custom registries, regex matching) documented under 'Admin Controls (Teams & Enterprises)' / 'Admin Configuration Guidelines' headings", required),
	)
}

// devinAgentsLandmarkOptions returns the landmark patterns for Devin's
// custom subagents (experimental). Anchors derived from
// .capmon-cache/devin/agents.0/extracted.json (cli/subagents.md).
//
// Required anchors are unique to the subagents doc:
//   - "Creating a Custom Subagent" — H3 for user-defined subagent profiles
//   - "Nesting Depth" — H2, subagent-specific
//
// per_agent_mcp is not emitted.
// Verified live: https://docs.devin.ai/cli/subagents.md on 2026-09-30.
// The frontmatter fields (name, description, model, allowed-tools,
// max-nesting) carry no per-agent MCP server configuration.
// Format YAML status: agents.per_agent_mcp unsupported (docs/provider-formats/devin.yaml).
func devinAgentsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Creating a Custom Subagent", CaseInsensitive: true},
		{Kind: "substring", Value: "Nesting Depth", CaseInsensitive: true},
	}
	return AgentsLandmarkOptions(
		AgentsLandmarkPattern("definition_format", "Definition File Format",
			"Markdown file with YAML frontmatter (same frontmatter as skills) followed by the system prompt, as agents/<name>.md or agents/<name>/AGENT.md", required),
		AgentsLandmarkPattern("tool_restrictions", "Frontmatter Fields",
			"allowed-tools frontmatter list (alias tools) restricts which tools the subagent can use", required),
		AgentsLandmarkPattern("invocation_patterns.automatic_delegation", "How Custom Subagents Are Used",
			"the agent sees each profile's description and chooses the most appropriate one when spawning a subagent", required),
		AgentsLandmarkPattern("invocation_patterns.natural_language", "How Custom Subagents Are Used",
			"the user can ask the agent to use a specific profile by name", required),
		AgentsLandmarkPattern("invocation_patterns.background", "Foreground / Background Switching",
			"subagents run in the foreground or background and can be switched while running", required),
		AgentsLandmarkPattern("agent_scopes.project", "Creating a Custom Subagent",
			"project scope at .devin/agents/ and .agents/agents/", required),
		AgentsLandmarkPattern("agent_scopes.user", "Creating a Custom Subagent",
			"global scope at ~/.config/devin/agents/ (Windows: %APPDATA%\\devin\\agents\\)", required),
		AgentsLandmarkPattern("model_selection", "Which Model Does a Subagent Use?",
			"model frontmatter field overrides the subagent's model (default is the router-selected subagent model, not the parent's)", required),
		AgentsLandmarkPattern("subagent_spawning", "Nesting Depth",
			"max-nesting frontmatter field lets a custom subagent spawn its own subagents; nesting is disabled by default", required),
	)
}

// devinCommandsLandmarkOptions returns the landmark patterns for Devin
// Desktop's Workflows documentation. Anchors derived from
// .capmon-cache/devin/commands.0/extracted.json (cascade/workflows.md).
//
// User-authored slash commands are called "Workflows" — markdown files
// under .devin/workflows/<name>.md (legacy .windsurf/workflows/) invoked via
// the /<name>
// shortcut. The doc has 10 workflow-specific landmarks: "Cascade workflows",
// "How it works", "How to create a Workflow", "Workflow Discovery",
// "Workflow Storage Locations", "Generate a Workflow with Cascade",
// "Example Workflows", "System-Level Workflows (Enterprise)",
// "Workflow Precedence", and the "Documentation Index" navigational header.
//
// Per docs/provider-formats/devin.yaml, neither canonical commands key
// has heading-level evidence in this doc:
//   - argument_substitution: curator marks supported=false (no {{args}} or
//     $ARGUMENTS placeholder syntax documented in the workflow body —
//     workflows execute as-is).
//   - builtin_commands: curator marks supported=true confirmed, but the
//     evidence lives in a separate Cascade UI doc (not in this cache
//     source). The /help, /search, /memory commands are documented on
//     a different docs page.
//
// Therefore this recognizer emits ONLY commands.supported=true via an
// empty-Capability pattern — it confirms devin has a user-authored
// commands surface without claiming any per-key capability beyond what
// the curator has manually documented. The curator's per-key flags in
// the format YAML stay authoritative; the recognizer adds no per-key
// signal here.
//
// Required anchors are unique to the workflows doc:
//   - "How to create a Workflow" — H2, workflow-specific
//   - "Workflow Storage Locations" — H2, workflow-specific
//
// Verified absent from devin's rules.{0,1}, skills.0, hooks.{0,1},
// agents.0, and mcp.0 caches — neither phrase appears in any other
// devin doc, so cross-content-type landmark merging cannot trigger
// a false positive.
func devinCommandsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "How to create a Workflow", CaseInsensitive: true},
		{Kind: "substring", Value: "Workflow Storage Locations", CaseInsensitive: true},
	}
	return CommandsLandmarkOptions(LandmarkPattern{
		Capability: "",
		Required:   required,
		Matchers:   []StringMatcher{{Kind: "substring", Value: "Workflows", CaseInsensitive: true}},
	})
}

// devinSkillsLandmarkOptions returns the landmark options for the skills
// doc (https://docs.devin.ai/desktop/cascade/skills.md). The page documents
// every canonical skills key under its own heading. Required anchors are
// "SKILL.md File Format" and "Skill Scopes", which appear in no other devin
// source doc.
func devinSkillsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "SKILL.md File Format", CaseInsensitive: true},
		{Kind: "substring", Value: "Skill Scopes", CaseInsensitive: true},
	}
	return SkillsLandmarkOptions(
		SkillsLandmarkPattern("canonical_filename", "SKILL.md File Format",
			"SKILL.md is the fixed filename in each skill directory (documented under 'SKILL.md File Format')", required),
		SkillsLandmarkPattern("display_name", "Required Frontmatter Fields",
			"name frontmatter field (documented under 'Required Frontmatter Fields')", required),
		SkillsLandmarkPattern("description", "Required Frontmatter Fields",
			"description frontmatter field used for automatic invocation (documented under 'Required Frontmatter Fields')", required),
		SkillsLandmarkPattern("skill_bundled_resources", "Adding Supporting Resources",
			"supporting files alongside SKILL.md in the skill directory (documented under 'Adding Supporting Resources')", required),
		SkillsLandmarkPattern("auto_invocable", "Automatic Invocation",
			"Cascade invokes a skill when the request matches its description (documented under 'Automatic Invocation')", required),
		SkillsLandmarkPattern("user_invocable", "Manual Invocation",
			"@-mention a skill by name (documented under 'Manual Invocation')", required),
		SkillsLandmarkPattern("project_scope", "Skill Scopes",
			"workspace skills under .devin/skills/<skill-name>/ (documented under 'Skill Scopes')", required),
		SkillsLandmarkPattern("global_scope", "Skill Scopes",
			"global skills under ~/.config/devin/skills/<skill-name>/ (documented under 'Skill Scopes')", required),
		SkillsLandmarkPattern("shared_scope", "System-Level Skills (Enterprise)",
			"OS-level system skills deployed by enterprise IT (documented under 'System-Level Skills (Enterprise)')", required),
	)
}

// recognizeDevin recognizes skills + rules + hooks + mcp + agents + commands
// capabilities for the Devin Desktop provider (formerly Windsurf). All six
// are landmark-based against skills.0 (cascade/skills.md), rules.{0,1}
// (Memories & Rules, AGENTS.md), hooks.{0,1} (Devin lifecycle hooks), mcp.0
// (cascade/mcp.md), agents.0 (cli/subagents.md), and commands.0
// (cascade/workflows.md).
func recognizeDevin(ctx RecognitionContext) RecognitionResult {
	skillsResult := recognizeLandmarks(ctx, devinSkillsLandmarkOptions())
	rulesResult := recognizeLandmarks(ctx, devinRulesLandmarkOptions())
	hooksResult := recognizeLandmarks(ctx, devinHooksLandmarkOptions())
	mcpResult := recognizeLandmarks(ctx, devinMcpLandmarkOptions())
	agentsResult := recognizeLandmarks(ctx, devinAgentsLandmarkOptions())
	commandsResult := recognizeLandmarks(ctx, devinCommandsLandmarkOptions())

	return mergeRecognitionResults(skillsResult, rulesResult, hooksResult, mcpResult, agentsResult, commandsResult)
}
