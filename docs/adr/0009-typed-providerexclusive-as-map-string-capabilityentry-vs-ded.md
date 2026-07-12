# 0009. Typed ProviderExclusive as map[string]CapabilityEntry vs dedicated struct vs status quo

Date: 2026-07-12
Status: Accepted
Feature: publish-layer

## Context

The lexicon defines a provider-exclusive capability as a provider behavior with no canonical key that graduates to one when 2+ providers converge (`lexicon.md:28`) — i.e., the same concept as a capability node, pre-registration. Reusing `CapabilityEntry` means graduation is a data move, not a shape change; consumers reuse one tree-walking code path; and since `provider_exclusive` appears in zero baselines today (research Q2), the retyping has no migration cost.

## Decision

Chose **Retype `ProviderCapabilities.ProviderExclusive` from `map[string]interface{}` (`capyaml/types.go:15`) to `map[string]CapabilityEntry` — provider-exclusive capabilities are ordinary capability nodes (disposition + optional nested capabilities) that simply have no canonical key yet; export emits them with the same node shape as the capability tree, definitionally never carrying `key`/`key_path`** over **A dedicated `ProviderExclusiveEntry` struct with extra bookkeeping fields (speculative — no data exists to justify any extra field); keeping `map[string]interface{}` and exporting opaquely (abdicates the "typed ProviderExclusive shapes exported JSON" requirement and leaves the published schema unable to say anything)**.

## Consequences

The published provider-capabilities schema models `provider_exclusive` values with the same recursive node definition as capability nodes, minus the key-metadata fields; the `key`-presence discriminator (ADR 0002) stays globally consistent — no `provider_exclusive` node ever has `key`. If a future exclusive needs non-disposition payload, that is a shape change with a `schema_version` bump on the owning schema.
