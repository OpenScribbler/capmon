# Provider Compatibility Matrix

This document shows which content types each provider supports natively. Whether any downstream consumer implements install/manage support for a given cell is that consumer's concern and is not tracked here.

**Content types:** Rules · Skills · Agents · Commands · MCP · Hooks

**Legend:**
- `✓` — Supported natively
- `~` — Cross-provider convention only (e.g., AGENTS.md); no provider-native format
- `⚙` — Built-in only; not user-definable (no files to install)
- `✗` — Not supported by this provider

Last updated: 2026-09-30. Authoritative sources: `docs/provider-sources/*.yaml` (native support + monitoring sources) and `docs/provider-formats/*.yaml` (per-capability detail).

---

## Matrix

| Provider         | Rules | Skills | Agents | Commands | MCP | Hooks |
|------------------|:-----:|:------:|:------:|:--------:|:---:|:-----:|
| amp              |   ✓   |   ✓    |   ✗    |    ✗     |  ✓  |   ✗   |
| claude-code      |   ✓   |   ✓    |   ✓    |    ✓     |  ✓  |   ✓   |
| cline            |   ✓   |   ✓    |   ✗    |    ✗     |  ✓  |   ✓   |
| codex            |   ✓   |   ✓    |   ✓    |    ✓     |  ✓  |   ✓   |
| copilot-cli      |   ✓   |   ✓    |   ✓    |    ✓     |  ✓  |   ✓   |
| cursor           |   ✓   |   ✓    |   ~    |    ~     |  ✓  |   ✓   |
| devin            |   ✓   |   ✓    |   ✓    |    ✓     |  ✓  |   ✓   |
| factory-droid    |   ✓   |   ✓    |   ✓    |    ✓     |  ✓  |   ✓   |
| gemini-cli       |   ✓   |   ✓    |   ~    |    ✓     |  ✓  |   ✓   |
| kiro             |   ✓   |   ✓    |   ✓    |    ✗     |  ✓  |   ✓   |
| opencode         |   ✓   |   ~    |   ⚙    |    ✓     |  ✓  |   ✗   |
| pi               |   ✓   |   ✓    |   ✗    |    ✓     |  ✗  |   ✓   |
| roo-code         |   ✓   |   ✓    |   ✓    |    ✓     |  ✓  |   ✗   |
| crush            |   ✓   |   ✓    |   ✗    |    ✗     |  ✓  |   ✓   |
| zed              |   ✓   |   ✗    |   ⚙    |    ⚙     |  ✓  |   ✗   |

---

## Provider Notes

### amp
Supports rules (AGENT.md), skills, and MCP (.amp/settings.json). Has an internal "checks" system but no user-facing lifecycle hooks. No agent file format, no custom commands.

### claude-code
All 6 content types supported. Most complete provider — also supports loadouts (unique among tracked providers). Hook events: 26+, including CC-exclusive events (TeammateIdle, TaskCreated, WorktreeCreate/Remove, etc.).

### cline
Supports rules (.clinerules/), hooks (9 events), and MCP. Also reads cross-provider rules files (.cursorrules, .windsurfrules, AGENTS.md). Skills are in the manifest for monitoring. Commands are CLI-only (not user-definable files).

### codex
All 6 content types. Hooks via JSON Schema (5 events: PreToolUse, PostToolUse, SessionStart, UserPromptSubmit, Stop). Agents are user-configurable TOML files. Skills use SKILL.md frontmatter. Config is TOML-based, not JSON.

### copilot-cli
All 6 content types. "Commands" maps to Copilot CLI's plugin system (.agent.md files). Docs use Liquid template variables — need stripping during extraction. Hooks docs are in 3 separate pages.

### cursor
Supports rules (.cursor/rules/*.mdc + legacy .cursorrules), skills, hooks (~23 events in camelCase), MCP. Agents and commands are tracked in manifest via AGENTS.md and .cursor/commands/ cross-provider conventions — no cursor-native format docs available. cursor.com/docs rate-limits automated fetching.

### devin
Devin Desktop, formerly Windsurf (slug `windsurf` in the frozen v1 feed). All 6 content types. Rules (`.devin/rules/`, legacy `.windsurf/rules/` and `.windsurfrules`, plus Cascade memories), skills, MCP, hooks (Devin CLI lifecycle hooks: PascalCase events, command and prompt handlers, Claude Code-style config), agents (custom subagents in `.devin/agents/` and `~/.config/devin/agents/`, experimental), and commands (Cascade Workflows in `.devin/workflows/`; the Devin Local agent does not run Workflows).

### factory-droid
All 6 content types. Hook schema matches Claude Code format exactly. Custom agents are called "Custom Droids" (`.factory/droids/<name>.md`). Tool restrictions use categorical names (filesystem, shell, search, browser, web_fetch) instead of per-tool allowlists. MCP config: `.factory/mcp.json` (project) and `~/.factory/mcp.json` (user).

### gemini-cli
Supports rules (GEMINI.md), skills, commands (TOML files in .gemini/commands/), MCP, hooks. No user-definable agent format (internal subagent only) — the cross-provider convention covers .gemini/agents/. Commands use `{{args}}` placeholders.

### kiro
Supports rules (called "steering", .kiro/steering/), skills (called "powers"), agents, MCP, hooks (10+ events including file system events). No custom commands. Unique events: File Save, File Create, File Delete, Pre/Post Task Execution.

### opencode
Supports rules (via contextPaths config including AGENTS.md), commands (.opencode/commands/), MCP, and 4 built-in agents (coder/summarizer/task/title — not user-definable). Skills have no native format — covered by the cross-provider SKILL.md convention. No hook system.

### pi
Supports rules (AGENTS.md), skills (`.pi/skills/`, `~/.pi/agent/skills/`), hooks (programmatic TypeScript extensions in `.pi/extensions/`), and commands (prompt templates in `.pi/prompts/`). Hooks are TypeScript files rather than declarative JSON — installers emit a generated hooks TypeScript file with marker comments for round-trip. No MCP support (users implement via extensions). No user-definable agent format.

### roo-code
Supports rules (.roo/rules/ with per-mode subdirs like .roo/rules-code/), skills, agents ("Custom Modes" in .roomodes), commands (.roo/commands/), MCP. **No hooks** — deliberately removed from Cline fork.

### crush
Supports rules (AGENTS.md project only), skills (`.crush/skills/`, `~/.config/crush/skills/` — XDG-compliant), MCP (`crush.json` with stdio/http/sse transports), and hooks (single `PreToolUse` event under the `crush.json` `hooks` key; Claude Code-compatible flat `HookConfig` shape). No agents, no commands.

### zed
Supports rules (.rules, plain markdown), MCP. Agent "profiles" (write/ask/minimal) and slash commands are all builtin — not user-definable files. No hooks, no skills.

---

## Content Type Coverage Summary

| Content Type | Total Providers | Native support (✓) | Cross-convention (~) | Built-in only (⚙) | Not supported (✗) |
|--------------|:--------------:|:------------------:|:--------------------:|:-----------------:|:-----------------:|
| Rules        |      15        |        15          |         0            |        0          |         0         |
| Skills       |      15        |        13          |         1            |        0          |         1         |
| Agents       |      15        |         7          |         2            |        2          |         4         |
| Commands     |      15        |         9          |         1            |        1          |         4         |
| MCP          |      15        |        14          |         0            |        0          |         1         |
| Hooks        |      15        |        11          |         0            |        0          |         4         |

**Rules** is the most universally supported content type — every provider has it.  
**Hooks** is the most selective — only 11 of 15 providers support lifecycle hooks.  
**Agents** has the most variation — true user-definable agent files in only 7 providers; 2 more use cross-provider AGENTS.md convention only.
