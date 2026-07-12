package capmon

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// sha256Hex returns the lowercase hex SHA-256 of b, matching the encoding the
// index uses for data_revision and per-file digests.
func sha256Hex(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// assertV1IndexInvariants checks the normative v1/index.json contract against
// the actual staged tree at dst: constants, data_revision, per-file hashing,
// provider sorting, and complete-and-disjoint coverage of every staged v1/ file
// (except v1/index.json) across the providers array and the files map.
func assertV1IndexInvariants(t *testing.T, dst string, idx map[string]any, opts ExportOptions) {
	t.Helper()

	if idx["schema_version"] != "1" {
		t.Errorf("schema_version = %v, want \"1\"", idx["schema_version"])
	}
	if idx["status"] != "live" {
		t.Errorf("status = %v, want \"live\"", idx["status"])
	}
	if idx["cadence"] != "daily" {
		t.Errorf("cadence = %v, want \"daily\"", idx["cadence"])
	}
	if idx["generated_at"] != opts.GeneratedAt {
		t.Errorf("generated_at = %v, want %q (verbatim)", idx["generated_at"], opts.GeneratedAt)
	}

	// max_staleness_hours must serialize as the bare integer 48 — not "48" and
	// not 48.0. Parsed JSON can't distinguish these, so inspect the raw bytes.
	raw, err := os.ReadFile(filepath.Join(dst, "v1", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"max_staleness_hours": 48`)) {
		t.Errorf("v1/index.json missing bare int `\"max_staleness_hours\": 48`:\n%s", raw)
	}
	if bytes.Contains(raw, []byte(`"max_staleness_hours": 48.0`)) ||
		bytes.Contains(raw, []byte(`"max_staleness_hours": "48"`)) {
		t.Error("max_staleness_hours not emitted as a bare int 48")
	}

	// data_revision is the SHA-256 of the staged all.json bytes.
	allBytes, err := os.ReadFile(filepath.Join(dst, "v1", "capabilities", "all.json"))
	if err != nil {
		t.Fatal(err)
	}
	if wantRev := sha256Hex(allBytes); idx["data_revision"] != wantRev {
		t.Errorf("data_revision = %v, want %s (sha256 of all.json)", idx["data_revision"], wantRev)
	}

	// Enumerate every staged file under v1/, excluding v1/index.json itself.
	v1Dir := filepath.Join(dst, "v1")
	staged := map[string][]byte{}
	err = filepath.WalkDir(v1Dir, func(p string, d fs.DirEntry, err error) error {
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
		if rel == "index.json" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		staged[rel] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}

	// providers: sorted by slug; each references a staged per-provider doc with a
	// correct sha256; last_verified appears only when the source doc carries it.
	provs, ok := idx["providers"].([]any)
	if !ok {
		t.Fatalf("providers is not an array: %T", idx["providers"])
	}
	var slugs []string
	for _, pv := range provs {
		pm := mustMap(t, pv)
		slug, _ := pm["slug"].(string)
		slugs = append(slugs, slug)

		path, _ := pm["path"].(string)
		if want := "capabilities/" + slug + ".json"; path != want {
			t.Errorf("provider %q path = %q, want %q", slug, path, want)
		}
		if pm["status"] != "tracked" {
			t.Errorf("provider %q status = %v, want \"tracked\"", slug, pm["status"])
		}

		b, ok := staged[path]
		if !ok {
			t.Errorf("provider %q references unstaged path %q", slug, path)
			continue
		}
		if want := sha256Hex(b); pm["sha256"] != want {
			t.Errorf("provider %q sha256 = %v, want %s", slug, pm["sha256"], want)
		}
		if seen[path] {
			t.Errorf("path %q appears more than once", path)
		}
		seen[path] = true

		doc := readJSONMap(t, filepath.Join(v1Dir, filepath.FromSlash(path)))
		if lv, has := doc["last_verified"]; has {
			if pm["last_verified"] != lv {
				t.Errorf("provider %q last_verified = %v, want %v", slug, pm["last_verified"], lv)
			}
		} else if _, has := pm["last_verified"]; has {
			t.Errorf("provider %q carries last_verified but its source doc omits it", slug)
		}
	}
	if !sort.StringsAreSorted(slugs) {
		t.Errorf("providers not sorted by slug: %v", slugs)
	}

	// files: every other staged file, with a correct sha256, disjoint from providers.
	files := mustChild(t, idx, "files")
	for rel, entry := range files {
		em := mustMap(t, entry)
		b, ok := staged[rel]
		if !ok {
			t.Errorf("files[%q] is not a staged file", rel)
			continue
		}
		if want := sha256Hex(b); em["sha256"] != want {
			t.Errorf("files[%q].sha256 = %v, want %s", rel, em["sha256"], want)
		}
		if seen[rel] {
			t.Errorf("path %q appears in both providers and files", rel)
		}
		seen[rel] = true
	}

	// Coverage: every staged file (except v1/index.json) is accounted for exactly once.
	for rel := range staged {
		if !seen[rel] {
			t.Errorf("staged file %q missing from both providers and files", rel)
		}
	}
	if len(seen) != len(staged) {
		t.Errorf("accounted for %d files across providers+files, staged %d", len(seen), len(staged))
	}
}

func TestBuildV1Index(t *testing.T) {
	t.Run("source_commit omitted when unset", func(t *testing.T) {
		opts := newExportFixture(t)
		opts.SourceCommit = ""
		dst := t.TempDir()
		if err := writeExportTree(dst, opts); err != nil {
			t.Fatalf("writeExportTree: %v", err)
		}
		idx := readJSONMap(t, filepath.Join(dst, "v1", "index.json"))
		if _, ok := idx["source_commit"]; ok {
			t.Errorf("source_commit present when opts.SourceCommit is empty: %v", idx["source_commit"])
		}
		assertV1IndexInvariants(t, dst, idx, opts)
	})

	t.Run("source_commit present when set", func(t *testing.T) {
		opts := newExportFixture(t)
		opts.SourceCommit = "abc123def4567890"
		dst := t.TempDir()
		if err := writeExportTree(dst, opts); err != nil {
			t.Fatalf("writeExportTree: %v", err)
		}
		idx := readJSONMap(t, filepath.Join(dst, "v1", "index.json"))
		if idx["source_commit"] != "abc123def4567890" {
			t.Errorf("source_commit = %v, want %q", idx["source_commit"], "abc123def4567890")
		}
		assertV1IndexInvariants(t, dst, idx, opts)
	})
}

func TestRootIndexBytes(t *testing.T) {
	opts := newExportFixture(t)
	dst := t.TempDir()
	if err := writeExportTree(dst, opts); err != nil {
		t.Fatalf("writeExportTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "index.json"))
	if err != nil {
		t.Fatal(err)
	}

	// The root index is the exact canonical serialization of the constant
	// document {latest, majors:[{prefix, status, index}]}: keys sorted, two-space
	// indent, single trailing LF.
	const wantRootIndex = "{\n" +
		"  \"latest\": \"v1\",\n" +
		"  \"majors\": [\n" +
		"    {\n" +
		"      \"index\": \"v1/index.json\",\n" +
		"      \"prefix\": \"v1\",\n" +
		"      \"status\": \"live\"\n" +
		"    }\n" +
		"  ]\n" +
		"}\n"

	if string(got) != wantRootIndex {
		t.Errorf("root index.json bytes mismatch\n got: %q\nwant: %q", got, wantRootIndex)
	}
}
