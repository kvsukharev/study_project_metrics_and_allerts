package analyzer_test

import (
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/cmd/linter/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPanicAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.PanicAnalyzer, "paniccheck")
}

func TestExitAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.ExitAnalyzer, "exitcheck")
}
