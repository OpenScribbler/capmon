package capmon

// Coverage drift observation for pipeline run manifests.
//
// Ported from syllago's internal/provider.CheckCoverage during the capmon
// extraction. Syllago's provider registry is not importable from this module
// (it lives under internal/), so the "what does the Go code claim" side of
// the comparison is read from cli/providers.json — the committed artifact
// generated from that registry and kept fresh by syllago's CI.
//
// Only the cross-file assertions are evaluated here (Go-vs-source-manifest
// and Go-vs-format-YAML). The Go-internal consistency assertions
// (ConfigLocations/InstallDir vs SupportsType) remain in syllago's own test
// suite (provider/invariant_test.go), which is the authoritative gate —
// this pass is observation-only, mirroring the original pipeline behavior.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// CoverageDrift describes a single mismatch between providers.json's
// supported claims and the documentation YAMLs.
type CoverageDrift struct {
	Provider    string
	ContentType string
	Assertion   string
	Message     string
}

func (d CoverageDrift) String() string {
	return fmt.Sprintf("%s/%s [%s]: %s", d.Provider, d.ContentType, d.Assertion, d.Message)
}

// Assertion names, kept identical to syllago's provider package so run
// manifests read the same before and after the extraction.
const (
	AssertionGoVsSourceManifest = "go-vs-source-manifest"
	AssertionGoVsFormatYAML     = "go-vs-format-yaml"
)

// CoverageContentTypes is the fixed set of content types evaluated.
// Loadouts is intentionally excluded — it's a meta-type composed of other
// content, not a per-provider install target.
var CoverageContentTypes = []string{"rules", "skills", "agents", "commands", "hooks", "mcp"}

// providersJSON mirrors the subset of cli/providers.json consulted here.
type providersJSON struct {
	Providers []struct {
		Slug    string `json:"slug"`
		Content map[string]struct {
			Supported bool `json:"supported"`
		} `json:"content"`
	} `json:"providers"`
}

// covSourceManifest mirrors the subset of docs/provider-sources/<slug>.yaml
// that the coverage check needs. Only the content_types map is consulted.
type covSourceManifest struct {
	Slug         string                                   `yaml:"slug"`
	ContentTypes map[string]covSourceManifestContentEntry `yaml:"content_types"`
}

// covSourceManifestContentEntry captures the two ways a source manifest
// asserts support: `supported: false` explicitly, or `sources: [...]`
// implicitly.
type covSourceManifestContentEntry struct {
	Supported *bool         `yaml:"supported,omitempty"`
	Sources   []interface{} `yaml:"sources,omitempty"`
}

func (e covSourceManifestContentEntry) supportAssertion() *bool {
	if e.Supported != nil {
		return e.Supported
	}
	if len(e.Sources) > 0 {
		t := true
		return &t
	}
	return nil
}

// covFormatYAML mirrors the subset of docs/provider-formats/<slug>.yaml that
// the coverage check needs.
type covFormatYAML struct {
	Provider     string                               `yaml:"provider"`
	ContentTypes map[string]covFormatYAMLContentEntry `yaml:"content_types"`
}

type covFormatYAMLContentEntry struct {
	Status string `yaml:"status,omitempty"`
}

// supportAssertion reports whether the format YAML asserts
// supported/unsupported for this content type. An empty status is treated as
// "not asserted" so stub entries don't generate false drift.
func (e covFormatYAMLContentEntry) supportAssertion() *bool {
	switch e.Status {
	case "supported":
		t := true
		return &t
	case "unsupported":
		f := false
		return &f
	default:
		return nil
	}
}

// CheckCoverage compares the supported-content claims in cli/providers.json
// against the source manifests and format YAMLs under docs/. It returns
// every drift it finds; callers render the full picture in one pass.
//
// repoRoot must be a syllago repository root (the directory containing
// docs/ and cli/).
func CheckCoverage(repoRoot string) ([]CoverageDrift, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("repoRoot is empty")
	}

	pj, err := loadProvidersJSON(filepath.Join(repoRoot, "cli", "providers.json"))
	if err != nil {
		return nil, fmt.Errorf("load providers.json: %w", err)
	}
	sourceManifests, err := loadCovSourceManifests(filepath.Join(repoRoot, "docs", "provider-sources"))
	if err != nil {
		return nil, fmt.Errorf("load source manifests: %w", err)
	}
	formatYAMLs, err := loadCovFormatYAMLs(filepath.Join(repoRoot, "docs", "provider-formats"))
	if err != nil {
		return nil, fmt.Errorf("load format YAMLs: %w", err)
	}

	var drifts []CoverageDrift
	for _, prov := range pj.Providers {
		for _, ct := range CoverageContentTypes {
			entry, ok := prov.Content[ct]
			if !ok {
				continue
			}
			goSupported := entry.Supported

			if sm, ok := sourceManifests[prov.Slug]; ok {
				if e, has := sm.ContentTypes[ct]; has {
					if asserted := e.supportAssertion(); asserted != nil && *asserted != goSupported {
						drifts = append(drifts, CoverageDrift{
							Provider:    prov.Slug,
							ContentType: ct,
							Assertion:   AssertionGoVsSourceManifest,
							Message:     fmt.Sprintf("source manifest says supported=%v but Go SupportsType(%s)=%v", *asserted, ct, goSupported),
						})
					}
				}
			}

			if fy, ok := formatYAMLs[prov.Slug]; ok {
				if e, has := fy.ContentTypes[ct]; has {
					if asserted := e.supportAssertion(); asserted != nil && *asserted != goSupported {
						drifts = append(drifts, CoverageDrift{
							Provider:    prov.Slug,
							ContentType: ct,
							Assertion:   AssertionGoVsFormatYAML,
							Message:     fmt.Sprintf("format YAML says supported=%v but Go SupportsType(%s)=%v", *asserted, ct, goSupported),
						})
					}
				}
			}
		}
	}
	return drifts, nil
}

func loadProvidersJSON(path string) (*providersJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pj providersJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &pj, nil
}

// loadCovSourceManifests reads every *.yaml file in dir and returns a map
// keyed by the manifest's slug. The _template.yaml file is skipped.
func loadCovSourceManifests(dir string) (map[string]*covSourceManifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make(map[string]*covSourceManifest, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		if e.Name() == "_template.yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var sm covSourceManifest
		if err := yaml.Unmarshal(data, &sm); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if sm.Slug == "" {
			continue
		}
		out[sm.Slug] = &sm
	}
	return out, nil
}

// loadCovFormatYAMLs reads every *.yaml file in dir and returns a map keyed
// by the format YAML's provider slug.
func loadCovFormatYAMLs(dir string) (map[string]*covFormatYAML, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make(map[string]*covFormatYAML, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var fy covFormatYAML
		if err := yaml.Unmarshal(data, &fy); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if fy.Provider == "" {
			continue
		}
		out[fy.Provider] = &fy
	}
	return out, nil
}
