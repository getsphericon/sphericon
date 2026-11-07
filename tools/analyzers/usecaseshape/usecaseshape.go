// Package usecaseshape checks the call contract of a use case (ADR 0104, review rule 5): a
// package under internal/usecase or ee/usecase exports New and the methods of its UseCase.
package usecaseshape

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

// Analyzer reports a use case package that breaks the call contract of ADR 0104.
var Analyzer = &analysis.Analyzer{
	Name: "usecaseshape",
	Doc:  "checks the call contract of a use case package (ADR 0104)",
	Run:  run,
}

const module = "github.com/getsphericon/sphericon/"

func run(pass *analysis.Pass) (any, error) {
	if !isUseCase(pass.Pkg.Path()) {
		return nil, nil
	}
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				checkFunc(pass, fn)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if f, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func); isFunc && f.Pkg() != nil &&
				f.Pkg().Path() == "time" && f.Name() == "Now" {
				pass.Reportf(sel.Pos(), "%s", rule.Message(5, "the use case reads time.Now",
					"read the clock the wiring gives to New (app.Clock) (ADR 0104)"))
			}
			return true
		})
	}
	return nil, nil
}

// isUseCase holds for a package of either use case tier.
func isUseCase(path string) bool {
	return strings.HasPrefix(path, module+"internal/usecase/") || strings.HasPrefix(path, module+"ee/usecase/")
}

func checkFunc(pass *analysis.Pass, fn *ast.FuncDecl) {
	if !fn.Name.IsExported() {
		return
	}
	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return
	}
	name := fn.Name.Name
	if sig.Recv() == nil {
		if name != "New" {
			report(pass, fn, name+" is an exported function of a use case package",
				"make the operation a method of UseCase, a dependency a parameter of New, and move a helper to the domain module or the driver that needs it")
			return
		}
		if sig.Results().Len() != 1 || !isUseCasePtr(sig.Results().At(0).Type()) {
			report(pass, fn, "New returns something other than *UseCase",
				"return the use case alone; the wiring builds and checks a dependency that can fail and hands it in")
		}
		return
	}
	recv := receiverName(sig.Recv().Type())
	if !ast.IsExported(recv) {
		return
	}
	checkResults(pass, fn, sig)
	if recv != "UseCase" {
		if takesContext(sig) {
			report(pass, fn, name+" is a method of "+recv+", an exported type other than UseCase",
				"make the operation a method of UseCase")
		}
		return
	}
	if sig.Results().Len() > 0 && isUseCasePtr(sig.Results().At(0).Type()) {
		report(pass, fn, name+" returns *UseCase, a builder",
			"make what it sets a parameter of New")
		return
	}
	checkParams(pass, fn, sig)
}

// checkParams refuses more than two arguments after ctx and the scope, the time or the clock as
// an argument, and an io.Writer.
func checkParams(pass *analysis.Pass, fn *ast.FuncDecl, sig *types.Signature) {
	params := sig.Params()
	name := fn.Name.Name
	rest := 0
	for i := range params.Len() {
		p := params.At(i)
		switch {
		case i == 0 && isNamed(p.Type(), "context", "Context"):
			continue
		case i <= 1 && isScope(p.Type()):
			continue
		}
		rest++
		switch {
		case p.Name() == "now" && isNamed(p.Type(), "time", "Time"):
			report(pass, fn, name+" takes now",
				"read the clock the wiring gives to New (app.Clock)")
		case isClock(p.Type()):
			report(pass, fn, name+" takes a clock",
				"take the clock as a parameter of New, fed from app.Clock")
		case isNamed(p.Type(), "io", "Writer"):
			report(pass, fn, name+" takes an io.Writer",
				"return the value and let the CLI, the API or the job runner write the report")
		}
	}
	if rest > 2 {
		report(pass, fn, fmt.Sprintf("%s takes %d arguments after ctx and the scope", name, rest),
			"keep at most two required arguments and put the rest, and every optional one, into a typed "+name+"Input")
	}
}

// isScope holds for *ent.Scoped and *ent.Client.
func isScope(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	return isNamed(ptr.Elem(), module+"ent", "Scoped") || isNamed(ptr.Elem(), module+"ent", "Client")
}

// isClock holds for func() time.Time.
func isClock(t types.Type) bool {
	sig, ok := t.Underlying().(*types.Signature)
	return ok && sig.Params().Len() == 0 && sig.Results().Len() == 1 && isNamed(sig.Results().At(0).Type(), "time", "Time")
}

// checkResults refuses a tuple, a second result other than error, named results and a []error.
func checkResults(pass *analysis.Pass, fn *ast.FuncDecl, sig *types.Signature) {
	res := sig.Results()
	name := fn.Name.Name
	const fix = "return one value, or put two or more into a named result struct"
	switch {
	case res.Len() > 2:
		report(pass, fn, fmt.Sprintf("%s returns %d values", name, res.Len()), fix)
	case res.Len() == 2 && !isError(res.At(1).Type()):
		report(pass, fn, name+" returns a second value that is not error", fix)
	case res.Len() > 0 && res.At(0).Name() != "":
		report(pass, fn, name+" has named results", "drop the names; "+fix)
	}
	for i := range res.Len() {
		if s, ok := res.At(i).Type().(*types.Slice); ok && isError(s.Elem()) {
			report(pass, fn, name+" returns []error",
				"return what became of each item as batch.Result (internal/kernel/batch)")
		}
	}
}

func isError(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

func report(pass *analysis.Pass, fn *ast.FuncDecl, what, fix string) {
	pass.Reportf(fn.Name.Pos(), "%s", rule.Message(5, what, fix+" (ADR 0104)"))
}

// isUseCasePtr holds for *UseCase of the package under analysis.
func isUseCasePtr(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	return ok && named.Obj().Name() == "UseCase"
}

func receiverName(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

func takesContext(sig *types.Signature) bool {
	for i := range sig.Params().Len() {
		if isNamed(sig.Params().At(i).Type(), "context", "Context") {
			return true
		}
	}
	return false
}

// isNamed holds for the named type pkg.name (pkg is the import path).
func isNamed(t types.Type, pkg, name string) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == pkg && named.Obj().Name() == name
}
