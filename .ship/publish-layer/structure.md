# Structure Outline: publish-layer

## Current / Desired / End State

**Current:** Provider capability data exists only as YAML in the repo (15 capability baselines + the 46-key canonical-key registry); consumers must clone capmon, there is no `capmon export`, no published JSON, and the one generated view embeds a run timestamp that makes even repeated local runs byte-different (`generate.go:52-53`). ~31 capability nodes sit in canonical-key position without registry keys, `ProviderExclusive` is an untyped `map[string]interface{}`, and the scheduled heartbeat commits directly to `main`.

**Desired:** `capmon export` compiles the capability baselines joined with the canonical-key registry into a deterministic, self-describing `/v1/` JSON tree that a fail-closed, SHA-pinned, attestation-emitting `publish.yml` workflow publishes to GitHub Pages; no scheduled job holds write access to `main`.

**End state:** A consumer fetches `v1/index.json`, compares `data_revision` for one-field change detection, and pulls `capabilities/<slug>.json`, where every registry-backed node self-describes via inlined `key_path` + `key` metadata (ADR 0002). Integrity-sensitive consumers (syllago) verify the provenance attestation, then per-file `sha256`, failing closed on mismatch (ADR 0004). The maintainer runs `capmon export` locally and gets byte-identical output for identical source data; a bad export never replaces a good site.

## Patterns to Follow

### Pattern: one file per cobra subcommand, self-registering `init()`

**Source:** `cmd/capmon/capmon_derive_cmd.go:71` (registration), `cmd/capmon/capmon_derive_cmd.go:11-15` (test override vars)

```go
func init() {
    capmonCmd.AddCommand(deriveCmd)
}
```

`capmon export` follows the established shape: its own `capmon_<name>_cmd.go` + paired `_test.go`, local flags defined in `init()` with literal defaults (`cmd/capmon/capmon_check_cmd.go:51-57`), package-level path-override vars for tests (`cmd/capmon/capmon_cmd.go:30`), structured errors via `output.NewStructuredError` with namespaced codes (`internal/output/errors.go:5-11`). Errors return through `RunE` → exit 1 (`cmd/capmon/main.go:14-17`); export does not need the pipeline exit classes.

### Pattern: drift guard — regenerate and `bytes.Equal` against the committed tree

**Source:** `seederspec_audit_test.go:77-121` (comparison at `:121`)

```go
if !bytes.Equal(got, committed) {
    t.Errorf("derived output for %s differs from committed file", path)
}
```

The committed byte-level export fixture and the determinism guarantee reuse this exact pattern: regenerate from `testdata` input, compare byte-for-byte against committed expected output — no `.golden` framework exists or is needed. Repo-root resolution via `docsRoot(t)` / `canonicalKeysPath(t)` (`docsroot_test.go:15-28`); table-driven style where applicable (`sanitize_test.go:10`); `output.SetForTest(t)` for CLI output capture (`internal/output/output.go:34-58`).

### Pattern: SHA-pinned actions + hash-verified artifact handoff

**Source:** `.github/workflows/pipeline.yml:64` (pinning), `:49-50` + `:86-93` + `:117-132` (SHA-256 job-output verification)

```yaml
- uses: actions/checkout@de0fac2e... # v6.0.2
```

The publish workflow pins `actions/upload-pages-artifact`, `actions/deploy-pages`, and `actions/attest-build-provenance` by full commit SHA with version comments, exactly as every existing action is pinned; job-level permission escalation over a `contents: read` top level (`pipeline.yml:32-33`, `:42-45`) is the permissions model to copy. Go setup resolves the toolchain from `go.mod` via `go-version-file` (`ci.yml:20-22`; `go.mod:3-5` pins `go 1.26` / `toolchain go1.26.3`) — this is part of the determinism envelope.

### Pattern: CI command set is canonical — there is no Makefile

**Source:** `.github/workflows/ci.yml:24-50`

```yaml
- run: go build ./...
- run: go vet ./...
- run: go test -race ./...
```

All new checks (export drift guard, double-export determinism test, schema-gate tests) must be reachable through `go test ./...` so `ci.yml`'s existing command set exercises them without new plumbing; the gofmt and legacy-name gates also apply to all new code.

### Pattern (negative): the generated-view banner timestamp is the anti-pattern

**Source:** `generate.go:52-53`

```go
fmt.Fprintf(&buf, "# Generated: %s\n", time.Now().UTC().Format(time.RFC3339))
```

This is the one run-varying byte source in the existing generation path and the exact thing the canonicalization profile forbids: no timestamp or run-varying value in any exported document except `generated_at` in `v1/index.json` (`docs/design/publish-layer.md:306-309`). The exporter must never call `time.Now()` in a document body.

### Pattern (constraint): a new registry loader — `loadCanonicalKeys` discards what export needs

**Source:** `formatdoc_validate.go:79-103`

```go
type canonicalKeysFile struct {
    ContentTypes map[string]map[string]interface{} `yaml:"content_types"`
}
```

The only existing registry parser returns bare per-content-type key-name sets, discarding `description`/`type`/`spec_ref` — the exact fields ADR 0002 requires inlined at every registry-backed node. Export gets its own loader that preserves all three fields per entry (`docs/spec/canonical-keys.yaml:54-61`); `loadCanonicalKeys` and its consumers stay untouched. No existing code looks keys up by full dot-path, so dot-path assembly (`<content_type>.<key>` → `key_path`) is new export logic.

### Pattern (constraint): capyaml structs are the source shape the exporter mirrors

**Source:** `capyaml/types.go:4-63`, loader `capyaml/load.go:12-22`

```go
Capabilities map[string]CapabilityEntry `yaml:"capabilities,omitempty"` // recursive
```

Export loads baselines with `capyaml.LoadCapabilityYAML` (same entry point as `generate.go:30` and `capmon verify`), skips seed YAMLs by the established `Slug == ""` test (`generate.go:34-37`), and mirrors the recursive `CapabilityEntry` tree into the recursive published tree. `events`/`tools`/`references` mirror source structure with `references` trimmed to `{url, verified_at}`; maps are omitted when empty but fully defined in published schemas; `supported` stays plain `bool` and the exporter always emits it at every node.

## Design Summary

All decisions are settled in `.ship/publish-layer/design.md` and the accepted spec `docs/design/publish-layer.md`; nothing below re-opens them. The exporter is a new root-package module built from: a new registry loader preserving `description`/`type`/`spec_ref` (design § "a new registry loader"), map-built documents serialized through one canonical-JSON writer (§ "Canonical JSON via map-built documents"), `ProviderExclusive` retyped to `map[string]CapabilityEntry` (§ "Typed ProviderExclusive"), a fail-closed gate using `santhosh-tekuri/jsonschema/v6` against the published schema files themselves (§ "JSON Schema validation"), a dedicated `publish.yml` (§ "New publish workflow"), the heartbeat rewritten as an Actions API keepalive (§ "Heartbeat keepalive"), and deletion of the stale source-side `schema.json` (§ "Delete stale source-side schema.json"). Design Question 1 is RESOLVED: A — every non-canonical node in canonical-key position relocates under the newly typed `provider_exclusive` as a launch-blocking audit within this feature. Structural mechanics fixed here (not re-debatable in Plan): exporter entry point is `RunExport(opts ExportOptions) error` in the root package; committed publish assets live under `docs/publish/` (so schema/spec edits hit `publish.yml`'s `docs/**` trigger); structured error codes are `EXPORT_001` (non-canonical node), `EXPORT_002` (schema validation), `EXPORT_003` (provider-set mismatch), `EXPORT_004` (verify mismatch); relocated audit nodes are keyed `<content_type>.<original_name>` under `provider_exclusive` to preserve graduation provenance.

## Slices

### Slice 1: Self-describing provider capability documents

**Observable outcome:** Given a capability baseline and the canonical-key registry, the exporter builds a `capabilities/<slug>.json` document — canonical bytes, `key_path` + `key` metadata inlined at every registry-backed node, `key`-presence discriminator intact, `display_name` slug fallback, empty `last_verified` omitted, `references` trimmed to `{url, verified_at}`, typed `provider_exclusive` nodes emitted without `key` — and fails closed with `EXPORT_001` on any non-canonical node in canonical-key position.

**Interfaces introduced or modified:**

- `capyaml.ProviderCapabilities.ProviderExclusive` — retyped `map[string]CapabilityEntry` (was `map[string]interface{}`, `capyaml/types.go:15`) — **Deps:** `in-process`
  - **Hides:** YAML round-trip mechanics for provider-exclusive nodes
  - **Exposes:** provider-exclusive capabilities as ordinary recursive `CapabilityEntry` nodes; graduation to a canonical key becomes a data move, not a shape change. Zero baselines carry the field today (research Q2), so no migration.
- `loadKeyRegistry` — `func loadKeyRegistry(path string) (keyRegistry, error)` where `keyRegistry` is `map[string]map[string]keyMeta` (content type → key → `{Description, Type, SpecRef string}`), new file `export_registry.go` — **Deps:** `local-substitutable` (path-parameterized; tests use temp-file fixtures)
  - **Hides:** YAML parsing of `docs/spec/canonical-keys.yaml`; existing `loadCanonicalKeys` and its consumers untouched
  - **Exposes:** full per-key metadata plus membership lookup for the non-canonical-node gate
- `canonicalJSON` — `func canonicalJSON(doc map[string]any) ([]byte, error)`, new file `export_canonical.go` — **Deps:** `in-process`
  - **Hides:** the entire canonicalization profile: `json.Encoder` with `SetEscapeHTML(false)` + `SetIndent("", "  ")`, trailing LF from `Encode`, sorted keys via map marshaling; returns an error on any float value (profile: integers only)
  - **Exposes:** the sole JSON serialization path for the export tree — the profile is auditable at one call site. Deliberately shallow (~2 functions): the shallowness IS the design; centralizing the profile is the module's whole job (design § "Canonical JSON").
- `buildProviderDoc` — `func buildProviderDoc(caps *capyaml.ProviderCapabilities, reg keyRegistry) (map[string]any, error)`, new file `export_provider.go` — **Deps:** `in-process` (pure: structs in, map tree out)
  - **Hides:** recursive node mapping (`supported` always emitted; `mechanism`/`confidence`/`refs` when present; `events`/`tools` mirrored, `EventEntry.Blocking` as plain string); `key_path`/`key` inlining for registry-backed direct children of `content_types.<ct>.capabilities`; descendants emitted as vocabulary members without `key`; `display_name` → slug fallback; empty `last_verified`/empty maps omitted (opencode publishes with `content_types` omitted); `references` trimmed; `provider_exclusive` emitted with the same node shape, definitionally never `key`/`key_path`; top-level `schema_version: "1"`, `status: "live"`
  - **Exposes:** one document map per provider plus the fail-closed non-canonical-node gate: a direct child of a content type's `capabilities` map not in the registry for that content type → `output.NewStructuredError` `EXPORT_001` naming provider + node

**Files:**

- `capyaml/types.go` — `ProviderExclusive` retype
- `capyaml/provider_exclusive_test.go` — new: round-trip test for the retyped field
- `export_registry.go` — registry loader + `keyRegistry`/`keyMeta` types
- `export_registry_test.go` — loader tests
- `export_canonical.go` — canonical JSON writer
- `export_canonical_test.go` — profile conformance tests
- `export_provider.go` — per-provider document builder + non-canonical-node gate
- `export_provider_test.go` — builder tests

**Test cases:**

- Unit: `TestProviderExclusiveRoundTrip` (`capyaml/provider_exclusive_test.go`) — YAML with nested `provider_exclusive` capabilities loads into `map[string]CapabilityEntry` via `capyaml/types.go` and `WriteCapabilityYAML` round-trips it byte-stably
- Unit: `TestLoadKeyRegistryPreservesMetadata` (`export_registry_test.go`) — fixture registry file loads with `description`/`type`/`spec_ref` intact per entry; missing content type / key returns absent, exercising `export_registry.go`
- Unit: `TestLoadKeyRegistryRealFile` (`export_registry_test.go`) — real `docs/spec/canonical-keys.yaml` via `canonicalKeysPath(t)`: 6 content types, 46 keys, every entry carries all three fields (drift guard on registry shape)
- Unit: `TestCanonicalJSONProfile` (`export_canonical_test.go`) — table-driven over `export_canonical.go`: keys sorted ascending by code point (incl. non-ASCII), two-space indent, LF + single trailing newline, `&<>` unescaped, raw UTF-8, `int` emitted bare, float input returns error
- Unit: `TestBuildProviderDocKeyMetadata` (`export_provider_test.go`) — registry-backed branch gets `key_path` + `key` and its children are vocabulary members without `key`; registry-backed leaf gets both; `supported` emitted at every node including `false`
- Unit: `TestBuildProviderDocNonCanonicalNodeFailsClosed` (`export_provider_test.go`) — unregistered direct child of a content type's `capabilities` → `EXPORT_001` naming provider + node; descendants of registry-backed keys are never name-checked
- Unit: `TestBuildProviderDocFallbacksAndTrimming` (`export_provider_test.go`) — `display_name: ""` → slug; `last_verified: ""` omitted; `content_types: {}` omitted; `references` → `{url, verified_at}` only; `events`/`tools`/`refs` mirrored; `provider_exclusive` nodes shaped like capability nodes with no `key`, via `export_provider.go`

**Checkpoint:** `go build ./... && go test -race -run 'ProviderExclusive|KeyRegistry|CanonicalJSON|BuildProviderDoc' ./...` passes.

### Slice 2: Non-canonical nodes relocated to provider_exclusive (launch-blocking audit)

**Observable outcome:** All 15 real capability baselines build provider documents with zero `EXPORT_001` errors — the ~31 non-canonical nodes in canonical-key position (codex 18, claude-code 9, pi 4, plus flagged nodes in amp and cline) now live under the newly typed `provider_exclusive`, keyed `<content_type>.<original_name>` (e.g. `skills.frontmatter`, `skills.baseDir`), preserving their disposition fields and nested capabilities. A permanent drift guard keeps future baselines exportable.

**Interfaces introduced or modified:**

- `TestAllBaselinesBuildProviderDocs` drift guard — new file `export_baselines_audit_test.go`, loads every `Slug != ""` baseline under `docs/provider-capabilities/` and runs `buildProviderDoc` against the real registry — **Deps:** `in-process` (reads committed `docs/` in the same checkout via `docsRoot(t)`, the established drift-guard pattern)
  - **Hides:** enumeration/skip logic for the 76 seed YAMLs
  - **Exposes:** the launch-blocking invariant as a CI-enforced test: every committed baseline passes the non-canonical-node gate, forever. This is the sole module of a data-centric slice; the relocation itself is YAML edits validated by this guard plus the untouched `capmon verify` path.

**Files:**

- `docs/provider-capabilities/codex.yaml` — 18 skill-frontmatter mirror nodes (`tools`, `transport`, `url`, …) relocate from `content_types.skills.capabilities` to `provider_exclusive`
- `docs/provider-capabilities/claude-code.yaml` — 9 nodes (`skills.frontmatter`, `skills.live_reload`, …) relocate
- `docs/provider-capabilities/pi.yaml` — 4 camelCase nodes (`skills.baseDir`, `skills.disableModelInvocation`, `skills.filePath`, `skills.sourceInfo`) relocate; original casing preserved (provider-exclusive names are definitionally not canonical keys, so the key grammar does not bind them)
- `docs/provider-capabilities/amp.yaml` — flagged node at `:72` (`executable_tools`) triaged: relocate iff it is a non-registry direct child in canonical-key position
- `docs/provider-capabilities/cline.yaml` — flagged node at `:84` (`file_references`) triaged the same way
- `export_baselines_audit_test.go` — the drift guard

**Test cases:**

- Integration: `TestAllBaselinesBuildProviderDocs` (`export_baselines_audit_test.go`) — every real baseline (`codex.yaml`, `claude-code.yaml`, `pi.yaml`, `amp.yaml`, `cline.yaml` among the 15) builds cleanly against the real registry; fails before the relocation, passes after
- Manual: source data still verifies after relocation
  1. `go run ./cmd/capmon verify` exits 0 (struct-parse validation of all baselines including the edited five)
  2. `git diff --stat docs/provider-capabilities/` shows only the five audited files changed

**Checkpoint:** `go test -race -run AllBaselinesBuildProviderDocs ./...` passes against the committed tree.

### Slice 3: Complete deterministic /v1/ document tree

**Observable outcome:** `writeExportTree` stages the full generated-document tree in a directory: 15 `capabilities/<slug>.json`, `capabilities/all.json`, six `by-content-type/<type>.json` pivots, `spec/canonical-keys.json`, `advisories.json` (empty array at launch), `v1/index.json` (with `data_revision`, per-file `sha256`, `generated_at`, optional `source_commit`), and the constant root `index.json`. Two runs with pinned inputs produce byte-identical trees.

**Interfaces introduced or modified:**

- `buildAllDoc` / `buildPivotDocs` / `buildAdvisoriesDoc` / `buildRegistryDoc` — `func(...) map[string]any` builders, new file `export_tree.go` — **Deps:** `in-process`
  - **Hides:** `all.json` = `{schema_version, providers: {<slug>: <exact per-provider document object>}}` (one code path, byte-reuse of Slice 1 maps); pivots = `{schema_version, content_type, providers: {<slug>: <same self-describing content-type node>}}`, one file per registry content type always emitted (stable URL set), `providers` omitted when no provider declares the type; `spec/canonical-keys.json` = `{schema_version, content_types: {<ct>: {<key>: {description, type, spec_ref}}}}`; `advisories.json` = `{schema_version: "1", advisories: []}`
  - **Exposes:** document maps ready for the canonical writer
- `writeExportTree` — `func writeExportTree(dst string, opts ExportOptions) error`, `export_tree.go` — **Deps:** `local-substitutable` (all source dirs/paths carried in `ExportOptions{CapsDir, CanonicalKeysPath, SourcesDir, PublishAssetsDir, OutDir, SourceCommit, GeneratedAt}`; tests point at fixture dirs)
  - **Hides:** staging order (documents → hashes → indexes), directory layout, `0644`/`0755` writes
  - **Exposes:** a complete `/index.json` + `/v1/…` tree at `dst`
- `buildV1Index` / `buildRootIndex` — new file `export_index.go` — **Deps:** `in-process`
  - **Hides:** `data_revision` = sha256 hex of the staged `capabilities/all.json` bytes; per-file `sha256` computed by walking the staged `v1/` tree (every file except `v1/index.json` itself — per-provider docs land in the `providers` array, everything else in `files`); `providers` array sorted by slug (arrays are order-significant; the writer only sorts map keys); constants `cadence: "daily"`, `max_staleness_hours: 48`, `status: "live"`, `schema_version: "1"`; `source_commit` omitted when unset; `generated_at` is the sole run-varying value in the whole tree; root index generated from code constants `{latest: "v1", majors: [{prefix: "v1", status: "live", index: "v1/index.json"}]}` — `site-static/` handling stays dormant until a freeze exists
  - **Exposes:** the two discovery indexes

**Files:**

- `export_tree.go` — aggregate/pivot/advisories/registry builders + tree staging
- `export_tree_test.go` — layout and shape tests
- `export_index.go` — v1 index, root index, `data_revision`, hashing
- `export_index_test.go` — index tests

**Test cases:**

- Unit: `TestWriteExportTreeLayout` (`export_tree_test.go`) — exact relative-path set for a fixture input via `export_tree.go`: per-provider docs, `all.json`, six pivots, `spec/canonical-keys.json`, `advisories.json`, `v1/index.json`, root `index.json`; pivot files exist for all six content types
- Unit: `TestBuildAllAndPivotsShareNodeShape` (`export_tree_test.go`) — a provider's node in `all.json` and in its pivot is byte-identical to the same node in `capabilities/<slug>.json`
- Unit: `TestBuildV1Index` (`export_index_test.go`) — `data_revision` equals sha256 of staged `all.json`; every non-index staged file appears exactly once across `providers`/`files` with correct `sha256`; `providers` sorted by slug; `source_commit` omitted when unset; exercising `export_index.go`
- Unit: `TestRootIndexBytes` (`export_index_test.go`) — root index is the exact constant document
- Integration: `TestExportTreeRealData` (`export_tree_test.go`) — `writeExportTree` over real `docs/` (post-audit) succeeds; 15 provider docs; index lists 15 `tracked` providers

**Checkpoint:** `go test -race -run 'ExportTree|BuildAll|V1Index|RootIndex' ./...` passes; a temp-dir tree built from real `docs/` contains all published files.

### Slice 4: Fail-closed schema gate and published contract artifacts

**Observable outcome:** Six hand-authored draft 2020-12 schemas and `field-semantics.md` are committed under `docs/publish/`, copied verbatim into the staged tree at `v1/schemas/` + `v1/spec/`, and every generated JSON document validates against its published schema before anything leaves the temp dir — schema failure or provider-set mismatch aborts with a structured error. The stale, unenforced `docs/provider-capabilities/schema.json` is deleted.

**Interfaces introduced or modified:**

- `validateExportTree` — `func validateExportTree(treeDir string) error`, new file `export_gate.go`, using new direct dependency `github.com/santhosh-tekuri/jsonschema/v6` — **Deps:** `local-substitutable` (compiles the schema files from the staged tree itself, local resources only — never fetched; tests validate temp trees)
  - **Hides:** schema compilation with `$id`s (`https://openscribbler.github.io/capmon/v1/schemas/<name>.json`) resolved to local files; file→schema routing (per-provider → `provider-capabilities.json`, `all.json` → `all-providers.json`, pivots → `by-content-type.json`, `v1/index.json` → `index.json`, `advisories.json` → `advisories.json`, `spec/canonical-keys.json` → `canonical-keys.json`); root `index.json` and `field-semantics.md` are not schema-gated (no published schema by design — the root index shape is normatively append-only and byte-verified by the fixture)
  - **Exposes:** pass/fail: `EXPORT_002` naming file + first violation; identical code path locally and in CI
- `assertProviderSet` — `func assertProviderSet(exportedSlugs []string, sourcesDir string) error`, `export_gate.go` — **Deps:** `local-substitutable`
  - **Hides:** reading source-manifest slugs under `docs/provider-sources/` (the design doc's assertion source; `lexicon.md:80`)
  - **Exposes:** count + set equality; `EXPORT_003` on mismatch. Deliberately small — it is a gate component inside `export_gate.go`, not a standalone module.
- `writeExportTree` (modified) — staging flow gains: copy `docs/publish/schemas/*` → `v1/schemas/`, `docs/publish/spec/field-semantics.md` → `v1/spec/`, verbatim, before hashing (so schemas/spec join the `files` sha256 map); gate runs after documents are staged and before the index is finalized, then the finalized `v1/index.json` is itself validated — **Deps:** `local-substitutable`
  - **Hides:** ordering: build docs → copy assets → validate docs → hash → build + validate index
  - **Exposes:** unchanged signature

**Files:**

- `docs/publish/schemas/provider-capabilities.json` — hand-authored; recursive node definition; `key`-presence discriminator; `provider_exclusive` values use the same node definition minus `key`/`key_path`; freeze fields (`status`, `superseded_by`, `frozen_at`) OPTIONAL, `status` default `"live"`; `events`/`tools`/`refs`/`references` fully defined
- `docs/publish/schemas/all-providers.json` — hand-authored, `$ref`s the same node shapes
- `docs/publish/schemas/by-content-type.json` — hand-authored pivot schema
- `docs/publish/schemas/index.json` — hand-authored; freeze fields OPTIONAL; `source_commit` optional
- `docs/publish/schemas/advisories.json` — hand-authored per the design doc's advisory shape
- `docs/publish/schemas/canonical-keys.json` — hand-authored registry-document schema
- `docs/publish/spec/field-semantics.md` — hand-authored: dot-path key grammar `^[a-z_]+(\.[a-z_]+)*$`, vocabulary-member relationship, parent-`supported` auto-flip (`seed.go:91-92`), `conversion` enum, provenance of `mechanism` strings — verifying during authoring how much ACIF now owns
- `export_gate.go` — schema gate + provider-set assert
- `export_gate_test.go` — gate tests
- `docs/provider-capabilities/schema.json` — **deleted** (no Go code loads it; its confidence enum is wrong)
- `go.mod` — add `santhosh-tekuri/jsonschema/v6`

**Test cases:**

- Unit: `TestValidateExportTreePassesFixture` (`export_gate_test.go`) — a staged fixture tree validates; all six schemas (`provider-capabilities.json`, `all-providers.json`, `by-content-type.json`, `index.json`, `advisories.json`, `canonical-keys.json`) compile as draft 2020-12 from local files, via `export_gate.go`
- Unit: `TestValidateExportTreeFailsClosed` (`export_gate_test.go`) — corrupt one staged document (wrong type for `supported`) → `EXPORT_002` naming the file
- Unit: `TestAssertProviderSetMismatch` (`export_gate_test.go`) — missing or extra slug vs `docs/provider-sources/` fixtures → `EXPORT_003` with count + set detail
- Unit: `TestFreezeFieldsOptionalFromLaunch` (`export_gate_test.go`) — documents carrying `status: "frozen"`, `superseded_by`, `frozen_at` still validate against `provider-capabilities.json` and `index.json` schemas (freeze pre-provisioning, the part that cannot be retrofitted)
- Integration: `TestSpecArtifactsCopiedVerbatim` (`export_tree_test.go`, extended) — staged `v1/spec/field-semantics.md` and each `v1/schemas/*.json` are `bytes.Equal` to their committed sources under `docs/publish/`
- Manual: dependency and deletion hygiene
  1. `go build ./...` passes after the `go.mod` addition; `go mod tidy` is clean
  2. `grep -r "provider-capabilities/schema.json" --include='*.go' .` returns nothing, confirming `docs/provider-capabilities/schema.json` can be deleted with no dangling reference

**Checkpoint:** `go test -race -run 'ValidateExportTree|ProviderSet|FreezeFields|SpecArtifacts' ./...` passes; a deliberately corrupted temp tree is rejected before any output reaches a destination dir.

### Slice 5: capmon export subcommand with byte-stable output

**Observable outcome:** `capmon export --out dist --generated-at 2026-01-01T00:00:00Z --source-commit <sha>` produces the complete gated tree at `dist/`; running it twice yields byte-identical trees; a committed fixture with a synthetic provider byte-verifies `events`/`tools`/`refs`/`references`-trimming/`provider_exclusive` export code that real data cannot reach; a gate failure leaves any pre-existing `--out` tree untouched.

**Interfaces introduced or modified:**

- `RunExport` — `func RunExport(opts ExportOptions) error`, new file `export.go` — **Deps:** `local-substitutable` (all paths in `ExportOptions`; defaults: `CapsDir: "docs/provider-capabilities"`, `CanonicalKeysPath: "docs/spec/canonical-keys.yaml"`, `SourcesDir: "docs/provider-sources"`, `PublishAssetsDir: "docs/publish"`)
  - **Hides:** stage-to-temp-dir → `writeExportTree` + gate → atomic replace of `OutDir` only on full success (fail closed: a partial export never replaces a good tree); `GeneratedAt` default `time.Now().UTC()` RFC 3339 `Z` — the only permitted `time.Now()` in the export path, confined to `v1/index.json`; `SourceCommit` omitted when empty (export stays git-free)
  - **Exposes:** the single exporter entry point used identically by the CLI, tests, and `publish.yml` — no workflow-only validation path
- `exportCmd` — new file `cmd/capmon/capmon_export_cmd.go`, self-registering `init()`; flags `--out` (default `dist`), `--source-commit`, `--generated-at` (validated RFC 3339 UTC `Z`) — **Deps:** `local-substitutable` (package-level override vars for test redirection, per house pattern)
  - **Hides:** flag parsing/validation, `ExportOptions` assembly
  - **Exposes:** `capmon export`; errors via `RunE` → structured stderr + exit 1

**Files:**

- `export.go` — `RunExport` orchestrator + `ExportOptions` defaults
- `export_test.go` — orchestrator tests
- `cmd/capmon/capmon_export_cmd.go` — subcommand
- `cmd/capmon/capmon_export_cmd_test.go` — CLI tests
- `testdata/fixtures/export/` — fixture input: a minimal realistic provider + a synthetic provider exercising `events`, `tools`, node-level `refs`, `references` (trimmed to `{url, verified_at}`), and `provider_exclusive`; a trimmed fixture registry; matching synthetic source manifests (so the provider-set assert passes); trimmed fixture publish assets
- `testdata/fixtures/export/expected/` — committed byte-exact generated documents (generated docs only; copied schema/spec assets are asserted `bytes.Equal` to their `docs/publish/` sources rather than committed twice)
- `export_fixture_test.go` — fixture drift guard + determinism tests

**Test cases:**

- Integration: `TestExportFixtureMatchesCommitted` (`export_fixture_test.go`) — `RunExport` over `testdata/fixtures/export` with pinned `GeneratedAt`/`SourceCommit`; every generated file `bytes.Equal` to `testdata/fixtures/export/expected/` (the drift-guard pattern); the synthetic provider's doc carries trimmed `references`, mirrored `events`/`tools`/`refs`, and keyless `provider_exclusive` nodes
- Integration: `TestDoubleExportDeterminism` (`export_fixture_test.go`) — two `RunExport` runs over real `docs/` with pinned flags → trees byte-identical file-for-file; runs under `go test -race ./...` in `ci.yml` on every PR
- Integration: `TestGeneratedAtConfinement` (`export_fixture_test.go`) — two runs differing only in `GeneratedAt` differ in exactly one file: `v1/index.json`
- Unit: `TestRunExportFailClosedPreservesOut` (`export_test.go`) — with a corrupt input triggering the gate, a pre-populated `OutDir` is left byte-identical via `export.go`
- Unit: `TestExportCmdFlags` (`cmd/capmon/capmon_export_cmd_test.go`) — defaults, invalid `--generated-at` rejected before any I/O, structured error output captured with `output.SetForTest(t)`, exercising `capmon_export_cmd.go`

**Checkpoint:** `go run ./cmd/capmon export --out /tmp/site --generated-at 2026-01-01T00:00:00Z` exits 0 and `/tmp/site/v1/index.json` exists; `go test -race ./...` passes.

### Slice 6: Export conformance verification against the published site

**Observable outcome:** `capmon export --verify <commit>` rebuilds the tree from that source commit and diffs it file-by-file against the live published site over HTTPS, exiting 0 on byte-identity and failing with `EXPORT_004` naming the first divergent path otherwise — a maintainer/consumer conformance tool, never part of the publish gate.

**Interfaces introduced or modified:**

- `RunExportVerify` — `func RunExportVerify(commit, baseURL string) error`, new file `export_verify.go` — **Deps:** `true-external` (HTTPS to GitHub Pages + `git` binary; tests substitute an `httptest.Server` and a temp `git init` repo — real network only in manual use)
  - **Hides:** materializing `docs/` at `<commit>` via `git archive <commit> -- docs/` into a temp dir (`--verify` is the only git-touching export path; plain export stays git-free); fetching the live `v1/index.json` first and rebuilding with `GeneratedAt`/`SourceCommit` taken from it, so the comparison is total byte equality on every file including `v1/index.json`; per-file fetch + `bytes.Equal`
  - **Exposes:** match/mismatch with the divergent path; `EXPORT_004`
- `exportCmd` (modified, `cmd/capmon/capmon_export_cmd.go`) — adds `--verify <commit>` (switches mode; `--out` ignored) and `--base-url` (default `https://openscribbler.github.io/capmon/`, overridable for tests) — **Deps:** `local-substitutable`
  - **Hides:** mode selection
  - **Exposes:** one subcommand, two modes, per the design's flag surface

**Files:**

- `export_verify.go` — rebuild-and-diff implementation
- `export_verify_test.go` — httptest + temp-git-repo tests
- `cmd/capmon/capmon_export_cmd.go` — `--verify`/`--base-url` flags

**Test cases:**

- Integration: `TestExportVerifyMatch` (`export_verify_test.go`) — temp git repo containing the fixture `docs/`; `httptest.Server` serving a `RunExport` build of the same commit; `RunExportVerify` reports match, via `export_verify.go`
- Integration: `TestExportVerifyMismatchFailsClosed` (`export_verify_test.go`) — server returns one altered per-provider document → `EXPORT_004` naming its path
- Unit: `TestExportVerifyFlagWiring` (`cmd/capmon/capmon_export_cmd_test.go`) — `--verify` routes to `RunExportVerify` with `--base-url` passed through; normal mode unaffected, exercising `capmon_export_cmd.go`

**Checkpoint:** `go test -race -run ExportVerify ./...` passes (fully offline via httptest).

### Slice 7: Attested publish workflow and scheduled-job hardening

**Observable outcome:** A push to `main` touching `docs/**` (or a dispatch) runs `publish.yml`: build → `capmon export` (fail-closed gate inside the binary) → `upload-pages-artifact` → `attest-build-provenance` over both discovery indexes → `deploy-pages`, all SHA-pinned; the daily cron re-runs the full gate as a health check and skips deploy when `data_revision` matches the live index. The pipeline heartbeat re-enables the workflow via the Actions API with `actions: write` only — no scheduled job holds `contents` permission on `main`. README documents the consumer contract. A drift guard keeps the permission posture honest.

**Interfaces introduced or modified:**

- `.github/workflows/publish.yml` — new workflow; triggers `push` (branches `[main]`, paths `['docs/**']`), `schedule` (daily, offset from the pipeline cron), `workflow_dispatch`; top-level `permissions: contents: read`; single publish job with `pages: write`, `id-token: write`, `attestations: write`; `concurrency: pages` — **Deps:** `true-external` (GitHub Actions/Pages/attestation infrastructure; verified by manual dispatch + the drift guard)
  - **Hides:** checkout with `fetch-depth: 0` (required for `git log -1 --format=%H -- docs/` — never `HEAD`); `setup-go` via `go-version-file: go.mod` (determinism envelope); `go build -o capmon-bin ./cmd/capmon`; `./capmon-bin export --out dist --source-commit "$SOURCE_COMMIT"`; schedule-only step comparing `jq -r .data_revision dist/v1/index.json` against the live index and short-circuiting deploy on equality (push/dispatch always deploy — contract-artifact changes must publish even when `data_revision` is unchanged); attestation `subject-path` = `dist/index.json` + `dist/v1/index.json` (the integrity chain roots); every action pinned to a full commit SHA with version comment
  - **Exposes:** the fail-closed publish path as one self-contained unit, structurally incapable of running on a pull request; `ci.yml` unchanged
- `.github/workflows/pipeline.yml` heartbeat job (rewritten) — permissions shrink from `contents: write` to `actions: write`; steps: compose the run manifest summary, upload it as a workflow artifact (run-manifest observability moves off the repo; delete the committed `.heartbeat/` directory if present), then `gh api -X PUT repos/${{ github.repository }}/actions/workflows/pipeline.yml/enable` — **Deps:** `true-external`
  - **Hides:** the 60-day scheduled-workflow disablement reset mechanics; documented fallback is the orphan-branch variant (ADR 0005 holds under both)
  - **Exposes:** a scheduled pipeline that holds zero `contents` permission in its keepalive
- Workflow-posture drift guard — new file `workflows_audit_test.go` (root package, parses the workflow YAML in the checkout) — **Deps:** `in-process`
  - **Hides:** YAML traversal of job permissions, triggers, and `uses:` pins
  - **Exposes:** ADR 0004/0005 invariants as CI-enforced tests

**Files:**

- `.github/workflows/publish.yml` — new publish workflow
- `.github/workflows/pipeline.yml` — heartbeat rewrite only; all other jobs untouched
- `workflows_audit_test.go` — permission/pinning drift guard
- `README.md` — new consumer-contract section: fetch flow (root index → `v1/index.json` → `data_revision` compare → per-file `sha256` → documents), mandatory fail-closed `gh attestation verify` for integrity-sensitive consumers, staleness/polling rules (conditional GET, at-most-daily, `max_staleness_hours`, re-fetch-on-mismatch window), pointer to `v1/spec/field-semantics.md`

**Test cases:**

- Unit: `TestPublishWorkflowHardening` (`workflows_audit_test.go`) — parses `publish.yml`: top-level `contents: read`; no `pull_request` trigger; job permissions exactly `{pages: write, id-token: write, attestations: write}`; every `uses:` matches `@[0-9a-f]{40}`
- Unit: `TestHeartbeatHoldsNoContentsWrite` (`workflows_audit_test.go`) — parses `pipeline.yml`: heartbeat job permissions are exactly `{actions: write}`; no step in the heartbeat job pushes to git
- Manual: first-publish verification
  1. Repo hardening lands first (ADR 0005 precondition): branch protection + required review on `main`; Pages source set to GitHub Actions
  2. `workflow_dispatch` `publish.yml`; the job builds, exports, gates, attests, and deploys
  3. `curl -fsSL https://openscribbler.github.io/capmon/v1/index.json` returns `data_revision` + 15 tracked providers; a fetched `capabilities/claude-code.json` hashes to its index `sha256`
  4. Follow the `README.md` consumer-contract section end-to-end, including `gh attestation verify` against the downloaded `v1/index.json`
  5. Trigger the schedule path (or dispatch after a no-op) and confirm the run ends without deploying when `data_revision` is unchanged

**Checkpoint:** `go test -race -run 'PublishWorkflowHardening|HeartbeatHoldsNoContentsWrite' ./...` passes; a dispatched `publish.yml` run deploys the site and `gh attestation verify` succeeds against the live `v1/index.json`.

## Acceptance

- Full `/v1/` tree published: 15 per-provider docs, `all.json`, six by-content-type pivots, `spec/canonical-keys.json` + `spec/field-semantics.md`, six `schemas/`, `v1/index.json`, `advisories.json`, root `index.json` — `TestWriteExportTreeLayout`, `TestExportTreeRealData`, Slice 7 manual step 3
- Every generated JSON document validates against its published schema, in-process, before leaving the temp dir — `TestValidateExportTreePassesFixture`, `TestValidateExportTreeFailsClosed`
- Committed byte-level fixture + double-export diff clean; only `generated_at` varies between data-identical runs — `TestExportFixtureMatchesCommitted`, `TestDoubleExportDeterminism`, `TestGeneratedAtConfinement`
- Fail-closed: schema failure, provider-set mismatch, or non-canonical node aborts with `EXPORT_00x`, existing output untouched, no deploy — `TestBuildProviderDocNonCanonicalNodeFailsClosed`, `TestAssertProviderSetMismatch`, `TestRunExportFailClosedPreservesOut`
- All ~31 non-canonical nodes relocated to `provider_exclusive` before first publish; invariant CI-enforced — `TestAllBaselinesBuildProviderDocs`
- Deploy via `upload-pages-artifact` + `deploy-pages` pinned by SHA with `attest-build-provenance` over both indexes — `TestPublishWorkflowHardening`, Slice 7 manual step 4
- Heartbeat off `main` per ADR 0005 (`actions: write` only) with branch protection as a launch precondition — `TestHeartbeatHoldsNoContentsWrite`, Slice 7 manual step 1
- Typed `ProviderExclusive` shapes exported JSON and its published schema — `TestProviderExclusiveRoundTrip`, `TestBuildProviderDocFallbacksAndTrimming`, synthetic-provider fixture bytes
- README consumer contract + published `v1/spec/field-semantics.md` — `TestSpecArtifactsCopiedVerbatim`, Slice 7 manual step 4
- NFR: pinned canonicalization profile centralized at one call site (sorted keys, LF, two-space indent, no HTML escaping, raw UTF-8, integers only) — `TestCanonicalJSONProfile`
- NFR: export shape + schemas fully cover `events`/`tools`/`refs`/`references` even though real data is empty today — synthetic provider in `TestExportFixtureMatchesCommitted` + `TestFreezeFieldsOptionalFromLaunch`
- `go build ./...`, `go vet ./...`, `go test -race ./...` all pass; every new check rides `ci.yml`'s existing command set (no Makefile)

## Out of Scope

- `capmon freeze` command — deferred until a v2 approaches; only the OPTIONAL freeze fields ship in v1 schemas (accepted design, "Deferred").
- Phase 4b internals: FormatDoc `schema_version`, manifest enums (`status`/`blocking`/`fetch_tier`) — internal pipeline state; `EventEntry.Blocking` publishes as a plain string.
- Publishing seeder specs, format docs, or source manifests — internal pipeline state, never published (`docs/design/publish-layer.md:24-27`).
- Historical archive on the site — git history is the archive (ADR 0001).
- Source-data backfill of `display_name`/`last_verified` and the flat-leaf reconciliation — pre-launch data audits, tracked separately; exporter behavior (fallback/omission) makes publication correct regardless.
- Source-YAML strict-parse hardening (yaml.v3 `KnownFields`) — not requested and not needed for the publish gate.
- syllago-side consumer changes — downstream flips to the published bundle after this ships.
- `cmd/provider-monitor` — untouched (separate flag-based binary).
