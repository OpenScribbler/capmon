# Plan: publish-layer

## Execution order

1. **Slice 1: Self-describing provider capability documents** — test bead + impl bead (TDD)
   - Test: `capyaml/provider_exclusive_test.go`, `export_registry_test.go`, `export_canonical_test.go`, `export_provider_test.go` — assert `TestProviderExclusiveRoundTrip`, `TestLoadKeyRegistryPreservesMetadata`, `TestLoadKeyRegistryRealFile`, `TestCanonicalJSONProfile`, `TestBuildProviderDocKeyMetadata`, `TestBuildProviderDocNonCanonicalNodeFailsClosed`, `TestBuildProviderDocFallbacksAndTrimming`
   - Impl: `capyaml/types.go` (ProviderExclusive retype), `export_registry.go`, `export_canonical.go`, `export_provider.go` — satisfy tests
   - Checkpoint: `go build ./... && go test -race -run 'ProviderExclusive|KeyRegistry|CanonicalJSON|BuildProviderDoc' ./...` passes

2. **Slice 2: Non-canonical nodes relocated to provider_exclusive (launch-blocking audit)** — test bead + impl bead (TDD)
   - Test: `export_baselines_audit_test.go` — asserts `TestAllBaselinesBuildProviderDocs` (fails before relocation, passes after)
   - Impl: `docs/provider-capabilities/{codex,claude-code,pi,amp,cline}.yaml` — relocate non-canonical nodes to `provider_exclusive` keyed `<content_type>.<original_name>`
   - Checkpoint: `go test -race -run AllBaselinesBuildProviderDocs ./...` passes against the committed tree; `go run ./cmd/capmon verify` exits 0; `git diff --stat docs/provider-capabilities/` shows only the five audited files changed

3. **Slice 3: Complete deterministic /v1/ document tree** — test bead + impl bead (TDD)
   - Test: `export_tree_test.go`, `export_index_test.go` — assert `TestWriteExportTreeLayout`, `TestBuildAllAndPivotsShareNodeShape`, `TestBuildV1Index`, `TestRootIndexBytes`, `TestExportTreeRealData`
   - Impl: `export_tree.go`, `export_index.go` — satisfy tests
   - Checkpoint: `go test -race -run 'ExportTree|BuildAll|V1Index|RootIndex' ./...` passes; a temp-dir tree built from real `docs/` contains all published files

4. **Slice 4: Fail-closed schema gate and published contract artifacts** — test bead + impl bead (TDD)
   - Test: `export_gate_test.go` (+ extension in `export_tree_test.go`) — assert `TestValidateExportTreePassesFixture`, `TestValidateExportTreeFailsClosed`, `TestAssertProviderSetMismatch`, `TestFreezeFieldsOptionalFromLaunch`, `TestSpecArtifactsCopiedVerbatim`
   - Impl: `export_gate.go`, six hand-authored schemas under `docs/publish/schemas/`, `docs/publish/spec/field-semantics.md`, `go.mod` (+ `santhosh-tekuri/jsonschema/v6`), `writeExportTree` staging-order modification, delete `docs/provider-capabilities/schema.json`
   - Checkpoint: `go test -race -run 'ValidateExportTree|ProviderSet|FreezeFields|SpecArtifacts' ./...` passes; a deliberately corrupted temp tree is rejected before any output reaches a destination dir; `go mod tidy` clean; no dangling reference to the deleted schema

5. **Slice 5: capmon export subcommand with byte-stable output** — test bead + impl bead (TDD)
   - Test: `export_test.go`, `cmd/capmon/capmon_export_cmd_test.go`, `export_fixture_test.go` — assert `TestExportFixtureMatchesCommitted`, `TestDoubleExportDeterminism`, `TestGeneratedAtConfinement`, `TestRunExportFailClosedPreservesOut`, `TestExportCmdFlags`
   - Impl: `export.go` (`RunExport` + atomic out-dir replace), `cmd/capmon/capmon_export_cmd.go`, `testdata/fixtures/export/` (synthetic provider + fixture registry + source manifests + publish assets), `testdata/fixtures/export/expected/` (committed byte-exact documents)
   - Checkpoint: `go run ./cmd/capmon export --out /tmp/site --generated-at 2026-01-01T00:00:00Z` exits 0 and `/tmp/site/v1/index.json` exists; `go test -race ./...` passes

6. **Slice 6: Export conformance verification against the published site** — test bead + impl bead (TDD)
   - Test: `export_verify_test.go` (+ flag wiring in `cmd/capmon/capmon_export_cmd_test.go`) — assert `TestExportVerifyMatch`, `TestExportVerifyMismatchFailsClosed`, `TestExportVerifyFlagWiring`
   - Impl: `export_verify.go` (`RunExportVerify`), `cmd/capmon/capmon_export_cmd.go` (`--verify`/`--base-url`)
   - Checkpoint: `go test -race -run ExportVerify ./...` passes (fully offline via httptest)

7. **Slice 7: Attested publish workflow and scheduled-job hardening** — test bead + impl bead (TDD)
   - Test: `workflows_audit_test.go` — asserts `TestPublishWorkflowHardening`, `TestHeartbeatHoldsNoContentsWrite` (fails before the workflow edits, passes after)
   - Impl: `.github/workflows/publish.yml` (new), `.github/workflows/pipeline.yml` (heartbeat rewrite to Actions API keepalive only), `README.md` (consumer-contract section)
   - Checkpoint: `go test -race -run 'PublishWorkflowHardening|HeartbeatHoldsNoContentsWrite' ./...` passes; manual first-publish steps (branch protection, Pages source, dispatch, `gh attestation verify`, cron skip) deferred to launch runbook in final-validate

## Gate

Before moving from one slice to the next: Checkpoint for the current slice must pass. If it fails, stop and involve the user — never skip ahead.

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

## Non-TDD exemptions

None.
