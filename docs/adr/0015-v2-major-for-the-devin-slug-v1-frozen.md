---
id: "0015"
title: "v2 Major for the Devin Slug; v1 Frozen, Not Forked"
status: accepted
date: 2026-09-30
enforcement: advisory
files: ["export*.go", "site-static/**", "docs/provider-sources/published-slugs.lock", ".github/workflows/publish.yml"]
tags: [publishing, versioning, freeze, renames, slugs]
---

# ADR 0015: v2 Major for the Devin Slug; v1 Frozen, Not Forked

## Status

Accepted

## Context

Cognition renamed Windsurf. Its FAQ says: "On June 2, 2026, Windsurf is
becoming Devin Desktop"
(https://docs.devin.ai/desktop/devin-desktop-faq). The product now reads
`.devin/` paths, and its docs moved to docs.devin.ai. Under ADR 0013, v1
could take this only as a display rename, so v1 kept the slug `windsurf`
with `display_name: Devin Desktop`. ADR 0013 tier 3 allows a slug rename
only at a major bump, and the old URL must keep serving and point at its
successor.

ADR 0001 decides how a major bump works: the exporter generates only the
current major, and the old major is frozen as static files. Generating v1
and v2 side by side from live data is the parallel-exporter design that
ADR 0001 rejected.

## Decision

- The live major is `v2`. The only difference from v1 is the slug
  `windsurf` → `devin`, and v2's schema `$id`s move to `/v2/schemas/`.
  Document shapes, field semantics, and each document's `schema_version`
  stay the same, because `schema_version` counts shape revisions and no
  shape changed.
- v1 is frozen with `capmon freeze v1`. The input was the last live v1
  tree, byte-identical to what Pages was serving. The freeze stores it in
  `site-static/v1/` and applies one mutation: `status: "frozen"`,
  `superseded_by`, and `frozen_at` on `v1/index.json` and on every
  per-provider document. Each document's `superseded_by` names its v2
  successor document, and `windsurf.json` names
  `/v2/capabilities/devin.json`. This satisfies ADR 0013's requirement that
  the old URL keep serving and point at its successor.
- The frozen root hash is recorded in `frozenMajors` and published in the
  root index as `frozen_root_sha256`. Every export re-verifies it and fails
  closed with `EXPORT_006`.
- The publish workflow still attests `v1/index.json` alongside `index.json`
  and `v2/index.json`, so a v1 client's signature check keeps passing.
- `published-slugs.lock` now records the v2 slug set. The frozen v1 set is
  the provider list in `site-static/v1/index.json`.

## Consequences

- Frozen v1 keeps its last live `generated_at` and
  `max_staleness_hours: 48`. A v1 client that enforces staleness starts
  failing 48 hours after that timestamp. The freeze is designed to do this:
  it is how v1 tells clients to move to v2.
- The signed git tag for the frozen root hash (publish-layer design,
  "Major transitions") is deferred. The recorded hash in source, together
  with the fail-closed export check, carries tamper evidence until the tag
  exists.
- Later renames wait for v3 and use the same command.
