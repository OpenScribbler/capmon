---
id: "0002"
title: Self-Describing Export Documents
status: accepted
date: 2026-07-11
enforcement: advisory
files: ["export*.go", "cmd/capmon/capmon_export_cmd*.go", "docs/spec/canonical-keys.yaml"]
tags: [publishing, export-shape, canonical-keys]
---

# ADR 0002: Self-Describing Export Documents

## Status

Accepted

## Context

A stranger fetching a published per-provider document
(`capabilities/<slug>.json`) should understand the data without reading
capmon's source. The capability values alone (`supported`, `mechanism`,
`confidence`) don't say what a key *means* or where its specification
lives — that knowledge is in `docs/spec/canonical-keys.yaml`, whose
`spec_ref` values point at the owning ACIF spec sections.

Alternatives considered:

- **Normalized model**: provider documents carry only observed values; key
  semantics live in a separately published registry file consumers join
  against. Rejected as the primary shape: every consumer needs two fetches
  and a join before anything is interpretable.
- **Faithful YAML→JSON dump** with no added metadata. Rejected: pushes all
  semantics into prose documentation that consumers must find and read.

Two properties of the real data shaped the decision. First, the capability
tree is recursive with variable depth — some canonical keys are branches
with nested sub-capabilities, not leaves. Second, those nested sub-values
(e.g. `at_mention` under `invocation_patterns`) are vocabulary members of
an object-typed key, not registered canonical keys themselves — there is no
registry metadata to inline at exactly the nodes a naive tree-walker hits.

## Decision

Each exported per-provider document inlines registry metadata at every
**registry-backed** node:

- `key_path`: the node's canonical dot-path (e.g.
  `"agents.invocation_patterns"`), so consumers can join back to the
  published registry without reconstructing tree position.
- `key`: `{description, type, spec_ref}` copied from the canonical-key
  registry.

The discriminator is normative: **`key` present ⇔ the node is a registered
canonical key.** Nodes without `key` are vocabulary members; their
semantics live in the parent key's description and spec reference.
`key.type` describes the registered concept, not the node's JSON shape —
consumers inspect nodes structurally.

The tree is recursive with unbounded depth. Branch nodes carry the same
disposition fields as leaves (`supported`, `mechanism`, `confidence`), so a
leaf gaining children is additive — existing reads stay valid.

The deduplicated registry is *also* published (`spec/canonical-keys.json`),
so bulk consumers can strip inline `key` blocks and join instead.

## Consequences

**What becomes easier:**
- One fetch fully interprets a provider: values, meanings, spec links.
- Generic tree-walkers are safe: the `key`-presence discriminator and
  uniform disposition fields are contractual, not incidental.

**What becomes harder:**
- Inline duplication makes the all-providers bundle and the
  by-content-type pivots larger (the same key metadata repeated per
  provider); acceptable at the current provider count, revisit via
  supersession if it grows an order of magnitude.
- The exporter owns a join (provider YAML × canonical-key registry) that
  must fail closed when a provider key is missing from the registry.

**What's deferred:**
- Registering vocabulary members as first-class registry entries (which
  would let them carry `key` metadata too); they are explicitly
  second-class by design for now.
