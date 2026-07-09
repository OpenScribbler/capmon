// Command capmon is the standalone CLI for the capability-monitor pipeline.
//
// Extracted from syllago's `syllago capmon` subcommand tree; the subcommands
// and their flags are unchanged. It runs against a checkout of the syllago
// repository (the --formats-dir/--sources-dir/--canonical-keys defaults
// assume the working directory is a syllago repo root).
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := capmonCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
