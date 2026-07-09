# capmon

Capability-monitor pipeline for [syllago](https://github.com/OpenScribbler/syllago).

Watches upstream AI-tool documentation for capability drift so syllago's
provider capability tables stay accurate. Extracted from syllago's
`internal/capmon` (with full git history) so the shipping product doesn't
carry the pipeline's dependencies — chromedp (headless Chrome), tree-sitter
(CGO), goquery, goldmark.

## What it does

```
fetch  →  extract  →  recognize/derive  →  diff  →  report / heal
```

- **fetch** — pulls ~150 upstream sources (raw GitHub files, HTTP pages,
  JS-rendered pages via chromedp) into a hash cache.
- **extract** — per-format extractors (HTML, Markdown, Go, Rust, TypeScript,
  JSON, JSON Schema, YAML, TOML) turn sources into comparable field sets.
- **recognize/derive** — per-provider recognizers map extracted fields onto
  syllago's canonical capability model.
- **diff/report** — compares against the committed state in the syllago
  repo and opens PRs/issues **on syllago** for drift (the healing layer).
- **check** — content-hash drift detection across all providers
  (`capmon check --all`).

The capability *data* lives in the syllago repo (`docs/provider-sources/`,
`docs/provider-formats/`, `docs/provider-capabilities/`) — this repo is only
the pipeline that maintains it.

## Usage

The CLI runs against a syllago checkout (defaults assume the working
directory is a syllago repo root):

```bash
go build -o capmon ./cmd/capmon
cd /path/to/syllago
capmon run                  # full pipeline
capmon run --stage fetch-extract
capmon run --stage report
capmon check --all --providers-json=cli/providers.json
capmon verify               # validate provider-capabilities YAML
```

`cmd/provider-monitor` is the sister tool that watches provider source URLs
for rename/deprecation/URL drift; point it at a syllago checkout with
`SYLLAGO_ROOT` or `--dir`.

## CI

`.github/workflows/pipeline.yml` runs the full pipeline every 4 days against
a fresh syllago checkout, using the `SYLLAGO_TOKEN` repository secret (a
fine-grained PAT scoped to OpenScribbler/syllago with contents/pull-requests/
issues read-write) for the healing PRs and issues. Each run commits a
heartbeat file so GitHub's 60-day-inactivity rule never disables the
schedule.

Tests that compare against the live syllago repo (canonical keys, seeder
specs, source manifests) resolve it via `SYLLAGO_ROOT` and skip when it is
unset; CI always sets it.

## License

Apache-2.0, same as syllago.
