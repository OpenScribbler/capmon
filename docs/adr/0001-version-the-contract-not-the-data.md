---
id: "0001"
title: Version the Contract, Not the Data
status: accepted
date: 2026-07-11
enforcement: advisory
files: ["export*.go", "cmd/capmon/capmon_export_cmd*.go", "site-static/**"]
tags: [publishing, versioning, freeze]
---

# ADR 0001: Version the Contract, Not the Data

## Status

Accepted

## Context

capmon publishes its provider capability data as JSON on GitHub Pages for
third-party consumers. The data changes daily; the document shapes and URL
layout change rarely. Versioned publishing goes wrong when "version" is
applied to the wrong thing: maintaining multiple live versions of a
published site means multiple build paths, each needing maintenance
indefinitely.

Alternatives considered:

- **Unversioned URLs** with only an in-document version field — rejected:
  retrofitting version paths after strangers depend on the URLs is the
  expensive, trust-burning path.
- **Parallel live exporters per major version** — rejected: this is the
  maintenance trap itself; every old major stays a build target forever.
- **Dated data snapshots** as the versioning unit — rejected: git history
  already archives the data; publishing is not archival.

## Decision

The published site is a **live view of current data**, never an archive.
The URL major (`/v1/`) versions the *contract*: document shapes, path
layout, field semantics. Four signals, each with exactly one job:

| Signal | Job | Changes when |
|---|---|---|
| URL major (`/v1/`) | compatibility contract | breaking change only |
| `schema_version` (per document/schema) | shape revision within a major | any shape change, per schema |
| `data_revision` (`index.json`) | data change detection | provider data changes |
| `generated_at` (`index.json`) | freshness heartbeat | every publish |

**Freeze, don't fork.** The exporter only ever generates the current major.
On a major transition the old tree is frozen as static files
(`site-static/`) via a single final mutation that sets
`status`/`superseded_by`/`frozen_at` — fields defined as OPTIONAL in every
schema from initial publication, so the freeze cannot produce a
schema-invalid document against its own pinned schemas. Old-major URLs
never 404.

**Advisories are the sole post-freeze mutation.** Each major carries
`advisories.json` (regenerated every deploy, even for frozen majors) as the
correction channel for frozen-but-wrong data; `status: "withdrawn"` marks a
major unsafe to trust. Frozen trees are tamper-evident via a root hash
recorded outside the tree and re-verified by CI — a frozen tree never
vouches for itself.

**Meaning changes must be visible.** Field and key semantics are immutable
within a major; a meaning change ships as a rename or a major bump, never
by repurposing an existing name. Published provider slugs are permanent
within a major (removal produces a tombstone, not a 404). Enum values are
open for consumers (unrecognized values are "unknown/other", never errors)
and append-only for producers.

## Consequences

**What becomes easier:**
- One live export code path forever; old majors are inert files, not build
  targets.
- Breaking-change classification is mechanical (rename rule, enum rule,
  path-permanence rule) rather than judgment-per-change.
- Consumers can pin a major knowing exactly what may and may not change.

**What becomes harder:**
- Frozen data goes stale by design; the advisories channel and per-document
  `status` fields exist to compensate.
- Every published shape must define the freeze fields from its first
  version — omitting them in a new schema is a latent freeze-time bug.

**What's deferred:**
- The freeze operation as a tested command (copy tree, apply the single
  final mutation, validate, record root hash) is specified but not built
  until a second major first approaches; only the schema field
  pre-provisioning ships now.
