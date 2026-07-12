# 0006. Heartbeat keepalive via Actions API vs orphan branch vs tag

Date: 2026-07-12
Status: Accepted
Feature: publish-layer

## Context

ADR 0005 pins only "no scheduled job holds write access to the publish branch"; the API keepalive is the strongest satisfaction — the scheduled workflow holds no `contents` permission at all, so there is nothing for branch protection to catch. GitHub's 60-day scheduled-workflow disablement is reset by re-enabling the workflow, which is exactly and only what the heartbeat exists to do (`lexicon.md:46`); the commit was always a means, not the end.

## Decision

Chose **Actions API keepalive — the heartbeat job re-enables the scheduled workflow via `gh api -X PUT repos/{owner}/{repo}/actions/workflows/pipeline.yml/enable` with job permissions `actions: write` only, replacing the commit-to-main step (`.github/workflows/pipeline.yml:233-260`)** over **Orphan-branch commit (keeps a commit trail but the scheduled job retains `contents: write`, relying on branch protection alone to keep it off `main`); force-moved git tag (same retained `contents: write`, plus tag-namespace pollution)**.

## Consequences

The heartbeat job's permissions shrink from `contents: write` (`pipeline.yml:239-242`) to `actions: write`; `.heartbeat/last-run.json` stops being committed, so the run manifest's repo-visible copy goes away — run-manifest observability moves to a workflow artifact and run logs (it is write-only observability, never a pipeline input, per `lexicon.md:45`). If GitHub ever changes the enable-endpoint clock-reset behavior, the documented fallback is the orphan-branch variant; the ADR 0005 constraint holds under both.
