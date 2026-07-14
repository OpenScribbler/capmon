---
id: "0013"
title: "Provider Renames: Display-Level Within a Major"
status: accepted
date: 2026-07-14
enforcement: advisory
files: ["docs/provider-sources/*"]
tags: [renames, slugs, publish-contract]
---

# ADR 0013: Provider Renames: Display-Level Within a Major

## Status

Accepted

## Context

Providers rebrand while their capmon slug is already published. The driving case
is Windsurf → "Devin Desktop" (Cognition, 2026-07): all six source URLs 301'd
across domains and the product name changed, but `slug: windsurf` is live at
`v1/capabilities/windsurf.json`. Prior art: Roo Code → Roomote (frozen), and the
opencode name collision (the published `opencode` slug now means the active
sst/opencode, not the archived opencode-ai/opencode it was onboarded from).

Constraints already in force:

- The publish contract (ADR-0001, ADR-0003) versions shapes and paths, never
  data: one stable URL per provider, slugs permanent within a major, a published
  URL must never 404. Renaming a slug is a breaking change by definition.
- The slug is a cross-repo identifier, not just a filename. syllago's coverage
  assertion 5 looks up `capabilities/<slug>.json` by its Go provider slug — a
  feed-side rename makes that provider **silently lose coverage** (a missing doc
  is "no check", not a failure). syllago's provider registry, install paths, and
  lexicon key on the same slug. Tracked syllago-side as
  [OpenScribbler/syllago#504](https://github.com/OpenScribbler/syllago/issues/504).
- Since PRs #22 and #26, `provider_status` and `display_name` publish from the
  source manifests: display identity is *data*, and value-level changes ship
  with no schema or contract implications.
- The export gate's EXPORT_003 asserts the exported provider set exactly matches
  the source-manifest set, but it has no memory: a slug rename mutates both
  sides together and passes. Nothing machine-enforces the contract's
  slug-permanence promise.

Alternatives considered: publishing an alias doc under the new slug within v1
(rejected: doubles maintenance, and syllago would still silently read the stale
old doc); commit-time strict enforcement via the ADR gate hook (rejected: the
hook matches file paths, not diffs, so it would block the first commit of every
routine curation session — ritual re-runs would erode it precisely because
manifests are the most-edited file class in the repo); a live fetch of the
published index inside the gate (rejected: a network dependency in a fail-closed
gate either blocks the daily publish heartbeat on transient errors or fails
open — the committed lockfile keeps the gate offline and deterministic, matching
ADR-0007).

## Decision

Renames are handled in three tiers:

1. **Display rename — the only rename permitted within a major.** The slug is
   unchanged. Update `display_name`, `vendor`, source URLs, and prose in the
   capability masters; record the former name in a manifest header comment.
   All value-level changes: no schema bump, ships immediately through the
   normal pipeline.

2. **Lineage metadata — sanctioned, not built.** If a consumer ever needs
   old-name → slug matching, the mechanism is an additive `former_names: []`
   array on the published provider doc, with the provider-capabilities
   `schema_version` bump that additive shape changes require. It is not
   implemented until a consumer needs it.

3. **Slug rename — only at a major-version bump**, gated on a coordinated
   syllago migration plan (syllago#504) and executed with old-URL continuity
   per the contract (the vacated slug's URL keeps serving, pointing at its
   successor). "The rebrand name is truer than the slug" is not sufficient
   cause: `windsurf` stays despite Devin Desktop, and `opencode` stays
   sst/opencode's slug despite the name collision.

Enforcement:

- The ADR gate hook stays **advisory** on `docs/provider-sources/*`.
- The real enforcement is fail-closed at the layer where the harm occurs:
  `docs/provider-sources/published-slugs.lock` is the committed record of every
  slug published under v1, and the export gate asserts (EXPORT_005) that the
  lockfile set exactly equals the exported set. Onboarding a provider appends a
  line; no line is ever removed within v1 (providers removed per the contract's
  tombstone rules still publish a doc, so they remain in the lock). A slug
  rename therefore cannot pass the gate accidentally — it requires deleting a
  lockfile line, which this ADR forbids within a major.

## Consequences

**What becomes easier:**

- Rebrands ship same-day as display-level data fixes, with no contract debate.
- The contract's slug-permanence promise is machine-enforced against any
  origin — including edits that never passed through this repo's commit hooks.
- A future v2 planning effort has a ready checklist (this ADR + syllago#504)
  for batching accumulated renames.

**What becomes harder:**

- Onboarding a provider touches one more file (a lockfile line; the EXPORT_005
  error message says exactly that).
- Slug and user-facing name drift apart over time (`windsurf` ≠ "Devin
  Desktop"). Consumers that render raw slugs show stale branding — `display_name`
  exists for exactly that, and tier 2 covers matching if it's ever needed.

**What's deferred:**

- The `former_names` field (build on first consumer need, with its schema bump).
- The slug-rename runbook execution itself (v2 planning, with syllago#504).
- syllago-side: making a missing capability doc a hard failure in coverage
  assertion 5 instead of a silent skip (asked in syllago#504).
