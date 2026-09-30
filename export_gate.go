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

// siteBase is the published site root every schema $id hangs off.
const siteBase = "https://openscribbler.github.io/capmon/"

// schemaIDBase is the published $id prefix every gate schema of major
// declares. The gate maps each $id to its local file under the staged
// <major>/schemas/ dir so $ref between schemas resolves offline — the compiler
// never touches the network.
func schemaIDBase(major string) string {
	return siteBase + major + "/schemas/"
}

// gateSchemaNames are the six published schemas the gate compiles, by base
// name (without extension). Each lives at <major>/schemas/<name>.json in the
// staged tree and publishes its $id as schemaIDBase(major)+<name>.json.
var gateSchemaNames = []string{
	"provider-capabilities",
	"all-providers",
	"by-content-type",
	"index",
	"advisories",
	"canonical-keys",
}

// validateExportTree is the fail-closed schema gate for the current major.
func validateExportTree(treeDir string) error {
	return validateMajorTree(treeDir, currentMajor)
}

// validateMajorTree compiles the six published schemas from
// treeDir/<major>/schemas/ as draft 2020-12 (local resources only, never
// fetched), then routes and validates every gated document under
// treeDir/<major>/. The root index.json, spec/field-semantics.md, and the
// schema files themselves are not gated by design. On the first violation it
// returns EXPORT_002 naming the offending file and the first violation detail.
// A frozen major is validated against its own pinned schemas.
func validateMajorTree(treeDir, major string) error {
	majorDir := filepath.Join(treeDir, major)
	schemas, err := compileGateSchemas(filepath.Join(majorDir, "schemas"), schemaIDBase(major))
	if err != nil {
		return err
	}

	return filepath.WalkDir(majorDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(majorDir, p)
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
				fmt.Sprintf("%s/%s failed schema validation against %s.json: %s", major, rel, name, firstViolation(verr)),
				"Fix the exporter or the published schema so the generated document conforms; a schema-invalid document must never publish.",
			)
		}
		return nil
	})
}

// routeSchema maps a slash-relative path under <major>/ to the base name of the
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
// A schema whose declared $id is not base+<name>.json fails closed: a v2 tree
// carrying a v1 $id would otherwise publish a schema that names the wrong
// major.
func compileGateSchemas(schemasDir, base string) (map[string]*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	for _, name := range gateSchemaNames {
		doc, err := readJSONInstance(filepath.Join(schemasDir, name+".json"))
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", name, err)
		}
		want := base + name + ".json"
		if m, ok := doc.(map[string]any); !ok || m["$id"] != want {
			return nil, fmt.Errorf("schema %s declares $id %v, want %s", name, idOf(doc), want)
		}
		if err := c.AddResource(want, doc); err != nil {
			return nil, fmt.Errorf("add schema resource %s: %w", name, err)
		}
	}

	out := make(map[string]*jsonschema.Schema, len(gateSchemaNames))
	for _, name := range gateSchemaNames {
		sch, err := c.Compile(base + name + ".json")
		if err != nil {
			return nil, fmt.Errorf("compile schema %s: %w", name, err)
		}
		out[name] = sch
	}
	return out, nil
}

// idOf returns a decoded schema's $id for error messages.
func idOf(doc any) any {
	if m, ok := doc.(map[string]any); ok {
		return m["$id"]
	}
	return nil
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
	sourceStatuses, err := sourceManifestStatuses(sourcesDir)
	if err != nil {
		return err
	}

	exportedSet := stringSet(exportedSlugs)
	sourceSet := make(map[string]bool, len(sourceStatuses))
	for s := range sourceStatuses {
		sourceSet[s] = true
	}

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

// publishedSlugsLockName is the committed record of every slug published under
// the current major, one per line, next to the source manifests. ADR-0013:
// within a major the file is append-only — a slug rename cannot ship as
// delete-plus-add.
const publishedSlugsLockName = "published-slugs.lock"

// assertPublishedSlugs fails closed with EXPORT_005 unless the exported
// provider set exactly matches the published-slugs lock under sourcesDir. A
// lock slug absent from the export is a slug-permanence violation (the rename
// case ADR-0013 forbids); an exported slug absent from the lock is a new
// provider onboarded without its lock line. A missing or unreadable lock also
// fails closed — deleting the file must not bypass the gate.
func assertPublishedSlugs(exportedSlugs []string, sourcesDir string) error {
	lockPath := filepath.Join(sourcesDir, publishedSlugsLockName)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return output.NewStructuredError(
			"EXPORT_005",
			fmt.Sprintf("published-slugs lock unreadable: %v", err),
			fmt.Sprintf("Restore %s — the committed record of every published slug (ADR-0013). The gate fails closed without it.", lockPath),
		)
	}

	lockSet := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lockSet[line] = true
	}

	exportedSet := stringSet(exportedSlugs)
	var vanished, unlocked []string
	for s := range lockSet {
		if !exportedSet[s] {
			vanished = append(vanished, s)
		}
	}
	for s := range exportedSet {
		if !lockSet[s] {
			unlocked = append(unlocked, s)
		}
	}
	if len(vanished) == 0 && len(unlocked) == 0 {
		return nil
	}
	sort.Strings(vanished)
	sort.Strings(unlocked)

	var parts []string
	if len(vanished) > 0 {
		parts = append(parts, "published slugs missing from the export: "+strings.Join(vanished, ", "))
	}
	if len(unlocked) > 0 {
		parts = append(parts, "exported slugs not in the lock: "+strings.Join(unlocked, ", "))
	}
	return output.NewStructuredError(
		"EXPORT_005",
		"slug-permanence mismatch: "+strings.Join(parts, "; "),
		"Slugs are permanent within a major (ADR-0013): a slug in published-slugs.lock must keep exporting — never delete its lock line to make this pass. For a newly onboarded provider, append its slug to the lock.",
	)
}

// manifestJoinFields carries the per-provider source-manifest values the
// exporter joins into published provider docs.
type manifestJoinFields struct {
	Status      string
	DisplayName string
}

// sourceManifestStatuses gathers slug → join fields (lifecycle status,
// display name) for every source manifest under sourcesDir, skipping
// _template.yaml, non-.yaml files (including manifest.schema.json), and
// directories — the same selection LoadAllManifests applies. The key set
// doubles as the declared provider set for the EXPORT_003 gate.
func sourceManifestStatuses(sourcesDir string) (map[string]manifestJoinFields, error) {
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		return nil, fmt.Errorf("read source manifests dir: %w", err)
	}
	fields := map[string]manifestJoinFields{}
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
			Slug        string `yaml:"slug"`
			Status      string `yaml:"status"`
			DisplayName string `yaml:"display_name"`
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
		// The manifest schema requires status; a manifest that omits it would
		// otherwise silently publish a provider doc without provider_status.
		if m.Status == "" {
			return nil, fmt.Errorf("source manifest %s: missing required field 'status'", name)
		}
		seen[m.Slug] = name
		fields[m.Slug] = manifestJoinFields{Status: m.Status, DisplayName: m.DisplayName}
	}
	return fields, nil
}

// stringSet returns the set of distinct values in ss.
func stringSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}
