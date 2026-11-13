// Package entityupdate refuses the update of a loaded entity (ADR 0017). An entity loaded through the
// scoped client still carries the raw client, so `tag.Update()` writes past the Workspace scope; the
// update goes by id through the scoped client instead (`s.Tag().UpdateOneID(id)`). What is an entity
// and which of its methods starts a write is the generator's to say: the manifest of roles of the ent
// package (ent/template/roles.tmpl, read through entx). Every package of the module is read, test code
// included, but the generated ent package itself.
package entityupdate

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// Analyzer reports a write started on a loaded entity.
var Analyzer = &analysis.Analyzer{
	Name:     "entityupdate",
	Doc:      "refuses a write started on a loaded ent entity (entity.Update()), which bypasses the Workspace scope (ADR 0017)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	path := pass.Pkg.Path()
	if !strings.HasPrefix(path, tier.Module) || path == entx.Path || strings.HasPrefix(path, entx.Path+"/") {
		return nil, nil
	}
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	insp.Preorder([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node) {
		sel, isSel := n.(*ast.SelectorExpr)
		if !isSel {
			return
		}
		_, named, ok := entx.Method(pass.TypesInfo, sel)
		if !ok || entx.KindOf(named) != entx.Entity || entx.RoleOf(named, sel.Sel.Name) != entx.Start {
			return
		}
		entity := named.Obj().Name()
		pass.Reportf(sel.Sel.Pos(), "%s", rule.Message(0,
			sel.Sel.Name+" on a loaded "+entity+" writes through the raw client it carries, past the Workspace scope (ADR 0017)",
			"update it by id through the scoped client: s."+entity+"().UpdateOneID(id)"))
	})
	return nil, nil
}
