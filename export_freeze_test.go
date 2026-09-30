package capmon

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFrozenV1MatchesRecordedRoot pins the committed frozen v1 tree: its root
// hash equals the value recorded in frozenMajors, and the single freeze
// mutation is present on the index and on every provider document, with the
// vacated windsurf slug pointing at its v2 successor.
func TestFrozenV1MatchesRecordedRoot(t *testing.T) {
	dir := filepath.Join(docsRoot(t), "site-static", "v1")
	got, err := frozenTreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != v1FrozenRoot || frozenMajors[0].RootSHA256 != v1FrozenRoot {
		t.Fatalf("frozen v1 root = %s, frozenMajors records %s, test pins %s", got, frozenMajors[0].RootSHA256, v1FrozenRoot)
	}

	idx := readJSONMap(t, filepath.Join(dir, "index.json"))
	if idx["status"] != "frozen" || idx["superseded_by"] != "/v2/" || idx["frozen_at"] == nil {
		t.Errorf("v1/index.json freeze fields = %v/%v/%v", idx["status"], idx["superseded_by"], idx["frozen_at"])
	}
	for _, pe := range idx["providers"].([]any) {
		entry := pe.(map[string]any)
		slug := entry["slug"].(string)
		b := readFileBytes(t, filepath.Join(dir, "capabilities", slug+".json"))
		if hashHex(b) != entry["sha256"] {
			t.Errorf("v1/index.json sha256 for %s does not match the frozen document", slug)
		}
		doc := readJSONMap(t, filepath.Join(dir, "capabilities", slug+".json"))
		want := "/v2/capabilities/" + slug + ".json"
		if slug == "windsurf" {
			want = "/v2/capabilities/devin.json"
		}
		if doc["status"] != "frozen" || doc["superseded_by"] != want || doc["frozen_at"] != idx["frozen_at"] {
			t.Errorf("%s freeze fields = %v/%v/%v, want frozen/%s/%v", slug, doc["status"], doc["superseded_by"], doc["frozen_at"], want, idx["frozen_at"])
		}
	}
}

// TestRunFreeze freezes a fixture export and checks the single mutation, the
// untouched data documents, the recomputed provider hashes, the returned root
// hash, and the one-shot and rename-source guards.
func TestRunFreeze(t *testing.T) {
	opts := committedFixtureOpts(t)
	opts.GeneratedAt = "2026-01-01T00:00:00Z"
	opts.OutDir = filepath.Join(t.TempDir(), "dist")
	if err := RunExport(opts); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(opts.OutDir, currentMajor)
	out := filepath.Join(t.TempDir(), "static")

	fo := FreezeOptions{
		Major:        currentMajor,
		FromDir:      opts.OutDir,
		OutDir:       out,
		SupersededBy: "/v9/",
		FrozenAt:     "2026-02-02T00:00:00Z",
		Renames:      map[string]string{"alpha": "alpha-next"},
	}
	sum, err := RunFreeze(fo)
	if err != nil {
		t.Fatalf("RunFreeze: %v", err)
	}
	frozen := filepath.Join(out, currentMajor)
	if got, _ := frozenTreeHash(frozen); got != sum {
		t.Errorf("returned root %s, tree hashes to %s", sum, got)
	}

	liveIdx := readJSONMap(t, filepath.Join(live, "index.json"))
	idx := readJSONMap(t, filepath.Join(frozen, "index.json"))
	if idx["status"] != "frozen" || idx["superseded_by"] != "/v9/" || idx["frozen_at"] != "2026-02-02T00:00:00Z" {
		t.Errorf("index freeze fields = %v/%v/%v", idx["status"], idx["superseded_by"], idx["frozen_at"])
	}
	if idx["data_revision"] != liveIdx["data_revision"] || idx["generated_at"] != liveIdx["generated_at"] {
		t.Errorf("freeze changed data_revision or generated_at")
	}
	alpha := readJSONMap(t, filepath.Join(frozen, "capabilities", "alpha.json"))
	if alpha["superseded_by"] != "/v9/capabilities/alpha-next.json" {
		t.Errorf("renamed alpha superseded_by = %v", alpha["superseded_by"])
	}
	for _, pe := range idx["providers"].([]any) {
		entry := pe.(map[string]any)
		if hashHex(readFileBytes(t, filepath.Join(frozen, entry["path"].(string)))) != entry["sha256"] {
			t.Errorf("index sha256 for %v does not match the frozen document", entry["slug"])
		}
	}
	for _, rel := range []string{"capabilities/all.json", "advisories.json", "spec/canonical-keys.json"} {
		if string(readFileBytes(t, filepath.Join(frozen, rel))) != string(readFileBytes(t, filepath.Join(live, rel))) {
			t.Errorf("freeze mutated %s", rel)
		}
	}

	// advisories.json is the one post-freeze mutation the root hash ignores.
	if err := os.WriteFile(filepath.Join(frozen, "advisories.json"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, _ := frozenTreeHash(frozen); got != sum {
		t.Errorf("advisories.json edit changed the root hash")
	}

	if _, err := RunFreeze(fo); err == nil {
		t.Errorf("second freeze of the same major succeeded")
	}
	fo.OutDir = filepath.Join(t.TempDir(), "static2")
	fo.Renames = map[string]string{"nope": "x"}
	if _, err := RunFreeze(fo); err == nil {
		t.Errorf("freeze with an unknown rename source succeeded")
	}
}

// TestCopyFrozenMajorsFailsClosed: a tampered frozen document or an
// unregistered directory under site-static/ fails the export with EXPORT_006;
// an advisories.json edit does not.
func TestCopyFrozenMajorsFailsClosed(t *testing.T) {
	src := filepath.Join(docsRoot(t), "site-static")

	fresh := func() string {
		dir := filepath.Join(t.TempDir(), "site-static")
		if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	static := fresh()
	if err := os.WriteFile(filepath.Join(static, "v1", "advisories.json"), []byte("{\"schema_version\":\"1\",\"advisories\":[]}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFrozenMajors(static, t.TempDir()); err != nil {
		t.Errorf("advisories-only edit failed the frozen check: %v", err)
	}

	static = fresh()
	p := filepath.Join(static, "v1", "capabilities", "amp.json")
	b := readFileBytes(t, p)
	if err := os.WriteFile(p, append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	requireStructured(t, copyFrozenMajors(static, t.TempDir()), "EXPORT_006")

	static = fresh()
	if err := os.MkdirAll(filepath.Join(static, "v9"), 0755); err != nil {
		t.Fatal(err)
	}
	requireStructured(t, copyFrozenMajors(static, t.TempDir()), "EXPORT_006")
}
