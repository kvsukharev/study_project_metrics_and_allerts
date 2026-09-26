// Package buildinfo prints build metadata set via -ldflags at compile time.
package buildinfo

import "fmt"

// Print writes version, date and commit to stdout.
// Variables are expected to be initialised to "N/A" at declaration so that
// the output is always meaningful, even without -ldflags.
func Print(version, date, commit string) {
	fmt.Printf("Build version: %s\nBuild date: %s\nBuild commit: %s\n",
		version, date, commit)
}
