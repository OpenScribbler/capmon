# 0011. Delete stale source-side schema.json vs rewrite and enforce it

Date: 2026-07-12
Status: Accepted
Feature: publish-layer

## Context

No Go code loads it (research Q3 — repo-wide grep for the path and for any schema library returns nothing), and it is wrong: its confidence enum says `high/medium/low` while every baseline uses `confirmed/inferred` (`lexicon.md:26`). Phase 4 creates the authoritative, machine-enforced schema for this shape at `v1/schemas/provider-capabilities.json`; keeping a second, near-identical schema for the YAML side is a standing drift liability with no enforcing consumer. Source-side validation remains `capyaml.ValidateAgainstSchema`'s version-gate + struct parse (`capyaml/validate.go:13-45`), which is what actually runs today.

## Decision

Chose **Delete `docs/provider-capabilities/schema.json`** over **Rewriting it to match reality (confidence enum `confirmed/inferred`, add `confidence` + recursive `capabilities` to its CapabilityEntry model) and wiring it into `capmon verify` via the new jsonschema dependency**.

## Consequences

Source YAML gains no new validation in this feature (hardening like yaml.v3 `KnownFields` is explicitly out of scope); if source-side JSON Schema validation is ever wanted, it should be derived from the published schema, not maintained in parallel. Nothing references the deleted file's `$id`.
