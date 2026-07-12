package capmon

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// keyMeta is the per-key canonical metadata inlined at every registry-backed
// node in an exported document.
type keyMeta struct {
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	SpecRef     string `yaml:"spec_ref"`
}

// keyRegistry maps content type → canonical key → metadata.
type keyRegistry map[string]map[string]keyMeta

// loadKeyRegistry parses the content_types mapping of
// docs/spec/canonical-keys.yaml, preserving description/type/spec_ref for every
// key. Unlike loadCanonicalKeys, which discards all but the key names, this
// retains the fields the exporter inlines at each registry-backed node.
func loadKeyRegistry(path string) (keyRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file struct {
		ContentTypes keyRegistry `yaml:"content_types"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return file.ContentTypes, nil
}
