package capmon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchForCheckPinsHeaders asserts the check fetch path pins its content
// negotiation: UA capmon/1.0, Accept text/markdown for *.md paths and */*
// otherwise, Accept-Encoding identity. These headers ARE the baseline-hash
// contract — several hosts (Mintlify, code.claude.com) vary bytes on them, so
// any change here invalidates every content_hash in docs/provider-formats/.
func TestFetchForCheckPinsHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	for path, wantAccept := range map[string]string{
		"/docs/en/hooks.md": "text/markdown",
		"/docs/hooks":       "*/*",
	} {
		if _, _, _, err := fetchForCheck(context.Background(), srv.URL+path); err != nil {
			t.Fatalf("fetchForCheck(%s): %v", path, err)
		}
		if ua := got.Get("User-Agent"); ua != "capmon/1.0" {
			t.Errorf("%s: User-Agent = %q, want capmon/1.0", path, ua)
		}
		if a := got.Get("Accept"); a != wantAccept {
			t.Errorf("%s: Accept = %q, want %q", path, a, wantAccept)
		}
		if ae := got.Get("Accept-Encoding"); ae != "identity" {
			t.Errorf("%s: Accept-Encoding = %q, want identity", path, ae)
		}
	}
}

func TestBuildProviderIssueBody_HashChanges(t *testing.T) {
	batch := &providerBatch{
		changes: []sourceChange{
			{contentType: "skills", sourceURI: "https://example.com/skills.md", oldHash: "sha256:old1", newHash: "sha256:new1"},
			{contentType: "hooks", sourceURI: "https://example.com/hooks.md", oldHash: "sha256:old2", newHash: "sha256:new2"},
		},
	}
	body := buildProviderIssueBody(batch)

	for _, want := range []string{"## skills", "## hooks", "https://example.com/skills.md", "sha256:old1", "sha256:new1", "https://example.com/hooks.md", "sha256:old2", "sha256:new2"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got:\n%s", want, body)
		}
	}
	if strings.Contains(body, "## Fetch Errors") {
		t.Errorf("body should not contain '## Fetch Errors' when there are no fetch errors")
	}
}

func TestBuildProviderIssueBody_FetchErrorsOnly(t *testing.T) {
	batch := &providerBatch{
		fetchErrors: []fetchErrorEntry{
			{contentType: "skills", sourceURI: "https://example.com/skills.md", reason: "fetch error: connection refused"},
		},
	}
	body := buildProviderIssueBody(batch)

	if !strings.Contains(body, "## Fetch Errors") {
		t.Errorf("body should contain '## Fetch Errors', got:\n%s", body)
	}
	if !strings.Contains(body, "connection refused") {
		t.Errorf("body should contain fetch error reason, got:\n%s", body)
	}
	// No hash-change sections should be present.
	if strings.Contains(body, "## skills") || strings.Contains(body, "## hooks") {
		t.Errorf("body should not contain content-type H2 sections when there are only fetch errors, got:\n%s", body)
	}
}

func TestBuildProviderIssueBody_Mixed(t *testing.T) {
	batch := &providerBatch{
		changes: []sourceChange{
			{contentType: "skills", sourceURI: "https://example.com/skills.md", oldHash: "sha256:old", newHash: "sha256:new"},
		},
		fetchErrors: []fetchErrorEntry{
			{contentType: "hooks", sourceURI: "https://example.com/hooks.md", reason: "fetch error: timeout"},
		},
	}
	body := buildProviderIssueBody(batch)

	if !strings.Contains(body, "## skills") {
		t.Errorf("body should contain '## skills' section, got:\n%s", body)
	}
	if !strings.Contains(body, "## Fetch Errors") {
		t.Errorf("body should contain '## Fetch Errors' section, got:\n%s", body)
	}
	if !strings.Contains(body, "timeout") {
		t.Errorf("body should contain fetch error reason, got:\n%s", body)
	}
}

func TestBuildProviderIssueBody_Empty(t *testing.T) {
	batch := &providerBatch{}
	body := buildProviderIssueBody(batch)
	if body != "" {
		t.Errorf("expected empty body for empty batch, got: %q", body)
	}
}
