// Command linter runs static analysis checks on the project.
// It reports:
//   - direct calls to the built-in panic function (paniccheck)
//   - calls to os.Exit and log.Fatal* outside of main.main (exitcheck)
//
// Usage:
//
//	go run ./cmd/linter ./...
package main

import (
	"github.com/kvsukharev/go-musthave-metrics-tpl/cmd/linter/analyzer"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(
		analyzer.PanicAnalyzer,
		analyzer.ExitAnalyzer,
	)
}
