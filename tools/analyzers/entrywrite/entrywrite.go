// Package entrywrite enforces rule 5 of docs/agents/review-rules.md for writes: a handler of any surface,
// an MCP tool, a job or an event subscriber receives input and calls a domain module; it does not write
// through ent itself. A write is a call that starts an ent mutation (Create, CreateBulk, Update,
// UpdateOne, UpdateOneID, Delete, DeleteOne, DeleteOneID) on a type of the ent package, which covers the
// raw client, a transaction and the generated scoped wrappers (`*ent.Scoped` and its per-entity
// wrappers). Reads are allowed. The raw-client packages listed in AGENTS.md are domain modules and are
// not entry points, so only the entry-point packages below are checked; test files are not.
package entrywrite

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const module = "github.com/getsphericon/sphericon/"

const entPath = module + "ent"

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

const fix = "move the write into the domain module that owns this entity and call that module from here"

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

func isEntryPoint(path string) bool {
	for _, p := range entryPoints {
		if path == module+p || strings.HasPrefix(path, module+p+"/") {
			return true
		}
	}
	return false
}

func run(pass *analysis.Pass) (any, error) {
	if !isEntryPoint(pass.Pkg.Path()) {
		return nil, nil
	}
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !starters[sel.Sel.Name] {
			return
		}
		if strings.HasSuffix(pass.Fset.Position(call.Pos()).Filename, "_test.go") {
			return
		}
		fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != entPath || fn.Signature().Recv() == nil {
			return
		}
		pass.Reportf(call.Pos(), "%s", rule.Message(5,
			"an entry point writes through ent ("+sel.Sel.Name+") and holds logic a domain module owns", fix))
	})
	return nil, nil
}
