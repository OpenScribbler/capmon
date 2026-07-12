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

// buildV1Index builds v1/index.json from the staged v1/ tree. data_revision is
// the sha256 of the staged all.json bytes; every per-provider document lands in
// the providers array (sorted by slug, since the canonical writer only sorts
// map keys, not arrays), and every other staged file — except v1/index.json
// itself, which is written afterward — lands in the files map. Both carry a
// per-file sha256 over the exact staged bytes.
func buildV1Index(staged map[string][]byte, providerDocs map[string]map[string]any, opts ExportOptions) map[string]any {
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

// buildRootIndex returns the constant, append-only root discovery document.
// It lives outside v1/ and is hashed by nothing.
func buildRootIndex() map[string]any {
	return map[string]any{
		"latest": "v1",
		"majors": []any{
			map[string]any{
				"prefix": "v1",
				"status": "live",
				"index":  "v1/index.json",
			},
		},
	}
}
