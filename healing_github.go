package capmon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// rawGitHubHost is the hostname for GitHub's raw content CDN.
const rawGitHubHost = "raw.githubusercontent.com"

// rawGitHubPathPattern matches paths of the form /{owner}/{repo}/{ref}/{path}.
// Example: /anthropics/claude-code/main/docs/settings.md
var rawGitHubPathPattern = regexp.MustCompile(`^/([^/]+)/([^/]+)/([^/]+)/(.+)$`)

// maxRenameCandidates caps how many candidate blobs we score. Most repos
// have fewer than a few thousand files; 256 is a safety lid against
// pathological trees.
const maxRenameCandidates = 256

// RenameCandidate is one possible replacement for a renamed file.
type RenameCandidate struct {
	// Path is the full path within the repo (e.g. "docs/new-name.md").
	Path string
	// URL is the raw.githubusercontent.com URL rebuilt from the candidate path.
	URL string
	// Score is a similarity score in [0,1] where 1 is a perfect identity-stem
	// match in the identical directory. Higher is better.
	Score float64
	// Reason briefly describes why this candidate was chosen (for PR body).
	Reason string
	// exactStem records whether the candidate's identity stem matched the
	// original exactly. That is the basename for an ordinary file and the
	// enclosing directory for a directory-as-identity basename, so
	// loader/mod.rs is an exact match for loader.rs and mod.rs is not.
	// Ranking treats this as a tier rather than folding it into Score — see
	// the sort in DetectGitHubRename.
	exactStem bool
}

// DetectGitHubRename looks for a likely replacement file in the same repo
// and ref when a raw.githubusercontent.com URL 404s. It calls the git/trees
// API with ?recursive=1 to list all blobs, scores candidates by identity-stem
// similarity plus IDF-weighted path context, and returns ranked candidates.
//
// Returns nil (with nil error) if rawURL is not a raw.githubusercontent.com
// URL or if the repo has no candidates above the similarity threshold.
//
// Callers should apply ValidateContentResponse to the top candidate's URL
// before accepting it as a heal — this function only identifies *likely*
// replacements, not verified ones.
func DetectGitHubRename(ctx context.Context, rawURL string) ([]RenameCandidate, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if u.Hostname() != rawGitHubHost {
		return nil, nil
	}
	m := rawGitHubPathPattern.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, fmt.Errorf("raw github URL %q does not match expected layout /{owner}/{repo}/{ref}/{path}", rawURL)
	}
	owner, repo, ref, filePath := m[1], m[2], m[3], m[4]

	origExt := path.Ext(path.Base(filePath))
	origStem, origDir := identityOf(filePath)

	tree, err := fetchRepoTree(ctx, owner, repo, ref)
	if err != nil {
		return nil, fmt.Errorf("list repo tree: %w", err)
	}
	dirIDF := dirTokenIDF(tree)

	var candidates []RenameCandidate
	for _, entry := range tree {
		if entry.Type != "blob" {
			continue
		}
		candBase := path.Base(entry.Path)
		candExt := path.Ext(candBase)
		// Only consider candidates with the same extension — a .md is never
		// a heal for a .json.
		if !strings.EqualFold(candExt, origExt) {
			continue
		}
		candStem, candDir := identityOf(entry.Path)

		stem := stemSimilarity(origStem, candStem)
		if stem < renameScoreFloor {
			continue
		}

		// Path context breaks ties the basename cannot. A repo can hold many
		// files with the same name; the plausible rename is the one whose
		// surrounding path still resembles the original. Without this,
		// codex-rs/core-skills/src/model.rs and codex-rs/tui/src/exec_cell/model.rs
		// both scored a perfect 1.0 and the winner was whichever the sort
		// happened to place first — which is how a skills source got bound to a
		// TUI render cell for two weeks.
		dir := dirSimilarity(origDir, candDir, dirIDF)
		score := stemWeight*stem + dirWeight*dir
		reason := fmt.Sprintf("stem similarity %.2f (%q → %q); path similarity %.2f (%q → %q)",
			stem, origStem, candStem, dir, origDir, candDir)
		if candStem != strings.TrimSuffix(candBase, candExt) {
			reason += fmt.Sprintf("; %q is a directory-as-identity basename, so its enclosing directory %q carries the stem",
				candBase, candStem)
		}

		// Rebuild the raw URL for this candidate.
		candURL := fmt.Sprintf("https://%s/%s/%s/%s/%s", rawGitHubHost, owner, repo, ref, entry.Path)
		candidates = append(candidates, RenameCandidate{
			Path:      entry.Path,
			URL:       candURL,
			Score:     score,
			Reason:    reason,
			exactStem: stem == 1.0,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		// A file that kept its identity stem exactly outranks any fuzzy match,
		// whatever the paths look like. Path context is only allowed to separate
		// candidates the stem could not, never to overturn it: because dir
		// contributes additively, a decoy sitting in the original directory
		// could otherwise outscore a genuine cross-tree move that kept its name
		// ("claude-code-hooks" losing to "claude-code-hooks-reference"). Tiering
		// states that invariant directly instead of tuning dirWeight low enough
		// to make the inversion improbable — the latter silently reopens the
		// moment stemSimilarity gets more generous.
		if candidates[i].exactStem != candidates[j].exactStem {
			return candidates[i].exactStem
		}
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		// Deterministic tiebreak: an arbitrary order here is what let an
		// unrelated file win a tie on different days.
		return candidates[i].Path < candidates[j].Path
	})
	// Cap AFTER ranking. Capping during the scan truncated the tree walk
	// instead of the result set, so in a large repo the best candidate could
	// be discarded before it was ever scored.
	if len(candidates) > maxRenameCandidates {
		candidates = candidates[:maxRenameCandidates]
	}
	return candidates, nil
}

// Weights for the composite rename score. Stem stays dominant — a file that
// kept its name is the strongest single rename signal — but path context gets
// enough weight to separate identical stems living in different parts of
// a repo. Exact identity-stem matches are tiered above fuzzy ones in the sort, so
// these weights only order candidates within a tier and cannot promote a
// fuzzy match over a file that kept its name.
const (
	stemWeight = 0.75
	dirWeight  = 0.25
)

// dirIdentityStems are basename stems that carry no identity of their own,
// because the enclosing directory names the thing. In Rust, foo.rs and
// foo/mod.rs are the same module; the same holds for index.* in docs and
// JS/TS trees and __init__.py in a Python package. Scoring such a basename
// against the original compares boilerplate to boilerplate: every mod.rs in
// the repo matches exactly and the real answer is indistinguishable from 98
// others.
var dirIdentityStems = map[string]bool{
	"mod":      true,
	"index":    true,
	"__init__": true,
}

// identityOf returns the stem and directory that carry a path's identity.
// For a directory-as-identity basename it lifts the enclosing directory name
// into the stem position and drops that segment from the directory, so
// codex-rs/ext/skills/src/loader/mod.rs is scored as stem "loader" in
// codex-rs/ext/skills/src — the same shape as codex-rs/core-skills/src/loader.rs,
// which is the rename it heals. The lift applies to the original path as well
// as to candidates: without it, a monitored mod.rs source draws every other
// mod.rs in the repo into the candidate set at an exact-stem tie.
func identityOf(filePath string) (stem, dir string) {
	base := path.Base(filePath)
	dir = path.Dir(filePath)
	stem = strings.TrimSuffix(base, path.Ext(base))
	if !dirIdentityStems[strings.ToLower(stem)] {
		return stem, dir
	}
	parent := path.Base(dir)
	if parent == "." || parent == "/" || parent == "" {
		return stem, dir
	}
	return parent, path.Dir(dir)
}

// dirTokenIDF weights directory tokens by inverse document frequency over the
// tree, so a token naming nearly every directory ("src", "codex") says less
// about where a file lives than one naming a few ("skills"). Unweighted
// overlap treats them as equally informative, which is how
// codex-rs/core-plugins/src and codex-rs/ext/skills/src both matched 4 of 5
// tokens against codex-rs/core-skills/src and tied.
//
// Documents are the distinct directories rather than the blobs, so one
// directory holding hundreds of files does not deflate its own tokens. The
// tree is already in memory from fetchRepoTree, so this costs no network I/O.
func dirTokenIDF(tree []gitTreeEntry) map[string]float64 {
	docFreq := make(map[string]int)
	counted := make(map[string]bool)
	total := 0
	for _, entry := range tree {
		if entry.Type != "blob" {
			continue
		}
		dir := path.Dir(entry.Path)
		if counted[dir] {
			continue
		}
		counted[dir] = true
		total++
		for tok := range uniqueTokens(tokenizePath(dir)) {
			docFreq[tok]++
		}
	}
	idf := make(map[string]float64, len(docFreq))
	for tok, df := range docFreq {
		// log(1 + N/df) rather than the bare log(N/df): a token present in
		// every directory keeps a small nonzero weight, so two paths built
		// only from ubiquitous tokens still score above zero instead of
		// dividing by a zero-weight union.
		idf[tok] = math.Log(1 + float64(total)/float64(df))
	}
	return idf
}

// dirSimilarity scores two directory paths by IDF-weighted token overlap
// across all their segments, so codex-rs/core-skills/src scores far closer to
// codex-rs/skills/src than to codex-rs/tui/src/exec_cell. Segments are split
// into words, which is what lets "core-skills" partially match "skills"
// instead of being treated as an unrelated segment.
//
// idf comes from dirTokenIDF over the same tree the candidates come from. A
// token no directory in the tree contains weighs nothing: it cannot separate
// one candidate from another, and it is absent from every candidate equally.
func dirSimilarity(a, b string, idf map[string]float64) float64 {
	if a == b {
		return 1.0
	}
	at := uniqueTokens(tokenizePath(a))
	bt := uniqueTokens(tokenizePath(b))
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	var intersect, union float64
	for tok := range at {
		union += idf[tok]
		if bt[tok] {
			intersect += idf[tok]
		}
	}
	for tok := range bt {
		if !at[tok] {
			union += idf[tok]
		}
	}
	if union == 0 {
		return 0
	}
	return intersect / union
}

// tokenizePath splits a directory path into lowercase words, treating the path
// separator as just another word boundary alongside -, _ and .
func tokenizePath(p string) []string {
	return tokenizeStem(strings.ToLower(strings.ReplaceAll(p, "/", " ")))
}

func uniqueTokens(toks []string) map[string]bool {
	set := make(map[string]bool, len(toks))
	for _, t := range toks {
		set[t] = true
	}
	return set
}

// renameScoreFloor is the minimum stem similarity to treat a file as a
// plausible rename candidate. The value is intentionally loose — content
// validation (ValidateContentResponse) and PR review are the real gate,
// not this score. Raising this just forces more fallbacks to the variant
// strategy or to auto-issue escalation.
const renameScoreFloor = 0.4

type gitTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type gitTreeResponse struct {
	Tree      []gitTreeEntry `json:"tree"`
	Truncated bool           `json:"truncated"`
}

// fetchRepoTree lists all blobs in a repo at the given ref via the
// git/trees API with ?recursive=1. The ref can be a branch, tag, or SHA.
func fetchRepoTree(ctx context.Context, owner, repo, ref string) ([]gitTreeEntry, error) {
	reqURL := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", githubBaseURL, owner, repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := httpDoer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github tree request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing actionable on close failure of a drained body

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github tree status %d: %s", resp.StatusCode, string(body))
	}

	var out gitTreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode tree response: %w", err)
	}
	// If the tree is truncated, candidates may be incomplete — we still
	// return what we have. The healing PR body will note this so reviewers
	// know the suggestion may miss files.
	return out.Tree, nil
}

// stemSimilarity returns a similarity score in [0,1] between two file stems.
// The scoring combines exact-match (1.0), common-prefix boost, and a
// token-overlap component that handles reorderings like create-workflow →
// workflow-create.
func stemSimilarity(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return 1.0
	}
	// Token overlap (Jaccard) on word-split stems.
	at := tokenizeStem(a)
	bt := tokenizeStem(b)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	intersect := 0
	for _, x := range at {
		for _, y := range bt {
			if x == y {
				intersect++
				break
			}
		}
	}
	union := len(at) + len(bt) - intersect
	jaccard := float64(intersect) / float64(union)

	// Common prefix contribution — "create-workflow" vs "create-workflow-v2"
	// should score higher than Jaccard alone.
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	prefixLen := 0
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			break
		}
		prefixLen++
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	prefixRatio := float64(prefixLen) / float64(maxLen)

	// Blend: Jaccard dominates (it handles reordering), prefix adds a small
	// boost for shared leading substrings.
	return 0.7*jaccard + 0.3*prefixRatio
}

// tokenizeStem splits a file stem on hyphens, underscores, dots, and case
// boundaries (camelCase). Used for token-overlap similarity.
func tokenizeStem(s string) []string {
	// Replace common separators with single delimiter, then split.
	replacer := strings.NewReplacer("-", " ", "_", " ", ".", " ")
	s = replacer.Replace(s)
	// Insert spaces before uppercase letters that follow lowercase (camelCase
	// splitting). We assume input has already been lowercased by the caller,
	// but guard anyway.
	fields := strings.Fields(s)
	var out []string
	for _, f := range fields {
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}
