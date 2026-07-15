package capmon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type graduationExtensionDetail struct {
	Name    string `json:"name,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type graduationProviderState struct {
	ScanDates    map[string]bool `json:"scan_dates"`
	Name         string          `json:"name,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	LastObserved time.Time       `json:"last_observed,omitempty"`
}

type graduationState struct {
	ExtensionID             string                              `json:"extension_id"`
	Providers               map[string]*graduationProviderState `json:"providers"`
	IssueNumber             int                                 `json:"issue_number,omitempty"`
	LastObserved            time.Time                           `json:"last_observed"`
	StaleApplied            bool                                `json:"stale_applied,omitempty"`
	LastQualifyingProviders []string                            `json:"last_qualifying_providers,omitempty"`
}

// ScanGraduationCandidates walks provider format docs, records debounced
// per-(extension id, provider) sightings, and files or refreshes ACIF class-c
// issues for extension ids observed in 2 or more qualifying providers.
func ScanGraduationCandidates(cacheRoot, formatDocsDir string, now time.Time) ([]int, error) {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	scanDate := now.Format("2006-01-02")

	sightings, err := collectGraduationCandidateSightings(formatDocsDir)
	if err != nil {
		return nil, err
	}

	var updatedIssues []int
	for _, extensionID := range sortedGraduationExtensionIDs(sightings) {
		if err := validateGraduationExtensionID(extensionID); err != nil {
			return nil, err
		}
		path := acifGraduationStatePath(cacheRoot, extensionID)
		state, err := readGraduationState(path)
		if err != nil {
			return nil, err
		}
		if state.ExtensionID == "" {
			state.ExtensionID = extensionID
			state.Providers = make(map[string]*graduationProviderState)
		} else if state.ExtensionID != extensionID {
			return nil, fmt.Errorf("graduation state key mismatch in %s", path)
		}
		if state.Providers == nil {
			state.Providers = make(map[string]*graduationProviderState)
		}

		for _, provider := range sortedGraduationProviders(sightings[extensionID]) {
			detail := sightings[extensionID][provider]
			providerState := state.Providers[provider]
			if providerState == nil {
				providerState = &graduationProviderState{ScanDates: make(map[string]bool)}
				state.Providers[provider] = providerState
			}
			if providerState.ScanDates == nil {
				providerState.ScanDates = make(map[string]bool)
			}
			providerState.ScanDates[scanDate] = true
			providerState.Name = detail.Name
			providerState.Summary = detail.Summary
			providerState.LastObserved = now
			state.LastObserved = now
		}

		qualifyingProviders := qualifyingGraduationProviders(state)
		if len(qualifyingProviders) < 2 {
			if err := writeJSONState(path, &state); err != nil {
				return nil, err
			}
			continue
		}

		if state.IssueNumber > 0 {
			changed := !sameStringSlice(qualifyingProviders, state.LastQualifyingProviders)
			if changed {
				if err := appendACIFGraduationComment(state.IssueNumber, state, qualifyingProviders); err != nil {
					return nil, err
				}
				state.LastQualifyingProviders = qualifyingProviders
				updatedIssues = append(updatedIssues, state.IssueNumber)
			}
			if err := writeJSONState(path, &state); err != nil {
				return nil, err
			}
			continue
		}

		issueNum, found, err := findOpenACIFGraduationIssue(extensionID)
		if err != nil {
			return nil, err
		}
		if found {
			state.IssueNumber = issueNum
			if !sameStringSlice(qualifyingProviders, state.LastQualifyingProviders) {
				if err := appendACIFGraduationComment(issueNum, state, qualifyingProviders); err != nil {
					return nil, err
				}
				state.LastQualifyingProviders = qualifyingProviders
				updatedIssues = append(updatedIssues, issueNum)
			}
			if err := writeJSONState(path, &state); err != nil {
				return nil, err
			}
			continue
		}

		issueNum, err = createACIFGraduationIssue(state, qualifyingProviders)
		if err != nil {
			return nil, err
		}
		state.IssueNumber = issueNum
		state.LastQualifyingProviders = qualifyingProviders
		if err := writeJSONState(path, &state); err != nil {
			return nil, err
		}
		updatedIssues = append(updatedIssues, issueNum)
	}
	return updatedIssues, nil
}

func collectGraduationCandidateSightings(formatDocsDir string) (map[string]map[string]graduationExtensionDetail, error) {
	entries, err := os.ReadDir(formatDocsDir)
	if err != nil {
		return nil, fmt.Errorf("read format docs dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	sightings := make(map[string]map[string]graduationExtensionDetail)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(formatDocsDir, entry.Name())
		doc, err := LoadFormatDoc(path)
		if err != nil {
			return nil, err
		}
		provider := doc.Provider
		if provider == "" {
			provider = strings.TrimSuffix(entry.Name(), ".yaml")
		}
		provider, err = SanitizeSlug(provider)
		if err != nil {
			return nil, fmt.Errorf("invalid provider in %s: %w", path, err)
		}

		contentTypes := make([]string, 0, len(doc.ContentTypes))
		for contentType := range doc.ContentTypes {
			contentTypes = append(contentTypes, contentType)
		}
		sort.Strings(contentTypes)
		for _, contentType := range contentTypes {
			ctDoc := doc.ContentTypes[contentType]
			for _, ext := range ctDoc.ProviderExtensions {
				if !ext.GraduationCandidate {
					continue
				}
				if ext.ID == "" {
					return nil, fmt.Errorf("graduation candidate in %s has empty id", path)
				}
				if sightings[ext.ID] == nil {
					sightings[ext.ID] = make(map[string]graduationExtensionDetail)
				}
				if _, exists := sightings[ext.ID][provider]; !exists {
					sightings[ext.ID][provider] = graduationExtensionDetail{
						Name:    ext.Name,
						Summary: ext.Summary,
					}
				}
			}
		}
	}
	return sightings, nil
}

func sortedGraduationExtensionIDs(sightings map[string]map[string]graduationExtensionDetail) []string {
	ids := make([]string, 0, len(sightings))
	for id := range sightings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedGraduationProviders(providers map[string]graduationExtensionDetail) []string {
	names := make([]string, 0, len(providers))
	for provider := range providers {
		names = append(names, provider)
	}
	sort.Strings(names)
	return names
}

func validateGraduationExtensionID(extensionID string) error {
	if extensionID == "" {
		return fmt.Errorf("extension id is required")
	}
	if strings.Contains(extensionID, "/") || strings.Contains(extensionID, "\\") {
		return fmt.Errorf("extension id %q is not safe for a state filename", extensionID)
	}
	return nil
}

func acifGraduationStatePath(cacheRoot, extensionID string) string {
	return filepath.Join(cacheRoot, acifStateDir, "graduation", extensionID+".json")
}

func readGraduationState(path string) (graduationState, error) {
	var state graduationState
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read ACIF graduation state %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse ACIF graduation state %s: %w", path, err)
	}
	return state, nil
}

func qualifyingGraduationProviders(state graduationState) []string {
	var providers []string
	for provider, providerState := range state.Providers {
		if providerState != nil && len(providerState.ScanDates) >= 2 {
			providers = append(providers, provider)
		}
	}
	sort.Strings(providers)
	return providers
}

func acifGraduationAnchor(extensionID string) string {
	return fmt.Sprintf("<!-- capmon-acif-change-graduation: %s -->", extensionID)
}

func findOpenACIFGraduationIssue(extensionID string) (int, bool, error) {
	anchor := acifGraduationAnchor(extensionID)
	out, err := ghRunner("issue", "list",
		"--repo", acifChangeRepo,
		"--label", acifChangeLabel,
		"--label", acifClassCLabel,
		"--state", "open",
		"--limit", "100",
		"--json", "number,body",
	)
	if err != nil {
		return 0, false, fmt.Errorf("gh issue list: %w", err)
	}
	var issues []struct {
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return 0, false, fmt.Errorf("parse issue list: %w", err)
	}
	for _, iss := range issues {
		if strings.Contains(iss.Body, anchor) {
			return iss.Number, true, nil
		}
	}
	return 0, false, nil
}

func createACIFGraduationIssue(state graduationState, qualifyingProviders []string) (int, error) {
	title := fmt.Sprintf("acif-change: extension %q observed in %d providers (class-c candidate)", state.ExtensionID, len(qualifyingProviders))
	body := buildACIFGraduationIssueBody(state, qualifyingProviders)
	out, err := ghRunner("issue", "create",
		"--repo", acifChangeRepo,
		"--title", title,
		"--label", acifChangeLabel,
		"--label", acifClassCLabel,
		"--body", body,
	)
	if err != nil {
		return 0, fmt.Errorf("gh issue create: %w", err)
	}
	return parseGHIssueNumber(out)
}

func buildACIFGraduationIssueBody(state graduationState, qualifyingProviders []string) string {
	var b strings.Builder
	b.WriteString(acifGraduationAnchor(state.ExtensionID))
	b.WriteString("\n\nA provider extension has crossed the debounced 2+ provider threshold for an ACIF Class C vocabulary candidate.\n\n")
	fmt.Fprintf(&b, "**Extension ID:** `%s`\n", state.ExtensionID)
	fmt.Fprintf(&b, "**Qualifying providers:** %d\n", len(qualifyingProviders))
	fmt.Fprintf(&b, "**Last seen:** `%s`\n\n", formatACIFTime(state.LastObserved))
	b.WriteString("## Qualifying Providers\n\n")
	for _, provider := range qualifyingProviders {
		detail := state.Providers[provider]
		name := ""
		summary := ""
		if detail != nil {
			name = detail.Name
			summary = detail.Summary
		}
		fmt.Fprintf(&b, "- `%s`: **%s** - %s\n", provider, name, summary)
	}
	b.WriteString("\n**What to do:** Review the ACIF `CHANGE-PROCESS.md` Class C section and decide whether this extension should graduate into canonical vocabulary.\n")
	b.WriteString("Reference: https://github.com/holdenhewett/agent-content-interchange-format/blob/main/CHANGE-PROCESS.md\n")
	return b.String()
}

func appendACIFGraduationComment(issueNum int, state graduationState, qualifyingProviders []string) error {
	body := fmt.Sprintf(
		"Class C candidate refreshed: qualifying providers are now `%s`; last seen `%s`.",
		strings.Join(qualifyingProviders, "`, `"),
		formatACIFTime(state.LastObserved),
	)
	if _, err := ghRunner("issue", "comment",
		strconv.Itoa(issueNum),
		"--repo", acifChangeRepo,
		"--body", body,
	); err != nil {
		return fmt.Errorf("gh issue comment: %w", err)
	}
	return nil
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func markStaleGraduationFilings(cacheRoot string, now time.Time) error {
	root := filepath.Join(cacheRoot, acifStateDir, "graduation")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat ACIF graduation state root: %w", err)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		state, err := readGraduationState(path)
		if err != nil {
			return err
		}
		if !shouldMarkACIFStale(state.IssueNumber, state.LastObserved, state.StaleApplied, now) {
			return nil
		}
		if err := addACIFStaleLabel(state.IssueNumber); err != nil {
			return err
		}
		state.StaleApplied = true
		return writeJSONState(path, &state)
	})
}
