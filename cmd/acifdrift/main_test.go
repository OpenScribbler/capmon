package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The committed canonical-keys.yaml must match the committed snapshot of
// ACIF's capability-vocabulary export. CI additionally diffs against the
// live export (acif-drift workflow); this hermetic test catches
// capmon-side edits that drift from the last synced snapshot.
func TestCanonicalKeysMatchSnapshot(t *testing.T) {
	drifts, err := Diff(
		filepath.Join("..", "..", "docs", "spec", "canonical-keys.yaml"),
		filepath.Join("testdata", "capability-vocabulary.snapshot.yaml"),
	)
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("canonical-keys.yaml drifted from the snapshot:\n  %s", strings.Join(drifts, "\n  "))
	}
}

func TestDiffDetectsDrift(t *testing.T) {
	vocabulary := `
vocabulary:
  skill:
    derivable: [auto_invocable]
    out_of_scope_at_l1: [display_name]
  rule:
    derivable: [activation_mode]
    out_of_scope_at_l1: []
`
	cases := []struct {
		name      string
		canonical string
		want      []string
	}{
		{
			name: "clean",
			canonical: `
content_types:
  skills:
    auto_invocable:
      spec_ref: ACIF-SKILL §10.1 (DERIVABLE)
    display_name:
      spec_ref: ACIF-SKILL §10.2 (OUT-OF-SCOPE-AT-L1)
  rules:
    activation_mode:
      spec_ref: ACIF-RULE §9.1 (DERIVABLE); activation-mode vocabulary
`,
			want: nil,
		},
		{
			name: "classification flip",
			canonical: `
content_types:
  skills:
    auto_invocable:
      spec_ref: ACIF-SKILL §10.2 (OUT-OF-SCOPE-AT-L1)
    display_name:
      spec_ref: ACIF-SKILL §10.2 (OUT-OF-SCOPE-AT-L1)
  rules:
    activation_mode:
      spec_ref: ACIF-RULE §9.1 (DERIVABLE)
`,
			want: []string{"skills.auto_invocable: classification OUT-OF-SCOPE-AT-L1 in spec_ref, ACIF says DERIVABLE"},
		},
		{
			name: "missing and extra keys",
			canonical: `
content_types:
  skills:
    auto_invocable:
      spec_ref: ACIF-SKILL §10.1 (DERIVABLE)
    invented_key:
      spec_ref: ACIF-SKILL §10.2 (OUT-OF-SCOPE-AT-L1)
  rules:
    activation_mode:
      spec_ref: ACIF-RULE §9.1 (DERIVABLE)
`,
			want: []string{
				"skills.display_name: key in the ACIF vocabulary but missing from canonical-keys.yaml",
				"skills.invented_key: key not in the ACIF vocabulary (removed or renamed upstream, or a capmon addition ACIF has not graduated)",
			},
		},
		{
			name: "missing kind",
			canonical: `
content_types:
  skills:
    auto_invocable:
      spec_ref: ACIF-SKILL §10.1 (DERIVABLE)
    display_name:
      spec_ref: ACIF-SKILL §10.2 (OUT-OF-SCOPE-AT-L1)
`,
			want: []string{"rule: kind present in ACIF vocabulary but missing from canonical-keys.yaml"},
		},
	}

	dir := t.TempDir()
	vocabPath := filepath.Join(dir, "vocabulary.yaml")
	if err := os.WriteFile(vocabPath, []byte(vocabulary), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonicalPath := filepath.Join(dir, "canonical.yaml")
			if err := os.WriteFile(canonicalPath, []byte(tc.canonical), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := Diff(canonicalPath, vocabPath)
			if err != nil {
				t.Fatalf("Diff() error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("Diff() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("drift[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
