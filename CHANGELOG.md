# Changelog

Changes to the published feed at https://openscribbler.github.io/capmon/.

## 2026-09-30 — v2

### Breaking

- The live major is now `/v2/` (`https://openscribbler.github.io/capmon/v2/index.json`).
  The root `index.json` reports `latest: "v2"`.
- Provider slug `windsurf` is now `devin` (Devin Desktop), so its document
  moved to `v2/capabilities/devin.json`
  ([Devin Desktop FAQ](https://docs.devin.ai/desktop/devin-desktop-faq)).
- Schema `$id`s moved to `https://openscribbler.github.io/capmon/v2/schemas/`.
  Document shapes and each document's `schema_version` did not change.

### Frozen

- `/v1/` is frozen in its last live state (ADR 0015). Every v1 URL keeps
  serving, and every v1 document now carries `status: "frozen"`,
  `superseded_by`, and `frozen_at`. v1 data and `generated_at` no longer
  change, so a v1 client's 48-hour staleness check fails after 48 hours.
- The root index entry for v1 carries `frozen_root_sha256`. The publish
  workflow still attests `v1/index.json`, using the same workflow identity
  as before.

### Data

- Devin Desktop has been recognized again against docs.devin.ai. Agents are
  now supported: custom subagents, with per-agent MCP absent. Hooks follow
  the Devin hook system: 8 canonical keys, with async execution absent.
  Skills are now published with 9 capabilities; the v1 `windsurf` document
  had no skills entry. The MCP, rules, skills, and workflow paths now use
  `.devin/`.
