---
id: "0004"
title: Attestation at Launch; Hashes Are Change-Detection Only
status: accepted
date: 2026-07-11
enforcement: strict
files: [".github/workflows/pipeline.yml", ".github/workflows/ci.yml"]
tags: [security, integrity, attestation, publishing]
---

# ADR 0004: Attestation at Launch; Hashes Are Change-Detection Only

## Status

Accepted

## Context

The published `index.json` carries per-file SHA-256 hashes. Those hashes
are served from the same origin as the files they hash and are unsigned —
anyone who can alter a published file can alter its hash in the same write.
Hash and hashed bytes share one trust domain and one fate. Same-origin
hashes therefore defend against accidental corruption (truncated deploys,
edge bit-rot) and enable change detection, but provide nothing against an
adversary who controls the publish path.

This matters because consumers may gate security-relevant decisions on the
data (sandboxing support, auto-approve gating), and the planned downstream
consumer (the syllago repo) automatically opens PRs from fetched bundles —
an unverified fetch is an injection path into another repository.

Alternatives considered:

- **Launch with honestly-labeled checksums, add signing later** — rejected:
  the verification obligation is cheapest to impose before consumers exist;
  retrofitting a MUST onto deployed consumers never lands.
- **Custom signing keys** — rejected: key management burden with no
  benefit over the platform's keyless workflow-identity attestation.

## Decision

Three inseparable parts:

1. **Provenance attestation from the first publish.** The publish job runs
   GitHub's build-provenance attestation over the deployed artifact,
   binding it to the workflow's OIDC identity and the source commit.
   Consumers verify with `gh attestation verify`.
2. **Verification is fail-closed for integrity-sensitive consumers.** The
   published consumer contract states it as MUST: verify the attestation
   over `index.json`, then each file's `sha256` against it, before use; any
   mismatch aborts the fetch and triggers nothing downstream. Any consumer
   that acts automatically on the data (syllago's auto-PR pipeline is the
   canonical case) is in this class. In-document hashes are documented as
   change/corruption detection, never as authenticity.
3. **Publishing is fail-closed.** The export writes to a temp directory,
   validates every output against the published schemas, and asserts the
   provider set matches the source (count + slugs) before any deploy. A
   partial or invalid export never replaces a good site — a missing
   provider file would be an unversioned breaking change, and Pages has no
   rollback.

## Consequences

**What becomes easier:**
- Every upstream compromise (token, workflow, repo write) is capped at the
  consumer's verification boundary — one choke point instead of policing
  each path separately.
- Reproducibility claims (source commit binding, byte-determinism) become
  meaningful because the binding is signed, not asserted.

**What becomes harder:**
- The publish workflow needs `id-token: write` and `attestations: write`
  permissions, and verification must be documented for consumers.
- Determinism regressions (a toolchain bump changing output bytes) surface
  as verification noise; the pinned canonicalization profile and a CI
  double-export diff exist to prevent this.

**What's deferred:**
- A reproducer command that rebuilds the export from a source commit and
  diffs against the published tree; it can land after the first publish.
