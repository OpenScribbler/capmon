package capmon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenScribbler/capmon/internal/output"
)

// frozenMajor records one frozen URL major. RootSHA256 is the value
// `capmon freeze` printed when the tree was frozen; it lives here, outside the
// tree, so a frozen tree never vouches for itself (ADR 0001). Every export
// re-hashes site-static/<Prefix>/ against it and fails closed on mismatch.
type frozenMajor struct {
	Prefix       string
	SupersededBy string
	RootSHA256   string
}

// frozenMajors lists every frozen major in publication order. The root index
// is append-only, so entries are never removed.
var frozenMajors = []frozenMajor{
	{Prefix: "v1", SupersededBy: "/v2/", RootSHA256: "cd0ad6101473289f4912903fc24dbd2366ea3bea5dde0f9bc9529313c3c93e54"},
}

// frozenExemptFile is the one file inside a frozen tree the root hash skips:
// advisories.json is the sole permitted post-freeze mutation (ADR 0001).
const frozenExemptFile = "advisories.json"

// frozenTreeHash returns the root hash of a frozen major tree: the sha256 of a
// sha256sum-format manifest ("<hex>  <path>\n" per file, paths slash-relative
// to dir, sorted byte-wise), skipping the top-level advisories.json. The same
// value falls out of:
//
//	cd site-static/v1 && find . -type f ! -path ./advisories.json -printf '%P\n' |
//	  LC_ALL=C sort | xargs sha256sum | sha256sum
func frozenTreeHash(dir string) (string, error) {
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("frozen tree entry %s is not a regular file", p)
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == frozenExemptFile {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		lines = append(lines, hashHex(b)+"  "+rel+"\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("frozen tree %s is empty", dir)
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i][66:] < lines[j][66:]
	})
	return hashHex([]byte(strings.Join(lines, ""))), nil
}

// copyFrozenMajors verifies every registered frozen major under staticDir
// against its recorded root hash, copies it verbatim into stageDir, and
// re-validates it against its own pinned schemas. An unregistered directory
// under staticDir, a missing frozen major, or a hash mismatch fails closed with
// EXPORT_006 before anything deploys.
func copyFrozenMajors(staticDir, stageDir string) error {
	registered := map[string]bool{}
	for _, fm := range frozenMajors {
		registered[fm.Prefix] = true
	}
	entries, err := os.ReadDir(staticDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if !registered[e.Name()] {
			return frozenError(fmt.Sprintf("%s/%s is not a registered frozen major", staticDir, e.Name()))
		}
	}

	for _, fm := range frozenMajors {
		src := filepath.Join(staticDir, fm.Prefix)
		got, err := frozenTreeHash(src)
		if err != nil {
			return frozenError(fmt.Sprintf("hash frozen major %s: %v", fm.Prefix, err))
		}
		if got != fm.RootSHA256 {
			return frozenError(fmt.Sprintf("frozen major %s root hash is %s, recorded %s", fm.Prefix, got, fm.RootSHA256))
		}
		if err := copyTree(src, filepath.Join(stageDir, fm.Prefix)); err != nil {
			return err
		}
		if err := validateMajorTree(stageDir, fm.Prefix); err != nil {
			return err
		}
	}
	return nil
}

func frozenError(msg string) error {
	return output.NewStructuredError(
		"EXPORT_006",
		msg,
		"A frozen major is immutable except advisories.json. Restore site-static/ from git; never re-record the hash to make the gate pass.",
	)
}

// copyTree copies every regular file under src to the same relative path
// under dst, which must not already exist.
func copyTree(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("copy %s: destination %s already exists", src, dst)
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return writeStagedFile(filepath.Join(dst, rel), b)
	})
}

// FreezeOptions parameterizes RunFreeze.
type FreezeOptions struct {
	// Major is the prefix being frozen, e.g. "v1".
	Major string
	// FromDir is an exported site root holding <Major>/ — the last live tree.
	FromDir string
	// OutDir is the static root the frozen tree lands under (site-static).
	OutDir string
	// SupersededBy is the successor major's path, e.g. "/v2/".
	SupersededBy string
	// FrozenAt is the RFC 3339 UTC freeze timestamp.
	FrozenAt string
	// Renames maps an old-major slug to its successor-major slug, so the
	// vacated slug's document points at the doc that replaced it (ADR 0013).
	Renames map[string]string
}

// RunFreeze copies the last live tree of opts.Major to opts.OutDir/<Major>/
// and applies the single final mutation: status "frozen", superseded_by, and
// frozen_at on <Major>/index.json and on every per-provider document. Each
// provider document's superseded_by names its successor document
// (<SupersededBy>capabilities/<slug>.json, after Renames); the index's names
// the successor major. The provider sha256 entries in the index are updated to
// the mutated bytes; nothing else changes — all.json, the pivots, and
// data_revision keep their live bytes. The frozen tree is validated against
// its own pinned schemas and its root hash returned for recording in
// frozenMajors.
func RunFreeze(opts FreezeOptions) (string, error) {
	if opts.Major == "" || opts.FromDir == "" || opts.OutDir == "" || opts.SupersededBy == "" || opts.FrozenAt == "" {
		return "", fmt.Errorf("freeze: major, from, out, superseded-by, and frozen-at are all required")
	}
	dst := filepath.Join(opts.OutDir, opts.Major)
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("freeze: %s already exists; a major freezes exactly once", dst)
	}
	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(opts.OutDir, ".freeze-stage-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)

	tree := filepath.Join(stage, opts.Major)
	if err := copyTree(filepath.Join(opts.FromDir, opts.Major), tree); err != nil {
		return "", err
	}

	idxPath := filepath.Join(tree, "index.json")
	idx, err := readJSONDoc(idxPath)
	if err != nil {
		return "", err
	}
	if idx["status"] != "live" {
		return "", fmt.Errorf("freeze: %s has status %v, want live", idxPath, idx["status"])
	}
	provs, ok := idx["providers"].([]any)
	if !ok {
		return "", fmt.Errorf("freeze: %s has no providers array", idxPath)
	}
	for _, pe := range provs {
		entry, ok := pe.(map[string]any)
		if !ok {
			return "", fmt.Errorf("freeze: malformed providers entry in %s", idxPath)
		}
		slug, _ := entry["slug"].(string)
		rel, _ := entry["path"].(string)
		if slug == "" || !filepath.IsLocal(filepath.FromSlash(rel)) {
			return "", fmt.Errorf("freeze: bad provider entry slug=%q path=%q", slug, rel)
		}
		successor := slug
		if r, ok := opts.Renames[slug]; ok {
			successor = r
		}
		docPath := filepath.Join(tree, filepath.FromSlash(rel))
		doc, err := readJSONDoc(docPath)
		if err != nil {
			return "", err
		}
		doc["status"] = "frozen"
		doc["superseded_by"] = opts.SupersededBy + "capabilities/" + successor + ".json"
		doc["frozen_at"] = opts.FrozenAt
		b, err := canonicalJSON(doc)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(docPath, b, 0644); err != nil {
			return "", err
		}
		entry["sha256"] = hashHex(b)
	}
	for slug := range opts.Renames {
		found := false
		for _, pe := range provs {
			if pe.(map[string]any)["slug"] == slug {
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("freeze: rename source %q is not a provider in %s", slug, idxPath)
		}
	}

	idx["status"] = "frozen"
	idx["superseded_by"] = opts.SupersededBy
	idx["frozen_at"] = opts.FrozenAt
	b, err := canonicalJSON(idx)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(idxPath, b, 0644); err != nil {
		return "", err
	}

	if err := validateMajorTree(stage, opts.Major); err != nil {
		return "", err
	}
	sum, err := frozenTreeHash(tree)
	if err != nil {
		return "", err
	}
	if err := os.Rename(tree, dst); err != nil {
		return "", err
	}
	return sum, nil
}

// readJSONDoc decodes a JSON object with numbers kept as json.Number, so a
// re-encode through canonicalJSON reproduces integer literals exactly.
func readJSONDoc(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}
