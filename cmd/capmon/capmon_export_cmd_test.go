package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenScribbler/capmon/internal/output"
)

// repoRootFromCmd resolves the repo root from the cmd/capmon package dir (the
// working directory for tests in this package) so tests can point the export
// command's source-path override vars at committed fixture input.
func repoRootFromCmd(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "publish")); err != nil {
		t.Fatalf("repo root %s missing docs/publish: %v", root, err)
	}
	return root
}

// setExportOverrides points the export command's package-level source-path
// override vars at the committed fixture (and the real docs/publish for the
// gate schemas), restoring them on cleanup. These override vars are the
// test-redirection contract the export subcommand must expose, mirroring
// capmonCapabilitiesDirOverride in capmon_cmd.go.
func setExportOverrides(t *testing.T) {
	t.Helper()
	root := repoRootFromCmd(t)
	fixture := filepath.Join(root, "testdata", "fixtures", "export")

	savedCaps := exportCapsDirOverride
	savedKeys := exportCanonicalKeysPathOverride
	savedSources := exportSourcesDirOverride
	savedAssets := exportPublishAssetsDirOverride
	t.Cleanup(func() {
		exportCapsDirOverride = savedCaps
		exportCanonicalKeysPathOverride = savedKeys
		exportSourcesDirOverride = savedSources
		exportPublishAssetsDirOverride = savedAssets
	})

	exportCapsDirOverride = filepath.Join(fixture, "caps")
	exportCanonicalKeysPathOverride = filepath.Join(fixture, "registry.yaml")
	exportSourcesDirOverride = filepath.Join(fixture, "sources")
	exportPublishAssetsDirOverride = filepath.Join(root, "docs", "publish")
}

func TestExportCmdFlags(t *testing.T) {
	t.Run("registered under capmon", func(t *testing.T) {
		found := false
		for _, sub := range capmonCmd.Commands() {
			if sub.Use == "export" {
				found = true
				break
			}
		}
		if !found {
			t.Error("export subcommand not registered under capmonCmd")
		}
	})

	t.Run("--out defaults to dist", func(t *testing.T) {
		flag := exportCmd.Flags().Lookup("out")
		if flag == nil {
			t.Fatal("--out flag not registered on export command")
		}
		if flag.DefValue != "dist" {
			t.Errorf("--out default = %q, want \"dist\"", flag.DefValue)
		}
	})

	t.Run("invalid --generated-at rejected before any I/O", func(t *testing.T) {
		output.SetForTest(t)
		setExportOverrides(t)

		outDir := filepath.Join(t.TempDir(), "should-not-exist")
		exportCmd.Flags().Set("out", outDir)
		exportCmd.Flags().Set("generated-at", "not-a-timestamp")
		defer func() {
			exportCmd.Flags().Set("out", "dist")
			exportCmd.Flags().Set("generated-at", "")
		}()

		if err := exportCmd.RunE(exportCmd, []string{}); err == nil {
			t.Fatal("expected error for invalid --generated-at")
		}
		if _, err := os.Stat(outDir); !os.IsNotExist(err) {
			t.Errorf("OutDir was created despite invalid --generated-at (I/O ran before validation): %v", err)
		}
	})

	t.Run("valid invocation produces v1/index.json", func(t *testing.T) {
		output.SetForTest(t)
		setExportOverrides(t)

		outDir := filepath.Join(t.TempDir(), "dist")
		exportCmd.Flags().Set("out", outDir)
		exportCmd.Flags().Set("generated-at", "2026-01-01T00:00:00Z")
		exportCmd.Flags().Set("source-commit", "0000000000000000000000000000000000000000")
		defer func() {
			exportCmd.Flags().Set("out", "dist")
			exportCmd.Flags().Set("generated-at", "")
			exportCmd.Flags().Set("source-commit", "")
		}()

		if err := exportCmd.RunE(exportCmd, []string{}); err != nil {
			t.Fatalf("valid export invocation: %v", err)
		}
		if _, err := os.Stat(filepath.Join(outDir, "v1", "index.json")); err != nil {
			t.Errorf("export did not produce %s/v1/index.json: %v", outDir, err)
		}
	})
}
