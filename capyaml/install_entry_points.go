package capyaml

import (
	"fmt"
	"sort"
	"strings"
)

// The closed value sets of the ACIF install-targets row model (spec §6, §7,
// §9) and the closed placeholder set of its path-template grammar (§8.1).
// Matching is exact byte comparison, as ACIF specifies.
var (
	installScopes   = map[string]bool{"user": true, "project": true, "managed": true}
	installLayouts  = map[string]bool{"single_file": true, "directory_of_files": true, "merged_into_shared_file": true}
	installStatuses = map[string]bool{"current": true, "superseded": true}
	installOSes     = map[string]bool{"darwin": true, "linux": true, "windows": true}
)

const (
	tokenContentName = "<content-name>"
	tokenAppdata     = "<appdata>"
)

// ValidateCapabilities checks the rules a capability file must satisfy
// beyond parsing. Today that is the install_entry_points list of every
// content type.
func ValidateCapabilities(caps *ProviderCapabilities) error {
	names := make([]string, 0, len(caps.ContentTypes))
	for name := range caps.ContentTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ValidateInstallEntryPoints(caps.ContentTypes[name].InstallEntryPoints); err != nil {
			return fmt.Errorf("content_types.%s.install_entry_points: %w", name, err)
		}
	}
	return nil
}

// ValidateInstallEntryPoints checks one (provider, content type) row list
// against the ACIF install-targets row model: the closed scope, layout and
// status sets, the os set rules, the closed path-template grammar and its
// anchors, layout and <content-name> agreement, and uniqueness of
// (scope, path_template). An empty list is valid: it publishes no location.
func ValidateInstallEntryPoints(rows []InstallEntryPoint) error {
	seen := make(map[string]int, len(rows))
	for i, r := range rows {
		if err := validateInstallRow(r); err != nil {
			return fmt.Errorf("row %d: %w", i, err)
		}
		key := r.Scope + "\x00" + r.PathTemplate
		if j, dup := seen[key]; dup {
			return fmt.Errorf("row %d: duplicate (scope %q, path_template %q) first seen at row %d", i, r.Scope, r.PathTemplate, j)
		}
		seen[key] = i
	}
	return nil
}

func validateInstallRow(r InstallEntryPoint) error {
	if !installScopes[r.Scope] {
		return fmt.Errorf("unknown scope %q (want user, project or managed)", r.Scope)
	}
	if !installLayouts[r.Layout] {
		return fmt.Errorf("unknown layout %q (want single_file, directory_of_files or merged_into_shared_file)", r.Layout)
	}
	if !installStatuses[r.Status] {
		return fmt.Errorf("unknown status %q (want current or superseded)", r.Status)
	}
	if strings.TrimSpace(r.AsOf) == "" {
		return fmt.Errorf("as_of is empty")
	}
	if err := validateInstallOS(r.OS); err != nil {
		return err
	}
	return validatePathTemplate(r)
}

// validateInstallOS enforces ACIF §6: when present, os is a non-empty set
// over {darwin, linux, windows}, sorted by byte order, without duplicates,
// and never all three (the unconstrained row omits the field instead).
func validateInstallOS(oses []string) error {
	if oses == nil {
		return nil
	}
	if len(oses) == 0 {
		return fmt.Errorf("os is present but empty; omit it for a row that applies on every OS")
	}
	for i, o := range oses {
		if !installOSes[o] {
			return fmt.Errorf("unknown os %q (want darwin, linux or windows)", o)
		}
		if i > 0 && oses[i-1] >= o {
			return fmt.Errorf("os %v is not sorted and duplicate-free", oses)
		}
	}
	if len(oses) == len(installOSes) {
		return fmt.Errorf("os lists all three members; omit it for a row that applies on every OS")
	}
	return nil
}

// validatePathTemplate enforces the ACIF §8 grammar and anchors and the §9
// layout rules for one row.
func validatePathTemplate(r InstallEntryPoint) error {
	t := r.PathTemplate
	if t == "" {
		return fmt.Errorf("path_template is empty")
	}
	if strings.ContainsAny(t, "\\\x00") {
		return fmt.Errorf("path_template %q: backslash or NUL (templates use / separators only)", t)
	}
	if strings.Contains(t, "//") {
		return fmt.Errorf("path_template %q: empty segment", t)
	}

	// Every '<' must open a token from the closed set, and no '>' may stand
	// outside one.
	hasContentName := false
	rest := t
	for {
		lt := strings.IndexByte(rest, '<')
		if gt := strings.IndexByte(rest, '>'); gt >= 0 && (lt < 0 || gt < lt) {
			return fmt.Errorf("path_template %q: stray '>'", t)
		}
		if lt < 0 {
			break
		}
		rest = rest[lt:]
		switch {
		case strings.HasPrefix(rest, tokenContentName):
			hasContentName = true
			rest = rest[len(tokenContentName):]
		case strings.HasPrefix(rest, tokenAppdata):
			if len(rest) != len(t) {
				return fmt.Errorf("path_template %q: <appdata> may appear only as the leading segment", t)
			}
			rest = rest[len(tokenAppdata):]
		default:
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return fmt.Errorf("path_template %q: unterminated placeholder", t)
			}
			return fmt.Errorf("path_template %q: placeholder %s is outside the closed set {<content-name>, <appdata>}", t, rest[:end+1])
		}
	}

	if i := strings.IndexByte(t, '~'); i >= 0 && (i != 0 || !strings.HasPrefix(t, "~/") || strings.Count(t, "~") != 1) {
		return fmt.Errorf("path_template %q: ~ may appear only as a leading ~/", t)
	}

	appdata := strings.HasPrefix(t, tokenAppdata)
	if appdata && !strings.HasPrefix(t, tokenAppdata+"/") {
		return fmt.Errorf("path_template %q: <appdata> must be a whole leading segment", t)
	}
	if appdata && !(len(r.OS) == 1 && r.OS[0] == "windows") {
		return fmt.Errorf("path_template %q: <appdata> requires os exactly [windows]", t)
	}

	home := strings.HasPrefix(t, "~/")
	absolute := strings.HasPrefix(t, "/") || isDriveAbsolute(t)
	switch r.Scope {
	case "user":
		if !home && !appdata {
			return fmt.Errorf("path_template %q: a user-scope template is anchored at ~/ or <appdata>/", t)
		}
	case "project":
		if home || appdata || absolute {
			return fmt.Errorf("path_template %q: a project-scope template is relative to the project root", t)
		}
	case "managed":
		if !absolute {
			return fmt.Errorf("path_template %q: a managed-scope template is absolute", t)
		}
	}

	switch r.Layout {
	case "single_file", "directory_of_files":
		if !hasContentName {
			return fmt.Errorf("path_template %q: layout %s names one item and must carry <content-name>", t, r.Layout)
		}
	case "merged_into_shared_file":
		if hasContentName {
			return fmt.Errorf("path_template %q: layout merged_into_shared_file names a shared file and must not carry <content-name>", t)
		}
	}
	return nil
}

// isDriveAbsolute reports whether t starts with a Windows drive letter
// written with a / separator, such as C:/.
func isDriveAbsolute(t string) bool {
	return len(t) >= 3 && t[1] == ':' && t[2] == '/' &&
		((t[0] >= 'A' && t[0] <= 'Z') || (t[0] >= 'a' && t[0] <= 'z'))
}
