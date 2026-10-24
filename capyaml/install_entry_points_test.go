package capyaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func row(scope, tmpl, layout string) InstallEntryPoint {
	return InstallEntryPoint{Scope: scope, PathTemplate: tmpl, Layout: layout, Status: "current", AsOf: "example.com/docs, verified 2026-01-01"}
}

func withOS(r InstallEntryPoint, oses ...string) InstallEntryPoint {
	r.OS = oses
	return r
}

func TestValidateInstallEntryPointsAcceptsGoodRows(t *testing.T) {
	superseded := row("project", "LEGACY.md", "merged_into_shared_file")
	superseded.Status = "superseded"
	rows := []InstallEntryPoint{
		row("user", "~/.demo/rules/<content-name>.md", "single_file"),
		row("user", "~/.demo/MEMORY.md", "merged_into_shared_file"),
		row("project", ".demo/skills/<content-name>/", "directory_of_files"),
		row("project", "MEMORY.md", "merged_into_shared_file"),
		superseded,
		withOS(row("user", "~/.config/demo/hooks.json", "merged_into_shared_file"), "darwin", "linux"),
		withOS(row("user", "<appdata>/demo/hooks.json", "merged_into_shared_file"), "windows"),
		withOS(row("managed", "/Library/Application Support/Demo/MEMORY.md", "merged_into_shared_file"), "darwin"),
		withOS(row("managed", "/etc/demo/skills/<content-name>/", "directory_of_files"), "linux"),
		withOS(row("managed", "C:/Program Files/Demo/MEMORY.md", "merged_into_shared_file"), "windows"),
		// Same template under a different scope is a different key.
		row("project", ".demo/rules/<content-name>.md", "single_file"),
	}
	if err := ValidateInstallEntryPoints(rows); err != nil {
		t.Fatalf("ValidateInstallEntryPoints(good rows) = %v, want nil", err)
	}
	if err := ValidateInstallEntryPoints(nil); err != nil {
		t.Fatalf("ValidateInstallEntryPoints(nil) = %v, want nil", err)
	}
}

func TestValidateInstallEntryPointsRejectsBadRows(t *testing.T) {
	good := row("user", "~/.demo/rules/<content-name>.md", "single_file")
	cases := []struct {
		name string
		rows []InstallEntryPoint
		want string
	}{
		{"unknown scope", []InstallEntryPoint{row("global", "~/.demo/rules/<content-name>.md", "single_file")}, `unknown scope "global"`},
		{"unknown layout", []InstallEntryPoint{row("user", "~/.demo/rules/<content-name>.md", "file_per_item")}, `unknown layout "file_per_item"`},
		{"unknown status", []InstallEntryPoint{func() InstallEntryPoint { r := good; r.Status = "deprecated"; return r }()}, `unknown status "deprecated"`},
		{"token outside closed set", []InstallEntryPoint{row("user", "~/.demo/<unknown-x>/<content-name>.md", "single_file")}, "outside the closed set"},
		{"unterminated token", []InstallEntryPoint{row("user", "~/.demo/<content-name.md", "single_file")}, "unterminated placeholder"},
		{"stray >", []InstallEntryPoint{row("user", "~/.demo/a>b/<content-name>.md", "single_file")}, "stray '>'"},
		{"duplicate key", []InstallEntryPoint{good, func() InstallEntryPoint { r := good; r.Layout = "directory_of_files"; return r }()}, "duplicate (scope"},
		{"os lists all three", []InstallEntryPoint{withOS(good, "darwin", "linux", "windows")}, "all three"},
		{"os empty", []InstallEntryPoint{func() InstallEntryPoint { r := good; r.OS = []string{}; return r }()}, "present but empty"},
		{"os unsorted", []InstallEntryPoint{withOS(good, "linux", "darwin")}, "not sorted"},
		{"os duplicate", []InstallEntryPoint{withOS(good, "linux", "linux")}, "not sorted and duplicate-free"},
		{"os unknown", []InstallEntryPoint{withOS(good, "freebsd")}, `unknown os "freebsd"`},
		{"appdata without windows", []InstallEntryPoint{row("user", "<appdata>/demo/<content-name>.md", "single_file")}, "requires os exactly [windows]"},
		{"appdata not leading", []InstallEntryPoint{withOS(row("user", "~/<appdata>/<content-name>.md", "single_file"), "windows")}, "only as the leading segment"},
		{"tilde not leading", []InstallEntryPoint{row("project", "demo/~/<content-name>.md", "single_file")}, "leading ~/"},
		{"backslash", []InstallEntryPoint{withOS(row("managed", `C:\Demo\<content-name>.md`, "single_file"), "windows")}, "backslash"},
		{"empty segment", []InstallEntryPoint{row("project", ".demo//<content-name>.md", "single_file")}, "empty segment"},
		{"user not home-anchored", []InstallEntryPoint{row("user", ".demo/<content-name>.md", "single_file")}, "anchored at ~/"},
		{"project absolute", []InstallEntryPoint{row("project", "/etc/demo/<content-name>.md", "single_file")}, "relative to the project root"},
		{"managed relative", []InstallEntryPoint{row("managed", ".demo/<content-name>.md", "single_file")}, "is absolute"},
		{"single_file without content-name", []InstallEntryPoint{row("project", ".demo/rules.md", "single_file")}, "must carry <content-name>"},
		{"merged with content-name", []InstallEntryPoint{row("project", ".demo/<content-name>.json", "merged_into_shared_file")}, "must not carry <content-name>"},
		{"empty as_of", []InstallEntryPoint{func() InstallEntryPoint { r := good; r.AsOf = " "; return r }()}, "as_of is empty"},
		{"empty template", []InstallEntryPoint{row("project", "", "merged_into_shared_file")}, "path_template is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInstallEntryPoints(tc.rows)
			if err == nil {
				t.Fatalf("ValidateInstallEntryPoints = nil, want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestValidateAgainstSchemaChecksInstallEntryPoints: capmon verify's entry
// point rejects a capability file whose install rows break the row model.
func TestValidateAgainstSchemaChecksInstallEntryPoints(t *testing.T) {
	base := "schema_version: \"1\"\nslug: demo\ncontent_types:\n  skills:\n    supported: true\n    install_entry_points:\n      - scope: %s\n        path_template: ~/.demo/skills/<content-name>/\n        layout: directory_of_files\n        status: current\n        as_of: example.com, verified 2026-01-01\n"
	dir := t.TempDir()

	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte(strings.Replace(base, "%s", "user", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgainstSchema(good, false); err != nil {
		t.Fatalf("ValidateAgainstSchema(good) = %v, want nil", err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte(strings.Replace(base, "%s", "everywhere", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ValidateAgainstSchema(bad, false)
	if err == nil || !strings.Contains(err.Error(), "content_types.skills.install_entry_points") {
		t.Fatalf("ValidateAgainstSchema(bad) = %v, want an install_entry_points error", err)
	}
}
