// Package analyzer provides static analysis passes for the project linter.
package analyzer

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// PanicAnalyzer reports every direct call to the built-in panic function.
var PanicAnalyzer = &analysis.Analyzer{
	Name:     "paniccheck",
	Doc:      "reports use of the built-in panic function",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runPanicCheck,
}

func runPanicCheck(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return
		}
		if ident.Name == "panic" {
			pass.Reportf(call.Pos(), "use of built-in panic is forbidden")
		}
	})
	return nil, nil
}
