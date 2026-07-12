# 0010. New publish workflow vs extending ci.yml

Date: 2026-07-12
Status: Superseded by ADR-0012
Feature: publish-layer

## Context

The trigger semantics the design doc pins (push to `main` touching `docs/`, plus a daily run that skips deploy when `data_revision` is unchanged) are preserved exactly; only the host file differs. A dedicated workflow makes the fail-closed publish path — build, export, gate, upload-pages-artifact, attest, deploy — one self-contained, SHA-pinned unit that is structurally incapable of running on a pull request, and keeps `ci.yml` permanently read-only.

## Decision

Chose **A new dedicated publish workflow (`publish.yml`) with triggers `push` to `main` filtered to `docs/**`, a daily `schedule`, and `workflow_dispatch`; job-level `pages: write`, `id-token: write`, `attestations: write` over a `contents: read` top level** over **Extending `ci.yml`, which the design doc's parenthetical "(from `ci.yml`)" suggests (`docs/design/publish-layer.md:275`) — but `ci.yml` runs on every `pull_request` with top-level `contents: read` only (`ci.yml:3-9`), has no `schedule` trigger for the daily re-stamp, and mixing deploy/attestation permissions into a PR-triggered workflow enlarges the attack surface ADR 0004 (strict, scope `.github/workflows/*.yml`) exists to keep auditable**.

## Consequences

The `go build` step is duplicated across workflows (already true of `pipeline.yml`'s four jobs; cheap). The daily cron in `publish.yml` runs the full export + fail-closed gate as a daily health check and compares the computed `data_revision` against the live `v1/index.json`, deploying only on difference — per the accepted mechanics, an unchanged-data cron run ends without deploying, so `generated_at` advances only on actual publishes. `pipeline.yml` is touched only for the heartbeat relocation; `ci.yml` is not modified.
