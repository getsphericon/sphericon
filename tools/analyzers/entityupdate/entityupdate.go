// Package entityupdate enforces rule 18 of docs/standards/review-rules.md for a loaded entity: it
// refuses its update (ADR 0017). An entity loaded through the
// scoped client still carries the raw client, so `tag.Update()` writes past the Workspace scope; the
// update goes by id through the scoped client instead (`s.Tag().UpdateOneID(id)`). What is an entity
// and which of its methods starts a write is the generator's to say: the manifest of roles of the ent
// package (ent/template/roles.tmpl, read through entx). Every package of the module is read, test code
// included (ent/schema too), but the generated ent package itself.
package entityupdate

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
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
	if !strings.HasPrefix(path, tier.Module) || path == entx.Path {
		return nil, nil
	}
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
	}
	for sel := range inspector.All[*ast.SelectorExpr](insp) {
		_, named, ok := entx.Method(pass.TypesInfo, sel)
		if !ok || entx.RoleOf(named, sel.Sel.Name) != entx.Start || entx.KindOf(named) != entx.Entity {
			continue
		}
		entity := named.Obj().Name()
		rule.Report(pass, sel.Sel.Pos(), 18,
			sel.Sel.Name+" on a loaded "+entity+" writes through the raw client it carries, past the Workspace scope (ADR 0017)",
			"update it by id through the scoped client: s."+entity+"().UpdateOneID(id)")
	}
	return nil, nil
}
