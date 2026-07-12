# 0007. JSON Schema validation via santhosh-tekuri/jsonschema vs alternatives vs hand-rolled

Date: 2026-07-12
Status: Accepted
Feature: publish-layer

## Context

The fail-closed gate's promise (ADR 0004; `docs/design/publish-layer.md:280-284`) is "every output file validates against its **published** schemas" — that is only true if the gate interprets the actual schema files consumers vendor, not a parallel Go re-implementation that can drift from them. No schema library exists in `go.mod` today (research Q3, `go.mod:7-16`); santhosh-tekuri v6 is the maintained, pure-Go, 2020-12-conformant standard choice and the schemas' declared dialect matches the existing (unused) source schema's draft (`docs/provider-capabilities/schema.json:1-8`).

## Decision

Chose **`github.com/santhosh-tekuri/jsonschema/v6` as a new direct dependency, compiling the published schema files themselves (draft 2020-12) with `$id`s resolved to the local export tree — never fetched over the network** over **`xeipuuv/gojsonschema` (unmaintained, no draft 2020-12); `kaptinlin/jsonschema` (younger, far less battle-tested); hand-rolled validation extending the existing approach, which is enum-check + struct parse only (`capyaml/validate.go:13-45`) and enforces none of the published schemas' required/enum/type constraints**.

## Consequences

One new direct dependency in `go.mod`. The gate validates every generated file against its schema in-process before anything leaves the temp dir, both in `capmon export` itself and therefore identically in local runs and CI — the same code path, no workflow-only validation. Schema compilation must use local resources exclusively so export works offline and deterministically.
