# capmon — Ubiquitous Language

_Canonical domain vocabulary for this repo. When a term has a bold canonical name, use it verbatim in code, docs, tests, commit messages, and PR descriptions. If you see an "alias to avoid," do not use it. Where terms overlap with `docs/design/publish-layer.md`, that accepted design's normative usage takes precedence._

## Domain & actors

| Term | Definition | Aliases to avoid |
| --- | --- | --- |
| **capmon** | The standalone source of truth for AI coding tool capability data plus the monitoring pipeline that keeps it accurate. | capability monitor (in prose) |
| **ACIF** | The Agent Content Interchange Format spec that owns capability dispositions, derivation predicates, and canonical vocabularies; capmon conforms to ACIF, never the reverse. | — |
| **Provider** | An AI coding tool tracked by capmon (claude-code, cursor, zed, …), as distinct from the vendor company behind it. | tool, agent, vendor |
| **Slug** | A provider's permanent lowercase identifier, used as filename, URL path segment, and JSON key; renaming one is a breaking change within a published major. | provider id, provider name |
| **Content type** | One of the six kinds of agent content a provider may support: rules, skills, agents, commands, MCP, hooks. | category, kind |
| **Consumer** | Anything that fetches published capmon data; an integrity-sensitive consumer (syllago's auto-PR pipeline is the canonical example) acts on the data automatically and must verify attestation fail-closed. | client, user |

## Capability model

| Term | Definition | Aliases to avoid |
| --- | --- | --- |
| **Canonical key** | A capability concept registered in the canonical-key registry with a `description`, `type`, and ACIF `spec_ref`, named by a lowercase dot-path segment. | capability key, field |
| **Canonical-key registry** | The machine-readable key vocabulary at `docs/spec/canonical-keys.yaml` (46 keys), capmon's conforming reference copy of ACIF v0.1. | key list, spec file |
| **Vocabulary member** | A sub-value of an object-typed canonical key (e.g. `at_mention` under `invocation_patterns`); in export it is exactly the node without inlined `key` metadata. | sub-capability, nested key, child capability |
| **Key path** | A node's canonical dot-path (e.g. `agents.invocation_patterns`), matching the grammar `^[a-z_]+(\.[a-z_]+)*$`; serialized as `key_path` on registry-backed export nodes. | key name |
| **Capability tree** | The recursive, unbounded-depth structure of capability nodes under each content type; branch nodes carry the same disposition fields as leaves. | capability map, capability list |
| **Disposition** | What a provider does for one capability node: `supported` (true / false / absent-means-unknown), `mechanism` (how, in prose), and `confidence`. | capability status, support flag |
| **Confidence** | The evidence tier of a disposition — `confirmed` (primary source cited), `inferred` (derived, no primary citation), `unknown` (re-verification failed or never verified); an open enum for consumers. | certainty, trust level |
| **Capability baseline** | The authoritative per-provider capability data at `docs/provider-capabilities/<slug>.yaml`, maintained by the pipeline. | provider capabilities file, baseline (unqualified) |
| **Provider-exclusive capability** | A provider behavior recorded under `provider_exclusive` that has no canonical key yet; graduates to a canonical key when 2+ providers demonstrate the same semantic concept. | provider extension |

## Monitoring pipeline

| Term | Definition | Aliases to avoid |
| --- | --- | --- |
| **Run** | One execution of the pipeline (`fetch → extract → recognize/derive → diff → report`), identified by a `run_id`. | sweep, cycle (see ambiguities) |
| **Source** | One fetchable upstream URL (docs page, schema, source code) that provides evidence for a provider's capabilities. | link, reference |
| **Source manifest** | The per-provider YAML under `docs/provider-sources/` listing every monitored source URL per content type, plus change-detection and fetch-tier config. | manifest (unqualified) |
| **Format doc** | The per-provider YAML under `docs/provider-formats/` describing how the provider represents each content type, with `canonical_mappings` from canonical keys to provider-native fields. | format YAML, provider format |
| **Extractor** | A per-format Stage-2 component (HTML, Markdown, Go, Rust, TypeScript, JSON, JSON Schema, YAML, TOML) that turns a fetched source into comparable field sets and landmarks. | parser |
| **Landmark** | A structural anchor (typically an H2/H3 heading) emitted by an extractor, which recognizers pattern-match against. | heading, anchor |
| **Recognizer** | A per-provider component that maps extracted fields and landmarks onto dispositions for canonical keys; recognizer silence about a capability requires live-URL verification, never cache content alone. | matcher, detector |
| **Seeder spec** | The human-in-the-loop review document (`<slug>-<content_type>.yaml`) derived from a format doc, carrying `proposed_mappings` of canonical keys to provider fields; internal pipeline state, never published. | proposed-mappings file, seed spec |
| **Drift** | Any divergence between committed capability data and what upstream provider sources currently say — including 404s, moved pages, and thin caches, which are evidence of drift, never won't-fix justification. | staleness, change |
| **Drift guard** | A CI test class that compares recognizers, format docs, and seeder specs against the committed data under `docs/` in the same checkout. | conformance test |
| **Healing** | The pipeline's self-repair of moved or renamed source URLs; each attempt is recorded as a heal event, success opens a heal PR, and repeated failure escalates to a heal-failure issue. | auto-fix, self-repair |
| **Run manifest** | The write-only observability record of one run (per-provider statuses, heal events, exit class), persisted as `last-run.json`; never a pipeline input. | manifest (unqualified) |
| **Heartbeat** | The scheduled commit that keeps GitHub's 60-day-inactivity rule from disabling the cron; per ADR 0005 it moves off `main` so no scheduled job holds write access to the publish branch. | keepalive |

## Publish layer

| Term | Definition | Aliases to avoid |
| --- | --- | --- |
| **Export** | Compilation (`capmon export`) of capability baselines joined with the canonical-key registry into validated JSON documents in a temp dir — distinct from the deploy step that publishes them. | publish (for the compile step) |
| **Export surface** | The set of published files on GitHub Pages: discovery indexes, per-provider capability documents, pivots, spec files, schemas, and advisories. | the site, the API |
| **Major** | The URL path prefix (`/v1/`) that versions the contract — document shapes, path layout, field semantics — never the data; only breaking changes bump it. | version (unqualified), API version |
| **schema_version** | An informational, per-schema monotonic shape-revision counter carried by every published document; never a compatibility signal and never a meaning-change signal. | version (unqualified) |
| **data_revision** | The hash in `v1/index.json` computed over provider data only, so "did anything change?" is one field compare. | content hash, etag |
| **Discovery index** | An `index.json`: the unversioned append-only root listing majors, or the per-major root carrying `generated_at`, cadence, staleness bounds, `data_revision`, `source_commit`, and per-file `sha256` values. | catalog, directory |
| **Advisory** | A published correction record in a major's `advisories.json` — the sole permitted post-freeze mutation — pointing at a path and key path with a corrected value. | errata, correction notice |
| **Freeze** | The single tested mutation (`capmon freeze <major>`) that turns a superseded major into an immutable static tree with `status: "frozen"`, `superseded_by`, `frozen_at`, and an externally recorded root hash. | archive, sunset, deprecate |
| **Tombstone** | The retained document and index entry (`status: "removed"`, `removed_at`) for a dropped provider, honoring slug permanence — published URLs never 404 within a major. | deletion, removal |
| **Canonicalization profile** | The pinned byte-level output rules (UTF-8, LF, sorted keys, two-space indent, no HTML escaping, integers only) that make a data-identical export byte-identical; changing it is breaking. | deterministic output, formatting rules |
| **Provenance attestation** | The workflow-identity binding (`actions/attest-build-provenance`) over the publish artifact that upgrades the index hashes from change-detection checksums to an authenticity chain. | signature, checksum |
| **Fail-closed publishing** | The gate that validates every export output against published schemas and asserts the provider set matches the source data before deploy; any deviation aborts so a partial export never replaces a good site. | safe deploy, best-effort publish |
| **Reference provenance** | The source-URL provenance carried by the `references` registry and node-level `refs` arrays — where a claim came from and when it was last checked. Distinct from **provenance attestation** (workflow identity over the publish artifact); never say bare "provenance". | provenance (unqualified) |

## Relationships

- A **provider** has exactly one **slug**, one **source manifest**, one **format doc**, and one **capability baseline**.
- A **capability baseline** contains one **capability tree** per **content type** it declares.
- A **canonical key** belongs to exactly one **content type**; an object-typed canonical key owns zero or more **vocabulary members**.
- A **format doc** derives one **seeder spec** per (provider, content type) pair.
- A **recognizer** consumes **landmarks** produced by **extractors** from fetched **sources** and emits **dispositions** keyed by **canonical keys**.
- Each **run** produces exactly one **run manifest** and zero or more heal events; failed **healing** escalates to a heal-failure issue.
- **Export** compiles all **capability baselines** joined with the **canonical-key registry** into the current **major** of the **export surface**; there is never more than one live export code path.
- Each **major** carries exactly one per-major **discovery index** and one **advisory** channel; after a **freeze**, the advisory channel is the only thing that may still change.
- **data_revision** changes iff provider data changes; **schema_version** changes iff a document shape changes; the **major** changes iff the contract breaks.

## Flagged ambiguities

- **"manifest"** names three unrelated artifacts: the provider **source manifest** (`provmon/manifest.go`, `docs/provider-sources/`), the **run manifest** (`types.go` `RunManifest`, observability output), and the export gate in `docs/design/publish-layer.md` ("asserts the provider set matches the source manifest"). These are distinct: one is pipeline input, one is pipeline output, one is a publish-time assertion source. Never write bare "manifest" — always **source manifest** or **run manifest**.
- **Stage 4 is named both "review" and "report."** `types.go` says "fetch → extract → diff → review"; the README, `capmon run --stage report`, and `pipeline.yml` say **report**. Recommend **report** everywhere (it is the CLI-facing name); update the `types.go` package comment.
- **"run" vs "sweep" vs "cycle"** all denote pipeline execution: `RunManifest`/`run_id` in code, "scheduled sweeps" in `canonical-keys.yaml` and the publish design's confidence rules, "one capmon cycle" in the key lifecycle policy. Recommend **run** for an execution; reserve **sweep** narrowly for the re-verification behavior that downgrades confidence to `unknown`; avoid "cycle".
- **"baseline"** means both the **capability baseline** (`docs/provider-capabilities/<slug>.yaml`) and `change_detection.baseline` in source manifests (`provmon/manifest.go` — an opaque version-tag comparison reference). Always say **capability baseline** for the former; call the latter the **detection baseline**.
- **"status"** is four unrelated enums sharing one field name: export document status (`live`/`frozen`/`removed`/`withdrawn`), index provider-entry status (`tracked`/`removed`), source manifest status (`active`/`archived`/`beta`), and format doc content-type status (`supported`/`unsupported`). Always qualify which status is meant; the publish layer must not reuse the source-manifest enum values.
- **"capability"** alone is ambiguous: in `capyaml` every tree node is a `CapabilityEntry`, but only registry-backed nodes are **canonical keys** — vocabulary members are also "capabilities" in the YAML. The export discriminator is normative (`key` present ⇔ canonical key); in prose, say **canonical key**, **vocabulary member**, or **capability node** rather than bare "capability" when the distinction matters.
- **"verify"** carries four meanings: `capmon verify` (schema validation of capability YAML), `capmon export --verify` (determinism rebuild-and-diff), consumer-side attestation verification (`gh attestation verify`), and `last_verified` (when capmon last checked the source — explicitly *not* when data changed, per the consumer contract). Qualify in any sentence where more than one could apply, especially in publish-layer docs.
- **`refs` vs `references`**: `refs` is a node-level array of reference IDs (on capability/event/tool entries); `references` is the per-provider provenance registry mapping those IDs to source URLs (`ReferenceEntry`; exported form trimmed to `url` + `verified_at` only). Never use one for the other.
- **`provider_exclusive` vs "provider_extensions"**: `capyaml/types.go` uses `ProviderExclusive`, while the `canonical-keys.yaml` lifecycle notes say "provider_extensions graduation candidates" for what appears to be the same concept. Recommend **provider-exclusive** (matches the shipped field and the Phase 4b typing work) and amending the registry prose.
