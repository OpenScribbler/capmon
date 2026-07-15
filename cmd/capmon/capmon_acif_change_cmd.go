package main

import (
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

func init() {
	capmonACIFChangeScanCmd.Flags().String("cache-root", ".capmon-cache", "Root directory for capmon cache")
	capmonACIFChangeScanCmd.Flags().String("format-docs", "docs/provider-formats", "Directory containing provider format docs")

	capmonACIFChangeCmd.AddCommand(capmonACIFChangeScanCmd)
	capmonCmd.AddCommand(capmonACIFChangeCmd)
}
