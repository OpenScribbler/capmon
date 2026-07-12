# Phase 4 — Publish Layer Design

Status: ACCEPTED — revised after four-reviewer panel (spec-purist,
registry-operator, solo-publisher, valsorda) and signed off by Holden,
2026-07-11. The three panel-surfaced decisions are settled below.
Date: 2026-07-11

## Settled decisions (2026-07-11 session with Holden)

1. **Self-describing per-provider documents.** Each exported
   `capabilities/<slug>.json` inlines canonical-key metadata (`description`,
   `type`, `spec_ref`) at every registry-backed capability node, alongside
   the provider's observed values. One fetch tells a stranger what the key
   means, where the ACIF spec section lives, and what this provider does.
2. **Versioned URL paths from day one** (`/v1/`), with the versioning
   strategy below.
3. **Phase 4b split by surface.** `ProviderExclusive` typing lands with
   Phase 4 (it shapes exported JSON). FormatDoc `schema_version` and the
   manifest enums (`status`/`blocking`/`fetch_tier`) are internal pipeline
   state and follow immediately after.

## Export surface

Compiled by `capmon export` from `docs/provider-capabilities/<slug>.yaml`
(15 providers today) joined with `docs/spec/canonical-keys.yaml` (46 keys).
Source manifests, format docs, and `<slug>-<type>.yaml` proposed-mappings
files are internal pipeline state and are **not** published.

```
https://openscribbler.github.io/capmon/
├── index.json                            # root discovery doc (unversioned, append-only)
└── v1/
    ├── index.json                        # per-major discovery root
    ├── advisories.json                   # corrections channel (mutable even after freeze)
    ├── capabilities/
    │   ├── <slug>.json                   # one per provider, self-describing
    │   └── all.json                      # every provider, same node shape
    ├── by-content-type/
    │   └── <type>.json                   # pivot: one content type × all providers
    ├── spec/
    │   ├── canonical-keys.json           # the key registry (46 keys)
    │   └── field-semantics.md            # prose semantics not discharged to ACIF
    └── schemas/
        ├── provider-capabilities.json    # for capabilities/<slug>.json
        ├── all-providers.json            # for capabilities/all.json
        ├── by-content-type.json          # for by-content-type/<type>.json
        ├── index.json                    # for v1/index.json
        ├── advisories.json               # for advisories.json
        └── canonical-keys.json           # for spec/canonical-keys.json
```

Schema `$id`s take their final form here:
`https://openscribbler.github.io/capmon/v1/schemas/<name>.json`. Every
published document shape has a published schema — a shape that is
normatively frozen but unvalidatable is not a contract. Consumers SHOULD
vendor the schemas rather than `$ref`-fetch them per validation.

## Consumer contract (normative)

RFC 2119 keywords. This section is the contract; everything else in this
document is design rationale.

### Compatibility rules

- Consumers MUST ignore unknown fields, unknown canonical keys, and unknown
  files. Producers MAY add any of these within a major.
- Consumers MUST treat every enum-valued field (`confidence`, `status`,
  `conversion`, …) as open: an unrecognized value MUST be handled as
  "unknown/other", never as an error. Adding an enum value is additive;
  removing or repurposing one is breaking.
- Field and canonical-key **semantics are immutable within a major**. A
  meaning change MUST ship as a rename (a detectable breaking change) or a
  major bump; it MUST NOT ship by repurposing an existing name.
  `schema_version` MUST NOT be used to signal a meaning change.
- **Slugs are permanent within a major.** A published
  `capabilities/<slug>.json` URL MUST NOT 404 for the life of the major. If
  a provider is dropped, its document remains published with
  `"status": "removed"` and its `index.json` entry becomes a tombstone
  (`"status": "removed"`, `"removed_at"`). Renaming a slug is breaking.
- All timestamps are RFC 3339 in UTC with a `Z` offset.

### The capability tree

- The tree is **recursive with unbounded depth**. Any node MAY carry a
  `capabilities` map of child nodes. Branch nodes carry the same disposition
  fields as leaves (`supported`, `mechanism`, `confidence`), so a leaf
  gaining children is additive: existing reads stay valid. (The source data
  already works this way — `agents.invocation_patterns` carries
  `supported: true` *and* nested capabilities.)
- Every node whose key is registered in the canonical-key registry carries
  `"key_path"` (its canonical dot-path, e.g.
  `"agents.invocation_patterns"`) and a `"key"` object
  (`description`, `type`, `spec_ref`) inlined from the registry.
- Nodes without `key` metadata are **vocabulary members** — sub-values of an
  object-typed canonical key (e.g. `at_mention` under
  `invocation_patterns`). The discriminator is normative: `key` present ⇔
  the node is a registered canonical key. Vocabulary-member semantics live
  in the parent key's `description` and its ACIF `spec_ref`.
- `key.type` describes the concept as registered; it does NOT predict the
  node's JSON shape. Consumers MUST inspect nodes structurally.

### Interpreting dispositions

| Observation | Meaning |
|---|---|
| `"supported": true` | verified or inferred support (see `confidence`) |
| `"supported": false` | affirmatively not supported |
| `supported` absent | **unknown** — never treat as false |
| `"confidence": "confirmed"` | verified against provider docs/source |
| `"confidence": "inferred"` | derived from provider docs, not directly verified |
| `"confidence": "unknown"` | previously known; re-verification failed (scheduled sweeps downgrade to this) |

`last_verified` means "when capmon last checked the source", not "when the
data last changed".

### Freshness, staleness, and polling

- `v1/index.json` carries `generated_at`, `cadence: "daily"`, and
  `max_staleness_hours`. If `generated_at` is older than
  `max_staleness_hours`, consumers SHOULD treat the feed as stale and keep
  their last-known-good copy.
- `data_revision` in `index.json` is a hash over provider data only; it
  changes **only when data changes**. "Did anything change?" is one field
  compare. Per-file `sha256` values identify which files changed.
- Consumers SHOULD poll at most daily and MUST use HTTP conditional GET
  (`If-None-Match` / `If-Modified-Since` — GitHub Pages serves ETags and
  returns 304).
- The feed is **best-effort, no SLA**. Consumers MUST cache last-known-good
  and tolerate origin outages.
- Deploys are not atomic at the CDN edge. On an `index.json`-vs-file hash
  mismatch, consumers MUST re-fetch after the propagation window (minutes)
  before treating it as an integrity failure.

### Integrity

- The `sha256` values in `index.json` are **change detection and
  corruption detection**, not authenticity: they share an origin and a fate
  with the files they hash.
- Authenticity comes from workflow provenance attestation (see Publish
  mechanics — pending decision). Integrity-sensitive consumers (syllago's
  auto-PR pipeline is the canonical example) MUST verify the attestation
  over `index.json`, then each file's `sha256` against it, and MUST fail
  closed — a mismatch aborts the fetch and triggers nothing downstream.

### `schema_version`

- Every published document and schema carries `schema_version`: a monotonic
  integer serialized as a string, compared numerically, **scoped per
  schema** (bumping one schema does not bump the others).
- It is informational: producers MUST bump it on any shape change so
  operators can detect revisions; consumers MUST NOT alter parsing behavior
  on it and MUST NOT treat an unrecognized value as an error. The URL major
  is the compatibility contract; `schema_version` is not a version to
  "upgrade to".

## Versioning strategy

### Principle: version the contract, not the data

The Pages site is a **live view of current data**, not an archive. Data
changes are not version events; git history is the archive. The URL major
versions the *contract*: document shapes, path layout, field semantics.

| Signal | Where | Job | Changes when |
|---|---|---|---|
| URL major (`/v1/`) | path prefix | compatibility contract | breaking change only |
| `schema_version` | every document + schema | shape revision within a major | any shape change, per schema |
| `data_revision` | `v1/index.json` | data change detection | provider data changes |
| `generated_at` | `v1/index.json` | freshness heartbeat | every publish |

### Breaking (bumps the URL major)

- Removing or renaming a published field, path, file, or provider slug
- Changing a field's type
- Changing a field's or key's meaning (which, per the rename rule, can only
  manifest as a rename)
- Removing or repurposing an enum value
- Changing the canonicalization profile or key grammar

### Additive (bumps the owning schema's `schema_version` only)

- New fields, canonical keys, providers, content types, enum values, files
- A leaf node gaining child capabilities

### Major transitions: freeze, don't fork

The exporter only ever generates the **current** major. There are never two
live export code paths.

- The freeze fields (`status`, `superseded_by`, `frozen_at`) are defined as
  OPTIONAL in `v1/schemas/index.json` and the per-provider schema **from
  initial publication**, `status` defaulting to `"live"` — so the freeze
  mutation cannot produce a schema-invalid document against the pinned v1
  schemas.
- `capmon freeze v1` performs the transition as one tested command, not a
  runbook: copy the last live v1 tree to `site-static/v1/`, apply exactly
  one final mutation (set `status: "frozen"`, `superseded_by: "/v2/"`,
  `frozen_at` in `v1/index.json` **and in every per-provider document**),
  validate the frozen tree against the v1 schemas, record the frozen tree's
  root hash, and update the root `index.json`.
- After that single mutation the tree is immutable — with exactly one
  carve-out: `advisories.json` (below).
- Frozen-tree tamper evidence: the frozen root hash is recorded outside the
  tree (in the live root `index.json` entry for the frozen major, and a
  signed git tag). CI re-hashes `site-static/` on every pipeline run and
  fails on mismatch. A frozen tree never vouches for itself.
- The deprecation signal lives **in the documents consumers actually
  fetch**: every per-provider document carries `status`, and frozen ones
  carry `superseded_by`/`frozen_at` — not only `index.json`.

### Advisories: the correction channel for frozen data

Freezing shapes forever is safe; freezing *wrong data* forever is a
liability (the "frozen major says hooks are sandboxed when they aren't"
scenario). Each major carries `advisories.json`, regenerated on every
deploy even for frozen majors — the sole permitted post-freeze mutation:

```json
{
  "schema_version": "1",
  "advisories": [
    {"id": "CAPMON-2027-001", "published_at": "…", "severity": "high",
     "path": "capabilities/claude-code.json",
     "key_path": "hooks.sandboxing", "note": "…",
     "corrected_value": {"supported": false}}
  ]
}
```

`status` gains a `"withdrawn"` value for the nuclear case (an entire major
unsafe to trust). Consumers pinning any major SHOULD fetch its
`advisories.json`; the repo's releases feed is the human-subscribe channel.

### index.json (per-major discovery root)

```json
{
  "schema_version": "1",
  "status": "live",
  "generated_at": "2026-07-12T09:00:00Z",
  "cadence": "daily",
  "max_staleness_hours": 48,
  "data_revision": "<sha256 over provider data only>",
  "source_commit": "<last commit touching docs/, not HEAD>",
  "providers": [
    {"slug": "claude-code", "path": "capabilities/claude-code.json",
     "status": "tracked", "sha256": "…", "last_verified": "…"}
  ],
  "files": {"capabilities/all.json": {"sha256": "…"}}
}
```

`source_commit` is derived from `git log -1 --format=%H -- docs/` — the
last commit that changed source data — never from `HEAD`, which moves daily
with heartbeat commits.

### Root index.json (unversioned, append-only)

```json
{
  "latest": "v1",
  "majors": [
    {"prefix": "v1", "status": "live", "index": "v1/index.json"}
  ]
}
```

Normative: this shape is **append-only for the life of the site** — fields
are never removed or repurposed; the `majors` array only grows; consumers
MUST ignore unknown fields and entries. On a freeze, the frozen entry gains
`superseded_by` and `frozen_root_sha256`.

## Publish mechanics

- **Primary trigger: push to `main` that changes `docs/`** (from `ci.yml`).
  The daily pipeline run re-stamps freshness and **always deploys** (ADR
  0012): a data-identical export differs only in `v1/index.json`, and
  shipping that one file daily is what makes `generated_at` a truthful
  liveness heartbeat under the `max_staleness_hours` contract.
- **Fail closed.** `capmon export` writes to a temp dir, validates every
  output file against the published schemas, and asserts the provider set
  matches the source manifest (count + slugs). Any deviation fails the job
  and no deploy happens — a partial export must never replace a good site,
  because a missing provider file is an unversioned breaking change and
  Pages has no rollback.
- Deploy via `actions/upload-pages-artifact` + `actions/deploy-pages`
  (pinned by commit SHA, as the rest of `pipeline.yml` already does); no
  `gh-pages` branch. Frozen majors live in `site-static/` and are copied
  into the artifact.
- **Provenance attestation** (pending decision): the publish job runs
  `actions/attest-build-provenance` over the artifact, binding it to the
  workflow identity and `source_commit`; consumers verify with
  `gh attestation verify`. This is what upgrades the hashes from checksums
  to an integrity chain.
- **Repo hardening that this design assumes:** branch protection + required
  review on `main`; the daily heartbeat moves off `main` (orphan branch,
  tag, or Actions API) so no scheduled job holds `contents: write` on the
  branch that feeds publishing.

### Determinism (conformance-testable)

The canonicalization profile is pinned, not aspirational:

- UTF-8, LF line endings, single trailing newline, two-space indentation.
- Object keys sorted ascending by Unicode code point; no HTML escaping
  (`SetEscapeHTML(false)`); non-ASCII emitted as raw UTF-8; integers only,
  no floats.
- No timestamp or run-varying value in any document except
  `v1/index.json` (`generated_at`) — a data-identical export is
  byte-identical everywhere else.

Enforced by: a committed byte-level fixture (`testdata` input →
exact-bytes JSON output), a CI double-export diff, a pinned Go toolchain
version, and `capmon export --verify <commit>` which rebuilds from a source
commit and diffs against the published tree. Changing the profile is a
breaking change — which is only enforceable because the profile is written
down.

## Per-provider document shape

```json
{
  "schema_version": "1",
  "status": "live",
  "slug": "claude-code",
  "display_name": "Claude Code",
  "last_verified": "2026-07-10",
  "content_types": {
    "agents": {
      "supported": true,
      "capabilities": {
        "invocation_patterns": {
          "supported": true,
          "key_path": "agents.invocation_patterns",
          "key": {
            "description": "…from canonical-keys.yaml…",
            "type": "object",
            "spec_ref": "ACIF-AGENT §9.1 (DERIVABLE)"
          },
          "capabilities": {
            "at_mention": {
              "supported": true,
              "mechanism": "@-mention syntax …",
              "confidence": "inferred"
            }
          }
        },
        "model_selection": {
          "supported": true,
          "mechanism": "per-subagent model override …",
          "confidence": "inferred",
          "key_path": "agents.model_selection",
          "key": {
            "description": "…",
            "type": "bool",
            "spec_ref": "ACIF-AGENT §9.1 (DERIVABLE)"
          }
        }
      }
    }
  }
}
```

Note both cases: `invocation_patterns` is a registry-backed **branch**
(has `key` + child `capabilities`; children are vocabulary members without
`key`); `model_selection` is a registry-backed **leaf**. `display_name`
falls back to the slug in export — it is never empty in published output.

## Field-semantics spec (audit finding #2)

Published at `v1/spec/field-semantics.md`. The load-bearing consumer rules
(nil-vs-false, confidence enum + sweep downgrade, enum openness) are in the
consumer contract above; the prose spec carries the remainder not
discharged by `spec_ref` → ACIF: dot-path key grammar
`^[a-z_]+(\.[a-z_]+)*$` and its relationship to vocabulary members,
parent-`supported` auto-flip, the `conversion` enum, provenance of
`mechanism` strings. (Check during implementation how much ACIF now owns.)

## Data-quality follow-ups (pre-launch, from panel)

- `key.type: object` keys (`decision_control`, `tool_filtering`) whose
  provider nodes are flat leaves: reconcile data or rely on the "inspect
  structurally" rule — audit before launch.
- Empty `display_name`/`last_verified` in source YAML: exporter fallback
  covers publication; backfill source data anyway.

## Panel-surfaced decisions (settled with Holden, 2026-07-11)

1. **Provenance attestation ships at launch.** The publish job runs
   `actions/attest-build-provenance` over the artifact; verification via
   `gh attestation verify` is mandatory (fail-closed) for syllago and
   documented as recommended for other consumers.
2. **Stable mutable paths, not content-addressed files.** One stable URL
   per provider is the headline feature; deploy-pages purges the CDN on
   deploy and data changes at most daily, so the skew window is handled by
   the re-fetch-on-mismatch rule. Frozen-tree tamper evidence comes from
   the externally recorded root hash, not hashed filenames.
3. **Repo hardening lands with Phase 4, before the first publish:** branch
   protection + required review on `main`; heartbeat moved off `main` so no
   scheduled job holds `contents: write` on the branch that feeds
   publishing.

**Deferred (deliberately not built now):** the `capmon freeze` command is
specified above but implemented when a v2 first approaches — freezing is
unexercisable until then, and building it now is speculative. What ships in
Phase 4 is the part that cannot be retrofitted: the freeze fields
pre-provisioned as OPTIONAL in every v1 schema from initial publication.

## Panel disposition ledger

Blocking findings and their resolution in this revision: variable-depth
tree + no node identifier → `key_path` + recursion made normative;
self-describing gap at vocabulary members → `key`-presence discriminator;
slug lifecycle → permanence + tombstones; freeze self-contradiction →
pre-provisioned optional fields + single-mutation freeze via
`capmon freeze`; meaning-change untestability → rename-or-major rule;
root index eternity → append-only rule; frozen-data corrections →
`advisories.json` + `withdrawn`; partial-export auto-publish →
fail-closed gating; same-origin hashes → labeled as change-detection,
attestation pending decision 1; frozen-tree self-vouching → external root
hash + CI re-verify; consumer verification unspecified → fail-closed MUST
for integrity-sensitive consumers.
