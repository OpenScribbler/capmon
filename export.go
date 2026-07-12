package capmon

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunExport is the single exporter entry point used identically by the CLI,
// tests, and the publish workflow. It stages the complete /v1/ document tree in
// a temp dir on the same filesystem as OutDir, runs the fail-closed schema gate
// and provider-set assert against the staged tree, and only on full success
// atomically replaces OutDir. A partial or invalid export never touches a
// pre-existing OutDir.
func RunExport(opts ExportOptions) error {
	if opts.CapsDir == "" {
		opts.CapsDir = "docs/provider-capabilities"
	}
	if opts.CanonicalKeysPath == "" {
		opts.CanonicalKeysPath = "docs/spec/canonical-keys.yaml"
	}
	if opts.SourcesDir == "" {
		opts.SourcesDir = "docs/provider-sources"
	}
	if opts.PublishAssetsDir == "" {
		opts.PublishAssetsDir = "docs/publish"
	}
	if opts.OutDir == "" {
		opts.OutDir = "dist"
	}
	if opts.GeneratedAt == "" {
		// The only permitted time.Now() in the export path; confined to
		// v1/index.json via buildV1Index.
		opts.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Stage on the same filesystem as OutDir's parent so the final rename is an
	// atomic in-directory move rather than a cross-device copy.
	parent := filepath.Dir(opts.OutDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}

	stageDir, err := os.MkdirTemp(parent, ".export-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	if err := writeExportTree(stageDir, opts); err != nil {
		return err
	}
	if err := validateExportTree(stageDir); err != nil {
		return err
	}

	slugs, err := stagedProviderSlugs(stageDir)
	if err != nil {
		return err
	}
	if err := assertProviderSet(slugs, opts.SourcesDir); err != nil {
		return err
	}

	return replaceOutDir(stageDir, opts.OutDir, parent)
}

// stagedProviderSlugs derives the exported provider set from the staged tree:
// every v1/capabilities/*.json filename except all.json, minus the .json
// extension.
func stagedProviderSlugs(stageDir string) ([]string, error) {
	capsDir := filepath.Join(stageDir, "v1", "capabilities")
	entries, err := os.ReadDir(capsDir)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".json" || name == "all.json" {
			continue
		}
		slugs = append(slugs, strings.TrimSuffix(name, ".json"))
	}
	return slugs, nil
}

// replaceOutDir atomically swaps the fully-staged tree into place: if outDir
// already exists it is renamed aside first, then the staging dir is renamed to
// outDir and the old tree removed. On any earlier error outDir is never
// touched, so a failed export leaves a good site intact.
func replaceOutDir(stageDir, outDir, parent string) error {
	var oldDir string
	if _, err := os.Stat(outDir); err == nil {
		// Reserve a unique aside path, then rename the live tree onto it.
		reserved, err := os.MkdirTemp(parent, ".export-old-*")
		if err != nil {
			return err
		}
		if err := os.RemoveAll(reserved); err != nil {
			return err
		}
		if err := os.Rename(outDir, reserved); err != nil {
			return err
		}
		oldDir = reserved
	}

	if err := os.Rename(stageDir, outDir); err != nil {
		return err
	}
	if oldDir != "" {
		return os.RemoveAll(oldDir)
	}
	return nil
}
