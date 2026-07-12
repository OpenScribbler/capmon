# 0008. Canonical JSON via map-built documents + one configured encoder vs struct-tag marshaling vs hand-rolled encoder

Date: 2026-07-12
Status: Accepted
Feature: publish-layer

## Context

`encoding/json` already sorts map keys ascending byte-wise, which for UTF-8 is exactly ascending Unicode code point — the profile's rule (`docs/design/publish-layer.md:301-305`) — and the repo has zero existing canonicalization helpers or `SetEscapeHTML` usage to build on (research Q6). Centralizing the entire profile (UTF-8, LF, trailing newline, two-space indent, sorted keys, no HTML escaping) in one helper makes "changing the profile is a breaking change" auditable at a single call site.

## Decision

Chose **Export documents are built as `map[string]any` trees (keys inserted only when present, satisfying "maps omitted when empty" and conditional fields naturally) and serialized by a single shared canonical-writer helper: `json.Encoder` with `SetEscapeHTML(false)` and `SetIndent("", "  ")`, whose `Encode` emits the trailing LF** over **Struct types with JSON tags (encoding/json emits struct fields in declaration order, not sorted — every struct edit risks silently violating the sorted-keys rule, and `omitempty` cannot express "always emit `supported: false`"); a hand-rolled canonical encoder (reimplements string escaping and UTF-8 handling for no gain)**.

## Consequences

Compile-time shape safety is traded away for maps; it is recovered by the fail-closed schema gate validating every output plus the committed byte-level fixture. Numeric values must be inserted as Go `int` (never `float64` — profile says integers only); the helper is the sole JSON serialization path for the export tree. The indirect `go-json-experiment/json` dependency (`go.mod:22`, via chromedp) is not adopted.
