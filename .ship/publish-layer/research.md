# Research: publish-layer

## Q1: How does GenerateContentTypeViews in generate.go join capability baselines with the canonical-key registry — iteration order, key-matching logic, recursion over nested capabilities, and output writing — and which helpers are shared/exported vs private?

- `GenerateContentTypeViews(capsDir, outDir string)` does not read or join against the canonical-key registry at all. `generate.go` never references `docs/spec/canonical-keys.yaml` or any registry loader (`generate.go:1-70`; the only imports are `capyaml` and `gopkg.in/yaml.v3`, `generate.go:3-12`).
- Iteration order: it lists the capability-baseline directory with `os.ReadDir` (`generate.go:18`), skips directories and non-`.yaml` files (`generate.go:27`), loads each file via `capyaml.LoadCapabilityYAML` (`generate.go:30`), and skips per-content-type seed YAMLs whose `Slug` is empty (`generate.go:34-37`).
- Grouping: it builds `byType := map[string]map[string]interface{}` (content type → provider slug → `ContentTypeEntry`) by ranging over `caps.ContentTypes` (`generate.go:24`, `generate.go:38-43`). Both the outer write loop (`for ct, providers := range byType`, `generate.go:50`) and the inner content-type range use Go map iteration, whose order is unspecified.
- Key-matching logic: none. The `ContentTypeEntry` value is stored opaquely as `interface{}` (`generate.go:42`); there is no per-key validation, no recursion over nested capabilities, and no descent into `CapabilityEntry.Capabilities`.
- Output writing: one file per content type at `outDir/<ct>.yaml` (`generate.go:51`), prefixed with a `# THIS FILE IS GENERATED` banner that embeds `time.Now().UTC().Format(time.RFC3339)` (`generate.go:52-53`) — the banner timestamp makes repeated runs byte-different even with identical data. Body is `yaml.Marshal` of `{schema_version: "1", content_type: ct, providers: providers}` (`generate.go:55-59`), written with `os.WriteFile(..., 0644)` after `os.MkdirAll(outDir, 0755)` (`generate.go:46`, `generate.go:65`).
- Helpers: `GenerateContentTypeViews` is the only function in the file and is exported (`generate.go:17`). The shared/exported helper it uses is `capyaml.LoadCapabilityYAML` (`capyaml/load.go:12-22`), also used by the `verify` command (`cmd/capmon/capmon_cmd.go:116`). There are no private helpers in `generate.go`.
- The CLI wrapper is `capmon generate`, hardcoding `capsDir = "docs/provider-capabilities"` and `outDir = capsDir + "/by-content-type"` (`cmd/capmon/capmon_cmd.go:352-359`). Generated files currently committed: `docs/provider-capabilities/by-content-type/{agents,commands,hooks,mcp,rules,skills}.yaml` (directory listing).
- Test coverage: `generate_test.go:12-42` writes one minimal baseline into a temp dir and asserts the banner and provider slug appear in `hooks.yaml`; no byte-level or registry assertions.

## Q2: What is the exact loaded struct shape in capyaml (ProviderCapabilities, ContentTypeEntry, CapabilityEntry, EventEntry, ToolEntry, ReferenceEntry): all fields, YAML tags, omitempty flags; how is provider_exclusive typed; and what value shapes actually appear under provider_exclusive, events, tools, refs, and references across the 15 files in docs/provider-capabilities/?

Struct shapes (`capyaml/types.go`):

- `ProviderCapabilities` (`capyaml/types.go:4-16`):
  - `SchemaVersion string` `yaml:"schema_version"`
  - `Slug string` `yaml:"slug"`
  - `DisplayName string` `yaml:"display_name"`
  - `LastVerified string` `yaml:"last_verified"`
  - `ProviderVersion string` `yaml:"provider_version,omitempty"`
  - `SourceManifest string` `yaml:"source_manifest,omitempty"`
  - `FormatReference string` `yaml:"format_reference,omitempty"`
  - `References map[string]ReferenceEntry` `yaml:"references,omitempty"` (`capyaml/types.go:13`)
  - `ContentTypes map[string]ContentTypeEntry` `yaml:"content_types"` (`capyaml/types.go:14`)
  - `ProviderExclusive map[string]interface{}` `yaml:"provider_exclusive,omitempty"` (`capyaml/types.go:15`)
- `ReferenceEntry` (`capyaml/types.go:20-25`): `URL string` `yaml:"url"`, `FetchMethod string` `yaml:"fetch_method"`, `VerifiedAt string` `yaml:"verified_at,omitempty"`, `LastContentHash string` `yaml:"last_content_hash,omitempty"`.
- `ContentTypeEntry` (`capyaml/types.go:28-34`): `Supported bool` `yaml:"supported"`, `Confidence string` `yaml:"confidence,omitempty"`, `Events map[string]EventEntry` `yaml:"events,omitempty"`, `Capabilities map[string]CapabilityEntry` `yaml:"capabilities,omitempty"`, `Tools map[string]ToolEntry` `yaml:"tools,omitempty"`.
- `EventEntry` (`capyaml/types.go:37-41`): `NativeName string` `yaml:"native_name"`, `Blocking string` `yaml:"blocking,omitempty"`, `Refs []string` `yaml:"refs,omitempty"`.
- `CapabilityEntry` (`capyaml/types.go:51-57`): `Supported bool` `yaml:"supported"`, `Mechanism string` `yaml:"mechanism,omitempty"`, `Confidence string` `yaml:"confidence,omitempty"`, `Refs []string` `yaml:"refs,omitempty"`, and the recursive `Capabilities map[string]CapabilityEntry` `yaml:"capabilities,omitempty"`. The doc comment states object-typed canonical keys hold vocabulary members under the recursive field, flat keys leave it nil, and adding a sub-capability auto-sets the parent's `Supported` to true (`capyaml/types.go:43-50`; the auto-set is implemented in `seed.go:91-92`).
- `ToolEntry` (`capyaml/types.go:60-63`): `Native string` `yaml:"native"`, `Refs []string` `yaml:"refs,omitempty"`.
- `provider_exclusive` is typed `map[string]interface{}` and round-trips as-is per the `WriteCapabilityYAML` comment (`capyaml/load.go:24-25`).

Actual value shapes across the 15 capability baselines (`docs/provider-capabilities/{amp,claude-code,cline,codex,copilot-cli,crush,cursor,factory-droid,gemini-cli,kiro,opencode,pi,roo-code,windsurf,zed}.yaml`):

- `provider_exclusive`: appears in zero baselines — `grep -n "provider_exclusive" docs/provider-capabilities/*.yaml` returns no matches (exit 1).
- `events`: appears in zero baselines. A recursive scan of all 15 files found no `events` maps under any content type; the only grep hits for `events:`-adjacent patterns were prose inside `mechanism` strings (e.g. `docs/provider-capabilities/kiro-mcp.yaml:43` — a seed file, not a baseline).
- `tools` (ToolEntry maps): appears in zero baselines. The `tools:` hit at `docs/provider-capabilities/codex.yaml:156` is a `CapabilityEntry` named `tools` under `skills.capabilities` (shape `supported/mechanism/confidence`, `docs/provider-capabilities/codex.yaml:156-159`), not a `ToolEntry` map.
- `refs` (node-level arrays): appear in zero baselines (recursive scan of all capability nodes in all 15 files).
- `references` (per-provider registry): appears in zero baselines (same grep, no matches).
- Every capability node in the baselines carries only `supported`, `mechanism`, `confidence`, and optionally nested `capabilities` (e.g. `docs/provider-capabilities/claude-code.yaml:9-47`).

## Q3: How are cobra subcommands registered in cmd/capmon/ — file layout, flag conventions, exit-code conventions — and how does the existing verify command perform schema validation (library, where schemas live, embed vs file load)?

File layout and registration:

- `cmd/capmon/main.go:13-18` executes the root `capmonCmd` and on error prints to stderr and `os.Exit(1)`.
- The root command plus six subcommands (`verify`, `fetch`, `run`, `generate`, `seed`, `test-fixtures`) live in one file, registered in its `init()` (`cmd/capmon/capmon_cmd.go:59-63`, `cmd/capmon/capmon_cmd.go:427-455`).
- Each remaining subcommand has its own `capmon_<name>_cmd.go` file with a paired `_test.go`, and self-registers in its own `init()`: `derive` (`cmd/capmon/capmon_derive_cmd.go:71`), `backfill` (`cmd/capmon/capmon_backfill_cmd.go:47`), `check` (`cmd/capmon/capmon_check_cmd.go:59`), `onboard` (`cmd/capmon/capmon_onboard_cmd.go:62`), `validate-sources` (`cmd/capmon/capmon_validate_sources_cmd.go:40`), `validate-format-doc` (`cmd/capmon/capmon_validate_format_doc_cmd.go:64`).
- Extractor packages are imported for side effects in the root file (`cmd/capmon/capmon_cmd.go:14-23`).

Flag conventions:

- All flags are local (`cmd.Flags()`), defined in `init()`; there are no persistent flags. No `--json`/`--quiet`/`--verbose` flags are registered anywhere in `cmd/capmon/` (grep for `"json"|"quiet"|"verbose"` in non-test files returns nothing), even though the `internal/output` globals `JSON`, `Quiet`, `Verbose` exist (`internal/output/output.go:21-27`) and command code branches on them (e.g. `cmd/capmon/capmon_cmd.go:205`, `263`); only tests set them (`cmd/capmon/capmon_fetch_cmd_test.go:161`).
- Two default styles coexist: empty-string flag default with fallback assignment inside `RunE` (`cmd/capmon/capmon_cmd.go:73-75`, `145-150`) and literal defaults in the flag definition (`cmd/capmon/capmon_check_cmd.go:51-57`, e.g. `--canonical-keys` defaulting to `docs/spec/canonical-keys.yaml`).
- Provider slugs are validated with `capmon.SanitizeSlug` before use (`cmd/capmon/capmon_cmd.go:139-144`); structured errors use `output.NewStructuredError` with namespaced codes like `INPUT_003` (`cmd/capmon/capmon_cmd.go:140-142`, `internal/output/errors.go:5-11`).
- Package-level override vars redirect paths for tests, e.g. `capmonCapabilitiesDirOverride` (`cmd/capmon/capmon_cmd.go:30`) and the derive overrides (`cmd/capmon/capmon_derive_cmd.go:11-15`).

Exit-code conventions:

- Default: any `RunE` error → stderr print + exit 1 (`cmd/capmon/main.go:14-17`).
- `capmon run` bypasses that by calling `os.Exit(exitClass)` directly (`cmd/capmon/capmon_cmd.go:343-348`), using the pipeline exit classes `ExitClean=0, ExitDrifted=1, ExitPartialFailure=2, ExitInfrastructureFailure=3, ExitFatal=4, ExitPaused=5` (`types.go:8-16`).
- `internal/output/output.go:14-19` defines a second, CLI-oriented set: `ExitSuccess=0, ExitError=1, ExitUsage=2, ExitDrift=3`.

Verify command schema validation:

- `capmon verify` walks `docs/provider-capabilities` (overridable via `capmonCapabilitiesDirOverride`), skips seed YAMLs with empty `slug`, and calls `capyaml.ValidateAgainstSchema(path, migrationWindow)` per file (`cmd/capmon/capmon_cmd.go:95-124`).
- `ValidateAgainstSchema` uses no JSON Schema library: it YAML-parses the `schema_version` header, checks it against `supportedSchemaVersions = []string{"1"}` (with an optional current-minus-one migration window), then does a full `yaml.Unmarshal` into `ProviderCapabilities` to catch type errors (`capyaml/validate.go:13-45`).
- A JSON Schema file exists at `docs/provider-capabilities/schema.json` (draft 2020-12, `$id` under the repo, `required: ["schema_version", "slug", "content_types"]`, `docs/provider-capabilities/schema.json:1-8`), but no Go code loads it: grep for `provider-capabilities/schema.json`, `jsonschema`, `xeipuuv`, or `santhosh` across `*.go` and `go.sum` returns no matches. Nothing is embedded (`go:embed` is not used in the repo). `go.mod` direct dependencies are BurntSushi/toml, goquery, chromedp, go-tree-sitter, cobra, goldmark, golang.org/x/net, and gopkg.in/yaml.v3 only (`go.mod:7-16`).

## Q4: What fields does each entry in docs/spec/canonical-keys.yaml carry, what top-level metadata does the file have, and which code paths currently parse it or look up keys by dot-path?

- Per-entry fields: every key entry carries exactly `description`, `type` (`string | bool | object`), and `spec_ref` per the in-file schema comment (`docs/spec/canonical-keys.yaml:54-61`); e.g. `skills.display_name` (`docs/spec/canonical-keys.yaml:66-70`) and object-typed `rules.activation_mode` (`docs/spec/canonical-keys.yaml:178-184`). No entry carries `aliases` or `deprecated` fields today (those are described only in the lifecycle comment, `docs/spec/canonical-keys.yaml:26-41`).
- Top-level structure: a `---` document marker (`docs/spec/canonical-keys.yaml:1`), then a large comment header covering Authority/ACIF direction (`:2-10`), the Confidence Enum (`:12-24`), the Key Lifecycle Policy (`:26-41`), and Staleness and Maintenance (`:43-46`), followed by a single mapping key `content_types:` (`docs/spec/canonical-keys.yaml:63`). There is no machine-readable top-level metadata (no version, date, or count fields). The file is 420 lines; the six content types hold 46 keys total (15 skills, 5 rules, 10 hooks, 8 mcp, 7 agents, 2 commands — counted by script over the parsed file; total matches the lexicon's "46 keys", `lexicon.md:21`).
- Object-typed keys (13): skills `compatibility`, `metadata_map`; rules `activation_mode`, `cross_provider_recognition`; hooks `handler_types`, `decision_control`, `hook_scopes`; mcp `transport_types`, `tool_filtering`; agents `invocation_patterns`, `agent_scopes`; commands `argument_substitution` (parsed from `docs/spec/canonical-keys.yaml` `type: object` entries, e.g. `:91`, `:183`, `:223`, `:297`, `:373`, `:411`).
- Code paths that parse it:
  - `loadCanonicalKeys` (`formatdoc_validate.go:85-103`) is the only parser. It unmarshals into `canonicalKeysFile{ContentTypes map[string]map[string]interface{}}` (`formatdoc_validate.go:79-81`) and discards `description`/`type`/`spec_ref`, returning only a per-content-type key-name set.
  - Consumers of `loadCanonicalKeys`: `ValidateFormatDoc` (`formatdoc_validate.go:152-153`, rule at `:209-214`), `DeriveSeederSpecs` (`derive.go:20-21`, rejection at `derive.go:56`), and `RunCapmonCheck` via `ValidateFormatDocWithWarnings` with default path `docs/spec/canonical-keys.yaml` (`check.go:143-144`, `check.go:179`).
  - CLI entry points passing the path: `capmon check --canonical-keys` (`cmd/capmon/capmon_check_cmd.go:57`), `capmon derive --canonical-keys` (`cmd/capmon/capmon_derive_cmd.go:29`), and `capmon validate-format-doc` (`cmd/capmon/capmon_validate_format_doc_cmd.go`).
- Dot-path lookup: no code looks up registry entries by full dot-path. Recognizers validate only the first dot-path segment against hardcoded per-content-type Go slices (e.g. `CanonicalRulesKeys`, `recognize_rules.go:45-51`; `firstSegment` extracts the head so `activation_mode.always` validates the same as `activation_mode`, `recognize_rules.go:118-129`). `seed.go:58` splits extracted dot-paths with `strings.SplitN(path, ".", 5)` and routes by segment count/position without consulting the registry (`seed.go:46-105`).
- Drift guards keep the Go slices and the registry in sync per content type, e.g. `TestCanonicalHooksKeys_MatchesCanonicalKeysYAML` (`recognize_hooks_test.go:17-55`) and `TestCanonicalRulesKeys_MatchesCanonicalKeysYAML` (`recognize_rules_test.go:17-18`), resolving the file via `canonicalKeysPath` (`docsroot_test.go:23-28`).

## Q5: What do .github/workflows/pipeline.yml and ci.yml currently do: triggers, permissions blocks, which steps commit to main (heartbeat), how actions are pinned, and what job/step structure exists for adding a publish job?

ci.yml:

- Triggers: `push` to `main` and all `pull_request` (`.github/workflows/ci.yml:3-6`).
- Permissions: top-level `contents: read` only (`.github/workflows/ci.yml:8-9`).
- Single job `test` on `ubuntu-latest` with steps: checkout, setup-go with `go-version-file: go.mod`, gofmt check, a legacy-name gate grepping for the old downstream project name, `go build ./...` (CGO on), `CGO_ENABLED=0 go build ./...`, `go vet ./...`, `go test -race ./...` (`.github/workflows/ci.yml:11-50`).

pipeline.yml:

- Triggers: daily cron `0 6 * * *` and `workflow_dispatch` with `stage`, `provider`, `dry_run` inputs (`.github/workflows/pipeline.yml:12-30`).
- Top-level permissions: `contents: read` (`.github/workflows/pipeline.yml:32-33`); jobs escalate individually.
- Jobs:
  - `fetch-extract` (Stage 1-2): runs in the `chromedp/headless-shell` container pinned by image digest (`.github/workflows/pipeline.yml:46-48`); job permissions `contents: write`, `pull-requests: write`, `issues: write` for reactive heal PRs (`:42-45`); installs git/gh, checks out, sets up Go via `go-version-file: go.mod` (`:66-69`), `go build -o capmon-bin ./cmd/capmon` (`:71-72`), runs `./capmon-bin run --stage fetch-extract` (`:79-84`), computes an artifact SHA-256 exposed as a job output (`:49-50`, `:86-93`), and uploads `.capmon-cache.tar.gz` as artifact `capmon-cache` with 7-day retention (`:95-102`).
  - `report` (Stage 3-4): `needs: fetch-extract` (`:107`), same write permissions (`:109-112`), downloads the artifact, verifies its SHA-256 against the upstream job output and fails on mismatch (`:117-132`), builds, runs `./capmon-bin run --stage report` (`:147-151`).
  - `staleness-check`: `needs: [fetch-extract, report]`, `if: always()`, permissions `contents: read`, `issues: write` (`:153-160`), runs `./capmon-bin verify --staleness-check --threshold-hours 36` (`:183-190`).
  - `hash-check`: independent job (no `needs`), permissions `contents: read`, `issues: write` (`:192-197`), runs `capmon check --all` on schedule or `--provider=...` on dispatch (`:210-231`).
  - `heartbeat`: the only step that commits to main. `needs` all four jobs, `if: always() && github.event_name == 'schedule'`, permissions `contents: write` (`:233-242`); it checks out the default branch, writes `.heartbeat/last-run.json`, commits as `github-actions[bot]`, and `git push` directly (`:247-260`). Per the lexicon, ADR 0005 moves the heartbeat off `main` (`lexicon.md:46`); the current workflow still pushes to the checked-out default branch.
- Action pinning: every action is pinned to a full commit SHA with a version comment — `actions/checkout@de0fac2e...# v6.0.2` (`:64`), `actions/setup-go@4a360112...# v5` (`:67`), `actions/upload-artifact@043fb46d...# v7.0.1` (`:97`), `actions/download-artifact@3e5f45b2...# v8.0.1` (`:118`) — and the container image is pinned by sha256 digest (`:47`).
- Cross-job data handoff pattern available for a new job: artifact upload/download plus a SHA-256 job output verified by the consumer (`:49-50`, `:86-93`, `:117-132`).

## Q6: What JSON-encoding and determinism utilities already exist in the repo (sorted-key marshaling, SetEscapeHTML usage, golden/fixture byte-comparison tests), and where is the Go toolchain version pinned (go.mod, CI workflow files)?

- `SetEscapeHTML` is used nowhere in the repo (grep across `*.go` returns no matches).
- JSON writing sites, all via `encoding/json` with `MarshalIndent(v, "", "  ")`: run-manifest persistence (`manifest_persist.go:17`), pipeline dry-run output (`pipeline.go:344`), and the CLI output package (`internal/output/output.go:65`, `:114`, `:143`). `cmd/provider-monitor/main.go:85` uses `json.NewEncoder`. There are no custom sorted-key or canonicalization helpers; no JSON canonicalization library is in `go.mod` (`go.mod:7-16`).
- YAML writing sites: `yaml.Marshal` in `healing_pr.go:88`, `derive.go:77`, `generate.go:55`; `yaml.NewEncoder` with `SetIndent(2)` in `capyaml.WriteCapabilityYAML` (`capyaml/load.go:26-33`) and `seed.go:131`.
- Byte-comparison / golden-style tests: `TestDeriveOutputMatchesCommitted` re-derives every seeder spec from format docs and compares `yaml.Marshal` output byte-for-byte (`bytes.Equal`) against the committed files under `docs/` (`seederspec_audit_test.go:77-121`) — the committed tree serves as the golden copy. There are no `.golden` files and no other `bytes.Equal`/`cmp.Diff` fixture comparisons in tests (repo-wide grep).
- Go toolchain pinning: `go.mod:3` declares `go 1.26` and `go.mod:5` declares `toolchain go1.26.3`. Both workflows resolve the Go version from `go.mod` via `actions/setup-go` `go-version-file: go.mod` (`.github/workflows/ci.yml:20-22`; `.github/workflows/pipeline.yml:66-69`, `:134-137`, `:165-168`, `:202-205`).

## Q7: Across docs/provider-capabilities/*.yaml: which providers have empty or missing display_name or last_verified; which have non-empty events, tools, refs, or references; and which nodes under object-typed canonical keys are flat leaves rather than parents of nested capabilities?

- `display_name` and `last_verified`: all 15 capability baselines carry both fields as literal empty strings (`display_name: ""`, `last_verified: ""`), e.g. `docs/provider-capabilities/claude-code.yaml:3-4` and `docs/provider-capabilities/zed.yaml:3-4`; a scripted scan of all 15 confirmed no baseline has a non-empty value for either.
- `events`, `tools` (ToolEntry maps), node-level `refs`, `references` registry: none of the 15 baselines contain any of these (see Q2 for grep/scan evidence). The apparent grep hits are mechanism prose (`docs/provider-capabilities/kiro-rules.yaml:25`) or capability nodes whose names happen to collide (`docs/provider-capabilities/codex.yaml:156` `tools` capability node; `docs/provider-capabilities/amp.yaml:72` `executable_tools`; `docs/provider-capabilities/cline.yaml:84` `file_references`).
- `docs/provider-capabilities/opencode.yaml` has `content_types: {}` — no capability tree at all (`docs/provider-capabilities/opencode.yaml:5`; whole file is 5 lines).
- Object-typed canonical keys present as flat leaves (no nested `capabilities` map), per scripted join of baselines against the registry's 13 object-typed keys:
  - amp: `mcp.tool_filtering`
  - claude-code: `agents.agent_scopes`, `hooks.handler_types`, `hooks.decision_control`, `hooks.hook_scopes`, `mcp.transport_types`, `mcp.tool_filtering`
  - cline: `hooks.handler_types`, `hooks.hook_scopes`, `mcp.transport_types`
  - codex: `hooks.handler_types`, `hooks.decision_control`, `hooks.hook_scopes`
  - copilot-cli: `hooks.handler_types`
  - crush: `skills.compatibility`, `skills.metadata_map`
  - cursor: `hooks.hook_scopes`, `mcp.transport_types`, `mcp.tool_filtering`
  - factory-droid: `commands.argument_substitution`, `hooks.decision_control`, `mcp.transport_types`, `mcp.tool_filtering`
  - gemini-cli: `agents.invocation_patterns`, `commands.argument_substitution`, `hooks.decision_control`, `mcp.transport_types`, `mcp.tool_filtering`
  - kiro: `mcp.transport_types`
  - pi: `commands.argument_substitution`, `hooks.handler_types`
  - roo-code: `skills.compatibility`, `skills.metadata_map`
  - windsurf: `hooks.hook_scopes`, `mcp.transport_types`, `mcp.tool_filtering`
  - zed: `mcp.tool_filtering`
  (Example flat leaf: `mcp.tool_filtering` in claude-code carries only `supported/mechanism/confidence` — the same shape used for its non-object siblings.)
- Object-typed keys that do carry vocabulary members (nested `capabilities`): e.g. claude-code `agents.invocation_patterns` with `at_mention`, `background`, `natural_language` (`docs/provider-capabilities/claude-code.yaml:17-31`); `rules.activation_mode` is nested in 11 baselines (amp, claude-code, cline, codex, copilot-cli, cursor, gemini-cli, kiro, windsurf, zed with member sets like `always,glob` / `always,glob,manual,model_decision`); `rules.cross_provider_recognition` is nested in 7 (member `agents_md`, plus `claude_md`/`gemini_md` for copilot-cli); `agents.agent_scopes` nested with `project,user` in cursor, kiro, roo-code.

## Q8: What test conventions does the repo use: testdata directory layout, golden-file patterns, table-driven test style, make targets or CI commands that run build and tests?

- Testdata layout: root package fixtures live under `testdata/fixtures/<name>/` with per-provider and per-language subdirectories — `testdata/fixtures/claude-code/hooks-docs.html`, `testdata/fixtures/windsurf/llms-full.txt`, `testdata/fixtures/rust/hooks.rs`, `testdata/fixtures/typescript/hooks.ts`, `testdata/fixtures/source-manifests/claude-code-minimal.yaml` (directory listing). `capyaml/testdata/` holds two YAML fixtures: `claude-code-minimal.yaml` and `schema-version-99.yaml`.
- Golden-file pattern: no `.golden` files exist. The golden-style pattern in use is comparing regenerated output byte-for-byte against committed files under `docs/` in the same checkout — `TestDeriveOutputMatchesCommitted` (`seederspec_audit_test.go:77-121`, `bytes.Equal` at `:121`). The lexicon calls this test class a drift guard (`lexicon.md:43`).
- Repo-root resolution for drift guards: `docsRoot(t)` asserts `docs/` exists in the working directory and returns `"."` (`docsroot_test.go:15-21`); `canonicalKeysPath(t)` builds on it (`docsroot_test.go:23-28`).
- Table-driven style: 18 of the root-package `_test.go` files use `tests := []struct{...}` / `for _, tt := range` tables (grep count), e.g. `sanitize_test.go:10`.
- Test doubles/conventions seen: temp-dir based tests with inline YAML fixtures (`generate_test.go:13-23`); package-level override vars for redirecting commands to temp dirs (`cmd/capmon/capmon_cmd.go:28-30`, `cmd/capmon/capmon_derive_cmd.go:11-15`); `output.SetForTest(t)` for capturing CLI output with automatic global restore (`internal/output/output.go:34-58`).
- Build/test commands: there is no Makefile anywhere in the repo (`find -name Makefile` returns nothing). CI is the canonical command set: `gofmt -l .` gate, legacy-name grep gate, `go build ./...`, `CGO_ENABLED=0 go build ./...`, `go vet ./...`, `go test -race ./...` (`.github/workflows/ci.yml:24-50`). The pipeline workflow builds the binary as `go build -o capmon-bin ./cmd/capmon` (`.github/workflows/pipeline.yml:71-72`).
- External-vs-internal test split: both `package capmon` internal tests (`check_internal_test.go`, `pipeline_internal_test.go`, `recognize_internal_test.go`) and `package capmon_test` external tests (`generate_test.go:1`, `docsroot_external_test.go`) coexist in the root directory.

## Cross-cutting observations

- The two binaries are `cmd/capmon` and `cmd/provider-monitor` (directory listing of `cmd/`); `cmd/provider-monitor/main.go` uses plain `flag` + `json.NewEncoder` rather than cobra (`cmd/provider-monitor/main.go:46`, `:85`).
- 76 of the 91 YAML files in `docs/provider-capabilities/` are per-content-type seed files (`<slug>-<content_type>.yaml`) with no top-level `slug:`; both `GenerateContentTypeViews` (`generate.go:34-37`) and `capmon verify` (`cmd/capmon/capmon_cmd.go:111-119`) skip them by the `Slug == ""` test.
- Several baselines contain capability nodes under `skills.capabilities` whose names are not canonical keys — e.g. codex has 18 such nodes mirroring its skill frontmatter fields (`skills.r#type`, `skills.tools`, `skills.transport`, `skills.url`, `docs/provider-capabilities/codex.yaml:156-171`); claude-code has 9 (`skills.frontmatter`, `skills.live_reload`, …); pi has camelCase node names (`skills.baseDir`, `skills.disableModelInvocation`, `skills.filePath`, `skills.sourceInfo`) (scripted scan against the registry). The `verify` command's struct-parse validation does not check node names against the registry (`capyaml/validate.go:39-43`).
- The `verify` command comment at `cmd/capmon/capmon_cmd.go:115` references a stale path `internal/capmon/generate.go`; the file now lives at the repo root (`generate.go`). `capmon test-fixtures` similarly reports a stale fixtures path `cli/internal/capmon/testdata/fixtures` (`cmd/capmon/capmon_cmd.go:417`).
- The generated `by-content-type` views' banner timestamp (`generate.go:52-53`) is the one non-deterministic byte source in an otherwise data-driven generation path; `TestGenerateContentTypeViews` asserts only substring presence, not bytes (`generate_test.go:36-41`).
- `go-json-experiment/json` appears in `go.mod` as an indirect dependency only (`go.mod:22`), pulled in by chromedp.
- The heal-PR path pushes branches (not main) using the jobs' `contents: write` permission; the anchor-embedding mechanism is in `healing_pr.go:115`.
