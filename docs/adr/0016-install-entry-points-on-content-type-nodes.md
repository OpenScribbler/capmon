---
id: "0016"
title: "Install Entry Points as ACIF Rows on Content-Type Nodes"
status: accepted
date: 2026-10-24
enforcement: advisory
files: ["capyaml/types.go", "capyaml/install_entry_points.go", "export_provider.go", "export_tree.go", "docs/publish/schemas/provider-capabilities.json", "docs/publish/schemas/by-content-type.json", "docs/provider-capabilities/*.yaml"]
tags: [publishing, schema, acif, install-paths]
---

# ADR 0016: Install Entry Points as ACIF Rows on Content-Type Nodes

## Status

Accepted

## Context

The feed recorded where a provider reads content only as prose: the
`mechanism` strings on capabilities such as `global_scope` and
`project_scope`. A consumer that installs content had to parse that prose,
which the field semantics forbid.

ACIF's install-targets spec defines the structure such a consumer needs.
Each (provider, content type) pair gets an ordered list of rows. A row
carries `scope`, an optional `os` set, a `path_template` in a closed
grammar, a `layout`, a `status`, and an informative `as_of`. Row order is
precedence: after filtering by the target OS, the first current row for a
scope is the write target. ACIF
also exports its own rows for every provider in
`conformance/install-entry-points.yaml`.

## Decision

- Each content-type node carries `install_entry_points`, an ordered list
  of rows with ACIF's field names and value spellings, verbatim. In source
  the list lives at `content_types.<type>.install_entry_points` in
  `docs/provider-capabilities/<slug>.yaml`. The export copies it to the
  same place in `capabilities/<slug>.json`, so `all.json` and
  `by-content-type/<type>.json` carry it unchanged. capmon's content-type
  names map to ACIF's singular names (`skills` to `skill`, `mcp` to
  `mcp_config`, and so on); the row fields need no mapping.
- The content-type node is the place because ACIF's unit is the
  (provider, content type) list. Three alternatives were rejected:
  - A node in the `capabilities` map would need a registered canonical
    key, and a map would lose the row order that encodes precedence.
  - A separate install-paths document would duplicate ACIF's export and
    split one provider's facts across two documents.
  - Joining ACIF's export into the feed at export time would publish
    ACIF's claims as capmon's. capmon publishes what its own evidence
    shows, and ACIF's export is what that evidence gets compared against.
- The rows are capmon's observation. Each row is verified against the
  provider documentation capmon tracks, and its `as_of` names the page and
  the date it was verified. Where that evidence disagrees with ACIF's row,
  the feed publishes the evidence, and the difference goes upstream as an
  ACIF row-data amendment. `status` follows ACIF's precedence model and
  does not encode provider deprecation: a location the provider documents
  as deprecated stays `current` while no row replaces it.
- `capyaml.ValidateInstallEntryPoints` enforces the closed sets for
  `scope`, `layout`, `status` and `os`, the `os` set rules, the closed
  template grammar (no glob or alternation characters, no `.` or `..`
  segments) and its anchors (`<appdata>` and drive-letter templates
  require `os` exactly `[windows]`; a `/`-rooted template requires an
  `os` set without `windows`, because an absent `os` includes it), the
  layout and `<content-name>` agreement, no trailing `/` on the two file
  layouts (`single_file`, `merged_into_shared_file`), and uniqueness of
  (scope, path_template). Decoding a row is strict as well: a key outside
  the six row fields, or an `os` key that is null or empty, is rejected,
  because either would read as "every OS"; a duplicate key or a field of
  the wrong YAML type is rejected the same way. `capmon verify` runs both,
  and the export runs them again and fails closed with `EXPORT_007`. The published schema keeps these fields as open strings.
  Under the consumer contract (ADR 0001) a consumer handles an
  unrecognized enum value as "unknown/other", and ACIF §11 handles an
  unrecognized template token per row. A closed schema would turn either case into a document error.
- The change is additive. publish-layer.md requires a producer to bump
  `schema_version` on any shape change, per schema, so operators can
  detect the revision in the documents they fetch. The provider-capabilities
  schema and documents move from "2" to "3". The by-content-type schema and
  pivots move from "1" to "2": a pivot embeds bare content-type nodes,
  which carry no `schema_version` of their own, so without its own bump a
  pivot consumer would get the new field with no signal. `all-providers.json`
  stays at "1": it embeds each whole provider document, and that document's
  own `schema_version` ("3") already signals the revision. The source YAML
  stays at `schema_version` "1" because the field is optional.
- When the evidence shows a location for a content type the baseline did
  not declare, the content type is added with `supported: true` and its
  rows. This change adds `commands` for claude-code and codex, and `mcp`
  for codex.

## Consequences

- Install tools can resolve a path from the feed by the ACIF rules without
  translation, and they can still read the prose `mechanism` fields, which
  are unchanged.
- The rows must be maintained by hand against provider documentation, like
  the rest of the baselines. The `as_of` string shows how stale a row is.
- Covering other providers is a data change: add rows to their baselines.
- A later grammar or enum change in ACIF reaches capmon's validator as a
  code change, while consumers of the feed keep working because the
  published schema is open.
