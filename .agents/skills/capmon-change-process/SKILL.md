---
name: capmon-change-process
description: Use when processing open `capmon-change` GitHub issues in OpenScribbler/capmon, the per-provider issues the daily drift check files when a monitored source's content hash changes or its fetch fails. Triages each changed or failing source against live upstream, curates the provider's format doc and capability baseline, re-baselines the hashes, and opens one drift PR per provider for a person to review. Takes an optional provider slug and an optional --dry-run.
---

# capmon-change-process

Works one provider's `capmon-change` issue at a time, from triage to a drift PR
that closes it. Run it from the capmon repo root.

Arguments:

- `<slug>`: process only that provider's issue. Without it, take the smallest
  open issue first and stop after each provider to report.
- `--dry-run`: run every step up to the commit (step 8), then report what the
  PR would contain. Commit nothing, push nothing, open nothing.

## What a capmon-change issue is

`capmon check --all` runs daily in `.github/workflows/pipeline.yml`. For each
provider it fetches every source listed in `docs/provider-formats/<slug>.yaml`
and compares the SHA-256 of the body against that source's `content_hash`.
Everything it finds for one provider goes into one issue titled
`capmon: changes detected for <slug>`, labelled `capmon-change` and
`provider:<slug>`:

```
<!-- capmon-check: <slug> -->
## <content_type>

- <source uri>
  Old hash: sha256:...
  New hash: sha256:...

## Fetch Errors

- <source uri> (<content_type>): fetch error: unexpected status 404
```

The check rewrites the body in place on every run, so the body is the latest
run's view and the issue's `updatedAt` says when that run was. The check never
closes the issue. It stops reporting a source once the format doc's
`content_hash` matches the live body again, and the drift PR closes the issue
with `Closes #<n>`.

A hash change says only that the bytes changed. Most changes are edits that
touch nothing capmon records, and the work is to find out which kind this is.

## The files a fix touches

| File | What it holds | How it changes |
|---|---|---|
| `docs/provider-formats/<slug>.yaml` | The format doc: sources with `uri` and `content_hash`, plus `canonical_mappings` and `provider_extensions` per content type | Edited by hand |
| `docs/provider-sources/<slug>.yaml` | The source manifest: the fetch URLs the pipeline and heal PRs use, plus `docs_conventions` quirks | Edited by hand, only when a URL moves |
| `docs/provider-capabilities/<slug>.yaml` | The capability baseline, which the published feed exports | Edited by hand, only when a published mapping changed |
| `docs/provider-capabilities/<slug>-<content_type>.yaml` | Derived seeder specs | Written by `capmon derive`, never by hand |
| `docs/provider-capabilities/by-content-type/*.yaml` | Cross-provider views | Written by `capmon generate` |

`TestDeriveOutputMatchesCommitted` fails when a format doc and its derived
seeder specs disagree, and `TestAllBaselinesBuildProviderDocs` fails when a
capability baseline has a non-canonical key in a canonical position.

Rules that bind this work:

- `.claude/rules/capmon-drift-detection.md`: a 404 or a thin page is evidence
  of drift and never a reason for a won't-fix. A won't-fix needs a
  `Verified live:` line.
- `docs/onboarding.md`, section 2: the `mapped`, `absent`, and `unmapped`
  statuses; `unmapped` needs a verbatim `source_form`; never invent a canonical
  key; reuse another provider's extension `id` for the same concept.
- ADR 0014: a heal PR is a proposal. Match a moved source by content, never by
  filename.
- `AGENTS.md`: a PR on a `capmon/drift-*` branch waits for a person's approval.
  Never merge it with `--admin`.

## Step 0: Preconditions

```bash
git status --porcelain            # must be empty
git switch main && git pull --ff-only
gh auth status
go build -o capmon ./cmd/capmon   # the binary is gitignored
go test -run 'TestDeriveOutputMatchesCommitted|TestAllBaselinesBuildProviderDocs' .
```

Stop if the test fails. A failing test means `main` already disagrees with
itself, and every PR this skill opens would carry that unrelated churn.

## Step 1: Pick the issue

```bash
gh issue list --label capmon-change --state open --limit 100 \
  --json number,title,labels,updatedAt,body
```

Take the provider from the `provider:<slug>` label or the anchor. An issue
without the `<!-- capmon-check: <slug> -->` anchor was filed by hand, so report
it and skip it.

Confirm the body is still current. The dry run prints one count line for the
provider and nothing when the provider is clean:

```bash
./capmon check --provider <slug> --dry-run
# dry-run: would create issue for <slug> (<n> changes, <m> fetch errors)
```

When the counts differ from the issue body, trust the dry run, because the body
dates from the last scheduled run that reached this provider. The check visits
providers in alphabetical order and stops at the first error, so one failing
provider leaves every later provider's issue stale. Check the job before you
trust any body:

```bash
gh run list --workflow pipeline.yml --limit 5 --json conclusion,createdAt
gh run view <run-id> --log-failed | grep -i error   # when the latest run failed
``` A provider that prints nothing has no work
left; its issue only needs closing, and a person closes it.

## Step 2: Check for an open heal PR

```bash
gh pr list --state open --json number,title,headRefName \
  --jq '.[] | select(.headRefName | startswith("capmon/heal-<slug>/"))'
```

A heal PR proposes a replacement URL for a source that stopped fetching. For
each one, read its diff with `gh pr diff <n>`, fetch the proposed URL, and
decide whether it is the same document by content: the same headings, fields,
or constants the old source was anchored for. A name match alone is not
evidence; ADR 0014 records a heal that matched a lone `loader.rs` and was wrong.

Never merge, close, or comment on a heal PR from this skill. Use a correct
heal's URL in your own fix and name the heal PR in your PR body, so the
reviewer can close it. Name a wrong one in your PR body with the evidence.

## Step 3: Triage each changed source

Build the work list from the issue body as `(content_type, uri)` pairs. One URI
can appear under several content types; curate it once for each.

For every pair:

1. Re-hash it the way the check does:

   ```bash
   ./capmon hash-url <uri>
   ```

   Use this command and never plain `curl` for any hash you write. Several
   hosts serve different bytes under different headers, and `hash-url` sends
   the check's exact `User-Agent`, `Accept`, and `Accept-Encoding`. When its
   hash differs from the issue's `New hash`, the source changed again since
   the issue was written; use the `hash-url` value.

2. Find what changed.
   - For a `raw.githubusercontent.com` source, list upstream commits to that
     path since the format doc's `fetched_at` for the source:

     ```bash
     gh api "repos/<owner>/<repo>/commits?path=<path>&since=<fetched_at>&per_page=100" \
       --jq '.[] | "\(.sha[:7]) \(.commit.message | split("\n")[0])"'
     ```

     Then read the diff of each commit that sounds relevant:
     `gh api repos/<owner>/<repo>/commits/<sha> --jq '.files[] | select(.filename == "<path>") | .patch'`.
   - For a docs page, fetch the current body with the check's headers and
     read it against the format doc's claims for that content type:

     ```bash
     curl -sL -H 'User-Agent: capmon/1.0' -H 'Accept: text/markdown' \
       -H 'Accept-Encoding: identity' <uri>    # Accept is */* when the path doesn't end in .md
     ```

3. Classify the change.
   - **Nothing capmon records.** Typos, examples, unrelated fields, or other
     features: update `content_hash` and `fetched_at` only.
   - **A recorded fact changed.** Update the mapping or extension in step 5.
   - **A new mechanism appeared.** Map it to an existing canonical key, or
     add a `provider_extensions` entry, per `docs/onboarding.md` section 2.

## Step 4: Triage each fetch error

First tell a moved page from a fetch-contract mismatch. Compare the check's
fetch with a plain one:

```bash
./capmon hash-url <uri>                                   # the check's view
curl -sS -o /dev/null -w '%{http_code} %{content_type}\n' -L <uri>   # a plain client's view
```

| What you see | What it means | Action |
|---|---|---|
| Both fail with 404 or 410 | The page moved or was removed | Follow the drift rule: search the provider's docs site and its `llms.txt`, check `docs_conventions` in the source manifest, and match the new page by content. Update `uri` in the format doc and the URL in the source manifest, then re-hash with `capmon hash-url` |
| `hash-url` fails and plain `curl` succeeds | The host answers differently to the check's pinned headers, so the page did not move | Stop work on this provider. The fix belongs in `CheckFetch` in `check.go` or in the choice of URL, and it changes the hash contract for every provider, so report it with both outputs |
| `content invalid: ...` | The page fetched but failed `ValidateContentResponse` in `fetch_validity.go`: an empty body, an HTML body under the size floor, a binary content type, or a redirect to another registrable domain | Read the reason, then treat it as a moved page or a contract mismatch |
| The source is the only one for its content type and nothing live replaces it | The provider may have dropped the feature | Stop and ask. Marking a content type `unsupported` is a product statement, not a mechanical fix |

Dropping a source the recognizer never anchored on is allowed only with a
`Verified live:` attestation, per the drift rule.

## Step 5: Curate the format doc

Edit `docs/provider-formats/<slug>.yaml`:

- For every source you re-verified, set `content_hash` to the `hash-url` value
  and `fetched_at` to now in UTC. Set the top-level `last_fetched_at` to the
  same time, and set `last_changed_at` when a recorded fact changed.
- Change `canonical_mappings` and `provider_extensions` only where step 3 or
  step 4 found a changed fact, and give each change its source in the
  `mechanism` text or the extension's `source_ref`.
- Before adding an extension `id`, grep for the concept across providers so the
  graduation scan can count it:

  ```bash
  grep -rin "<concept keyword>" docs/provider-formats/
  ```

Do not file graduation issues. The daily `acif-change-scan` workflow files
graduation candidates from the shared extension `id`s on its own.

## Step 6: Regenerate and update the capability baseline

```bash
./capmon validate-format-doc --provider <slug>
./capmon validate-sources --provider <slug>
./capmon derive --provider <slug> --output-dir docs/provider-capabilities
```

`derive` defaults to `.develop/seeder-specs`, so always pass `--output-dir`.

When a change touches a fact the capability baseline publishes (a capability's
`supported`, `mechanism`, or `confidence`, or a path), edit
`docs/provider-capabilities/<slug>.yaml` by hand to match, then:

```bash
./capmon verify
./capmon generate     # only when a capability baseline changed
```

Skip `generate` when no capability baseline changed. It restamps the
`Generated at` line in all six views, which adds six lines of churn.

Do not run `capmon seed` in this loop. It merges recognizer output from
`.capmon-cache/` into the capability baseline, and that overwrites curated
`mechanism` text. Use it only to create a baseline that does not exist yet.

## Step 7: Prove the issue is resolved

```bash
./capmon check --provider <slug> --dry-run     # prints nothing when clean
go test ./...
git status --porcelain                          # only the files from the table above
```

A count line from the dry run means a source is still drifting or still
failing. Go back to step 3 or step 4 for it, or name it in the PR body as left
open with the reason.

## Step 8: Open the drift PR

Skip this step under `--dry-run`, and report the diff and the draft PR body
instead.

```bash
git switch -c capmon/drift-<slug>-$(date -u +%Y%m%d)
git add docs/provider-formats/<slug>.yaml docs/provider-capabilities/<slug>-*.yaml
# plus, only when changed: the capability baseline, the source manifest,
# and docs/provider-capabilities/by-content-type/
git commit
git push -u origin HEAD
gh pr create --base main --title "Curate <slug> drift from <date>" --body "..."
```

Write the PR body for the reviewer:

- `Closes #<n>`.
- Each source, with the upstream change that caused the hash change and what
  changed in capmon's data because of it, or "hash only" when nothing did.
- Each fetch error and how it was resolved, or why it was left open.
- Any heal PR this fix makes redundant or shows to be wrong.

Leave the PR open for a person. Do not comment on or close the issue; the merge
closes it.

## Gotchas

- **The issue body lags.** It reflects the last scheduled run. Trust
  `capmon check --provider <slug> --dry-run` and `capmon hash-url` over it.
- **Never hash with plain curl.** Mintlify sites and `code.claude.com` vary
  bytes on headers. `capmon hash-url` is the only hash the check will agree
  with.
- **cursor.com rejects `Accept: text/markdown` on `.md` paths.** Since about
  July 2026, `https://cursor.com/docs/<page>.md` returns 404 to the check's
  headers and 200 to a plain client. The bare path returns HTML to the check's
  `Accept: */*`. This is a fetch-contract mismatch from step 4, not five moved
  pages.
- **A failure partway leaves the format doc half edited.** Check
  `git status` before retrying, and finish or revert that provider before
  starting the next one.
