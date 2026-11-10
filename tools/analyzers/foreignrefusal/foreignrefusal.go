// Package foreignrefusal refuses a reference to another package's refusal sentinel outside
// errors.Is (ADR 0107, review rule 5). A refusal sentinel is a package-level variable of type
// *validation.RefusalError: the module that declares it is the one that refuses with it. Another
// package that returns it, wraps it or re-exports it refuses on the owner's behalf with the
// owner's own words, which is a rule of the owner's rows held by a caller; matching it with
// errors.Is is how a caller reads the owner's answer, and stays allowed. The owner's subpackages
// (its `<module>unchecked` operations) speak for it. Tests are not read.
package foreignrefusal

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

// validationPath is the import path the refusal type lives under; a suffix, so the analyzer's
// testdata declares its own.
const validationPath = "internal/kernel/validation"

// Analyzer reports a use of another package's refusal sentinel that is not an argument of errors.Is.
var Analyzer = &analysis.Analyzer{
	Name:     "foreignrefusal",
	Doc:      "refuses a reference to another package's refusal sentinel outside errors.Is (ADR 0107)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel, isSel := n.(*ast.SelectorExpr)
		if !isSel || isTest(pass, sel) {
			return true
		}
		v, isVar := pass.TypesInfo.Uses[sel.Sel].(*types.Var)
		if !isVar || v.Pkg() == nil || ownedBy(pass.Pkg, v.Pkg()) || v.Parent() != v.Pkg().Scope() || !isRefusal(v.Type()) {
			return true
		}
		if insideErrorsIs(pass, stack) {
			return true
		}
		pass.Reportf(sel.Pos(), "%s", rule.Message(5,
			v.Pkg().Name()+"."+v.Name()+" is "+v.Pkg().Name()+"'s refusal, used here outside errors.Is",
			"call the owner's function that refuses, or match its answer with errors.Is"))
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
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Name() != "RefusalError" || named.Obj().Pkg() == nil {
		return false
	}
	path := named.Obj().Pkg().Path()
	return path == validationPath || strings.HasSuffix(path, "/"+validationPath)
}

// insideErrorsIs reports whether the innermost call around the selector (the last in stack) is a
// call of errors.Is that takes it as an argument.
func insideErrorsIs(pass *analysis.Pass, stack []ast.Node) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		call, ok := stack[i].(*ast.CallExpr)
		if !ok {
			continue
		}
		fun, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		f, ok := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return ok && f.Pkg() != nil && f.Pkg().Path() == "errors" && f.Name() == "Is"
	}
	return false
}

// isTest reports whether n is in a test file.
func isTest(pass *analysis.Pass, n ast.Node) bool {
	return strings.HasSuffix(pass.Fset.Position(n.Pos()).Filename, "_test.go")
}
