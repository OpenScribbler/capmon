---
id: "0014"
title: "Heals Are Always Review-Gated, Never Auto-Applied"
status: accepted
date: 2026-08-24
enforcement: strict
files: ["healing.go", "healing_github.go", "healing_pr.go", "pipeline.go"]
tags: [healing, drift, review-gate]
---

# ADR 0014: Heals Are Always Review-Gated, Never Auto-Applied

## Status

Accepted

## Context

**Healing** repairs a **source** URL that moved. On success the pipeline opens a
**heal PR** against the **source manifest**; on repeated failure it escalates to a
heal-failure issue. This is already how the code behaves — `tryHealSource` always
ends at `ProposeManifestHealPR`, and no auto-merge exists in the Go or in
`pipeline.yml`. The invariant was never written down, and nothing prevented a
future change from removing it.

The reason it must not be removed is that capmon cannot detect a *plausible but
wrong* heal. `ValidateContentResponse` checks that the replacement is reachable,
is text, is large enough, and is same-host. A different crate's `loader.rs` passes
every one of those checks while being the wrong file.

Measured against the live `openai/codex` tree (6,504 files) while resolving
capmon-x88:

| basename | occurrences in tree | that heal is |
| --- | --- | --- |
| `loader.rs` | 1 | wrong |
| `model.rs` | 7 | correct |

This rules out the obvious safeguard. A gate that declines "low-confidence"
renames would score the *wrong* heal highest — it is the only `loader.rs` in the
repository, so it has no competitors and the widest possible margin — while the
*correct* heal competes against six same-named files. Score floors, margin gates,
and candidate-ambiguity gates all rank these two cases backwards. Match strength
is not evidence of correctness here, so no threshold over it can be one.

(Note: **confidence** in this repo's lexicon means the evidence tier of a
**disposition** — `confirmed`, `inferred`, `unknown`. The rename-match sense is
unrelated; say *match score* for that, never "confidence".)

## Decision

**A heal is a proposal, never an application.** Every successful heal opens a heal
PR for a person to approve. capmon never merges its own heal PR and never edits a
source manifest on `main`.

capmon does **not** gate heals on match score. The evidence above shows such a
gate would invert on the known cases. Investment goes into making the human gate
effective instead:

- The heal PR diff must be minimal — one changed URL is one changed line
  (ADR-relevant work landed in PR #63; `UpdateManifestURL` splices into the
  original bytes rather than re-encoding the document).
- The heal PR body must show **every ranked candidate**, including candidates that
  were ranked but never probed because an earlier one validated. Previously the
  body showed a single row in exactly that case, which is the case most likely to
  be wrong.

Improving *ranking* remains in scope and is tracked separately (capmon-636);
improving ranking is not a substitute for review.

## Consequences

- `TestHealPathNeverMerges` fails the build if a merge invocation appears in the
  heal path or in `pipeline.yml`. This is what makes the enforcement `strict`.
- Heal PRs accumulate when nobody triages them. That is the intended failure mode:
  drift stays visible and unapplied, rather than being silently absorbed.
- capmon's PR-creating credential needs no merge capability. `CAPMON_HEAL_PR`
  should not be granted one.
- A wrong heal costs one closed PR. An auto-applied wrong heal would bind a
  provider's source to an unrelated file and be discovered only when the
  **capability baseline** drifted — the failure mode that bound a skills source to
  a TUI render cell for two weeks.
