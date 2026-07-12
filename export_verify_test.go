package capmon

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- verify test helpers ----------------------------------------------------

// requireGit skips the calling test when the git binary is absent. RunExportVerify
// materializes docs/ at a commit via `git archive`, so these tests need a real
// git on PATH; a missing binary is a skip, never a silent pass.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available on PATH: %v", err)
	}
}

// runGit runs git in dir and returns its combined output, failing the test on a
// non-zero exit. Used only to build the throwaway t.TempDir() repo the verify
// tests archive from — no state on the real repo is ever touched.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// copyFile copies src to dst verbatim, creating dst's parent directory.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", dst, err)
	}
	if err := os.WriteFile(dst, b, 0644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

// buildVerifyRepo creates a throwaway git repo under t.TempDir() whose docs/
// tree mirrors the real repo layout — provider-capabilities/, spec/canonical-keys.yaml,
// provider-sources/, and a verbatim copy of the committed docs/publish/ contract
// assets — populated from the committed export fixture. It commits docs/ and
// returns the repo path and the commit SHA. RunExportVerify archives docs/ at
// that SHA, so the committed tree is the exact source the rebuild sees.
func buildVerifyRepo(t *testing.T) (string, string) {
	t.Helper()
	fixture := committedFixtureRoot(t)
	realPublish := filepath.Join(docsRoot(t), "docs", "publish")

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	if err := os.CopyFS(filepath.Join(repo, "docs", "provider-capabilities"), os.DirFS(filepath.Join(fixture, "caps"))); err != nil {
		t.Fatalf("copy caps: %v", err)
	}
	if err := os.CopyFS(filepath.Join(repo, "docs", "provider-sources"), os.DirFS(filepath.Join(fixture, "sources"))); err != nil {
		t.Fatalf("copy sources: %v", err)
	}
	if err := os.CopyFS(filepath.Join(repo, "docs", "publish"), os.DirFS(realPublish)); err != nil {
		t.Fatalf("copy publish assets: %v", err)
	}
	copyFile(t, filepath.Join(fixture, "registry.yaml"), filepath.Join(repo, "docs", "spec", "canonical-keys.yaml"))

	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "verify@example.com")
	runGit(t, repo, "config", "user.name", "Verify Test")
	runGit(t, repo, "add", "docs")
	runGit(t, repo, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "fixture docs")

	sha := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	return repo, sha
}

// dirtyWorkingTree makes a data-visible edit to a committed baseline in the
// working tree only (the commit is untouched). A correct RunExportVerify
// materializes docs/ at the commit via git archive and never sees the edit; an
// implementation that reads the working directory rebuilds a divergent tree
// and fails the match test.
func dirtyWorkingTree(t *testing.T, repo string) {
	t.Helper()
	path := filepath.Join(repo, "docs", "provider-capabilities", "alpha.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read baseline to dirty: %v", err)
	}
	mutated := bytes.Replace(data, []byte("supported: true"), []byte("supported: false"), 1)
	if bytes.Equal(mutated, data) {
		t.Fatalf("dirtyWorkingTree found no 'supported: true' to flip in %s", path)
	}
	if err := os.WriteFile(path, mutated, 0644); err != nil {
		t.Fatalf("dirty working tree: %v", err)
	}
}

// buildPublishedSite runs the real exporter over the temp repo's docs/ into a
// fresh directory, pinning GeneratedAt and SourceCommit so the published tree is
// deterministic. The returned dir is what an httptest FileServer serves as the
// "live site". RunExportVerify re-reads generated_at + source_commit from the
// served v1/index.json, so the pinned values here define what a matching rebuild
// must reproduce.
func buildPublishedSite(t *testing.T, repo, sha string) string {
	t.Helper()
	site := filepath.Join(t.TempDir(), "site")
	opts := ExportOptions{
		CapsDir:           filepath.Join(repo, "docs", "provider-capabilities"),
		CanonicalKeysPath: filepath.Join(repo, "docs", "spec", "canonical-keys.yaml"),
		SourcesDir:        filepath.Join(repo, "docs", "provider-sources"),
		PublishAssetsDir:  filepath.Join(repo, "docs", "publish"),
		OutDir:            site,
		GeneratedAt:       "2026-07-12T09:00:00Z",
		SourceCommit:      sha,
	}
	if err := RunExport(opts); err != nil {
		t.Fatalf("build published site: %v", err)
	}
	return site
}

// --- verify tests -----------------------------------------------------------

// TestExportVerifyMatch: a site published from a commit's docs/ verifies clean
// against a rebuild of that same commit. RunExportVerify archives docs/ at the
// SHA from the CWD repo (t.Chdir), rebuilds with generated_at + source_commit
// pinned from the fetched v1/index.json, and byte-compares every published file.
func TestExportVerifyMatch(t *testing.T) {
	requireGit(t)

	repo, sha := buildVerifyRepo(t)
	site := buildPublishedSite(t, repo, sha)
	dirtyWorkingTree(t, repo)

	srv := httptest.NewServer(http.FileServer(http.Dir(site)))
	defer srv.Close()

	// RunExportVerify runs `git archive` in the CWD repo.
	t.Chdir(repo)

	if err := RunExportVerify(sha, srv.URL+"/"); err != nil {
		t.Fatalf("RunExportVerify over a matching site: %v", err)
	}
}

// TestExportVerifyMismatchFailsClosed: when one published per-provider document
// diverges from the rebuild, verification fails closed with EXPORT_004 naming
// that document's path. The site is built correctly, then a single supported
// boolean byte is flipped in the served synthetic.json only — the index and
// every other file stay byte-identical, so synthetic.json is the sole (hence
// first) divergent path.
func TestExportVerifyMismatchFailsClosed(t *testing.T) {
	requireGit(t)

	repo, sha := buildVerifyRepo(t)
	site := buildPublishedSite(t, repo, sha)
	dirtyWorkingTree(t, repo)

	target := filepath.Join(site, "v1", "capabilities", "synthetic.json")
	good, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read published synthetic.json: %v", err)
	}
	altered := bytes.Replace(good, []byte("true"), []byte("false"), 1)
	if bytes.Equal(altered, good) {
		t.Fatalf("synthetic.json had no boolean to flip; fixture changed")
	}
	if err := os.WriteFile(target, altered, 0644); err != nil {
		t.Fatalf("overwrite served synthetic.json: %v", err)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(site)))
	defer srv.Close()

	t.Chdir(repo)

	err = RunExportVerify(sha, srv.URL+"/")
	se := requireStructured(t, err, "EXPORT_004")
	if !strings.Contains(se.Message, "synthetic.json") {
		t.Errorf("EXPORT_004 does not name the divergent file synthetic.json: %v", se)
	}
}
