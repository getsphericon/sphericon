// Package rawsql enforces rule 4 of docs/standards/review-rules.md for raw SQL: a statement run as text
// (ExecContext, QueryContext, QueryRowContext and database/sql's Exec, Query, QueryRow) or a raw
// fragment spliced into an ent query (entgo.io/ent/dialect/sql's P, ExprFunc, Expr, ExprP, Raw) must
// carry a comment `// raw sql: <why ent cannot express it>` on the line above the statement or call, or
// trailing on its line. Migrations are not Go code and are not checked here.
package rawsql

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const marker = "raw sql:"

const entSQLPath = "entgo.io/ent/dialect/sql"

var fix = "build it with ent (a predicate, an aggregate, an upsert), or if ent cannot express it write `// " +
	marker + " <reason>` on the line above"

// Analyzer reports raw SQL that gives no reason.
var Analyzer = &analysis.Analyzer{
	Name:     "rawsql",
	Doc:      "refuses raw SQL statements and fragments without a `raw sql:` reason comment",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var contextMethods = map[string]bool{"ExecContext": true, "QueryContext": true, "QueryRowContext": true}

var stdMethods = map[string]bool{"Exec": true, "Query": true, "QueryRow": true}

var fragmentFuncs = map[string]bool{"P": true, "ExprFunc": true, "Expr": true, "ExprP": true, "Raw": true}

func run(pass *analysis.Pass) (any, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call, isNode := n.(*ast.CallExpr)
		if !isNode {
			return true
		}
		kind := classify(pass, call)
		if kind == "" {
			return true
		}
		if hasReason(pass, call, stack) {
			return true
		}
		pass.Reportf(call.Pos(), "%s", rule.Message(4,
			"raw SQL "+kind+" with no reason written next to it", fix))
		return true
	})
	return nil, nil
}

// classify returns "statement" or "fragment" for a raw SQL call, "" otherwise.
func classify(pass *analysis.Pass, call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok {
		return ""
	}
	sig := fn.Signature()
	if sig.Recv() == nil {
		if fn.Pkg() != nil && fn.Pkg().Path() == entSQLPath && fragmentFuncs[fn.Name()] {
			return "fragment"
		}
		return ""
	}
	if contextMethods[fn.Name()] && hasQueryString(sig, 1) {
		return "statement"
	}
	if stdMethods[fn.Name()] && fn.Pkg() != nil && fn.Pkg().Path() == "database/sql" && hasQueryString(sig, 0) {
		return "statement"
	}
	return ""
}

// hasQueryString reports whether parameter i is the query string.
func hasQueryString(sig *types.Signature, i int) bool {
	if sig.Params().Len() <= i {
		return false
	}
	b, ok := sig.Params().At(i).Type().Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// hasReason finds a `raw sql: <reason>` comment on the call's line, or ending on the line above the
// call or above the statement that holds it.
func hasReason(pass *analysis.Pass, call *ast.CallExpr, stack []ast.Node) bool {
	file := enclosingFile(pass, call.Pos())
	if file == nil {
		return false
	}
	lines := map[int]bool{pass.Fset.Position(call.Pos()).Line: true}
	for _, node := range slices.Backward(stack) {
		if stmt, ok := node.(ast.Stmt); ok {
			lines[pass.Fset.Position(stmt.Pos()).Line] = true
			break
		}
	}
	for _, cg := range file.Comments {
		text := cg.Text()
		_, reason, found := strings.Cut(text, marker)
		if !found || strings.TrimSpace(reason) == "" {
			continue
		}
		start, end := pass.Fset.Position(cg.Pos()).Line, pass.Fset.Position(cg.End()).Line
		for l := range lines {
			if end == l-1 || (start <= l && l <= end) {
				return true
			}
		}
	}
	return false
}

func enclosingFile(pass *analysis.Pass, pos token.Pos) *ast.File {
	for _, f := range pass.Files {
		if f.FileStart <= pos && pos <= f.FileEnd {
			return f
		}
	}
	return nil
}
