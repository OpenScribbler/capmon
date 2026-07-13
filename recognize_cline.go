package capmon

func init() {
	RegisterRecognizer("cline", RecognizerKindDoc, recognizeCline)
}

// clineLandmarkOptions returns the landmark patterns for Cline's skills doc.
// Anchors derived from .capmon-cache/cline/skills.0/extracted.json. The two
// required anchors guard against false positives from other cline content
// docs (rules, hooks, mcp, commands) — those docs do not contain these
// skills-specific phrases, so the recognizer suppresses cleanly when only
// non-skills sources are present.
//
// Capability names are intentionally distinct from amp/claude-code where the
// underlying feature differs. Cline's skills doc emphasizes the bundled-files
// concept (docs/, templates/, scripts/) and a per-skill enable/disable toggle
// — both are surfaced as named capabilities here.
func clineLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "How Skills Work", CaseInsensitive: true},
		{Kind: "substring", Value: "Where Skills Live", CaseInsensitive: true},
	}
	pattern := func(cap, anchor, mechanism string) LandmarkPattern {
		return LandmarkPattern{
			Capability: cap,
			Required:   required,
			Matchers:   []StringMatcher{{Kind: "substring", Value: anchor, CaseInsensitive: true}},
			Mechanism:  mechanism,
		}
	}
	return LandmarkOptions{
		ContentType: "skills",
		Patterns: []LandmarkPattern{
			pattern("directory_structure", "Skill Structure", "documented under 'Skill Structure' heading"),
			pattern("creation_workflow", "Creating a Skill", "documented under 'Creating a Skill' heading"),
			pattern("toggling", "Toggling Skills", "per-skill enable/disable documented under 'Toggling Skills' heading"),
			pattern("frontmatter", "Writing Your SKILL.md", "frontmatter format documented under 'Writing Your SKILL.md' heading"),
			pattern("naming_conventions", "Naming Conventions", "documented under 'Naming Conventions' heading"),
			pattern("description_guidance", "Writing Effective Descriptions", "documented under 'Writing Effective Descriptions' heading"),
			pattern("bundled_files", "Bundling Supporting Files", "documented under 'Bundling Supporting Files' heading"),
			pattern("file_references", "Referencing Bundled Files", "documented under 'Referencing Bundled Files' heading"),
		},
	}
}

// clineRulesLandmarkOptions returns the landmark patterns for Cline's rules
// documentation. Anchors derived from .capmon-cache/cline/rules.0/extracted.json
// (cline-rules.md).
//
// Required anchors are unique to the rules doc — skills.0 uses "Where Skills
// Live" (different word), so substring matching does not collide:
//   - "Where Rules Live"
//   - "Conditional Rules"
//
// Per the seeder spec, cline supports a smaller activation_mode vocabulary
// than cursor/kiro/windsurf — only always_on (no conditional) and
// frontmatter_globs (paths conditional). file_imports,
// cross_provider_recognition, and auto_memory are intentionally absent.
func clineRulesLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Where Rules Live", CaseInsensitive: true},
		{Kind: "substring", Value: "Conditional Rules", CaseInsensitive: true},
	}
	return RulesLandmarkOptions(
		RulesLandmarkPattern("activation_mode.always", "Conditional Rules",
			"rules without conditionals load for every request (documented under 'Conditional Rules' / 'How It Works')", required),
		RulesLandmarkPattern("activation_mode.glob", "The paths Conditional",
			"'paths' Conditional uses glob-based path matching to scope rule activation (documented under 'The paths Conditional' / 'Writing Conditional Rules')", required),
		RulesLandmarkPattern("hierarchical_loading", "Global Rules Directory",
			"two-tier scope: Project rules (.clinerules/ in workspace) + Global rules (~/.cline/rules/ user-wide) — documented under 'Where Rules Live' / 'Global Rules Directory'", required),
	)
}

// clineHooksLandmarkOptions returns the landmark patterns for Cline's hooks
// documentation.
//
// Verified live: https://docs.cline.bot/customization/hooks (and .../hooks.md)
// on 2026-07-13. The dedicated hooks page is now a two-line stub ("See details
// under SDK Hooks page" / "See details under SDK Plugins"); the refreshed cache
// (.capmon-cache/cline/hooks.0) contains only the "Hooks" heading. The hook
// documentation moved into the Cline SDK plugin system
// (docs.cline.bot/customization/plugins, /sdk/plugins, /sdk/guides/writing-plugins),
// which are NOT tracked sources in cline's format doc. The former filesystem
// hook model (named scripts, JSON via stdin/stdout, "Hook Types" / "Hook
// Lifecycle" / "Hook Locations" / "Input Structure" / "Context Modification"
// headings) no longer exists on this page.
//
// The required anchors below ("Hook Lifecycle", "Hook Locations") therefore no
// longer match the live stub, so this fragment SUPPRESSES cleanly — the honest
// state, since the tracked hooks source no longer documents any hook capability
// at the heading level. Re-curation to the SDK plugin model (and re-pointing the
// hooks source to the plugin docs) is deferred to the maintainer; see the hooks
// notes block in docs/provider-formats/cline.yaml. Format YAML hooks status:
// supported (curator), pending that re-point.
func clineHooksLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Hook Lifecycle", CaseInsensitive: true},
		{Kind: "substring", Value: "Hook Locations", CaseInsensitive: true},
	}
	return HooksLandmarkOptions(
		HooksLandmarkPattern("handler_types", "Hook Types",
			"hook handler types documented under 'Hook Types' heading", required),
		HooksLandmarkPattern("hook_scopes", "Hook Locations",
			"hook scopes documented under 'Hook Locations' heading (project + global)", required),
		HooksLandmarkPattern("json_io_protocol", "Input Structure",
			"JSON I/O protocol documented under 'Input Structure' / 'Output Structure' headings", required),
		HooksLandmarkPattern("context_injection", "Context Modification",
			"context injection documented under 'Context Modification' heading", required),
	)
}

// clineMcpLandmarkOptions returns the landmark patterns for Cline's MCP
// documentation.
//
// Re-anchored 2026-07-13 (issue #5): the two prior MCP pages
// (configuring-mcp-servers, mcp-transport-mechanisms) now 301-redirect to a
// single consolidated page, docs.cline.bot/mcp/mcp-overview. The old H1s
// "Adding & Configuring Servers" and "MCP Transport Mechanisms" and the
// "Finding MCP Servers" marketplace section no longer exist. The refreshed
// cache (.capmon-cache/cline/mcp.0, mcp.1) contains the overview headings:
// "What MCP gives you", "Quick start", "Add servers", "CLI MCP wizard",
// "Configuration examples", "Transport types", "Managing servers",
// "Security basics".
//
// Only 1 canonical MCP key now has heading-level evidence:
//   - transport_types → "Transport types" — STDIO vs remote HTTP/SSE, with the
//     streamableHttp/sse split shown in the "Configuration examples" JSON.
//
// marketplace was DROPPED: the consolidated docs (verified live 2026-07-13,
// incl. the docs.cline.bot/llms.txt sitemap) no longer document an in-IDE MCP
// Marketplace; the curated format YAML now records marketplace: supported=false.
// tool_filtering / auto_approve remain curated as supported (the per-server
// `autoApprove` JSON field, renamed from `alwaysAllow`) but live only in body
// text / config examples, not headings, so the recognizer stays silent on them.
//
// Required anchors are unique to the MCP overview doc and appear in no other
// cline content-type doc:
//   - "What MCP gives you"
//   - "Transport types"
func clineMcpLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "What MCP gives you", CaseInsensitive: true},
		{Kind: "substring", Value: "Transport types", CaseInsensitive: true},
	}
	return McpLandmarkOptions(
		McpLandmarkPattern("transport_types", "Transport types",
			"transport types documented under 'Transport types' heading (STDIO + remote streamableHttp/sse), with the transport split shown in 'Configuration examples'", required),
	)
}

// clineCommandsLandmarkOptions returns the landmark patterns for Cline's
// slash-commands documentation. Anchors derived from
// .capmon-cache/cline/commands.0/extracted.json. Six built-in slash commands
// are exposed as individual landmarks (/newtask, /smol, /newrule, etc.) — the
// strongest possible evidence for builtin_commands at the heading layer.
//
// Required anchors are unique to commands.0:
//   - "Slash Commands" — the H1 page heading; appears in no other cline cache.
//   - "/newtask" — the first specific built-in command landmark; would never
//     appear in skills/rules/hooks/mcp content-type docs.
//
// argument_substitution is intentionally NOT mapped — per the curated YAML
// (docs/provider-formats/cline.yaml), cline's custom workflows are plain
// prompt templates with no documented argument substitution mechanism. The
// commands.0 cache confirms this: no $ARGUMENTS, $1, {{args}}, or similar
// syntax appears in landmarks or fields.
func clineCommandsLandmarkOptions() LandmarkOptions {
	required := []StringMatcher{
		{Kind: "substring", Value: "Slash Commands", CaseInsensitive: true},
		{Kind: "substring", Value: "/newtask", CaseInsensitive: true},
	}
	return CommandsLandmarkOptions(
		CommandsLandmarkPattern("builtin_commands", "Slash Commands",
			"5 built-in slash commands documented as individual headings (/newtask, /smol, /newrule, /deep-planning, /reportbug); hardcoded, not user-modifiable", required),
	)
}

// recognizeCline recognizes skills + rules + hooks + mcp + commands
// capabilities for the Cline provider. Source for all five content types is
// markdown; recognition uses landmark (heading) matching. Static facts merge
// in at "confirmed" confidence after a successful skills landmark match.
func recognizeCline(ctx RecognitionContext) RecognitionResult {
	skillsResult := recognizeLandmarks(ctx, clineLandmarkOptions())
	if len(skillsResult.Capabilities) > 0 {
		mergeInto(skillsResult.Capabilities, capabilityDotPaths("skills", "project_scope", "Skills stored in .cline/skills/<name>/SKILL.md (recommended), .clinerules/skills/<name>/SKILL.md, or .claude/skills/<name>/SKILL.md", "confirmed"))
		mergeInto(skillsResult.Capabilities, capabilityDotPaths("skills", "global_scope", "Skills stored in ~/.cline/skills/<name>/SKILL.md", "confirmed"))
		mergeInto(skillsResult.Capabilities, capabilityDotPaths("skills", "canonical_filename", "SKILL.md", "confirmed"))
	}

	rulesResult := recognizeLandmarks(ctx, clineRulesLandmarkOptions())
	hooksResult := recognizeLandmarks(ctx, clineHooksLandmarkOptions())
	mcpResult := recognizeLandmarks(ctx, clineMcpLandmarkOptions())
	commandsResult := recognizeLandmarks(ctx, clineCommandsLandmarkOptions())

	return mergeRecognitionResults(skillsResult, rulesResult, hooksResult, mcpResult, commandsResult)
}
