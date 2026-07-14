package main

import (
	"fmt"

	"github.com/OpenScribbler/capmon"
	"github.com/OpenScribbler/capmon/internal/output"
	"github.com/spf13/cobra"
)

var capmonHashURLCmd = &cobra.Command{
	Use:   "hash-url <url>",
	Short: "Compute the canonical content_hash baseline for a source URL",
	Long: "Fetch a URL through the exact code path the drift check uses (pinned " +
		"User-Agent, Accept, and Accept-Encoding headers) and print the SHA-256 " +
		"content hash. Always use this — not curl — when computing content_hash " +
		"baselines for docs/provider-formats/*.yaml: several hosts (Mintlify sites, " +
		"code.claude.com) serve different bytes under different headers.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := capmon.HashURL(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(output.Writer, "content_hash: %s\n", res.Hash)
		fmt.Fprintf(output.Writer, "content_type: %s\n", res.ContentType)
		fmt.Fprintf(output.Writer, "final_url:    %s\n", res.FinalURL)
		fmt.Fprintf(output.Writer, "bytes:        %d\n", res.Size)
		return nil
	},
}

func init() {
	capmonCmd.AddCommand(capmonHashURLCmd)
}
