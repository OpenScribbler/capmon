# Onboarding a New Provider

This is the checklist for adding a new agent tool ("provider") to capmon's
capability monitoring. Order matters: each artifact feeds the next.

Throughout, `<slug>` is the provider's lowercase kebab-case identifier
(e.g. `opencode`, `claude-code`). Use the same slug in every file.

## 1. Source manifest — `docs/provider-sources/<slug>.yaml`

Copy `docs/provider-sources/_template.yaml` and fill it in. This declares
*where* to fetch each content type's authoritative definition (docs URLs,
source files, or schemas) and how change detection works for the provider.

- Set `slug`, `display_name`, `vendor`, `status`, `fetch_tier`, and the
  `repo`/`repo_branch` (omit repo if closed-source).
- Under `content_types`, list the fetch `sources` for each of the six content
  types (rules, hooks, mcp, skills, agents, commands). Set
  `supported: false` and drop the sources for types the provider lacks.
- The six content types are co-equal — cover every type the provider actually
  supports, not a convenient subset.

## 2. Format doc — `docs/provider-formats/<slug>.yaml`

This is the curated mapping from the provider's real fields to ACIF's
canonical vocabulary. For each content type, every provider field lands in
exactly one of two places:

- **`canonical_mappings.<canonical_key>`** — when the field corresponds to an
  existing canonical key. The `<canonical_key>` **must already exist in
  `docs/spec/canonical-keys.yaml`**. Record `supported`, `mechanism`,
  `confidence` (`confirmed` | `inferred` | `unknown`), and `provider_field`
  where applicable.
- **`provider_extensions[]`** — when the field has no canonical key yet. Each
  entry needs `id`, `name`, `summary`, `source_ref`, `conversion`, and
  `graduation_candidate`.

**Never invent a canonical key.** Canonical keys live in
`docs/spec/canonical-keys.yaml`, which is capmon's conforming copy of ACIF's
published vocabulary. The `acifdrift` CI check (`.github/workflows/acif-drift.yml`)
fails on any key in that file absent from ACIF's `capability-vocabulary.yaml`
export. A concept that isn't canonical yet belongs in `provider_extensions` —
it graduates into a canonical key only through ACIF's change process, once the
graduation scan shows 2+ providers use it (see below).

### The load-bearing rule: reuse extension `id`s across providers

When you author a `provider_extension`, **reuse the same `id` another provider
already gave the same semantic concept** — same *meaning*, not just same name.

The extension `id` is the join key the graduation scanner counts across
providers (`ScanGraduationCandidates` in `acifchange_graduation.go`). A concept
observed under the same `id` in 2+ providers, across 2+ distinct scan dates,
files an ACIF Class C graduation candidate. If two providers name the same
concept with divergent `id`s, the count never reaches the threshold and the
concept never graduates.

Before minting a new `id`, grep existing format docs for the concept:

```bash
grep -rn "id: <candidate-id>" docs/provider-formats/
grep -rin "<concept keyword>" docs/provider-formats/
```

Set `graduation_candidate: true` only on extensions you believe are genuine
cross-provider vocabulary candidates (not provider-idiosyncratic quirks) — the
scan only counts candidates flagged `true`.

## 3. Recognizer — `recognize_<slug>.go` (+ `recognize_<slug>_test.go`)

The recognizer detects the provider's content in a repository. Follow the most
recent provider as the template — `recognize_opencode.go` (PR #506). Register
it in an `init()`:

```go
func init() {
    RegisterRecognizer("<slug>", RecognizerKindGoStruct, recognize<Slug>)
}
```

Write table-driven tests alongside it, mirroring the existing
`recognize_*_test.go` files.

## 4. Bootstrap the baseline — `capmon onboard --provider <slug>`

```bash
capmon onboard --provider <slug>
```

This validates the source manifest, fetches every source to populate the cache
baseline, and files a GitHub issue recording the initial content hashes per
content type (`onboard.go`). Because there is no prior baseline, every source
is treated as changed on the first run. Use `--dry-run` to preview without
filing issues.

## What runs automatically afterward

- **`ci` / `acif-drift`** gate every PR: build, test, and the canonical-keys
  drift check against ACIF's published vocabulary.
- **`pipeline`** (daily) fetches, extracts, diffs, and opens heal PRs / drift
  issues when a provider's content changes.
- **`acif-change-scan`** (daily) runs `capmon acif-change scan`, which detects
  extension `id`s that have crossed the 2+ provider / 2+ distinct-date
  threshold and files Class C graduation candidates in the ACIF spec repo.
  This is the mechanism your reused `id`s feed.
