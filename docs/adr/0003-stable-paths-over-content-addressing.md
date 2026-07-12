---
id: "0003"
title: Stable Mutable Paths over Content-Addressed Files
status: accepted
date: 2026-07-11
enforcement: advisory
files: ["export*.go", "cmd/capmon/capmon_export_cmd*.go", ".github/workflows/pipeline.yml", ".github/workflows/ci.yml"]
tags: [publishing, urls, deploys]
---

# ADR 0003: Stable Mutable Paths over Content-Addressed Files

## Status

Accepted

## Context

GitHub Pages deploys are not atomic at the CDN edge: during a deploy window
a consumer can fetch a fresh `index.json` and a stale data file (or the
reverse), producing a spurious hash mismatch against the index.

Content-addressed filenames
(`capabilities/claude-code.<sha256>.json`, with `index.json` as the single
mutable pointer) would solve this by construction: every referenced file is
immutable, deploys are atomic, and frozen trees are tamper-evident by URL.
The security argument for them is real and will keep resurfacing — any
future security review of this system should expect to re-derive it.

The operational counterargument: the data changes at most daily, the deploy
action purges the CDN cache, and the skew window is minutes. Content
addressing would also mean every consumer needs an index fetch before every
data fetch, old blobs accumulate forever (or need a pruning policy), and —
decisively — "one memorable URL per provider" stops existing.

## Decision

**Stable mutable paths.** `/v1/capabilities/<slug>.json` is the URL forever
within a major. One stable URL per provider is the headline feature of the
publish layer and outweighs constructive atomicity at a daily change
cadence.

The skew window is handled contractually instead: on an index-vs-file hash
mismatch, consumers re-fetch after the propagation window (minutes) before
treating it as an integrity failure. Frozen-tree tamper evidence comes from
an externally recorded root hash plus CI re-verification (ADR-0001).
Partial-deploy corruption is prevented by fail-closed publish gating
(ADR-0004), not by URL schemes.

## Consequences

**What becomes easier:**
- "Pull this one URL" stays true; consumers hardcode one path per provider.
- No blob accumulation or garbage-collection policy.

**What becomes harder:**
- A minutes-long eventual-consistency window exists after each deploy;
  verifying consumers must implement the re-fetch rule or tolerate rare
  false mismatches.

**What's deferred:**
- Nothing. If the change cadence ever becomes hourly or the consumer base
  makes the skew window material, supersede this ADR rather than bolting
  hashed paths alongside stable ones.
