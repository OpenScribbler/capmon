package capmon

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FormatDoc is the top-level structure of a provider format document
// (docs/provider-formats/<slug>.yaml). It captures how a provider implements
// each content type, which canonical keys it supports, and any provider-specific
// extensions that have no canonical equivalent yet.
type FormatDoc struct {
	Provider         string                          `yaml:"provider"`
	DocsURL          string                          `yaml:"docs_url"`
	Category         string                          `yaml:"category"`
	LastFetchedAt    string                          `yaml:"last_fetched_at"`
	LastChangedAt    string                          `yaml:"last_changed_at"`
	GenerationMethod string                          `yaml:"generation_method"`
	ContentTypes     map[string]ContentTypeFormatDoc `yaml:"content_types"`
}

// ValidProviderCategories enumerates the allowed values for FormatDoc.Category.
// Changes here must stay in sync with the category validator in
// formatdoc_validate.go. Downstream consumers of the published data may also
// maintain category label maps keyed by these values.
var ValidProviderCategories = []string{
	"cli",
	"ide-extension",
	"standalone-app",
	"web-based",
}

// ContentTypeFormatDoc describes how a provider supports a single content type
// (e.g., "skills").
type ContentTypeFormatDoc struct {
	Status             string                      `yaml:"status"`
	Sources            []SourceRef                 `yaml:"sources"`
	CanonicalMappings  map[string]CanonicalMapping `yaml:"canonical_mappings"`
	ProviderExtensions []ProviderExtension         `yaml:"provider_extensions"`
	LoadingModel       string                      `yaml:"loading_model"`
	Notes              string                      `yaml:"notes"`
}

// SourceRef describes a single source URI that was fetched to populate the format doc.
// ContentHash stores the SHA-256 hash of the fetched content at fetch time, enabling
// drift detection via comparison in subsequent capmon check runs.
type SourceRef struct {
	URI         string `yaml:"uri"`
	Type        string `yaml:"type"`
	FetchMethod string `yaml:"fetch_method"`
	ContentHash string `yaml:"content_hash"`
	FetchedAt   string `yaml:"fetched_at"`
	Name        string `yaml:"name,omitempty"`
	Section     string `yaml:"section,omitempty"`
}

// MappingStatus values classify how a provider relates to a canonical key.
// The bool `supported` field this replaces was overloaded: `false` conflated
// "the provider genuinely lacks this concept" with "the provider has a
// mechanism I could not map" — two cases with opposite downstream meaning.
//
//	absent   — the provider has no such mechanism; nothing to map, ever.
//	           Never a Class B candidate.
//	mapped   — the mechanism maps to the canonical key (the former supported: true).
//	unmapped — the provider HAS a mechanism but it does not map to the canonical
//	           key yet; a Class B (mapping-row) candidate. Requires SourceForm.
const (
	MappingStatusAbsent   = "absent"
	MappingStatusMapped   = "mapped"
	MappingStatusUnmapped = "unmapped"
)

// ValidMappingStatuses enumerates the allowed values for CanonicalMapping.Status.
var ValidMappingStatuses = []string{
	MappingStatusAbsent,
	MappingStatusMapped,
	MappingStatusUnmapped,
}

// CanonicalMapping records how a provider implements a canonical capability key.
// The canonical key itself is the map key in ContentTypeFormatDoc.CanonicalMappings.
//
// Status classifies the relationship (absent | mapped | unmapped); see the
// MappingStatus constants.
//
// SourceForm is required when Status is "unmapped": it is the verbatim minimal
// provider content snippet exhibiting the unmapped mechanism — canonicalizer-
// feedable input, NOT a prose description (prose belongs in Mechanism). The
// deferred Class B confirmation step runs a conforming canonicalizer over this
// snippet to confirm the totality-net diagnostic before any filing.
//
// MechanismToken is the curator's source-mechanism token claim for a
// status:"unmapped" entry, when the mechanism is nameable in ACIF's published
// source-mechanism vocabulary (conformance/source-mechanisms.yaml). Optional —
// leave empty when no token plausibly names the mechanism. Consumed by
// `capmon acif-change confirm`: a claimed token that is a member of the export
// with recognition_requiring:false is machine-rejected as already-mapped.
// Only meaningful when Status is "unmapped".
//
// ProviderField names the actual native field (frontmatter key, config key, TOML
// field, dot-notation path) when the mapping corresponds to a specific named
// field. Omitted when the mapping is structural or behavioral.
//
// ExtensionID points to a ProviderExtension entry that describes the same concept
// in provider-specific detail. The component renders one unified row when set.
// The canonical mapping is the authority; the extension is the detail — so the
// authority points to the detail, not the other way around.
type CanonicalMapping struct {
	Status         string   `yaml:"status"`
	SourceForm     string   `yaml:"source_form,omitempty"`
	MechanismToken string   `yaml:"mechanism_token,omitempty"`
	Mechanism      string   `yaml:"mechanism"`
	Paths          []string `yaml:"paths,omitempty"`
	Confidence     string   `yaml:"confidence"`
	ProviderField  string   `yaml:"provider_field,omitempty"`
	ExtensionID    string   `yaml:"extension_id,omitempty"`
}

// ExtensionExample is a runnable snippet attached to a provider extension. Examples
// let the provider pages render copyable code blocks beside each capability so
// readers can see the syntax in context.
type ExtensionExample struct {
	Title string `yaml:"title,omitempty"`
	Lang  string `yaml:"lang"`
	Code  string `yaml:"code"`
	Note  string `yaml:"note,omitempty"`
}

// ProviderExtension captures a provider-specific capability that has no canonical key yet.
// Each extension has a stable ID for structural diff (detecting new additions across runs),
// a source reference pointing to where the capability was found, and a graduation candidate
// flag indicating whether the capability may be common enough across providers to warrant
// a canonical key.
//
// Summary is one sentence (~150 chars max) describing what the feature is. Deep
// explanations belong on the page linked by SourceRef. (Replaces the longer
// Description field used in earlier schema versions.)
//
// ProviderField names the actual native field when the extension describes a
// frontmatter key, config key, or TOML field. Also serves as the UI grouping
// signal: entries with ProviderField appear in "fields"; entries without it in
// "other".
//
// Conversion declares what happens to the feature during format conversion. Must
// be one of: translated | embedded | dropped | preserved | not-portable.
type ProviderExtension struct {
	ID                  string             `yaml:"id"`
	Name                string             `yaml:"name"`
	Summary             string             `yaml:"summary"`
	SourceRef           string             `yaml:"source_ref"`
	GraduationCandidate bool               `yaml:"graduation_candidate"`
	GraduationNotes     string             `yaml:"graduation_notes,omitempty"`
	Required            *bool              `yaml:"required"`
	ValueType           string             `yaml:"value_type,omitempty"`
	Examples            []ExtensionExample `yaml:"examples,omitempty"`
	ProviderField       string             `yaml:"provider_field,omitempty"`
	Conversion          string             `yaml:"conversion"`
}

// LoadFormatDoc reads and unmarshals a format doc YAML file.
func LoadFormatDoc(path string) (*FormatDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load format doc %s: %w", path, err)
	}
	var doc FormatDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse format doc %s: %w", path, err)
	}
	return &doc, nil
}

// FormatDocPath returns the canonical filesystem path for a provider's format doc.
func FormatDocPath(formatsDir, provider string) string {
	return filepath.Join(formatsDir, provider+".yaml")
}
