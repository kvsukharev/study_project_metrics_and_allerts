package analyzer

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// ExitAnalyzer reports calls to os.Exit and log.Fatal* outside of the main
// function of the main package.
var ExitAnalyzer = &analysis.Analyzer{
	Name:     "exitcheck",
	Doc:      "reports os.Exit and log.Fatal calls outside of main.main",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runExitCheck,
}

// forbiddenCalls maps real import paths to the function names we forbid.
// Keys are full import paths, not local aliases, so aliased imports are handled
// correctly (e.g. import mylog "log" still triggers on mylog.Fatal).
var forbiddenCalls = map[string][]string{
	"os":  {"Exit"},
	"log": {"Fatal", "Fatalf", "Fatalln"},
}

func runExitCheck(pass *analysis.Pass) (interface{}, error) {
	// Only applies to the main package.
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect the AST node for the main() function declaration.
	var mainFuncBody *ast.BlockStmt
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn, ok := n.(*ast.FuncDecl)
		if ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFuncBody = fn.Body
		}
	})

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return
		}

		// Resolve the identifier through the type system to obtain the real
		// import path. This handles aliased imports correctly: the alias is
		// whatever the programmer wrote before the dot, but Imported().Path()
		// always returns the canonical import path regardless of the alias.
		obj, ok := pass.TypesInfo.Uses[pkgIdent]
		if !ok {
			return
		}
		pkgName, ok := obj.(*types.PkgName)
		if !ok {
			return
		}

		names, known := forbiddenCalls[pkgName.Imported().Path()]
		if !known {
			return
		}
		for _, name := range names {
			if sel.Sel.Name == name {
				// Allow the call if it is inside the main() function body.
				if mainFuncBody != nil && nodeInBlock(call, mainFuncBody) {
					return
				}
				pass.Reportf(call.Pos(), "call to %s.%s is forbidden outside of main.main",
					pkgIdent.Name, sel.Sel.Name)
			}
		}
	})
	return nil, nil
}

// nodeInBlock reports whether node's position falls within the given block.
func nodeInBlock(node ast.Node, block *ast.BlockStmt) bool {
	pos := node.Pos()
	return pos >= block.Lbrace && pos <= block.Rbrace
}
