package capmon

func init() {
	RegisterRecognizer("crush", RecognizerKindGoStruct, recognizeCrush)
}

// Hooks recognition is intentionally NOT landmark-wired for crush. Crush
// shipped PreToolUse hooks with official docs (docs/hooks/README.md), and the
// hooks capabilities are curated at "confirmed" confidence directly from that
// doc in docs/provider-formats/crush.yaml and mirrored into the master
// docs/provider-capabilities/crush.yaml (7 of 9 canonical keys supported;
// handler_types command-only and async_execution blocking are the two false).
// A landmark recognizer would emit those keys at "inferred" — strictly weaker
// than the confirmed curation already published — so it is deliberately not
// added. If crush later ships additional hook events or the curation must be
// machine-cross-checked, wire landmark recognition against the cached
// hooks.0 (docs/hooks/README.md) then.
//
// recognizeCrush recognizes skills capabilities for the Crush provider.
// Crush implements the Agent Skills open standard (GoStruct pattern).
//
// Rules recognition is intentionally NOT implemented for crush. The cached
// rules source (rules.0) is crush's OWN AGENTS.md instance file (their
// internal Rust dev guide with landmarks like "Build/Test/Lint Commands",
// "Code Style Guidelines", "Working on the TUI"). These are example content,
// not capability vocabulary — using them as recognition evidence would be the
// same instance-vs-spec mismatch pattern the codex multi-struct allow-list
// fix addressed. No external AGENTS.md format-spec doc is cached for crush.
//
// The seeder spec at .develop/seeder-specs/crush-rules.yaml flagged this for
// reviewer choice: (1) recognize on presence, (2) cross-reference amp/codex
// specs, or (3) leave unrecognized. We chose (3) — evidence-based extraction
// must extract from vocabulary, not from examples. Crush's rules support is
// real but undocumented separately, so its rules.* dot-paths remain
// "not_evaluated" until either a format-spec doc is added or the policy
// changes.
//
// MCP recognition is intentionally NOT implemented for crush. The cached
// MCP evidence (.capmon-cache/crush/mcp.0 + mcp.1) splits across two
// incompatible shapes:
//
//   - mcp.0: JSON Schema with field-level paths like
//     "$defs.MCPConfig.properties.disabled_tools" and ".type", ".url",
//     ".command", ".args", ".env", ".headers", ".timeout". The
//     recognizeGoStruct field extractor reads "Type.field" prefixes (e.g.
//     "SkillMetadata.") — a different shape from JSON Schema's nested
//     "$defs.X.properties.Y" paths that GoStructOptions cannot match.
//   - mcp.1: README-style markdown with one inline mention
//     ("Extensible: add capabilities via MCPs (http, stdio, and sse)").
//     A single sentence in a feature bullet is insufficient anchor
//     evidence to disambiguate canonical MCP keys via landmark matching.
//
// Same scope constraint as codex MCP — wiring crush MCP recognition would
// require a JSON-Schema field extractor analogous to GoStructOptions but
// reading "$defs.X.properties.Y" paths. Out of scope for Phase 6 Epic 4.
// Crush's MCP capabilities therefore remain "not_evaluated" in
// docs/provider-capabilities/crush.yaml until either the JSON-Schema
// field extractor exists or a curated docs/provider-formats/crush.yaml
// supplies values.
func recognizeCrush(ctx RecognitionContext) RecognitionResult {
	result := recognizeGoStruct(ctx.Fields, SkillsGoStructOptions())
	if len(result) == 0 {
		return wrapCapabilities(result)
	}
	// Scope: crush supports project-local and global skill directories
	mergeInto(result, capabilityDotPaths("skills", "project_scope", "per-project .crush/skills/ directory", "confirmed"))
	mergeInto(result, capabilityDotPaths("skills", "global_scope", "user-global ~/.crush/skills/ directory", "confirmed"))
	// Filename: crush uses the canonical SKILL.md filename
	mergeInto(result, capabilityDotPaths("skills", "canonical_filename", "SKILL.md", "confirmed"))
	return wrapCapabilities(result)
}
