package capmon

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// Workflow-posture drift guard (Slice 7). These tests encode the ADR 0004
// (fail-closed, SHA-pinned, attesting publish) and ADR 0005 (no scheduled
// job holds contents:write on the publish branch) invariants directly against
// the workflow YAML in the checkout, so a regression in permissions, triggers,
// or action pinning fails CI.
//
// yaml.v3 note: the workflow trigger key `on:` parses as the plain string
// "on" (tag !!str) under yaml.v3's 1.2 core schema — it is NOT coerced to the
// YAML 1.1 boolean true. Verified empirically; the map[string]any lookups
// below therefore key off the literal "on".

// fullSHA matches a full 40-hex-char commit pin, e.g. actions/checkout@de0fac2...
var fullSHA = regexp.MustCompile(`@[0-9a-f]{40}$`)

// loadWorkflow reads and parses a workflow file into a generic map tree.
// A missing or unparsable file is fatal with a clear message — for
// publish.yml that missing-file fatal is the RED signal until the impl bead
// creates it.
func loadWorkflow(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("workflow %s could not be read (expected to exist): %v", path, err)
	}
	var wf map[string]any
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("workflow %s failed to parse as YAML: %v", path, err)
	}
	return wf
}

// asMap coerces a parsed YAML node to a string-keyed map.
func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// asSlice coerces a parsed YAML node to a slice.
func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

// jobsOf returns the jobs mapping of a workflow.
func jobsOf(t *testing.T, wf map[string]any) map[string]any {
	t.Helper()
	jobs, ok := asMap(wf["jobs"])
	if !ok {
		t.Fatalf("workflow has no jobs mapping")
	}
	return jobs
}

// permsEqual reports whether a parsed permissions map equals want exactly
// (same keys, same string values, no extras).
func permsEqual(got map[string]any, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for k, wv := range want {
		gv, ok := got[k].(string)
		if !ok || gv != wv {
			return false
		}
	}
	return true
}

// grantsWrite reports whether a parsed permissions map grants any "write".
func grantsWrite(perms map[string]any) bool {
	for _, v := range perms {
		if s, ok := v.(string); ok && s == "write" {
			return true
		}
	}
	return false
}

// collectStringField walks an arbitrary parsed YAML tree and returns every
// string value stored under the given key name, at any depth.
func collectStringField(node any, key string) []string {
	var out []string
	switch n := node.(type) {
	case map[string]any:
		for k, v := range n {
			if k == key {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			out = append(out, collectStringField(v, key)...)
		}
	case []any:
		for _, v := range n {
			out = append(out, collectStringField(v, key)...)
		}
	}
	return out
}

// findConcurrency reports whether a concurrency group is declared anywhere in
// the tree — either `concurrency: pages` (bare string) or
// `concurrency: {group: ...}`.
func findConcurrency(node any) bool {
	switch n := node.(type) {
	case map[string]any:
		for k, v := range n {
			if k == "concurrency" {
				switch c := v.(type) {
				case string:
					if c != "" {
						return true
					}
				case map[string]any:
					if g, ok := c["group"].(string); ok && g != "" {
						return true
					}
				}
			}
			if findConcurrency(v) {
				return true
			}
		}
	case []any:
		for _, v := range n {
			if findConcurrency(v) {
				return true
			}
		}
	}
	return false
}

// sliceHas reports whether a parsed YAML slice contains the given string.
func sliceHas(s []any, want string) bool {
	for _, v := range s {
		if str, ok := v.(string); ok && str == want {
			return true
		}
	}
	return false
}

// TestPublishWorkflowHardening encodes the ADR 0004 publish posture against
// .github/workflows/publish.yml. Today the file does not exist, so
// loadWorkflow fatals — the RED signal. After the impl bead creates a
// fail-closed, SHA-pinned, attesting publish workflow, every assertion holds.
func TestPublishWorkflowHardening(t *testing.T) {
	path := filepath.Join(docsRoot(t), ".github", "workflows", "publish.yml")
	wf := loadWorkflow(t, path)

	// Top-level permissions is exactly {contents: read}.
	top, ok := asMap(wf["permissions"])
	if !ok {
		t.Fatalf("publish.yml has no top-level permissions block")
	}
	if !permsEqual(top, map[string]string{"contents": "read"}) {
		t.Errorf("top-level permissions = %v, want exactly {contents: read}", top)
	}

	// Triggers.
	on, ok := asMap(wf["on"])
	if !ok {
		t.Fatalf("publish.yml has no `on` triggers mapping")
	}
	if _, present := on["pull_request"]; present {
		t.Errorf("publish.yml must not trigger on pull_request (structurally cannot run on a PR)")
	}
	push, ok := asMap(on["push"])
	if !ok {
		t.Errorf("publish.yml `on.push` missing or not a mapping")
	} else {
		branches, _ := asSlice(push["branches"])
		if !sliceHas(branches, "main") {
			t.Errorf("on.push.branches = %v, want to include main", push["branches"])
		}
		paths, _ := asSlice(push["paths"])
		if !sliceHas(paths, "docs/**") {
			t.Errorf("on.push.paths = %v, want to include docs/**", push["paths"])
		}
	}
	sched, ok := asSlice(on["schedule"])
	if !ok || len(sched) == 0 {
		t.Errorf("on.schedule missing or empty, want at least one cron entry")
	} else {
		crons := collectStringField(sched, "cron")
		nonEmpty := false
		for _, c := range crons {
			if c != "" {
				nonEmpty = true
			}
		}
		if !nonEmpty {
			t.Errorf("on.schedule has no non-empty cron expression")
		}
	}
	if _, present := on["workflow_dispatch"]; !present {
		t.Errorf("publish.yml must declare a workflow_dispatch trigger")
	}

	// Job permissions: no job grants contents:write; any escalated job holds
	// exactly the publish triad {pages, id-token, attestations}: write.
	jobs := jobsOf(t, wf)
	escalated := 0
	for name, jv := range jobs {
		job, ok := asMap(jv)
		if !ok {
			continue
		}
		rawPerms, declared := job["permissions"]
		if !declared {
			continue
		}
		perms, ok := asMap(rawPerms)
		if !ok {
			// Scalar permission forms: `read-all` is harmless; anything else
			// (notably `write-all`, which grants contents: write) is a bypass
			// of the exact-triad rule and must fail the guard.
			if sv, isStr := rawPerms.(string); !isStr || sv != "read-all" {
				t.Errorf("job %q declares scalar permissions %v — only the map form or read-all is allowed", name, rawPerms)
			}
			continue
		}
		if cv, ok := perms["contents"].(string); ok && cv == "write" {
			t.Errorf("job %q grants contents: write — no publish job may hold it", name)
		}
		if grantsWrite(perms) {
			escalated++
			// Job-level permissions replace the top-level block, so the
			// escalated job re-grants contents: read alongside the triad;
			// exact-set equality still forbids any extra write scope.
			want := map[string]string{"contents": "read", "pages": "write", "id-token": "write", "attestations": "write"}
			if !permsEqual(perms, want) {
				t.Errorf("job %q escalated permissions = %v, want exactly %v", name, perms, want)
			}
		}
	}
	if escalated == 0 {
		t.Errorf("no job escalates to the publish triad {pages, id-token, attestations}: write")
	}

	// Every `uses:` is pinned to a full 40-hex commit SHA.
	uses := collectStringField(wf, "uses")
	if len(uses) == 0 {
		t.Errorf("publish.yml declares no `uses:` actions")
	}
	for _, u := range uses {
		if !fullSHA.MatchString(u) {
			t.Errorf("uses %q is not pinned to a full 40-char commit SHA", u)
		}
	}

	// Pages deploy concurrency group present.
	if !findConcurrency(wf) {
		t.Errorf("publish.yml declares no concurrency group (expected the pages deploy concurrency)")
	}
}

// TestHeartbeatHoldsNoContentsWrite encodes ADR 0005 against the pipeline
// heartbeat job. Today the heartbeat holds contents:write and pushes to main,
// so both assertions fail — the RED signal. The scope is the heartbeat job
// only: fetch-extract and report legitimately hold contents:write to open heal
// PRs against branches (not main), which ADR 0005 permits.
func TestHeartbeatHoldsNoContentsWrite(t *testing.T) {
	path := filepath.Join(docsRoot(t), ".github", "workflows", "pipeline.yml")
	wf := loadWorkflow(t, path)

	jobs := jobsOf(t, wf)
	hbAny, present := jobs["heartbeat"]
	if !present {
		t.Fatalf("pipeline.yml has no heartbeat job")
	}
	hb, ok := asMap(hbAny)
	if !ok {
		t.Fatalf("heartbeat job is not a mapping")
	}

	// Heartbeat permissions are exactly {actions: write}.
	perms, ok := asMap(hb["permissions"])
	if !ok {
		t.Fatalf("heartbeat job has no permissions block")
	}
	if !permsEqual(perms, map[string]string{"actions": "write"}) {
		t.Errorf("heartbeat permissions = %v, want exactly {actions: write}", perms)
	}

	// No step in the heartbeat job pushes or commits to git.
	for _, run := range collectStringField(hb, "run") {
		if regexp.MustCompile(`\bgit\s+push\b`).MatchString(run) {
			t.Errorf("heartbeat job runs `git push` — a scheduled job must not write to the publish branch:\n%s", run)
		}
		if regexp.MustCompile(`\bgit\s+commit\b`).MatchString(run) {
			t.Errorf("heartbeat job runs `git commit` — a scheduled job must not write to the publish branch:\n%s", run)
		}
	}
}
