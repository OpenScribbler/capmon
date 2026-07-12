package main

import (
	"strings"
	"time"

	"github.com/OpenScribbler/capmon"
	"github.com/OpenScribbler/capmon/internal/output"
	"github.com/spf13/cobra"
)

// Override vars for test redirection: they point the exporter's source paths at
// fixture dirs. Empty (the CLI default) → RunExport applies its own repo-root
// defaults. Mirrors capmonCapabilitiesDirOverride in capmon_cmd.go.
var (
	exportCapsDirOverride           string
	exportCanonicalKeysPathOverride string
	exportSourcesDirOverride        string
	exportPublishAssetsDirOverride  string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the deterministic /v1/ capability JSON tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString("out")
		sourceCommit, _ := cmd.Flags().GetString("source-commit")
		generatedAt, _ := cmd.Flags().GetString("generated-at")

		// Validate --generated-at as an RFC 3339 UTC timestamp with a Z offset
		// BEFORE any filesystem work, so a bad flag never creates or replaces an
		// output tree.
		if generatedAt != "" {
			t, err := time.Parse(time.RFC3339, generatedAt)
			if err != nil || t.Location() != time.UTC || !strings.HasSuffix(generatedAt, "Z") {
				return output.NewStructuredError(output.ErrInputInvalid,
					"invalid --generated-at: must be an RFC 3339 UTC timestamp with a Z offset (e.g. 2026-01-01T00:00:00Z)",
					"Pass a UTC timestamp ending in Z, or omit --generated-at to use the current time")
			}
		}

		opts := capmon.ExportOptions{
			CapsDir:           exportCapsDirOverride,
			CanonicalKeysPath: exportCanonicalKeysPathOverride,
			SourcesDir:        exportSourcesDirOverride,
			PublishAssetsDir:  exportPublishAssetsDirOverride,
			OutDir:            out,
			SourceCommit:      sourceCommit,
			GeneratedAt:       generatedAt,
		}
		return capmon.RunExport(opts)
	},
}

func init() {
	exportCmd.Flags().String("out", "dist", "Output directory for the exported /v1/ tree")
	exportCmd.Flags().String("source-commit", "", "Source commit SHA to embed in v1/index.json (omitted when empty)")
	exportCmd.Flags().String("generated-at", "", "Pinned RFC 3339 UTC generated_at (Z offset); default: current time")
	capmonCmd.AddCommand(exportCmd)
}
