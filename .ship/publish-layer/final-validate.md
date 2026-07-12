# Final Validate — publish-layer

Validated on branch `capmon/publish-layer` at the slice-7 commit (`3e4fb61`), 2026-07-12.

## Acceptance criteria checked

- [x] Full `/v1/` tree published: 15 per-provider docs, `all.json`, six by-content-type pivots, `spec/canonical-keys.json` + `spec/field-semantics.md`, six `schemas/`, `v1/index.json`, `advisories.json`, root `index.json` — `TestWriteExportTreeLayout`, `TestExportTreeRealData`, Slice 7 manual step 3
  - Command: `go run ./cmd/capmon export --out <scratch>/site --generated-at 2026-07-12T00:00:00Z --source-commit "$(git log -1 --format=%H -- docs/)"` plus `ls` counts and a JSON inspection of both indexes; `go test -race ./...` (includes TestWriteExportTreeLayout + TestExportTreeRealData)
  - Observed: export exit 0; 15 provider docs under `v1/capabilities/` (+ `all.json`), 6 pivots, 6 schemas, `spec/canonical-keys.json` + `spec/field-semantics.md` staged; `v1/index.json` lists 15 providers all `"tracked"` with `data_revision` and `source_commit` set; root index `latest: v1`, 1 major; `advisories: []`. Live-URL confirmation (manual step 3) deferred to the launch runbook — publish requires Pages + branch protection setup on GitHub first.
  - Evidence: scratchpad `final-validate.log`; export tree at scratchpad `site/`

- [x] Every generated JSON document validates against its published schema, in-process, before leaving the temp dir — `TestValidateExportTreePassesFixture`, `TestValidateExportTreeFailsClosed`
  - Command: `go test -race ./...` (both tests included); the real-export command above also exercises the gate inside `RunExport` (validateExportTree runs before the atomic OutDir replace)
  - Observed: full suite PASS; real export succeeded, meaning all 26 generated documents validated against the six schemas compiled from the staged tree
  - Evidence: scratchpad `final-validate.log`

- [x] Committed byte-level fixture + double-export diff clean; only `generated_at` varies between data-identical runs — `TestExportFixtureMatchesCommitted`, `TestDoubleExportDeterminism`, `TestGeneratedAtConfinement`
  - Command: `go test -race -run 'TestExportFixtureMatchesCommitted|TestDoubleExportDeterminism|TestGeneratedAtConfinement' .`
  - Observed: `ok github.com/OpenScribbler/capmon 1.494s` — fixture bytes match committed `testdata/fixtures/export/expected/`, two real-repo exports byte-identical, generated_at confined to exactly `v1/index.json`
  - Evidence: test run output above; committed fixture at `testdata/fixtures/export/expected/`

- [x] Fail-closed: schema failure, provider-set mismatch, or non-canonical node aborts with `EXPORT_00x`, existing output untouched, no deploy — `TestBuildProviderDocNonCanonicalNodeFailsClosed`, `TestAssertProviderSetMismatch`, `TestRunExportFailClosedPreservesOut`
  - Command: `go test -race ./...` (all three included; TestRunExportFailClosedPreservesOut proves atomicity at both early EXPORT_001 and post-staging EXPORT_003 failure points against a sentinel OutDir)
  - Observed: full suite PASS; sentinel trees byte-identical after failed exports in both subtests
  - Evidence: scratchpad `final-validate.log`

- [x] All ~31 non-canonical nodes relocated to `provider_exclusive` before first publish; invariant CI-enforced — `TestAllBaselinesBuildProviderDocs`
  - Command: `go test -race ./...` (drift guard included); `go run ./cmd/capmon verify`
  - Observed: guard PASS over all 15 committed baselines; actual audit was larger than researched — 66 nodes across 10 providers (amp 4, claude-code 9, cline 8, codex 18, copilot-cli 1, crush 5, factory-droid 4, kiro 7, pi 4, roo-code 5), all relocated in commit `0660330`; `capmon verify` exit 0
  - Evidence: commit `0660330`; scratchpad `final-validate.log`

- [x] Deploy via `upload-pages-artifact` + `deploy-pages` pinned by SHA with `attest-build-provenance` over both indexes — `TestPublishWorkflowHardening`, Slice 7 manual step 4
  - Command: `go test -race ./...` (TestPublishWorkflowHardening parses `.github/workflows/publish.yml`: full-40-hex SHA pins on every `uses:`, permission posture, trigger shape, concurrency)
  - Observed: PASS — publish job holds exactly `{contents: read, pages: write, id-token: write, attestations: write}`, no `pull_request` trigger, attestation `subject-path` covers `dist/index.json` + `dist/v1/index.json`. Live `gh attestation verify` (manual step 4) deferred to the launch runbook — requires a real published run.
  - Evidence: `.github/workflows/publish.yml`; scratchpad `final-validate.log`

- [x] Heartbeat off `main` per ADR 0005 (`actions: write` only) with branch protection as a launch precondition — `TestHeartbeatHoldsNoContentsWrite`, Slice 7 manual step 1
  - Command: `go test -race ./...` (TestHeartbeatHoldsNoContentsWrite parses `pipeline.yml`)
  - Observed: PASS — heartbeat job permissions exactly `{actions: write}`, keepalive via `gh api -X PUT .../actions/workflows/pipeline.yml/enable`, no git commit/push step; tracked `.heartbeat/last-run.json` removed in commit `3e4fb61`. Branch protection itself is a GitHub settings action (manual step 1, launch runbook) — documented as a launch precondition.
  - Evidence: `.github/workflows/pipeline.yml` heartbeat job; scratchpad `final-validate.log`

- [x] Typed `ProviderExclusive` shapes exported JSON and its published schema — `TestProviderExclusiveRoundTrip`, `TestBuildProviderDocFallbacksAndTrimming`, synthetic-provider fixture bytes
  - Command: `go test -race ./...` (all included)
  - Observed: PASS — `capyaml.ProviderExclusive` is `map[string]CapabilityEntry`; synthetic fixture's committed expected bytes carry keyless `provider_exclusive` nodes; `docs/publish/schemas/provider-capabilities.json` models them as capability nodes forbidden from carrying `key`/`key_path`
  - Evidence: `capyaml/types.go`; `testdata/fixtures/export/expected/v1/capabilities/synthetic.json`

- [x] README consumer contract + published `v1/spec/field-semantics.md` — `TestSpecArtifactsCopiedVerbatim`, Slice 7 manual step 4
  - Command: `grep -c "Consuming the published data\|gh attestation verify" README.md`; `go test -race ./...` (TestSpecArtifactsCopiedVerbatim asserts staged schemas + field-semantics.md bytes.Equal to committed docs/publish sources and pins the exact expected asset set)
  - Observed: grep count 2 (section present with the fail-closed verify command); test PASS; `v1/spec/field-semantics.md` present in the real export tree
  - Evidence: `README.md` "Consuming the published data"; `docs/publish/spec/field-semantics.md`

- [x] NFR: pinned canonicalization profile centralized at one call site (sorted keys, LF, two-space indent, no HTML escaping, raw UTF-8, integers only) — `TestCanonicalJSONProfile`
  - Command: `go test -race ./...` (TestCanonicalJSONProfile: code-point sort incl. non-ASCII, indent, single trailing LF, unescaped `&<>`, raw UTF-8, bare ints, float → error)
  - Observed: PASS; `canonicalJSON` in `export_canonical.go` is the sole serialization path for the export tree
  - Evidence: `export_canonical.go`; scratchpad `final-validate.log`

- [x] NFR: export shape + schemas fully cover `events`/`tools`/`refs`/`references` even though real data is empty today — synthetic provider in `TestExportFixtureMatchesCommitted` + `TestFreezeFieldsOptionalFromLaunch`
  - Command: `go test -race -run 'TestExportFixtureMatchesCommitted|TestDoubleExportDeterminism|TestGeneratedAtConfinement' .` and full suite for TestFreezeFieldsOptionalFromLaunch
  - Observed: PASS — synthetic doc mirrors events (blocking as plain string), tools, node refs; references trimmed to exactly `{url, verified_at}` (fetch_method/last_content_hash absent from bytes); freeze fields validate as OPTIONAL against both schemas
  - Evidence: `testdata/fixtures/export/expected/v1/capabilities/synthetic.json`

- [x] `go build ./...`, `go vet ./...`, `go test -race ./...` all pass; every new check rides `ci.yml`'s existing command set (no Makefile) — full battery
  - Command: `gofmt -l . && go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test -race ./...`
  - Observed: all five stages PASS (gofmt empty, both builds clean, vet clean, full race suite green); every new test is reachable via `go test ./...` with no new plumbing
  - Evidence: scratchpad `final-validate.log`

## Manual interactions performed

1. Ran a real `capmon export` against the repository's committed data and inspected the staged tree (counts, index contents, advisories, spec files) — recorded under criterion 1.
2. Ran `go run ./cmd/capmon verify` over the post-relocation baselines — exit 0.
3. Launch-runbook items deliberately NOT performable pre-merge/pre-publish, carried to the PR description: enable branch protection + required review on `main`; set Pages source to GitHub Actions; dispatch `publish.yml`; `curl` the live `v1/index.json`; `gh attestation verify` against the downloaded index; confirm a no-op cron run skips deploy.

## Status: PASS
