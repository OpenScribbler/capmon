package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenScribbler/capmon"
	"github.com/spf13/cobra"
)

var capmonACIFChangeCmd = &cobra.Command{
	Use:   "acif-change",
	Short: "File ACIF vocabulary-change signals",
}

var capmonACIFChangeScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan provider format docs for ACIF graduation candidates",
	RunE: func(cmd *cobra.Command, args []string) error {
		cacheRoot, _ := cmd.Flags().GetString("cache-root")
		formatDocsDir, _ := cmd.Flags().GetString("format-docs")
		now := time.Now()
		if _, err := capmon.ScanGraduationCandidates(cacheRoot, formatDocsDir, now); err != nil {
			return err
		}
		return capmon.MarkStaleFilings(cacheRoot, now)
	},
}

var capmonACIFChangeConfirmCmd = &cobra.Command{
	Use:   "confirm",
	Short: "Machine-confirm status:unmapped candidates against the ACIF source-mechanism export",
	Long: `Machine-confirm curator-flagged status:unmapped canonical mappings.

Already-mapped candidates are rejected (report-only: curator YAML is never
edited). Candidates that cannot be machine-confirmed land unconfirmed and
file or refresh a needs-human issue on the ACIF repo with pinned evidence.
Every degraded path (export unreadable, adapter down, timeout, unparsable
source form) fails toward unconfirmed, never toward closure.

The export file must be fetched by the caller pinned to a git rev, passed
via --export and --export-rev. The hook probe adapter is an executable
speaking ACIF conformance adapter protocol 2 (--adapter, or the
CAPMON_ACIF_ADAPTER environment variable).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cacheRoot, _ := cmd.Flags().GetString("cache-root")
		formatDocsDir, _ := cmd.Flags().GetString("format-docs")
		exportPath, _ := cmd.Flags().GetString("export")
		exportRev, _ := cmd.Flags().GetString("export-rev")
		adapterPath, _ := cmd.Flags().GetString("adapter")
		probeTimeout, _ := cmd.Flags().GetDuration("probe-timeout")
		if adapterPath == "" {
			adapterPath = os.Getenv("CAPMON_ACIF_ADAPTER")
		}

		result, err := capmon.ConfirmUnmappedCandidates(capmon.ConfirmOptions{
			FormatDocsDir: formatDocsDir,
			CacheRoot:     cacheRoot,
			ExportPath:    exportPath,
			ExportRev:     exportRev,
			AdapterPath:   adapterPath,
			ProbeTimeout:  probeTimeout,
			Now:           time.Now(),
		})
		if result != nil {
			out := cmd.OutOrStdout()
			for _, f := range result.Findings {
				token := f.MechanismToken
				if token == "" {
					token = "(none)"
				}
				line := fmt.Sprintf("%-11s %s/%s/%s token=%s reason=%s", strings.ToUpper(f.Verdict), f.Provider, f.ContentType, f.CanonicalKey, token, f.Reason)
				if f.IssueNumber > 0 {
					line += fmt.Sprintf(" issue=#%d", f.IssueNumber)
				}
				fmt.Fprintln(out, line)
			}
			fmt.Fprintf(out, "%d candidate(s) confirmed\n", len(result.Findings))
		}
		if err != nil {
			return err
		}
		if result.Degraded {
			return fmt.Errorf("confirm run degraded: at least one candidate landed unconfirmed via a degraded path (export fetch, adapter, or source form)")
		}
		return nil
	},
}

func init() {
	capmonACIFChangeScanCmd.Flags().String("cache-root", ".capmon-cache", "Root directory for capmon cache")
	capmonACIFChangeScanCmd.Flags().String("format-docs", "docs/provider-formats", "Directory containing provider format docs")

	capmonACIFChangeConfirmCmd.Flags().String("cache-root", ".capmon-cache", "Root directory for capmon cache")
	capmonACIFChangeConfirmCmd.Flags().String("format-docs", "docs/provider-formats", "Directory containing provider format docs")
	capmonACIFChangeConfirmCmd.Flags().String("export", "", "Local path to ACIF conformance/source-mechanisms.yaml (fetched pinned to a git rev)")
	capmonACIFChangeConfirmCmd.Flags().String("export-rev", "", "Git rev the export file was fetched at (recorded in evidence)")
	capmonACIFChangeConfirmCmd.Flags().String("adapter", "", "Path to a hook-capable ACIF conformance adapter executable (default: $CAPMON_ACIF_ADAPTER)")
	capmonACIFChangeConfirmCmd.Flags().Duration("probe-timeout", 30*time.Second, "Per-request adapter timeout")
	capmonACIFChangeConfirmCmd.MarkFlagRequired("export")     //nolint:errcheck // flag name is a compile-time constant
	capmonACIFChangeConfirmCmd.MarkFlagRequired("export-rev") //nolint:errcheck // flag name is a compile-time constant

	capmonACIFChangeCmd.AddCommand(capmonACIFChangeScanCmd)
	capmonACIFChangeCmd.AddCommand(capmonACIFChangeConfirmCmd)
	capmonCmd.AddCommand(capmonACIFChangeCmd)
}
