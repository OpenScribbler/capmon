package capmon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectGitHubRename_NotAGitHubURL(t *testing.T) {
	got, err := DetectGitHubRename(context.Background(), "https://example.com/docs/foo.md")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-github URL, got %v", got)
	}
}

func TestDetectGitHubRename_MalformedPath(t *testing.T) {
	// raw.githubusercontent.com with too-short path.
	_, err := DetectGitHubRename(context.Background(), "https://raw.githubusercontent.com/owner/repo")
	if err == nil {
		t.Fatal("expected error for malformed github raw path")
	}
}

func TestDetectGitHubRename_RenamedFile(t *testing.T) {
	// Simulate a realistic rename: docs/settings.md → docs/settings-strict.md.
	// The new name shares the old stem as a prefix — the most common pattern
	// for real-world doc renames (adding qualifiers or version suffixes).
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			{Path: "README.md", Type: "blob"},
			{Path: "docs/settings-strict.md", Type: "blob"},
			{Path: "docs/unrelated.md", Type: "blob"},
			{Path: "src/code.go", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/git/trees/main" {
			t.Errorf("unexpected tree path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(), "https://raw.githubusercontent.com/owner/repo/main/docs/settings.md")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one candidate")
	}
	// Top candidate should be docs/settings-strict.md — shares stem "settings"
	// token + prefix match + same directory.
	if got[0].Path != "docs/settings-strict.md" {
		t.Errorf("top candidate = %q, want docs/settings-strict.md", got[0].Path)
	}
	wantURL := "https://raw.githubusercontent.com/owner/repo/main/docs/settings-strict.md"
	if got[0].URL != wantURL {
		t.Errorf("top candidate URL = %q, want %q", got[0].URL, wantURL)
	}
}

func TestDetectGitHubRename_ExtensionMismatch(t *testing.T) {
	// docs/foo.md is the original; repo has docs/foo.json with an identical
	// stem. We should NOT return it — ext mismatch.
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			{Path: "docs/foo.json", Type: "blob"},
			{Path: "docs/unrelated.md", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()
	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, _ := DetectGitHubRename(context.Background(), "https://raw.githubusercontent.com/owner/repo/main/docs/foo.md")
	for _, c := range got {
		if c.Path == "docs/foo.json" {
			t.Errorf("extension-mismatched candidate should be filtered: %v", c)
		}
	}
}

func TestDetectGitHubRename_NoCandidates(t *testing.T) {
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			{Path: "completely/different.md", Type: "blob"},
			{Path: "zzz.md", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()
	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(), "https://raw.githubusercontent.com/owner/repo/main/docs/settings.md")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	// No candidate meets the score floor — got may be nil or empty. Neither
	// is an error.
	for _, c := range got {
		if c.Score >= 1.0 {
			t.Errorf("unexpected exact match: %v", c)
		}
	}
}

func TestDetectGitHubRename_TreeAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	_, err := DetectGitHubRename(context.Background(), "https://raw.githubusercontent.com/owner/repo/main/docs/x.md")
	if err == nil {
		t.Fatal("expected error when tree API returns 404")
	}
}

func TestDetectGitHubRename_FiltersTreesAndCommits(t *testing.T) {
	// Only blob entries should be considered — "tree" and "commit" types
	// represent directories and submodules, not files.
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			{Path: "docs", Type: "tree"},
			{Path: "docs/new-name.md", Type: "blob"},
			{Path: "external-submodule", Type: "commit"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()
	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(), "https://raw.githubusercontent.com/owner/repo/main/docs/old-name.md")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	for _, c := range got {
		if c.Path == "docs" || c.Path == "external-submodule" {
			t.Errorf("non-blob entry leaked into candidates: %q", c.Path)
		}
	}
}

func TestStemSimilarity(t *testing.T) {
	tests := []struct {
		name   string
		a, b   string
		wantGt float64 // want score > this
		wantLt float64 // want score < this (0 means no upper bound)
	}{
		{"identical", "foo-bar", "foo-bar", 0.99, 0},
		{"token reorder", "create-workflow", "workflow-create", 0.6, 0},
		{"substring extension", "settings", "settings-v2", 0.4, 0},
		{"completely different", "alpha", "zebra", -0.01, 0.5},
		{"case insensitive", "FooBar", "foobar", 0.9, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stemSimilarity(tt.a, tt.b)
			if got <= tt.wantGt {
				t.Errorf("stemSimilarity(%q, %q) = %.3f, want > %.3f", tt.a, tt.b, got, tt.wantGt)
			}
			if tt.wantLt > 0 && got >= tt.wantLt {
				t.Errorf("stemSimilarity(%q, %q) = %.3f, want < %.3f", tt.a, tt.b, got, tt.wantLt)
			}
		})
	}
}

func TestTokenizeStem(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"foo-bar", []string{"foo", "bar"}},
		{"foo_bar", []string{"foo", "bar"}},
		{"foo.bar", []string{"foo", "bar"}},
		{"foo-bar_baz.qux", []string{"foo", "bar", "baz", "qux"}},
		{"plain", []string{"plain"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := tokenizeStem(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("tokenizeStem(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("tokenizeStem(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// TestDetectGitHubRename_PrefersPathContextOverBareBasename reproduces the
// codex/skills regression. openai/codex renamed the crate core-skills → skills,
// so codex-rs/skills/src/model.rs is the correct heal for
// codex-rs/core-skills/src/model.rs. But codex-rs/tui/src/exec_cell/model.rs
// also has a perfect basename match, and under basename-only scoring both tied
// at 1.0 — the winner was whichever the sort happened to place first. The
// pipeline picked the TUI file and pushed it as a branch every day for two
// weeks.
func TestDetectGitHubRename_PrefersPathContextOverBareBasename(t *testing.T) {
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			// Listed before the correct answer on purpose: if path context is
			// ignored, this ties and wins on ordering alone.
			{Path: "codex-rs/tui/src/exec_cell/model.rs", Type: "blob"},
			{Path: "codex-rs/app-server/src/model.rs", Type: "blob"},
			{Path: "codex-rs/skills/src/model.rs", Type: "blob"},
			{Path: "codex-rs/skills/src/parser.rs", Type: "blob"},
			{Path: "README.md", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(),
		"https://raw.githubusercontent.com/openai/codex/main/codex-rs/core-skills/src/model.rs")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one candidate")
	}
	if got[0].Path != "codex-rs/skills/src/model.rs" {
		t.Errorf("top candidate = %q, want codex-rs/skills/src/model.rs (path context must outrank a bare basename tie)", got[0].Path)
	}
	// The margin must be real, not a coin flip: an equal top-two score means
	// the ordering is still arbitrary even if this run happened to pass.
	if len(got) > 1 && got[0].Score == got[1].Score {
		t.Errorf("top two candidates tied at %.4f (%q vs %q); the winner is arbitrary",
			got[0].Score, got[0].Path, got[1].Path)
	}
}

// TestDetectGitHubRename_CapAppliesAfterRanking guards the second half of the
// same bug: maxRenameCandidates used to break out of the tree walk, so in a
// large repo the best candidate could be discarded before it was ever scored.
func TestDetectGitHubRename_CapAppliesAfterRanking(t *testing.T) {
	entries := []gitTreeEntry{}
	// Enough weak-but-passing candidates to overflow the cap, all listed
	// before the obvious correct answer.
	for i := 0; i < maxRenameCandidates+50; i++ {
		entries = append(entries, gitTreeEntry{
			Path: fmt.Sprintf("vendor/pkg%d/settings-legacy.md", i),
			Type: "blob",
		})
	}
	entries = append(entries, gitTreeEntry{Path: "docs/settings.md", Type: "blob"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitTreeResponse{Tree: entries})
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(),
		"https://raw.githubusercontent.com/owner/repo/main/docs/settings-old.md")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected candidates")
	}
	if got[0].Path != "docs/settings.md" {
		t.Errorf("top candidate = %q, want docs/settings.md — the cap must truncate the ranked result, not the scan", got[0].Path)
	}
	if len(got) > maxRenameCandidates {
		t.Errorf("returned %d candidates, want at most %d", len(got), maxRenameCandidates)
	}
}

// TestDetectGitHubRename_ExactBasenameBeatsSuffixedSiblingInOriginalDir pins the
// tiering invariant. The composite score alone gets this wrong: the decoy keeps
// the original directory (dir 1.00) and its stem clears the 0.667 break-even,
// so 0.75*0.71 + 0.25*1.00 = 0.785 outranks the genuine cross-tree move at
// 0.75*1.00 + 0.25*0.00 = 0.750. Only the exact-stem tier saves it.
func TestDetectGitHubRename_ExactBasenameBeatsSuffixedSiblingInOriginalDir(t *testing.T) {
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			// Suffixed sibling left behind in the original directory. Listed
			// first on purpose so an ordering bug cannot mask a scoring bug.
			{Path: "docs/claude-code/build/claude-code-hooks-reference.md", Type: "blob"},
			// The real move: same basename, entirely different subtree.
			{Path: "api/reference/tools/claude-code-hooks.md", Type: "blob"},
			{Path: "README.md", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(),
		"https://raw.githubusercontent.com/anthropics/docs/main/docs/claude-code/build/claude-code-hooks.md")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected both candidates to be scored, got %d", len(got))
	}
	if got[0].Path != "api/reference/tools/claude-code-hooks.md" {
		t.Errorf("top candidate = %q, want api/reference/tools/claude-code-hooks.md (an exact basename must outrank a fuzzy match in the original directory)", got[0].Path)
	}
	if !got[0].exactStem {
		t.Errorf("top candidate %q should be an exact-stem match", got[0].Path)
	}
	// Guard the tier, not the arithmetic: this must hold even where the decoy
	// wins on raw composite score, which is exactly the case here.
	if got[0].Score >= got[1].Score {
		t.Logf("note: exact match also wins on composite (%.4f vs %.4f); the tier is untested by this fixture",
			got[0].Score, got[1].Score)
	}
}

func TestIdentityOf(t *testing.T) {
	cases := []struct {
		path     string
		wantStem string
		wantDir  string
	}{
		// Plain basename: identity is the basename, directory is the parent.
		{"codex-rs/core-skills/src/loader.rs", "loader", "codex-rs/core-skills/src"},
		// Directory-as-identity: the enclosing directory names the module, so
		// it becomes the stem and drops out of the directory.
		{"codex-rs/ext/skills/src/loader/mod.rs", "loader", "codex-rs/ext/skills/src"},
		{"docs/hooks/index.md", "hooks", "docs"},
		{"pkg/__init__.py", "pkg", "."},
		// Case-insensitive: docs trees are inconsistent about this.
		{"docs/hooks/INDEX.MD", "hooks", "docs"},
		// At the repo root there is no enclosing directory to lift.
		{"index.md", "index", "."},
		{"mod.rs", "mod", "."},
	}
	for _, c := range cases {
		stem, dir := identityOf(c.path)
		if stem != c.wantStem || dir != c.wantDir {
			t.Errorf("identityOf(%q) = (%q, %q), want (%q, %q)", c.path, stem, dir, c.wantStem, c.wantDir)
		}
	}
}

// TestDetectGitHubRename_DirectoryAsIdentityBasenameEntersCandidateSet pins the
// first half of the scoring bug. The heal for codex-rs/core-skills/src/loader.rs
// is codex-rs/ext/skills/src/loader/mod.rs — the same module, moved. Scoring
// the literal basename compares "loader" to "mod", which scores 0 and drops the
// only correct answer before ranking ever runs.
func TestDetectGitHubRename_DirectoryAsIdentityBasenameEntersCandidateSet(t *testing.T) {
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			{Path: "codex-rs/ext/skills/src/loader/mod.rs", Type: "blob"},
			{Path: "codex-rs/ext/skills/src/loader/tests.rs", Type: "blob"},
			{Path: "README.md", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(),
		"https://raw.githubusercontent.com/openai/codex/main/codex-rs/core-skills/src/loader.rs")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected the moved module as a candidate; a mod.rs basename must not be scored literally")
	}
	if got[0].Path != "codex-rs/ext/skills/src/loader/mod.rs" {
		t.Errorf("top candidate = %q, want codex-rs/ext/skills/src/loader/mod.rs", got[0].Path)
	}
	if !got[0].exactStem {
		t.Errorf("top candidate %q should be an exact-stem match on the enclosing directory", got[0].Path)
	}
	// The PR body is the review surface, so the normalization has to be
	// visible in it — a reviewer seeing stem "loader" against a path ending in
	// mod.rs needs to know why those were compared.
	if !strings.Contains(got[0].Reason, "directory-as-identity") {
		t.Errorf("reason %q does not explain the directory-as-identity normalization", got[0].Reason)
	}
}

// TestDetectGitHubRename_ModuleOriginalDoesNotMatchEveryModule pins the same
// normalization on the original side. codex.yaml already monitors a mod.rs
// path, so this is the shape a future heal starts from: scoring the literal
// basename makes every mod.rs in the repo an exact-stem match (99 of them in
// the live openai/codex tree), leaving the ranking to path context alone and
// sending all 99 on to content validation.
func TestDetectGitHubRename_ModuleOriginalDoesNotMatchEveryModule(t *testing.T) {
	tree := gitTreeResponse{
		Tree: []gitTreeEntry{
			{Path: "codex-rs/ext/skills/src/loader.rs", Type: "blob"},
			{Path: "codex-rs/ext/skills/src/tools/mod.rs", Type: "blob"},
			{Path: "codex-rs/ext/memories/src/tools/mod.rs", Type: "blob"},
			{Path: "codex-rs/tui/src/pets/mod.rs", Type: "blob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(),
		"https://raw.githubusercontent.com/openai/codex/main/codex-rs/ext/skills/src/loader/mod.rs")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) != 1 || got[0].Path != "codex-rs/ext/skills/src/loader.rs" {
		t.Fatalf("candidates = %v, want exactly [codex-rs/ext/skills/src/loader.rs]; the unrelated mod.rs files share only boilerplate", paths(got))
	}
}

// TestDetectGitHubRename_WeightsDirTokensByInformativeness pins the second half
// of the bug. Normalization alone leaves the correct answer in an exact tie
// with the wrong one — both directories match 4 of 5 tokens against
// codex-rs/core-skills/src — and the tie falls to the lexicographic tiebreak,
// which the wrong answer wins. Sharing "skills" has to count for more than
// sharing "core", and the tree itself says which is which: many directories
// here are named with "core", only one other with "skills".
func TestDetectGitHubRename_WeightsDirTokensByInformativeness(t *testing.T) {
	entries := []gitTreeEntry{
		// The correct answer, listed after the decoy so an ordering bug cannot
		// masquerade as a scoring fix.
		{Path: "codex-rs/core-plugins/src/loader.rs", Type: "blob"},
		{Path: "codex-rs/ext/skills/src/loader.rs", Type: "blob"},
	}
	// Filler crates that make "core" a common directory token and leave
	// "skills" a rare one.
	for _, crate := range []string{"core", "core-exec", "core-config", "core-mcp", "core-protocol", "core-tui"} {
		entries = append(entries, gitTreeEntry{Path: "codex-rs/" + crate + "/src/lib.rs", Type: "blob"})
	}
	tree := gitTreeResponse{Tree: entries}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer srv.Close()

	SetGitHubBaseURLForTest(srv.URL)
	defer SetGitHubBaseURLForTest("")

	got, err := DetectGitHubRename(context.Background(),
		"https://raw.githubusercontent.com/openai/codex/main/codex-rs/core-skills/src/loader.rs")
	if err != nil {
		t.Fatalf("DetectGitHubRename: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected both loader.rs candidates to be scored, got %v", paths(got))
	}
	if got[0].Path != "codex-rs/ext/skills/src/loader.rs" {
		t.Errorf("top candidate = %q, want codex-rs/ext/skills/src/loader.rs (sharing %q must outweigh sharing %q)",
			got[0].Path, "skills", "core")
	}
	// An equal top-two score means the winner is still the lexicographic
	// tiebreak, which is what this test exists to remove.
	if got[0].Score == got[1].Score {
		t.Errorf("top two tied at %.4f (%q vs %q); unweighted token overlap is still deciding",
			got[0].Score, got[0].Path, got[1].Path)
	}
}

func paths(cands []RenameCandidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.Path)
	}
	return out
}
