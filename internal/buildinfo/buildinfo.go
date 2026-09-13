// Package buildinfo prints build metadata set via -ldflags at compile time.
package buildinfo

import "fmt"

// Print writes version, date and commit to stdout.
// Empty values (not set via -ldflags) are shown as "N/A".
func Print(version, date, commit string) {
	fmt.Printf("Build version: %s\nBuild date: %s\nBuild commit: %s\n",
		orNA(version), orNA(date), orNA(commit))
}

func orNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}
