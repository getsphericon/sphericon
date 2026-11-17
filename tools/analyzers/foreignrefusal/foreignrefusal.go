// Package foreignrefusal refuses a reference to another package's refusal sentinel outside
// errors.Is (ADR 0107, review rule 5). A refusal sentinel is a package-level variable of type
// *validation.RefusalError: the module that declares it is the one that refuses with it. Another
// package that returns it, wraps it or re-exports it refuses on the owner's behalf with the
// owner's own words, which is a rule of the owner's rows held by a caller; matching it with
// errors.Is is how a caller reads the owner's answer, and stays allowed. The owner's subpackages
// (its `<module>unchecked` operations) speak for it. Tests are not read.
package foreignrefusal

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// validationPath is the package of the refusal type.
const validationPath = tier.Module + "internal/kernel/validation"

// Analyzer reports a use of another package's refusal sentinel that is not an argument of errors.Is.
var Analyzer = &analysis.Analyzer{
	Name:     "foreignrefusal",
	Doc:      "refuses a reference to another package's refusal sentinel outside errors.Is (ADR 0107)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
	}
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel, isSel := n.(*ast.SelectorExpr)
		if !isSel {
			return true
		}
		v, isVar := pass.TypesInfo.Uses[sel.Sel].(*types.Var)
		if !isVar || v.Pkg() == nil || ownedBy(pass.Pkg, v.Pkg()) || v.Parent() != v.Pkg().Scope() || !isRefusal(v.Type()) {
			return true
		}
		if analysisx.InTestFile(pass, sel.Pos()) || insideErrorsIs(pass, stack) {
			return true
		}
		rule.Report(pass, sel.Pos(), 5,
			v.Pkg().Name()+"."+v.Name()+" is "+v.Pkg().Name()+"'s refusal, used here outside errors.Is",
			"call the owner's function that refuses, or match its answer with errors.Is")
		return true
	})
	return nil, nil
}

// ownedBy reports whether pkg is owner or one of its subpackages (an owner's `<module>unchecked`
// operations are the owner's own, review rule 5), which refuse in the owner's words.
func ownedBy(pkg, owner *types.Package) bool {
	return pkg == owner || strings.HasPrefix(pkg.Path(), owner.Path()+"/")
}

// isRefusal reports whether t is *validation.RefusalError.
func isRefusal(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	return ok && analysisx.IsNamed(ptr.Elem(), validationPath, "RefusalError")
}

// insideErrorsIs reports whether the innermost call around the selector (the last in stack) is a
// call of errors.Is that takes it as an argument.
func insideErrorsIs(pass *analysis.Pass, stack []ast.Node) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		call, ok := stack[i].(*ast.CallExpr)
		if !ok {
			continue
		}
		return analysisx.Calls(pass.TypesInfo, call, "errors", "Is")
	}
	return false
}
