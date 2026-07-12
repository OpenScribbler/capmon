---
id: "0012"
title: Daily Cron Deploys the Re-Stamped Index
status: accepted
date: 2026-07-12
enforcement: strict
files: [".github/workflows/publish.yml"]
tags: [publishing, freshness, cron, pages]
---

# ADR 0012: Daily Cron Deploys the Re-Stamped Index

## Status

Accepted

**Supersedes ADR-0010** ([New publish workflow vs extending ci.yml](0010-new-publish-workflow-vs-extending-ci-yml.md)).

## Context

ADR-0010 decided two things: publishing lives in a dedicated `publish.yml`
rather than an extension of `ci.yml`, and the daily cron run "ends without
deploying" when `data_revision` is unchanged, "so `generated_at` advances
only on actual publishes."

The second part contradicts the published consumer contract. The contract
(`README.md`, `docs/design/publish-layer.md`) tells consumers to treat the
feed as stale when `generated_at` is older than `max_staleness_hours` (48).
Capability data routinely sits unchanged for longer than 48 hours, so under
the skip-deploy rule a perfectly healthy feed permanently reads as stale —
the freshness fields can never signal the one thing they exist to signal:
whether the pipeline is alive. Consumers get no way to distinguish
"healthy but unchanged" from "publish path dead", which matters once an
automated consumer (syllago's pull job) gates on the feed.

Alternatives considered:

- **Keep the skip; reword the contract so staleness is not keyed to
  `generated_at`** — rejected: it removes the liveness signal entirely
  instead of fixing it, and leaves `max_staleness_hours` published but
  meaningless.
- **Defer to the launch runbook** — rejected: no consumers exist yet, but
  merging a self-contradicting contract and fixing it after launch costs
  strictly more than fixing it while the PR is still draft.

## Decision

The dedicated-workflow half of ADR-0010 is carried forward unchanged:
publishing stays in `publish.yml`, a self-contained SHA-pinned unit that is
structurally incapable of running on a pull request, with job-level
`pages`/`id-token`/`attestations: write` over a `contents: read` top level.

The cron consequence is reversed: **the daily scheduled run always deploys.**
Every cron run re-exports through the fail-closed gate and publishes the
result, carrying the re-stamped `generated_at` to the live site. The
skip-deploy comparison against the live `data_revision` is deleted — a
data-identical export differs only in `v1/index.json`, and shipping that one
file daily is what makes `generated_at` a truthful liveness heartbeat.
`data_revision` remains the change-detection signal; `generated_at` is
purely freshness.

## Consequences

**What becomes easier:**
- The staleness contract is honest: `generated_at` older than
  `max_staleness_hours` now genuinely means the publish path has been down
  for two consecutive daily runs, so consumers (and the maintainer) get a
  real dead-pipeline signal.
- `publish.yml` loses the live-fetch skip check — one less step, no curl
  against the live site, no first-publish special case.

**What becomes harder:**
- Pages deploys and provenance attestations happen daily even when data is
  unchanged. Both are free-tier operations on a tiny artifact; attestations
  accumulate harmlessly.
- `generated_at` no longer identifies data-change publishes. Consumers that
  want "did the data change?" must use `data_revision`, which the consumer
  contract already instructs.

**What's deferred:**
- Nothing relative to ADR-0010.
