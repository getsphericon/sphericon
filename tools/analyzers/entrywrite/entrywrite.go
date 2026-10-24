// Package entrywrite enforces rule 5 of docs/agents/review-rules.md for writes: a handler of any surface,
// an MCP tool, a job or an event subscriber receives input and calls a domain module; it does not write
// through ent itself. A write is a call that starts an ent mutation (Create, CreateBulk, Update,
// UpdateOne, UpdateOneID, Delete, DeleteOne, DeleteOneID) or an upsert (OnConflict, OnConflictColumns,
// OnConflictColumnsWhere) on a type of the ent package, which covers the raw client, a transaction and
// the generated scoped wrappers (`*ent.Scoped` and its per-entity wrappers). Reads are allowed. The
// raw-client packages listed in docs/agents/workspace-scoping.md are domain modules and are not entry points, so only the
// entry-point packages below are checked; test files are not. `ee/staff` holds the Staff surface's
// handlers and its domain module in one package, so there only the methods of the handler types are
// checked (`*Module` is the domain module).
package entrywrite

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const module = "github.com/getsphericon/sphericon/"

// entryPoints are the packages that receive input from outside (an API surface, MCP, a queue, the
// ingest hooks) and hand it to a domain module.
var entryPoints = []string{
	"internal/api/site",
	"internal/api/external",
	"internal/api/collect",
	"internal/mcpserver",
	"internal/jobs",
	"internal/server",
}

// mixedPackages hold handlers next to their domain module, by package, with the receiver types that
// are entry points.
var mixedPackages = map[string][]string{
	"ee/staff": {"Handlers", "ServiceHandlers", "SecurityHandler", "ServiceSecurityHandler", "Surface", "ServiceSurface"},
}

// Analyzer reports ent writes in entry-point packages.
var Analyzer = &analysis.Analyzer{
	Name:     "entrywrite",
	Doc:      "refuses writes through ent in handlers, MCP tools, jobs and event subscribers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var starters = map[string]bool{
	"Create": true, "CreateBulk": true,
	"Update": true, "UpdateOne": true, "UpdateOneID": true,
	"Delete": true, "DeleteOne": true, "DeleteOneID": true,
}

var upserts = map[string]bool{"OnConflict": true, "OnConflictColumns": true, "OnConflictColumnsWhere": true}

func isIn(path, pkg string) bool {
	return path == module+pkg || strings.HasPrefix(path, module+pkg+"/")
}

func isEntryPoint(path string) bool {
	return slices.ContainsFunc(entryPoints, func(p string) bool { return isIn(path, p) })
}

// handlerReceivers is the set of entry-point receiver types of a mixed package, or nil for a package
// that is an entry point as a whole.
func handlerReceivers(path string) (recvs []string, mixed bool) {
	for pkg, names := range mixedPackages {
		if isIn(path, pkg) {
			return names, true
		}
	}
	return nil, false
}

func run(pass *analysis.Pass) (any, error) {
	recvs, mixed := handlerReceivers(pass.Pkg.Path())
	if !mixed && !isEntryPoint(pass.Pkg.Path()) {
		return nil, nil
	}
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		call, isNode := n.(*ast.CallExpr)
		if !isNode {
			return true
		}
		if !push || strings.HasSuffix(pass.Fset.Position(call.Pos()).Filename, "_test.go") {
			return true
		}
		if mixed && !slices.Contains(recvs, enclosingReceiver(stack)) {
			return true
		}
		check(pass, call)
		return true
	})
	return nil, nil
}

func check(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (!starters[sel.Sel.Name] && !upserts[sel.Sel.Name]) {
		return
	}
	_, named, ok := entx.Method(pass.TypesInfo, sel)
	if !ok || (upserts[sel.Sel.Name] && chainStartsWrite(sel.X)) {
		return
	}
	entity := entx.Entity(named)
	pass.Reportf(call.Pos(), "%s", rule.Message(5,
		"an entry point writes through ent ("+sel.Sel.Name+" on "+entity+") and holds logic a domain module owns",
		"move the write into the domain module that owns "+entity+" and call that module from here"))
}

// chainStartsWrite reports whether the receiver chain already holds a call that starts a write, so
// that `Create().OnConflict...` is one finding and an upsert on a builder variable is another.
func chainStartsWrite(e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if starters[sel.Sel.Name] {
			return true
		}
		e = sel.X
	}
}

// enclosingReceiver is the name of the receiver type, without its pointer, of the method that holds
// the node on top of the stack, or "" outside a method.
func enclosingReceiver(stack []ast.Node) string {
	var decl *ast.FuncDecl
	for _, n := range slices.Backward(stack) {
		if d, ok := n.(*ast.FuncDecl); ok {
			decl = d
			break
		}
	}
	if decl == nil || decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	t := decl.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
