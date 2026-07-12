---
id: "0005"
title: No Scheduled Job Holds Write Access to the Publish Branch
status: accepted
date: 2026-07-11
enforcement: strict
files: [".github/workflows/pipeline.yml"]
tags: [security, ci, permissions]
---

# ADR 0005: No Scheduled Job Holds Write Access to the Publish Branch

## Status

Accepted

## Context

Publishing turns "whatever is on `main`" into "what every consumer receives
within a day." The data's safety story is that capability values only
change through human-reviewed PRs — the pipeline's automated writes are
deliberately confined to opening PRs and issues, which stop at human
review.

One exception undermined that story: the daily pipeline committed a
heartbeat marker file directly to `main`, which required the scheduled
workflow to hold `contents: write` on the very branch that feeds
publishing. A compromised workflow step (a poisoned action, a malicious
dependency in the job) could use that permission to write anything to
`main`, and the next publish would ship it to every consumer with no human
in the loop. The heartbeat was the only reason the permission existed.

Alternatives considered:

- **Keep the permission, police the steps** (pin actions, audit
  dependencies) — rejected as the primary defense: pinning is already done
  and worth keeping, but it polices the attack rather than removing the
  capability.
- **Accept the risk** (the heartbeat write is tiny) — rejected: the size of
  the legitimate write is irrelevant; the permission is branch-wide.

## Decision

No scheduled workflow holds `contents: write` on the publish branch.

- The heartbeat moves off `main` — to an orphan branch, a tag, or the
  Actions API — so the scheduled pipeline's permissions on `main` are
  read-only.
- Branch protection with required review is enabled on `main`, so "on
  `main`" and "human-reviewed" are the same set even against a token that
  somehow gains write.
- Both land before the first publish: the guarantee must hold from day one
  of consumers existing.

## Consequences

**What becomes easier:**
- The claim "published data only changes through human review" is enforced
  by permissions, not by convention — it stays true even when a workflow
  step is compromised.

**What becomes harder:**
- Heartbeat/staleness checks read from a less obvious location than a file
  on `main`.
- Branch protection adds a review step to every change, including trivial
  ones by the maintainer.

**What's deferred:**
- Nothing.
