package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/OpenScribbler/capmon"
	"github.com/OpenScribbler/capmon/internal/output"
	"github.com/spf13/cobra"
)

var freezeCmd = &cobra.Command{
	Use:   "freeze <major>",
	Short: "Freeze the last live tree of a URL major into site-static/",
	Long: `Freeze copies the last live export of <major> (e.g. v1) from --from into
--out/<major>/ and applies the single final mutation: status "frozen",
superseded_by, and frozen_at on <major>/index.json and every per-provider
document. It validates the frozen tree against its own pinned schemas and
prints the root hash to record in frozenMajors (export_freeze.go).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		out, _ := cmd.Flags().GetString("out")
		supersededBy, _ := cmd.Flags().GetString("superseded-by")
		frozenAt, _ := cmd.Flags().GetString("frozen-at")
		renameFlags, _ := cmd.Flags().GetStringSlice("rename")

		t, err := time.Parse(time.RFC3339, frozenAt)
		if err != nil || t.Location() != time.UTC || !strings.HasSuffix(frozenAt, "Z") {
			return output.NewStructuredError(output.ErrInputInvalid,
				"invalid --frozen-at: must be an RFC 3339 UTC timestamp with a Z offset",
				"Pass a UTC timestamp ending in Z, e.g. 2026-01-01T00:00:00Z")
		}
		renames := map[string]string{}
		for _, r := range renameFlags {
			old, succ, ok := strings.Cut(r, "=")
			if !ok || old == "" || succ == "" {
				return output.NewStructuredError(output.ErrInputInvalid,
					fmt.Sprintf("invalid --rename %q", r),
					"Pass --rename old-slug=new-slug")
			}
			renames[old] = succ
		}

		sum, err := capmon.RunFreeze(capmon.FreezeOptions{
			Major:        args[0],
			FromDir:      from,
			OutDir:       out,
			SupersededBy: supersededBy,
			FrozenAt:     frozenAt,
			Renames:      renames,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), sum)
		return nil
	},
}

func init() {
	freezeCmd.Flags().String("from", "", "Exported site root holding the last live <major>/ tree")
	freezeCmd.Flags().String("out", "site-static", "Static root the frozen tree is written under")
	freezeCmd.Flags().String("superseded-by", "", "Successor major path, e.g. /v2/")
	freezeCmd.Flags().String("frozen-at", "", "RFC 3339 UTC freeze timestamp (Z offset)")
	freezeCmd.Flags().StringSlice("rename", nil, "old-slug=new-slug for a slug renamed at the major bump (repeatable)")
	_ = freezeCmd.MarkFlagRequired("from")
	_ = freezeCmd.MarkFlagRequired("superseded-by")
	_ = freezeCmd.MarkFlagRequired("frozen-at")
	capmonCmd.AddCommand(freezeCmd)
}
