// Command capmon is the standalone CLI for the capability-monitor pipeline.
//
// It runs against the capability data under docs/ in this repository (the
// --formats-dir/--sources-dir/--canonical-keys defaults assume the working
// directory is the repo root).
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
