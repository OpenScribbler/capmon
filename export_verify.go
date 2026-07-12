package capmon

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenScribbler/capmon/internal/output"
)

// verifyHTTPTimeout bounds every fetch against the published site so a hung
// server can never wedge the verifier.
const verifyHTTPTimeout = 30 * time.Second

// verifyMaxBodyBytes caps every fetched body so a misbehaving endpoint can
// never balloon the verifier's memory; the largest published document is
// orders of magnitude smaller.
const verifyMaxBodyBytes = 64 << 20

// RunExportVerify rebuilds the /v1/ tree from a source commit and diffs it
// byte-for-byte against the live published site under baseURL, returning nil on
// total byte-identity and EXPORT_004 naming the first divergent path otherwise.
// It is a maintainer/consumer conformance tool and is never part of the publish
// gate. The live v1/index.json is fetched first so GeneratedAt/SourceCommit can
// be pinned from it, making the comparison total byte equality on every file —
// v1/index.json included. docs/ is materialized at the commit via git archive,
// so the rebuild reflects the committed tree, never the working directory.
func RunExportVerify(commit, baseURL string) error {
	base := strings.TrimRight(baseURL, "/") + "/"
	client := &http.Client{Timeout: verifyHTTPTimeout}

	indexURL := base + "v1/index.json"
	idxBytes, err := verifyFetch(client, indexURL)
	if err != nil {
		return fmt.Errorf("fetch live index %s: %w", indexURL, err)
	}
	var idx struct {
		GeneratedAt  string `json:"generated_at"`
		SourceCommit string `json:"source_commit"`
	}
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return fmt.Errorf("parse live index %s: %w", indexURL, err)
	}

	srcDir, err := os.MkdirTemp("", "capmon-verify-src-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(srcDir)
	if err := archiveDocs(commit, srcDir); err != nil {
		return err
	}

	outParent, err := os.MkdirTemp("", "capmon-verify-out-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(outParent)

	docs := filepath.Join(srcDir, "docs")
	outDir := filepath.Join(outParent, "site")
	opts := ExportOptions{
		CapsDir:           filepath.Join(docs, "provider-capabilities"),
		CanonicalKeysPath: filepath.Join(docs, "spec", "canonical-keys.yaml"),
		SourcesDir:        filepath.Join(docs, "provider-sources"),
		PublishAssetsDir:  filepath.Join(docs, "publish"),
		OutDir:            outDir,
		GeneratedAt:       idx.GeneratedAt,
		SourceCommit:      idx.SourceCommit,
	}
	if err := RunExport(opts); err != nil {
		return err
	}

	return filepath.Walk(outDir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel := filepath.ToSlash(mustRel(outDir, p))
		want, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fileURL := base + rel
		got, err := verifyFetch(client, fileURL)
		if err != nil {
			return fmt.Errorf("fetch published file %s: %w", fileURL, err)
		}
		if !bytes.Equal(want, got) {
			return output.NewStructuredError(
				"EXPORT_004",
				fmt.Sprintf("published v1/ document %s diverges from the rebuild of commit %s", rel, commit),
				"The live site no longer matches a deterministic rebuild of this commit; republish from a clean export or investigate the drift.",
			)
		}
		return nil
	})
}

// mustRel returns the slash-free rel path of p under base; Walk only ever
// yields paths under base, so a failure is a programming error.
func mustRel(base, p string) string {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return rel
}

// verifyFetch GETs url and returns its body, treating any transport error or
// non-200 status as a fetch failure.
func verifyFetch(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, verifyMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > verifyMaxBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", verifyMaxBodyBytes)
	}
	return b, nil
}

// archiveDocs materializes the docs/ subtree at commit into dst via git archive,
// running git in the current working directory. Using the archive (not the
// working tree) guarantees the rebuild sees exactly the committed bytes.
func archiveDocs(commit, dst string) error {
	cmd := exec.Command("git", "archive", "--format=tar", commit, "docs")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git archive %s docs: %v: %s", commit, err, strings.TrimSpace(stderr.String()))
	}
	return extractTar(&stdout, dst)
}

// extractTar unpacks a tar stream into dst, creating directories and regular
// files with their recorded modes.
func extractTar(r io.Reader, dst string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// git archive of a committed tree can't produce these, but the
		// extractor must not rely on its caller: reject any entry that would
		// land outside dst.
		if !filepath.IsLocal(filepath.FromSlash(hdr.Name)) {
			return fmt.Errorf("tar entry %q escapes the extraction dir", hdr.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}
