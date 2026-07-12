package capmon

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/OpenScribbler/capmon/internal/output"
)

// schemaIDBase is the published $id prefix every gate schema declares. The
// gate maps each $id to its local file under the staged v1/schemas/ dir so
// $ref between schemas resolves offline — the compiler never touches the
// network.
const schemaIDBase = "https://openscribbler.github.io/capmon/v1/schemas/"

// gateSchemaNames are the six published schemas the gate compiles, by base
// name (without extension). Each lives at v1/schemas/<name>.json in the
// staged tree and publishes its $id as schemaIDBase+<name>.json.
var gateSchemaNames = []string{
	"provider-capabilities",
	"all-providers",
	"by-content-type",
	"index",
	"advisories",
	"canonical-keys",
}

// validateExportTree is the fail-closed schema gate. It compiles the six
// published schemas from treeDir/v1/schemas/ as draft 2020-12 (local
// resources only, never fetched), then routes and validates every gated
// document under treeDir/v1/. The root index.json, v1/spec/field-semantics.md,
// and the schema files themselves are not gated by design. On the first
// violation it returns EXPORT_002 naming the offending file and the first
// violation detail.
func validateExportTree(treeDir string) error {
	schemas, err := compileGateSchemas(filepath.Join(treeDir, "v1", "schemas"))
	if err != nil {
		return err
	}

	v1Dir := filepath.Join(treeDir, "v1")
	return filepath.WalkDir(v1Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(v1Dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		name := routeSchema(rel)
		if name == "" {
			return nil // not gated
		}

		inst, err := readJSONInstance(p)
		if err != nil {
			return err
		}
		if verr := schemas[name].Validate(inst); verr != nil {
			return output.NewStructuredError(
				"EXPORT_002",
				fmt.Sprintf("v1/%s failed schema validation against %s.json: %s", rel, name, firstViolation(verr)),
				"Fix the exporter or the published schema so the generated document conforms; a schema-invalid document must never publish.",
			)
		}
		return nil
	})
}

// routeSchema maps a slash-relative path under v1/ to the base name of the
// schema that gates it, or "" when the file is not schema-gated (root index,
// field-semantics.md, and the schema files themselves).
func routeSchema(rel string) string {
	switch {
	case rel == "index.json":
		return "index"
	case rel == "advisories.json":
		return "advisories"
	case rel == "spec/canonical-keys.json":
		return "canonical-keys"
	case rel == "capabilities/all.json":
		return "all-providers"
	case strings.HasPrefix(rel, "capabilities/") && strings.HasSuffix(rel, ".json"):
		return "provider-capabilities"
	case strings.HasPrefix(rel, "by-content-type/") && strings.HasSuffix(rel, ".json"):
		return "by-content-type"
	default:
		return ""
	}
}

// compileGateSchemas compiles the six published schemas from schemasDir. Every
// schema is added as a compiler resource under its published $id first, so
// cross-schema $ref resolves against the local files and nothing is fetched.
func compileGateSchemas(schemasDir string) (map[string]*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	for _, name := range gateSchemaNames {
		doc, err := readJSONInstance(filepath.Join(schemasDir, name+".json"))
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", name, err)
		}
		if err := c.AddResource(schemaIDBase+name+".json", doc); err != nil {
			return nil, fmt.Errorf("add schema resource %s: %w", name, err)
		}
	}

	out := make(map[string]*jsonschema.Schema, len(gateSchemaNames))
	for _, name := range gateSchemaNames {
		sch, err := c.Compile(schemaIDBase + name + ".json")
		if err != nil {
			return nil, fmt.Errorf("compile schema %s: %w", name, err)
		}
		out[name] = sch
	}
	return out, nil
}

// readJSONInstance decodes a JSON file into the any-shaped value the jsonschema
// validator and compiler both consume (numbers decoded as json.Number).
func readJSONInstance(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jsonschema.UnmarshalJSON(f)
}

// firstViolation renders the deepest, most specific message from a validation
// error so EXPORT_002 carries an actionable detail alongside the file name.
// It descends to the innermost cause and returns its rendered error (which
// carries the instance location and the failing keyword) via the library's
// own printer, avoiding a hand-built one.
func firstViolation(err error) string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err.Error()
	}
	for len(ve.Causes) > 0 {
		ve = ve.Causes[0]
	}
	return strings.TrimSpace(ve.Error())
}

// assertProviderSet fails closed with EXPORT_003 unless the exported provider
// set exactly matches the set declared by the source manifests under
// sourcesDir — count and membership, both directions. The error names every
// divergent slug and both counts.
func assertProviderSet(exportedSlugs []string, sourcesDir string) error {
	sourceSlugs, err := sourceManifestSlugs(sourcesDir)
	if err != nil {
		return err
	}

	exportedSet := stringSet(exportedSlugs)
	sourceSet := stringSet(sourceSlugs)

	var divergent []string
	for s := range exportedSet {
		if !sourceSet[s] {
			divergent = append(divergent, s)
		}
	}
	for s := range sourceSet {
		if !exportedSet[s] {
			divergent = append(divergent, s)
		}
	}

	if len(divergent) == 0 && len(exportedSet) == len(sourceSet) {
		return nil
	}
	sort.Strings(divergent)

	return output.NewStructuredError(
		"EXPORT_003",
		fmt.Sprintf(
			"provider-set mismatch: exported %d providers, source manifests declare %d; divergent slugs: %s",
			len(exportedSet), len(sourceSet), strings.Join(divergent, ", "),
		),
		"Add or remove a source manifest under docs/provider-sources/, or the corresponding capability baseline, so the two sets match exactly.",
	)
}

// sourceManifestSlugs gathers the slug of every source manifest under
// sourcesDir, skipping _template.yaml, non-.yaml files (including
// manifest.schema.json), and directories — the same selection LoadAllManifests
// applies.
func sourceManifestSlugs(sourcesDir string) ([]string, error) {
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		return nil, fmt.Errorf("read source manifests dir: %w", err)
	}
	var slugs []string
	seen := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".yaml" || name == "_template.yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sourcesDir, name))
		if err != nil {
			return nil, err
		}
		var m struct {
			Slug string `yaml:"slug"`
		}
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse source manifest %s: %w", name, err)
		}
		if m.Slug == "" {
			continue
		}
		// Two manifests with one slug would collapse into a single set member
		// and slip past the count check — fail closed instead.
		if prev, dup := seen[m.Slug]; dup {
			return nil, fmt.Errorf("duplicate source manifest slug %q in %s and %s", m.Slug, prev, name)
		}
		seen[m.Slug] = name
		slugs = append(slugs, m.Slug)
	}
	return slugs, nil
}

// stringSet returns the set of distinct values in ss.
func stringSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}
