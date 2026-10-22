// Package noprint is the sample analyzer that proves the check infrastructure: it refuses the
// predeclared print and println, which write to stderr past the logger. It carries rule number 0,
// the "infrastructure" rule; the first analyzer for a rule of docs/agents/review-rules.md is
// written the same way and uses that rule's number.
package noprint

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

// Analyzer reports a call of the predeclared print or println.
var Analyzer = &analysis.Analyzer{
	Name:     "noprint",
	Doc:      "refuses the predeclared print and println (sample analyzer of the check infrastructure)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call, isNode := n.(*ast.CallExpr)
		if !isNode {
			return
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			return
		}
		b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
		if !ok || (b.Name() != "print" && b.Name() != "println") {
			return
		}
		pass.Reportf(call.Pos(), "%s", rule.Message(0,
			"the builtin "+b.Name()+" writes to stderr past the logger",
			"log through log/slog"))
	})
	return nil, nil
}
