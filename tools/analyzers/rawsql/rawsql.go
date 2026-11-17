// Package rawsql enforces rule 4 of docs/standards/review-rules.md for raw SQL: a statement run as text
// (ExecContext, QueryContext, QueryRowContext and database/sql's Exec, Query, QueryRow) or a raw
// fragment spliced into an ent query (entgo.io/ent/dialect/sql's P, ExprFunc, Expr, ExprP, Raw, or
// SQL text glued from a string literal and handed to any other function or method of that package,
// such as sql.As or Selector.Select) must carry a comment `// raw sql: <why ent cannot express it>`
// on the line above the statement or call, or trailing on its line. A Builder's own methods are
// left out: they write inside a P or ExprFunc, which is reported itself. Migrations are not Go code
// and are not checked here.
package rawsql

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/reason"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const marker = "raw sql:"

const entSQLPath = "entgo.io/ent/dialect/sql"

const fix = "build it with ent (a predicate, an aggregate, an upsert), or if ent cannot express it write `// " +
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
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
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
		if reason.Given(pass.Fset, stack, marker) {
			return true
		}
		rule.Report(pass, call.Pos(), 4,
			"raw SQL "+kind+" with no reason written next to it", fix)
		return true
	})
	return nil, nil
}

// classify returns "statement", "fragment" or "text" for a raw SQL call, "" otherwise.
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
	inEntSQL := fn.Pkg() != nil && fn.Pkg().Path() == entSQLPath
	if sig.Recv() == nil && inEntSQL && fragmentFuncs[fn.Name()] {
		return "fragment"
	}
	if inEntSQL && !isBuilder(sig.Recv()) && gluesText(call.Args) {
		return "text"
	}
	if sig.Recv() == nil {
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

// isBuilder reports whether recv is the receiver of a *sql.Builder method.
func isBuilder(recv *types.Var) bool {
	if recv == nil {
		return false
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Name() == "Builder"
}

// gluesText reports whether an argument is SQL text glued together with + from a string literal
// ("COUNT(*) FILTER (WHERE " + column + ")").
func gluesText(args []ast.Expr) bool {
	return slices.ContainsFunc(args, glued)
}

func glued(e ast.Expr) bool {
	bin, ok := ast.Unparen(e).(*ast.BinaryExpr)
	if !ok || bin.Op != token.ADD {
		return false
	}
	return literal(bin.X) || literal(bin.Y) || glued(bin.X) || glued(bin.Y)
}

func literal(e ast.Expr) bool {
	lit, ok := ast.Unparen(e).(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}
