package capmon

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// hashHex returns the lowercase hex SHA-256 of b, matching the encoding the
// index uses for data_revision and per-file digests.
func hashHex(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// buildMajorIndex builds <major>/index.json from the staged <major>/ tree. data_revision is
// the sha256 of the staged all.json bytes; every per-provider document lands in
// the providers array (sorted by slug, since the canonical writer only sorts
// map keys, not arrays), and every other staged file — except the index
// itself, which is written afterward — lands in the files map. Both carry a
// per-file sha256 over the exact staged bytes.
func buildMajorIndex(staged map[string][]byte, providerDocs map[string]map[string]any, opts ExportOptions) map[string]any {
	idx := map[string]any{
		"schema_version":      "1",
		"status":              "live",
		"generated_at":        opts.GeneratedAt,
		"cadence":             "daily",
		"max_staleness_hours": 48,
		"data_revision":       hashHex(staged["capabilities/all.json"]),
	}
	if opts.SourceCommit != "" {
		idx["source_commit"] = opts.SourceCommit
	}

	slugs := make([]string, 0, len(providerDocs))
	for slug := range providerDocs {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	providerPaths := make(map[string]bool, len(slugs))
	provs := make([]any, 0, len(slugs))
	for _, slug := range slugs {
		path := "capabilities/" + slug + ".json"
		providerPaths[path] = true
		entry := map[string]any{
			"slug":   slug,
			"path":   path,
			"status": "tracked",
			"sha256": hashHex(staged[path]),
		}
		if lv, ok := providerDocs[slug]["last_verified"]; ok {
			entry["last_verified"] = lv
		}
		provs = append(provs, entry)
	}
	idx["providers"] = provs

	files := map[string]any{}
	for rel, b := range staged {
		if rel == "index.json" || providerPaths[rel] {
			continue
		}
		files[rel] = map[string]any{"sha256": hashHex(b)}
	}
	idx["files"] = files

	return idx
}

// buildRootIndex returns the append-only root discovery document. It lives
// outside every major and is hashed by nothing. Frozen majors keep their
// entries forever (the majors array only grows), each carrying the root hash
// recorded at freeze time so the frozen tree never vouches for itself.
func buildRootIndex() map[string]any {
	majors := make([]any, 0, len(frozenMajors)+1)
	for _, fm := range frozenMajors {
		majors = append(majors, map[string]any{
			"prefix":             fm.Prefix,
			"status":             "frozen",
			"index":              fm.Prefix + "/index.json",
			"superseded_by":      fm.SupersededBy,
			"frozen_root_sha256": fm.RootSHA256,
		})
	}
	majors = append(majors, map[string]any{
		"prefix": currentMajor,
		"status": "live",
		"index":  currentMajor + "/index.json",
	})
	return map[string]any{
		"latest": currentMajor,
		"majors": majors,
	}
}
