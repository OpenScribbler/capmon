// Command acifdrift diffs capmon's docs/spec/canonical-keys.yaml against
// ACIF's published capability-vocabulary export
// (conformance/capability-vocabulary.yaml in the ACIF repository).
//
// Authority direction (ACIF CHANGE-PROCESS.md, "Source-of-truth rule"):
// ACIF owns the vocabulary; canonical-keys.yaml is a derived copy. If the
// two disagree, capmon has the bug — this program exits non-zero and
// capmon's CI fails, never ACIF's. The comparison target is the
// machine-readable export, never ACIF spec prose.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// capmon section name -> ACIF kind token.
var sectionToKind = map[string]string{
	"skills":   "skill",
	"rules":    "rule",
	"commands": "command",
	"agents":   "agent",
	"hooks":    "hook",
	"mcp":      "mcp_config",
}

type vocabularyEntry struct {
	Derivable      []string `yaml:"derivable"`
	OutOfScopeAtL1 []string `yaml:"out_of_scope_at_l1"`
}

type vocabularyFile struct {
	Vocabulary map[string]vocabularyEntry `yaml:"vocabulary"`
}

type canonicalKey struct {
	SpecRef string `yaml:"spec_ref"`
}

type canonicalKeysFile struct {
	ContentTypes map[string]map[string]canonicalKey `yaml:"content_types"`
}

func main() {
	canonicalPath := flag.String("canonical", "docs/spec/canonical-keys.yaml", "capmon canonical-keys.yaml path")
	vocabularyPath := flag.String("vocabulary", "", "ACIF capability-vocabulary.yaml path (required)")
	flag.Parse()
	if *vocabularyPath == "" {
		fmt.Fprintln(os.Stderr, "acifdrift: --vocabulary is required")
		os.Exit(2)
	}

	drifts, err := Diff(*canonicalPath, *vocabularyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "acifdrift: %v\n", err)
		os.Exit(2)
	}
	if len(drifts) > 0 {
		fmt.Println("canonical-keys.yaml drifted from the ACIF capability vocabulary (capmon has the bug — ACIF governs):")
		for _, d := range drifts {
			fmt.Println("  " + d)
		}
		os.Exit(1)
	}
	fmt.Println("acifdrift: canonical-keys.yaml matches the ACIF capability vocabulary")
}

// Diff returns one message per divergence between capmon's canonical-keys
// file and ACIF's capability-vocabulary export.
func Diff(canonicalPath, vocabularyPath string) ([]string, error) {
	canonical, err := loadCanonical(canonicalPath)
	if err != nil {
		return nil, err
	}
	vocabulary, err := loadVocabulary(vocabularyPath)
	if err != nil {
		return nil, err
	}

	var drifts []string
	seenKinds := map[string]bool{}
	for section, keys := range canonical.ContentTypes {
		kind, ok := sectionToKind[section]
		if !ok {
			drifts = append(drifts, fmt.Sprintf("unknown content_types section %q (no ACIF kind mapping)", section))
			continue
		}
		seenKinds[kind] = true
		entry, ok := vocabulary.Vocabulary[kind]
		if !ok {
			drifts = append(drifts, fmt.Sprintf("%s: kind missing from ACIF vocabulary export", kind))
			continue
		}
		drifts = append(drifts, diffKind(section, keys, entry)...)
	}
	for kind := range vocabulary.Vocabulary {
		if !seenKinds[kind] {
			drifts = append(drifts, fmt.Sprintf("%s: kind present in ACIF vocabulary but missing from canonical-keys.yaml", kind))
		}
	}
	sort.Strings(drifts)
	return drifts, nil
}

func diffKind(section string, keys map[string]canonicalKey, entry vocabularyEntry) []string {
	classOf := map[string]string{}
	for _, k := range entry.Derivable {
		classOf[k] = "DERIVABLE"
	}
	for _, k := range entry.OutOfScopeAtL1 {
		classOf[k] = "OUT-OF-SCOPE-AT-L1"
	}

	var drifts []string
	for key, meta := range keys {
		wantClass, known := classOf[key]
		if !known {
			drifts = append(drifts, fmt.Sprintf("%s.%s: key not in the ACIF vocabulary (removed or renamed upstream, or a capmon addition ACIF has not graduated)", section, key))
			continue
		}
		if got := classify(meta.SpecRef); got != wantClass {
			drifts = append(drifts, fmt.Sprintf("%s.%s: classification %s in spec_ref, ACIF says %s", section, key, got, wantClass))
		}
	}
	for key := range classOf {
		if _, ok := keys[key]; !ok {
			drifts = append(drifts, fmt.Sprintf("%s.%s: key in the ACIF vocabulary but missing from canonical-keys.yaml", section, key))
		}
	}
	return drifts
}

// classify extracts the disposition token from a canonical-keys spec_ref,
// e.g. "ACIF-SKILL §10.1 (DERIVABLE); ..." -> DERIVABLE.
func classify(specRef string) string {
	switch {
	case strings.Contains(specRef, "(OUT-OF-SCOPE-AT-L1"):
		return "OUT-OF-SCOPE-AT-L1"
	case strings.Contains(specRef, "(DERIVABLE"):
		return "DERIVABLE"
	default:
		return "UNCLASSIFIED"
	}
}

func loadCanonical(path string) (*canonicalKeysFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file canonicalKeysFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(file.ContentTypes) == 0 {
		return nil, fmt.Errorf("%s: no content_types sections", path)
	}
	return &file, nil
}

func loadVocabulary(path string) (*vocabularyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file vocabularyFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(file.Vocabulary) == 0 {
		return nil, fmt.Errorf("%s: no vocabulary mapping", path)
	}
	return &file, nil
}
