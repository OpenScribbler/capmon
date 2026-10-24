package capmon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenScribbler/capmon/capyaml"
	"github.com/OpenScribbler/capmon/internal/output"
)

// TestBuildContentTypeNodeInvalidInstallRowFailsClosed: a row that breaks the
// ACIF row model never reaches a published document — the export fails
// closed with EXPORT_007 naming the provider and content type.
func TestBuildContentTypeNodeInvalidInstallRowFailsClosed(t *testing.T) {
	ct := capyaml.ContentTypeEntry{
		Supported: true,
		InstallEntryPoints: []capyaml.InstallEntryPoint{
			{Scope: "global", PathTemplate: "~/.demo/<content-name>.md", Layout: "single_file", Status: "current", AsOf: "x"},
		},
	}
	_, err := buildContentTypeNode("demo", "rules", ct, keyRegistry{})
	var se output.StructuredError
	if !errors.As(err, &se) {
		t.Fatalf("error is not output.StructuredError: %T (%v)", err, err)
	}
	if se.Code != "EXPORT_007" {
		t.Errorf("error code = %q, want EXPORT_007", se.Code)
	}
	for _, want := range []string{`"demo"`, "content_types.rules.install_entry_points", `"global"`} {
		if !strings.Contains(se.Message, want) {
			t.Errorf("message %q does not mention %s", se.Message, want)
		}
	}
}

// TestRealExportPublishesInstallEntryPoints runs the export over the real
// baselines — which validates every document against the published schemas
// and fails closed otherwise — and asserts claude-code and codex publish
// install_entry_points for every content type they declare, in the
// per-provider document and in the by-content-type pivot.
func TestRealExportPublishesInstallEntryPoints(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist")
	opts := ExportOptions{
		OutDir:       out,
		GeneratedAt:  "2026-01-01T00:00:00Z",
		SourceCommit: "0000000000000000000000000000000000000000",
	}
	if err := RunExport(opts); err != nil {
		t.Fatalf("RunExport over real docs (defaults): %v", err)
	}

	// The write target (first current row per scope) each provider must
	// publish, keyed by content type and scope.
	wantFirst := map[string]map[string]map[string]string{
		"claude-code": {
			"agents":   {"user": "~/.claude/agents/<content-name>.md", "project": ".claude/agents/<content-name>.md"},
			"commands": {"user": "~/.claude/commands/<content-name>.md", "project": ".claude/commands/<content-name>.md"},
			"hooks":    {"user": "~/.claude/settings.json", "project": ".claude/settings.json"},
			"mcp":      {"user": "~/.claude.json", "project": ".mcp.json"},
			"rules":    {"user": "~/.claude/rules/<content-name>.md", "project": ".claude/rules/<content-name>.md"},
			"skills":   {"user": "~/.claude/skills/<content-name>/", "project": ".claude/skills/<content-name>/"},
		},
		"codex": {
			"agents":   {"user": "~/.codex/agents/<content-name>.toml", "project": ".codex/agents/<content-name>.toml"},
			"commands": {"user": "~/.codex/prompts/<content-name>.md"},
			"hooks":    {"user": "~/.codex/hooks.json", "project": ".codex/hooks.json"},
			"mcp":      {"user": "~/.codex/config.toml", "project": ".codex/config.toml"},
			"rules":    {"user": "~/.codex/AGENTS.md", "project": "AGENTS.md"},
			"skills":   {"user": "~/.agents/skills/<content-name>/", "project": ".agents/skills/<content-name>/"},
		},
	}

	for slug, byType := range wantFirst {
		doc := readJSONMap(t, filepath.Join(out, "v2", "capabilities", slug+".json"))
		if doc["schema_version"] != "3" {
			t.Errorf("%s schema_version = %v, want \"3\"", slug, doc["schema_version"])
		}
		cts := mustChild(t, doc, "content_types")
		for ct, scopes := range byType {
			assertWriteTargets(t, slug+" "+ct, mustChild(t, cts, ct)["install_entry_points"], scopes)

			pivot := readJSONMap(t, filepath.Join(out, "v2", "by-content-type", ct+".json"))
			node := mustChild(t, mustChild(t, pivot, "providers"), slug)
			assertWriteTargets(t, "by-content-type/"+ct+" "+slug, node["install_entry_points"], scopes)
		}
	}
}

// assertWriteTargets checks that raw is a row list whose first current row
// for each scope in want has the wanted path_template.
func assertWriteTargets(t *testing.T, label string, raw any, want map[string]string) {
	t.Helper()
	rows, ok := raw.([]any)
	if !ok || len(rows) == 0 {
		t.Errorf("%s: install_entry_points = %v, want a non-empty row list", label, raw)
		return
	}
	got := map[string]string{}
	for _, r := range rows {
		row := mustMap(t, r)
		scope, _ := row["scope"].(string)
		if _, seen := got[scope]; !seen && row["status"] == "current" {
			got[scope], _ = row["path_template"].(string)
		}
	}
	for scope, tmpl := range want {
		if got[scope] != tmpl {
			t.Errorf("%s: write target for scope %s = %q, want %q", label, scope, got[scope], tmpl)
		}
	}
}

// blankOSBaseline is a capability file whose darwin-only managed row lost its
// os value. YAML decodes `os:` to a nil list, so without a decode-time check
// the export would publish the macOS location as a row for every OS.
const blankOSBaseline = `schema_version: "1"
slug: alpha
display_name: Alpha
content_types:
  rules:
    supported: true
    install_entry_points:
      - scope: managed
        os:
        path_template: /Library/Application Support/Alpha/MEMORY.md
        layout: merged_into_shared_file
        status: current
        as_of: example.com, verified 2026-01-01
`

// TestRunExportBlankOSRowFailsClosed: a row whose os key is null fails the
// export with EXPORT_007, like any other row that breaks the row model, and
// leaves a pre-existing OutDir untouched.
func TestRunExportBlankOSRowFailsClosed(t *testing.T) {
	base := committedFixtureOpts(t)

	caps := t.TempDir()
	if err := os.WriteFile(filepath.Join(caps, "alpha.yaml"), []byte(blankOSBaseline), 0644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	outDir, before := sentinelOutDir(t)

	err := RunExport(ExportOptions{
		CapsDir:           caps,
		CanonicalKeysPath: base.CanonicalKeysPath,
		SourcesDir:        base.SourcesDir,
		PublishAssetsDir:  base.PublishAssetsDir,
		OutDir:            outDir,
		GeneratedAt:       "2026-01-01T00:00:00Z",
	})
	se := requireStructured(t, err, "EXPORT_007")
	if !strings.Contains(se.Message, "alpha.yaml") || !strings.Contains(se.Message, "os is present but null or empty") {
		t.Errorf("message %q does not name alpha.yaml and the blank os", se.Message)
	}
	assertSentinelUntouched(t, outDir, before)
}
