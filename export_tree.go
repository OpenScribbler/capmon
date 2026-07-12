package capmon

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/OpenScribbler/capmon/capyaml"
)

// ExportOptions carries every source path and run-varying value the exporter
// needs. All source locations are parameterized so tests can point at fixture
// dirs; GeneratedAt and SourceCommit are passed through verbatim — the exporter
// never calls time.Now() and never shells out to git.
type ExportOptions struct {
	CapsDir           string
	CanonicalKeysPath string
	SourcesDir        string
	PublishAssetsDir  string
	OutDir            string
	SourceCommit      string
	GeneratedAt       string
}

// writeExportTree stages the complete deterministic /v1/ document tree under
// dst: per-provider docs, all.json, one by-content-type pivot per registry
// content type, the registry document, advisories, any verbatim publish
// assets, then v1/index.json, and finally the constant root index.json. The
// staging order is fixed — every v1/ document and asset is written before any
// hash is computed, v1/index.json is written after everything it hashes, and
// the root index is written last.
func writeExportTree(dst string, opts ExportOptions) error {
	reg, err := loadKeyRegistry(opts.CanonicalKeysPath)
	if err != nil {
		return err
	}

	providerDocs, err := loadProviderDocs(opts.CapsDir, reg)
	if err != nil {
		return err
	}

	// staged maps every written v1/ file (slash-relative to v1/) to its exact
	// bytes, so the index hashes the same bytes that landed on disk.
	staged := map[string][]byte{}

	stage := func(rel string, doc map[string]any) error {
		b, err := canonicalJSON(doc)
		if err != nil {
			return err
		}
		staged[rel] = b
		return writeStagedFile(filepath.Join(dst, "v1", filepath.FromSlash(rel)), b)
	}

	for slug, doc := range providerDocs {
		if err := stage("capabilities/"+slug+".json", doc); err != nil {
			return err
		}
	}
	if err := stage("capabilities/all.json", buildAllDoc(providerDocs)); err != nil {
		return err
	}
	for ct, pivot := range buildPivotDocs(reg, providerDocs) {
		if err := stage("by-content-type/"+ct+".json", pivot); err != nil {
			return err
		}
	}
	if err := stage("spec/canonical-keys.json", buildRegistryDoc(reg)); err != nil {
		return err
	}
	if err := stage("advisories.json", buildAdvisoriesDoc()); err != nil {
		return err
	}

	if err := copyAssets(opts.PublishAssetsDir, dst, staged); err != nil {
		return err
	}

	idxBytes, err := canonicalJSON(buildV1Index(staged, providerDocs, opts))
	if err != nil {
		return err
	}
	if err := writeStagedFile(filepath.Join(dst, "v1", "index.json"), idxBytes); err != nil {
		return err
	}

	rootBytes, err := canonicalJSON(buildRootIndex())
	if err != nil {
		return err
	}
	return writeStagedFile(filepath.Join(dst, "index.json"), rootBytes)
}

// loadProviderDocs builds one document map per capability baseline under
// capsDir, skipping directories, non-.yaml files, and seed YAMLs (Slug == "").
// A non-canonical node in any baseline propagates buildProviderDoc's
// fail-closed EXPORT_001 error.
func loadProviderDocs(capsDir string, reg keyRegistry) (map[string]map[string]any, error) {
	entries, err := os.ReadDir(capsDir)
	if err != nil {
		return nil, fmt.Errorf("read capabilities dir: %w", err)
	}
	docs := map[string]map[string]any{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		caps, err := capyaml.LoadCapabilityYAML(filepath.Join(capsDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		if caps.Slug == "" {
			continue
		}
		doc, err := buildProviderDoc(caps, reg)
		if err != nil {
			return nil, err
		}
		docs[caps.Slug] = doc
	}
	return docs, nil
}

// buildAllDoc aggregates every per-provider document under providers.<slug>,
// reusing the exact document object (byte-identical node shape) built for the
// per-provider file.
func buildAllDoc(providerDocs map[string]map[string]any) map[string]any {
	providers := make(map[string]any, len(providerDocs))
	for slug, doc := range providerDocs {
		providers[slug] = doc
	}
	return map[string]any{
		"schema_version": "1",
		"providers":      providers,
	}
}

// buildPivotDocs emits one by-content-type document per registry content type
// (a stable URL set), each reusing the exact self-describing content-type node
// from each provider that declares it. providers is omitted when no provider
// declares the type.
func buildPivotDocs(reg keyRegistry, providerDocs map[string]map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(reg))
	for ct := range reg {
		providers := map[string]any{}
		for slug, doc := range providerDocs {
			cts, ok := doc["content_types"].(map[string]any)
			if !ok {
				continue
			}
			node, ok := cts[ct]
			if !ok {
				continue
			}
			providers[slug] = node
		}
		pivot := map[string]any{
			"schema_version": "1",
			"content_type":   ct,
		}
		if len(providers) > 0 {
			pivot["providers"] = providers
		}
		out[ct] = pivot
	}
	return out
}

// buildAdvisoriesDoc emits the launch-empty advisories document.
func buildAdvisoriesDoc() map[string]any {
	return map[string]any{
		"schema_version": "1",
		"advisories":     []any{},
	}
}

// buildRegistryDoc emits the published canonical-key registry: content type →
// key → {description, type, spec_ref}.
func buildRegistryDoc(reg keyRegistry) map[string]any {
	cts := make(map[string]any, len(reg))
	for ct, keys := range reg {
		km := make(map[string]any, len(keys))
		for key, meta := range keys {
			km[key] = map[string]any{
				"description": meta.Description,
				"type":        meta.Type,
				"spec_ref":    meta.SpecRef,
			}
		}
		cts[ct] = km
	}
	return map[string]any{
		"schema_version": "1",
		"content_types":  cts,
	}
}

// copyAssets copies every file under assetsDir verbatim into dst/v1/,
// preserving relative paths, and records each in staged so it joins the index
// files map. A empty assetsDir path is a no-op.
func copyAssets(assetsDir, dst string, staged map[string][]byte) error {
	if assetsDir == "" {
		return nil
	}
	return filepath.WalkDir(assetsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(assetsDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		staged[rel] = b
		return writeStagedFile(filepath.Join(dst, "v1", filepath.FromSlash(rel)), b)
	})
}

// writeStagedFile writes b to path (0644), creating parent directories (0755).
func writeStagedFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
