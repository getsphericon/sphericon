// Package entrywrite enforces rule 5 of docs/standards/review-rules.md for writes: a handler of any surface,
// an MCP tool, a job or an event subscriber receives input and calls a domain module; it does not write
// through ent itself. A write is a call that starts an ent mutation (Create, CreateBulk, Update,
// UpdateOne, UpdateOneID, Delete, DeleteOne, DeleteOneID) or an upsert (OnConflict, OnConflictColumns,
// OnConflictColumnsWhere) on a type of the ent package, which covers the raw client, a transaction and
// the generated scoped wrappers (`*ent.Scoped` and its per-entity wrappers). The raw-client packages
// listed in docs/architecture.md are domain modules and are not entry points, so only the entry-point
// packages below are checked; test files are not. `ee/staff` holds the handlers of the Staff console's
// Enterprise sections and their domain module in one package, so there only the methods of the handler
// types are checked (`*Module` is the domain module).
//
// The handlers of the API surfaces (`surfaces`) do not read through ent either: a call that starts a
// read (Get, GetX, a Query method, or Workspace on the scoped client) on a type of the ent package is a
// finding there, and the read goes into the module that owns the entity. The other entry points may read.
// Nor does such a handler open a transaction (a Within… method of the events bus): the module does.
package entrywrite

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/mixed"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const module = "github.com/getsphericon/sphericon/"

// eventsPath is the package of the events bus, whose Within… methods open a transaction.
const eventsPath = module + "internal/events"

// entryPoints are the packages that receive input from outside (an API surface, MCP, a queue, the
// ingest hooks) and hand it to a domain module.
var entryPoints = []string{
	"internal/api/site",
	"internal/api/external",
	"internal/api/collect",
	"internal/api/staff",
	"internal/mcpserver",
	"internal/jobs",
	"internal/server",
}

// surfaces are the entry points that read through a domain module too: the handlers of the API surfaces.
var surfaces = []string{
	"internal/api/site",
	"internal/api/external",
	"internal/api/collect",
	"internal/api/staff",
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

func isSurface(path string) bool {
	return slices.ContainsFunc(surfaces, func(p string) bool { return isIn(path, p) })
}

// startsRead reports whether a method of the ent package named name starts a read: the generated
// Get and GetX, a query (Query, QueryWithDiscarded, QueryDiscarded, an edge's QueryX) and the scoped
// client's Workspace.
func startsRead(name, named string) bool {
	return name == "Get" || name == "GetX" || strings.HasPrefix(name, "Query") ||
		(name == "Workspace" && named == "Scoped")
}

func run(pass *analysis.Pass) (any, error) {
	recvs, isMixed := mixed.Receivers(pass.Pkg.Path())
	if !isMixed && !isEntryPoint(pass.Pkg.Path()) {
		return nil, nil
	}
	reads := isSurface(pass.Pkg.Path())
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
		if isMixed && !slices.Contains(recvs, mixed.EnclosingReceiver(stack)) {
			return true
		}
		check(pass, call, reads)
		return true
	})
	return nil, nil
}

func check(pass *analysis.Pass, call *ast.CallExpr, reads bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if reads && opensTx(pass.TypesInfo, sel) {
		pass.Reportf(call.Pos(), "%s", rule.Message(5,
			"a handler opens a transaction ("+sel.Sel.Name+" on the events bus) and holds logic a domain module owns",
			"move the transaction and what runs in it into the domain module and call that module from here"))
		return
	}
	_, named, ok := entx.Method(pass.TypesInfo, sel)
	if !ok {
		return
	}
	if reads && startsRead(sel.Sel.Name, named.Obj().Name()) {
		entity := entx.Entity(named)
		if entity == "Scoped" {
			entity = "Workspace"
		}
		pass.Reportf(call.Pos(), "%s", rule.Message(5,
			"a handler reads through ent ("+sel.Sel.Name+" on "+entity+") instead of asking a domain module",
			"give the domain module that owns the read a function for it (a Get or the operation's own read) and call that from here"))
		return
	}
	if (!starters[sel.Sel.Name] && !upserts[sel.Sel.Name]) || (upserts[sel.Sel.Name] && chainStartsWrite(sel.X)) {
		return
	}
	entity := entx.Entity(named)
	pass.Reportf(call.Pos(), "%s", rule.Message(5,
		"an entry point writes through ent ("+sel.Sel.Name+" on "+entity+") and holds logic a domain module owns",
		"move the write into the domain module that owns "+entity+" and call that module from here"))
}

// opensTx reports whether the selector names a method of the events bus that opens a transaction.
func opensTx(info *types.Info, sel *ast.SelectorExpr) bool {
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Signature().Recv() == nil || fn.Pkg() == nil || fn.Pkg().Path() != eventsPath {
		return false
	}
	return strings.HasPrefix(fn.Name(), "Within")
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
