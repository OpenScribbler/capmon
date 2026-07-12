# ADR Index

Architectural decisions for this project. Before modifying files in a listed scope, read the full ADR.

| ADR | Title | Status | Enforcement | Scope | Summary |
|-----|-------|--------|-------------|-------|---------|
| [0001](0001-version-the-contract-not-the-data.md) | Version the Contract, Not the Data | accepted | advisory | `export*.go` | URL major versions shapes/paths/semantics, never data; old majors freeze as static files, advisories are the sole post-freeze mutation |
| [0002](0002-self-describing-export-documents.md) | Self-Describing Export Documents | accepted | advisory | `export*.go` | Exported docs inline key_path + registry metadata at registry-backed nodes; key presence discriminates canonical keys from vocabulary members |
| [0003](0003-stable-paths-over-content-addressing.md) | Stable Mutable Paths over Content-Addressed Files | accepted | advisory | `export*.go` | One stable URL per provider; CDN skew handled by re-fetch rule, not hashed filenames |
| [0004](0004-attestation-and-fail-closed-publishing.md) | Attestation at Launch; Hashes Are Change-Detection Only | accepted | strict | `.github/workflows/*.yml` | Publish artifact gets provenance attestation; integrity-sensitive consumers verify fail-closed; invalid exports never deploy |
| [0005](0005-no-scheduled-write-to-publish-branch.md) | No Scheduled Job Holds Write Access to the Publish Branch | accepted | strict | `.github/workflows/pipeline.yml` | Heartbeat moves off main; branch protection makes "on main" equal "human-reviewed" |
