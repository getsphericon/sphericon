// Package statemoment enforces rule 7 of docs/agents/review-rules.md for a state kept as a moment
// (`verified_at`, `disabled_at`, `suspended_at`): a field a background job moves on its own races with
// users, so the move is a compare-and-swap, an update that matches only while the row is still in the
// state it leaves. A write of such a field through an ent update builder must carry a `Where` on that
// builder (in the same chain, or called on the same variable in the same function). Creating a row in
// its first state is not a move, and a column whose write is the owner's own act (clearing a webhook's
// `disabled_at` on re-enable) is not listed. A state that is a machine (`status`, `tracking_state`) is
// guarded by the ast-grep rule `no-status-write-outside-lifecycle`.
package statemoment

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const entPath = "github.com/getsphericon/sphericon/ent"

// moments lists, by entity, the builder methods that move a state moment.
var moments = map[string]map[string]bool{
	"SendingDomain":   {"SetVerifiedAt": true, "SetVerificationLostAt": true},
	"WebhookEndpoint": {"SetDisabledAt": true},
	"Workspace":       {"SetSuspendedAt": true},
	"JoinDomain":      {"SetVerifiedAt": true, "ClearVerifiedAt": true},
}

// builderSuffixes name the update builders of the raw client and of the scoped wrappers, longest first.
var builderSuffixes = []string{"ScopedUpdateOne", "ScopedUpdate", "UpdateOne", "Update"}

const fix = "add .Where(...) on the state the move leaves, so the write lands only while the row is still in it, and handle ent.IsNotFound as a lost race"

// Analyzer reports unconditional writes of a state moment.
var Analyzer = &analysis.Analyzer{
	Name:     "statemoment",
	Doc:      "refuses an update of a state moment that is not a compare-and-swap",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		decl := n.(*ast.FuncDecl)
		if decl.Body == nil || strings.HasSuffix(pass.Fset.Position(decl.Pos()).Filename, "_test.go") {
			return
		}
		fenced := fencedRoots(pass, decl.Body)
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !movesMoment(pass, sel) || chainHasWhere(sel.X) {
				return true
			}
			if root := rootObject(pass, sel.X); root != nil && fenced[root] {
				return true
			}
			pass.Reportf(call.Pos(), "%s", rule.Message(7,
				sel.Sel.Name+" writes a state moment without a condition on the state it leaves", fix))
			return true
		})
	})
	return nil, nil
}

// movesMoment reports whether sel is a listed method of an ent update builder.
func movesMoment(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != entPath || fn.Signature().Recv() == nil {
		return false
	}
	recv := fn.Signature().Recv().Type()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	named, ok := recv.(*types.Named)
	if !ok {
		return false
	}
	name := named.Obj().Name()
	for _, suffix := range builderSuffixes {
		if entity, ok := strings.CutSuffix(name, suffix); ok {
			return moments[entity][sel.Sel.Name]
		}
	}
	return false
}

// fencedRoots collects the variables a Where is called on anywhere in the function body.
func fencedRoots(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]bool {
	fenced := map[types.Object]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Where" {
			if root := rootObject(pass, sel.X); root != nil {
				fenced[root] = true
			}
		}
		return true
	})
	return fenced
}

// chainHasWhere reports whether a call named Where sits in the receiver chain.
func chainHasWhere(e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == "Where" {
			return true
		}
		e = sel.X
	}
}

// rootObject is the variable a call chain starts from, or nil when it starts from anything else.
func rootObject(pass *analysis.Pass, e ast.Expr) types.Object {
	for {
		switch x := e.(type) {
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return nil
			}
			e = sel.X
		case *ast.Ident:
			return pass.TypesInfo.Uses[x]
		default:
			return nil
		}
	}
}
