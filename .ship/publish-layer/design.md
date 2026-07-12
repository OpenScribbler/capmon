# Design Discussion: publish-layer

Composes from the ACCEPTED normative spec `docs/design/publish-layer.md` (consumer
contract, export surface, versioning, determinism profile, publish mechanics) and
ADRs 0001–0005 (`docs/adr/INDEX.md`). Nothing below contradicts them; decisions
already settled there are listed under "Decisions made", not re-opened.

## Summary

**Current state:** Provider capability data exists only as YAML in the repo (15 capability baselines + the 46-key canonical-key registry); consumers must clone capmon, there is no `capmon export`, no published JSON, and the one generated view embeds a run timestamp that makes even repeated local runs byte-different (`generate.go:52-53`).
**Desired state:** `capmon export` compiles the capability baselines joined with the canonical-key registry into a deterministic, self-describing `/v1/` JSON tree that a fail-closed, SHA-pinned, attestation-emitting workflow publishes to GitHub Pages.
**End state (narrative):** A consumer fetches `v1/index.json`, compares `data_revision` for one-field change detection, and pulls `capabilities/<slug>.json`, where every registry-backed node self-describes via inlined `key_path` + `key` metadata (ADR 0002). Integrity-sensitive consumers (syllago) verify the provenance attestation, then per-file `sha256`, failing closed on mismatch (ADR 0004). The maintainer runs `capmon export` locally and gets byte-identical output for identical source data; a bad export never replaces a good site.

## Research questions answered

1. How does `GenerateContentTypeViews` in `generate.go` join capability baselines with the canonical-key registry — iteration order, key-matching logic, recursion over nested capabilities, and output writing — and which helpers are shared/exported vs private?
2. What is the exact loaded struct shape in capyaml (`ProviderCapabilities`, `ContentTypeEntry`, `CapabilityEntry`, `EventEntry`, `ToolEntry`, `ReferenceEntry`): all fields, YAML tags, omitempty flags; how is `provider_exclusive` typed; and what value shapes actually appear under `provider_exclusive`, `events`, `tools`, `refs`, and `references` across the 15 files in `docs/provider-capabilities/`?
3. How are cobra subcommands registered in `cmd/capmon/` — file layout, flag conventions, exit-code conventions — and how does the existing verify command perform schema validation (library, where schemas live, embed vs file load)?
4. What fields does each entry in `docs/spec/canonical-keys.yaml` carry, what top-level metadata does the file have, and which code paths currently parse it or look up keys by dot-path?
5. What do `.github/workflows/pipeline.yml` and `ci.yml` currently do: triggers, permissions blocks, which steps commit to main (heartbeat), how actions are pinned, and what job/step structure exists for adding a publish job?
6. What JSON-encoding and determinism utilities already exist in the repo (sorted-key marshaling, `SetEscapeHTML` usage, golden/fixture byte-comparison tests), and where is the Go toolchain version pinned (go.mod, CI workflow files)?
7. Across `docs/provider-capabilities/*.yaml`: which providers have empty or missing `display_name` or `last_verified`; which have non-empty `events`, `tools`, `refs`, or `references`; and which nodes under object-typed canonical keys are flat leaves rather than parents of nested capabilities?
8. What test conventions does the repo use: testdata directory layout, golden-file patterns, table-driven test style, make targets or CI commands that run build and tests?

## Patterns to Follow

### Pattern: one file per cobra subcommand, self-registering `init()`

**Source:** `cmd/capmon/capmon_derive_cmd.go:71` (registration), `cmd/capmon/capmon_derive_cmd.go:11-15` (test override vars)

**Snippet:**
```go
func init() {
    capmonCmd.AddCommand(deriveCmd)
}
```

**Why it applies here:** `capmon export` is a new subcommand and follows the established shape: its own `capmon_<name>_cmd.go` + paired `_test.go`, local flags defined in `init()` (`cmd/capmon/capmon_check_cmd.go:51-57` shows literal flag defaults, the preferred of the two coexisting default styles), package-level path-override vars for tests (`cmd/capmon/capmon_cmd.go:30`), slug handling via `capmon.SanitizeSlug` (`cmd/capmon/capmon_cmd.go:139-144`), and structured errors via `output.NewStructuredError` with namespaced codes (`internal/output/errors.go:5-11`). Errors return through `RunE` → exit 1 (`cmd/capmon/main.go:14-17`); export does not need the pipeline exit classes.

### Pattern: drift guard — regenerate and `bytes.Equal` against the committed tree

**Source:** `seederspec_audit_test.go:77-121` (comparison at `:121`)

**Snippet:**
```go
if !bytes.Equal(got, committed) {
    t.Errorf("derived output for %s differs from committed file", path)
}
```

**Why it applies here:** The committed byte-level export fixture and the determinism guarantee reuse this exact pattern: regenerate from `testdata` input, compare byte-for-byte against committed expected output — no `.golden` framework exists or is needed (research Q8). Repo-root resolution via `docsRoot(t)` / `canonicalKeysPath(t)` (`docsroot_test.go:15-28`); table-driven style where applicable (`sanitize_test.go:10`); `output.SetForTest(t)` for CLI output capture (`internal/output/output.go:34-58`).

### Pattern: SHA-pinned actions + hash-verified artifact handoff

**Source:** `.github/workflows/pipeline.yml:64` (pinning), `:49-50` + `:86-93` + `:117-132` (SHA-256 job-output verification)

**Snippet:**
```yaml
- uses: actions/checkout@de0fac2e... # v6.0.2
```

**Why it applies here:** The publish workflow pins `actions/upload-pages-artifact`, `actions/deploy-pages`, and `actions/attest-build-provenance` by full commit SHA with version comments, exactly as every existing action is pinned; job-level permission escalation over a `contents: read` top level (`pipeline.yml:32-33`, `:42-45`) is the permissions model to copy. Go setup resolves the toolchain from `go.mod` via `go-version-file` (`ci.yml:20-22`; `go.mod:3-5` pins `go 1.26` / `toolchain go1.26.3`) — this is part of the determinism envelope.

### Pattern: CI command set is canonical — there is no Makefile

**Source:** `.github/workflows/ci.yml:24-50`

**Snippet:**
```yaml
- run: go build ./...
- run: go vet ./...
- run: go test -race ./...
```

**Why it applies here:** All new checks (export drift guard, double-export determinism test, schema-gate tests) must be reachable through `go test ./...` so `ci.yml`'s existing command set exercises them without new plumbing; the gofmt and legacy-name gates also apply to all new code.

### Pattern (negative): the generated-view banner timestamp is the anti-pattern

**Source:** `generate.go:52-53`

**Snippet:**
```go
fmt.Fprintf(&buf, "# Generated: %s\n", time.Now().UTC().Format(time.RFC3339))
```

**Why it applies here:** This is the one run-varying byte source in the existing generation path and the exact thing the canonicalization profile forbids: no timestamp or run-varying value in any exported document except `generated_at` in `v1/index.json` (`docs/design/publish-layer.md:306-309`). The exporter must never call `time.Now()` in a document body.

### Pattern (constraint): a new registry loader — `loadCanonicalKeys` discards what export needs

**Source:** `formatdoc_validate.go:79-103`

**Snippet:**
```go
type canonicalKeysFile struct {
    ContentTypes map[string]map[string]interface{} `yaml:"content_types"`
}
```

**Why it applies here:** The only existing registry parser returns bare per-content-type key-name sets, discarding `description`/`type`/`spec_ref` — the exact fields ADR 0002 requires inlined at every registry-backed node. Export gets its own loader that preserves all three fields per entry (`docs/spec/canonical-keys.yaml:54-61` documents the entry shape); `loadCanonicalKeys` and its consumers (`ValidateFormatDoc`, `DeriveSeederSpecs`, `RunCapmonCheck`) stay untouched. No existing code looks keys up by full dot-path (research Q4), so dot-path assembly (`<content_type>.<key>` → `key_path`) is new export logic.

### Pattern (constraint): capyaml structs are the source shape the exporter mirrors

**Source:** `capyaml/types.go:4-63`, loader `capyaml/load.go:12-22`

**Snippet:**
```go
Capabilities map[string]CapabilityEntry `yaml:"capabilities,omitempty"` // recursive
```

**Why it applies here:** Export loads baselines with `capyaml.LoadCapabilityYAML` (same entry point as `generate.go:30` and `capmon verify`), skips seed YAMLs by the established `Slug == ""` test (`generate.go:34-37`), and mirrors the recursive `CapabilityEntry` tree into the recursive published tree. The confirmed sign-off decisions bind the mapping: `events`/`tools`/`references` mirror source structure with `references` trimmed to `{url, verified_at}` (dropping `fetch_method`, `last_content_hash` — `capyaml/types.go:20-25`); maps are omitted when empty but fully defined in published schemas; `supported` stays plain `bool` in capyaml and the exporter always emits it at every node.

### Disambiguation: Heartbeat keepalive via Actions API vs orphan branch vs tag

**Chosen:** Actions API keepalive — the heartbeat job re-enables the scheduled workflow via `gh api -X PUT repos/{owner}/{repo}/actions/workflows/pipeline.yml/enable` with job permissions `actions: write` only, replacing the commit-to-main step (`.github/workflows/pipeline.yml:233-260`)
**Considered:** Orphan-branch commit (keeps a commit trail but the scheduled job retains `contents: write`, relying on branch protection alone to keep it off `main`); force-moved git tag (same retained `contents: write`, plus tag-namespace pollution)
**Why:** ADR 0005 pins only "no scheduled job holds write access to the publish branch"; the API keepalive is the strongest satisfaction — the scheduled workflow holds no `contents` permission at all, so there is nothing for branch protection to catch. GitHub's 60-day scheduled-workflow disablement is reset by re-enabling the workflow, which is exactly and only what the heartbeat exists to do (`lexicon.md:46`); the commit was always a means, not the end.
**Consequences:** The heartbeat job's permissions shrink from `contents: write` (`pipeline.yml:239-242`) to `actions: write`; `.heartbeat/last-run.json` stops being committed, so the run manifest's repo-visible copy goes away — run-manifest observability moves to a workflow artifact and run logs (it is write-only observability, never a pipeline input, per `lexicon.md:45`). If GitHub ever changes the enable-endpoint clock-reset behavior, the documented fallback is the orphan-branch variant; the ADR 0005 constraint holds under both.

### Disambiguation: JSON Schema validation via santhosh-tekuri/jsonschema vs alternatives vs hand-rolled

**Chosen:** `github.com/santhosh-tekuri/jsonschema/v6` as a new direct dependency, compiling the published schema files themselves (draft 2020-12) with `$id`s resolved to the local export tree — never fetched over the network
**Considered:** `xeipuuv/gojsonschema` (unmaintained, no draft 2020-12); `kaptinlin/jsonschema` (younger, far less battle-tested); hand-rolled validation extending the existing approach, which is enum-check + struct parse only (`capyaml/validate.go:13-45`) and enforces none of the published schemas' required/enum/type constraints
**Why:** The fail-closed gate's promise (ADR 0004; `docs/design/publish-layer.md:280-284`) is "every output file validates against its **published** schemas" — that is only true if the gate interprets the actual schema files consumers vendor, not a parallel Go re-implementation that can drift from them. No schema library exists in `go.mod` today (research Q3, `go.mod:7-16`); santhosh-tekuri v6 is the maintained, pure-Go, 2020-12-conformant standard choice and the schemas' declared dialect matches the existing (unused) source schema's draft (`docs/provider-capabilities/schema.json:1-8`).
**Consequences:** One new direct dependency in `go.mod`. The gate validates every generated file against its schema in-process before anything leaves the temp dir, both in `capmon export` itself and therefore identically in local runs and CI — the same code path, no workflow-only validation. Schema compilation must use local resources exclusively so export works offline and deterministically.

### Disambiguation: Canonical JSON via map-built documents + one configured encoder vs struct-tag marshaling vs hand-rolled encoder

**Chosen:** Export documents are built as `map[string]any` trees (keys inserted only when present, satisfying "maps omitted when empty" and conditional fields naturally) and serialized by a single shared canonical-writer helper: `json.Encoder` with `SetEscapeHTML(false)` and `SetIndent("", "  ")`, whose `Encode` emits the trailing LF
**Considered:** Struct types with JSON tags (encoding/json emits struct fields in declaration order, not sorted — every struct edit risks silently violating the sorted-keys rule, and `omitempty` cannot express "always emit `supported: false`"); a hand-rolled canonical encoder (reimplements string escaping and UTF-8 handling for no gain)
**Why:** `encoding/json` already sorts map keys ascending byte-wise, which for UTF-8 is exactly ascending Unicode code point — the profile's rule (`docs/design/publish-layer.md:301-305`) — and the repo has zero existing canonicalization helpers or `SetEscapeHTML` usage to build on (research Q6). Centralizing the entire profile (UTF-8, LF, trailing newline, two-space indent, sorted keys, no HTML escaping) in one helper makes "changing the profile is a breaking change" auditable at a single call site.
**Consequences:** Compile-time shape safety is traded away for maps; it is recovered by the fail-closed schema gate validating every output plus the committed byte-level fixture. Numeric values must be inserted as Go `int` (never `float64` — profile says integers only); the helper is the sole JSON serialization path for the export tree. The indirect `go-json-experiment/json` dependency (`go.mod:22`, via chromedp) is not adopted.

### Disambiguation: Typed ProviderExclusive as map[string]CapabilityEntry vs dedicated struct vs status quo

**Chosen:** Retype `ProviderCapabilities.ProviderExclusive` from `map[string]interface{}` (`capyaml/types.go:15`) to `map[string]CapabilityEntry` — provider-exclusive capabilities are ordinary capability nodes (disposition + optional nested capabilities) that simply have no canonical key yet; export emits them with the same node shape as the capability tree, definitionally never carrying `key`/`key_path`
**Considered:** A dedicated `ProviderExclusiveEntry` struct with extra bookkeeping fields (speculative — no data exists to justify any extra field); keeping `map[string]interface{}` and exporting opaquely (abdicates the "typed ProviderExclusive shapes exported JSON" requirement and leaves the published schema unable to say anything)
**Why:** The lexicon defines a provider-exclusive capability as a provider behavior with no canonical key that graduates to one when 2+ providers converge (`lexicon.md:28`) — i.e., the same concept as a capability node, pre-registration. Reusing `CapabilityEntry` means graduation is a data move, not a shape change; consumers reuse one tree-walking code path; and since `provider_exclusive` appears in zero baselines today (research Q2), the retyping has no migration cost.
**Consequences:** The published provider-capabilities schema models `provider_exclusive` values with the same recursive node definition as capability nodes, minus the key-metadata fields; the `key`-presence discriminator (ADR 0002) stays globally consistent — no `provider_exclusive` node ever has `key`. If a future exclusive needs non-disposition payload, that is a shape change with a `schema_version` bump on the owning schema.

### Disambiguation: New publish workflow vs extending ci.yml

**Chosen:** A new dedicated publish workflow (`publish.yml`) with triggers `push` to `main` filtered to `docs/**`, a daily `schedule`, and `workflow_dispatch`; job-level `pages: write`, `id-token: write`, `attestations: write` over a `contents: read` top level
**Considered:** Extending `ci.yml`, which the design doc's parenthetical "(from `ci.yml`)" suggests (`docs/design/publish-layer.md:275`) — but `ci.yml` runs on every `pull_request` with top-level `contents: read` only (`ci.yml:3-9`), has no `schedule` trigger for the daily re-stamp, and mixing deploy/attestation permissions into a PR-triggered workflow enlarges the attack surface ADR 0004 (strict, scope `.github/workflows/*.yml`) exists to keep auditable
**Why:** The trigger semantics the design doc pins (push to `main` touching `docs/`, plus a daily run that skips deploy when `data_revision` is unchanged) are preserved exactly; only the host file differs. A dedicated workflow makes the fail-closed publish path — build, export, gate, upload-pages-artifact, attest, deploy — one self-contained, SHA-pinned unit that is structurally incapable of running on a pull request, and keeps `ci.yml` permanently read-only.
**Consequences:** The `go build` step is duplicated across workflows (already true of `pipeline.yml`'s four jobs; cheap). The daily cron in `publish.yml` runs the full export + fail-closed gate as a daily health check and compares the computed `data_revision` against the live `v1/index.json`, deploying only on difference — per the accepted mechanics, an unchanged-data cron run ends without deploying, so `generated_at` advances only on actual publishes. `pipeline.yml` is touched only for the heartbeat relocation; `ci.yml` is not modified.

### Disambiguation: Delete stale source-side schema.json vs rewrite and enforce it

**Chosen:** Delete `docs/provider-capabilities/schema.json`
**Considered:** Rewriting it to match reality (confidence enum `confirmed/inferred`, add `confidence` + recursive `capabilities` to its CapabilityEntry model) and wiring it into `capmon verify` via the new jsonschema dependency
**Why:** No Go code loads it (research Q3 — repo-wide grep for the path and for any schema library returns nothing), and it is wrong: its confidence enum says `high/medium/low` while every baseline uses `confirmed/inferred` (`lexicon.md:26`). Phase 4 creates the authoritative, machine-enforced schema for this shape at `v1/schemas/provider-capabilities.json`; keeping a second, near-identical schema for the YAML side is a standing drift liability with no enforcing consumer. Source-side validation remains `capyaml.ValidateAgainstSchema`'s version-gate + struct parse (`capyaml/validate.go:13-45`), which is what actually runs today.
**Consequences:** Source YAML gains no new validation in this feature (hardening like yaml.v3 `KnownFields` is explicitly out of scope); if source-side JSON Schema validation is ever wanted, it should be derived from the published schema, not maintained in parallel. Nothing references the deleted file's `$id`.

## Design Questions

1. **Pre-launch data audit: where do the non-canonical nodes in canonical-key position go?** Research found ~31+ capability nodes that are direct children of a content type's `capabilities` map but are not registry keys — codex has 18 (skill-frontmatter mirrors like `tools`, `transport`, `url`; `docs/provider-capabilities/codex.yaml:156-171`), claude-code 9 (`skills.frontmatter`, `skills.live_reload`, …), pi 4 camelCase (`skills.baseDir`, …), plus flagged nodes in amp (`amp.yaml:72`) and cline (`cline.yaml:84`). Since export fails closed on these (see Decisions), the audit must relocate every one before first publish.
   - A) Move them under `provider_exclusive` (newly typed), which is definitionally where keyless provider behaviors live until 2+ providers converge
   - B) Register the recurring concepts as new canonical keys in `docs/spec/canonical-keys.yaml` (requires ACIF `spec_ref` work — capmon conforms to ACIF, never the reverse)
   - C) <free-form: per-node triage mix, or another disposition>
   - **Recommended:** A — it matches the lexicon's graduation model exactly and requires no ACIF registry changes to launch; individual keys can still graduate later via B when convergence appears.
   - **RESOLVED (user, 2026-07-11): A.** All non-canonical nodes in canonical-key position relocate under the newly typed `provider_exclusive`. The audit is a launch-blocking work item within this feature.

## Decisions made (not questions)

Settled upstream — recorded here so Structure does not re-open them:

- **Version the contract, not the data**; `/v1/` URL major, per-schema informational `schema_version`, `data_revision`, `generated_at` — ADR 0001; `docs/design/publish-layer.md:156-169`.
- **Self-describing documents**: `key_path` + `key` (`description`/`type`/`spec_ref`) at every registry-backed node; `key`-presence discriminates canonical keys from vocabulary members — ADR 0002.
- **Stable mutable paths**, no content addressing; CDN skew handled by the re-fetch rule — ADR 0003.
- **Attestation at launch + fail-closed publishing**; hashes are change-detection only — ADR 0004.
- **No scheduled write to the publish branch**; branch protection + required review on `main` land before first publish — ADR 0005; `docs/design/publish-layer.md:387-401`.
- Slug permanence + tombstones, advisories as sole post-freeze mutation, freeze fields pre-provisioned OPTIONAL in v1 schemas, `capmon freeze` deferred — `docs/design/publish-layer.md:75-79`, `:186-232`, `:403-407`.
- CONFIRMED at sign-off: exported `events`/`tools`/`references` mirror source structure; `references` trimmed to `{url, verified_at}` only; maps omitted when empty but fully defined in published schemas.
- CONFIRMED at sign-off: `supported` stays plain `bool`; exporter always emits it at every node; absent-means-unknown is a consumer-side rule only.

Decided in this discussion:

- **Export fails closed on non-canonical nodes in canonical-key position.** A direct child of `content_types.<ct>.capabilities` whose name is not a registry key for that content type aborts the export with a structured error naming provider + node. Publishing such nodes would make the normative `key`-presence discriminator (`docs/design/publish-layer.md:94-98`) mislabel them as vocabulary members of a nonexistent parent key; fail-closed publishing exists precisely to stop contract-violating output. Descendants of any registry-backed key are open vocabulary and are never name-checked (member semantics live in the parent key's `description`/`spec_ref`). Launch therefore depends on the Design Question 1 audit.
- **`display_name` falls back to slug in export — mandatory today**, since all 15 baselines carry `display_name: ""` (research Q7; normative fallback at `docs/design/publish-layer.md:364-367`). Published documents never have an empty `display_name`.
- **Empty `last_verified` is omitted from published output** (all 15 baselines carry `""` today); the published schemas make it optional, and absence means "never verified" — consistent with the consumer contract's absent-means-unknown posture. Source backfill is a data follow-up, not a code dependency.
- **opencode publishes with `content_types` omitted** (`docs/provider-capabilities/opencode.yaml:5` is `content_types: {}`): the confirmed empty-map-omission rule applies uniformly, and the published provider-capabilities schema does not require `content_types`. The document still publishes — slug permanence applies from first publish.
- **Flat leaves under object-typed keys export as-is** with their `key` metadata; the normative "`key.type` does not predict JSON shape — inspect structurally" rule covers consumers (`docs/design/publish-layer.md:99-100`). The 14/15-provider flat-leaf inventory (research Q7) feeds the pre-launch data audit already tracked by the accepted design (`:379-385`); export does not block on it.
- **Provider-set assertion**: the fail-closed gate asserts exported slugs (count + set) equal the slugs of the **source manifests** under `docs/provider-sources/` (the design doc's assertion source; `lexicon.md:80` disambiguates the three "manifest" senses). Baselines are discovered by the established `Slug != ""` filter.
- **`data_revision` = sha256 of the exported `capabilities/all.json` bytes.** all.json is exactly "provider data" in its published form, is deterministic (no `generated_at`), and correctly captures registry-metadata edits — which change what consumers read and therefore are data changes from the consumer's side. One field compare answers "did anything change?".
- **Per-file `sha256` values in `v1/index.json`** are computed over the exact bytes staged for deploy, after the schema gate passes.
- **Attestation subjects are `v1/index.json` and the root `index.json`.** The integrity chain is: attestation → index → per-file `sha256` → files (`docs/design/publish-layer.md:139-143`); attesting the indexes roots the chain without attesting every file.
- **`--source-commit` and `--generated-at` are exporter flags.** The publish workflow supplies `--source-commit "$(git log -1 --format=%H -- docs/)"` (never HEAD; `docs/design/publish-layer.md:253-255`); tests and the byte-level fixture supply both flags fixed, which is what makes the CI double-export diff fully clean and the fixture byte-stable. Defaults: `generated_at` = now (the sole run-varying value, confined to `v1/index.json`); `source_commit` omitted when not supplied (optional in the index schema), keeping `capmon export` git-free.
- **The double-export determinism check is a Go test** (export twice to temp dirs with pinned flags, compare trees byte-for-byte), so it runs under `go test -race ./...` in `ci.yml` on every PR — plus the committed byte-level fixture as the drift guard against the expected bytes themselves.
- **The byte-level fixture includes a synthetic provider** exercising `events`, `tools`, node-level `refs`, `references` (trimming to `{url, verified_at}`), and `provider_exclusive` — all empty in real data today (research Q2/Q7) but fully schema-defined per the confirmed decision; without a synthetic fixture none of that export code would be byte-verified.
- **The six published schemas and `v1/spec/field-semantics.md` are hand-authored, committed artifacts** that `capmon export` copies verbatim into the tree (and validates outputs against, for the schemas). Schemas are the contract and are written fresh from the design doc — deliberately authored, not generated from Go types. Schema `$id`s take their final published form (`docs/design/publish-layer.md:52-56`); freeze fields (`status`, `superseded_by`, `frozen_at`) are OPTIONAL from initial publication with `status` defaulting `"live"`.
- **field-semantics.md content** carries what `spec_ref` → ACIF does not discharge: dot-path key grammar `^[a-z_]+(\.[a-z_]+)*$`, vocabulary-member relationship, parent-`supported` auto-flip (source behavior at `seed.go:91-92`), the `conversion` enum, provenance of `mechanism` strings — verifying during implementation how much ACIF now owns (`docs/design/publish-layer.md:369-377`).
- **Root `index.json` is generated from code constants** (`latest: "v1"`, one live major entry) until a freeze exists; `site-static/` handling is dormant until then. `advisories.json` publishes with an empty `advisories` array at launch.
- **Daily cron semantics** (per the accepted mechanics, not re-litigated): the `publish.yml` cron runs export + the full fail-closed gate daily as a health check and deploys only when `data_revision` differs from the live index — an unchanged-data run never churns bytes.
- **`capmon export --verify <commit>`** rebuilds the tree from a source commit and diffs against the published site over HTTPS (`docs/design/publish-layer.md:311-314`); it is a consumer-side/maintainer conformance tool and never part of the publish gate.

## Out of Scope

- `capmon freeze` command — deferred until a v2 approaches; only the OPTIONAL freeze fields ship in v1 schemas (accepted design, "Deferred").
- Phase 4b internals: FormatDoc `schema_version`, manifest enums (`status`/`blocking`/`fetch_tier`) — internal pipeline state; `EventEntry.Blocking` publishes as a plain string.
- Publishing seeder specs, format docs, or source manifests — internal pipeline state, never published (`docs/design/publish-layer.md:24-27`).
- Historical archive on the site — git history is the archive (ADR 0001).
- Source-data backfill of `display_name`/`last_verified` and the flat-leaf reconciliation — pre-launch data audits, tracked separately; exporter behavior (fallback/omission) makes publication correct regardless.
- Source-YAML strict-parse hardening (yaml.v3 `KnownFields`) — tempting given the deleted source schema, but not requested and not needed for the publish gate.
- syllago-side consumer changes — downstream flips to the published bundle after this ships.
- `cmd/provider-monitor` — untouched (research: separate flag-based binary).

## Interfaces affected (preview)

- `capyaml` — `ProviderExclusive` retyped to `map[string]CapabilityEntry`; loader and validation otherwise untouched.
- Exporter (new, root package alongside `generate.go`) — registry loader preserving `description`/`type`/`spec_ref`; recursive document builders (per-provider, all.json, by-content-type pivots, indexes, advisories); canonical JSON writer (the single serialization path); jsonschema fail-closed gate; `data_revision` computation; non-canonical-node gate.
- `cmd/capmon` — new `export` subcommand (out-dir, `--source-commit`, `--generated-at`, `--verify <commit>` flags), one-file-per-subcommand pattern.
- `.github/workflows/publish.yml` — new: push-to-main-on-`docs/` + daily cron + dispatch; build → export → gate → upload-pages-artifact → attest-build-provenance → deploy-pages, all SHA-pinned.
- `.github/workflows/pipeline.yml` — heartbeat job rewritten to Actions API keepalive (`actions: write`, no `contents` permission); run manifest persistence moves to a workflow artifact.
- `.github/workflows/ci.yml` — unchanged; new tests ride the existing `go test -race ./...`.
- `docs/provider-capabilities/schema.json` — deleted.
- Published-schema + spec artifacts — six hand-authored JSON Schemas and `field-semantics.md`, committed and copied into the tree by export.
- `testdata` — byte-level export fixture including a synthetic provider covering events/tools/refs/references/provider_exclusive.
- `README` — consumer-contract section (fetch flow, attestation verification, staleness/polling rules).
- `go.mod` — one new direct dependency: `santhosh-tekuri/jsonschema/v6`.
