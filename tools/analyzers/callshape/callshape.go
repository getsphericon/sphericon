// Package callshape checks the call contract of an operation (review rule 5): the use case tier's
// (ADR 0104) and the domain tier's (ADR 0167), which share their rules. The use case tier adds its
// own: it exports New and the methods of its UseCase, and reads no time.Now. That neither tier
// writes to an io.Writer is nowriter's.
package callshape

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// exemptList is the list of domain packages exempt while they move to the contract (ADR 0167): one
// path relative to the repository per line, # for a comment. It may only shrink: mise run
// check:callshape-exempt fails when it gains an entry, and ExemptPackages' test when an entry names
// no package of the domain tier.
//
//go:embed exempt.txt
var exemptList string

// Analyzer reports a use case or domain package that breaks the call contract.
var Analyzer = New(ExemptPackages())

// ExemptPackages is the exemption list, in its order.
func ExemptPackages() []string {
	var paths []string
	for line := range strings.Lines(exemptList) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			paths = append(paths, line)
		}
	}
	return paths
}

// New builds the analyzer with an exemption list (paths relative to the repository); Analyzer is the
// one built with the list of exempt.txt.
func New(exempt []string) *analysis.Analyzer {
	set := make(map[string]bool, len(exempt))
	for _, p := range exempt {
		set[tier.Module+p] = true
	}
	return &analysis.Analyzer{
		Name: "callshape",
		Doc:  "checks the call contract of a use case or domain package (ADR 0104, ADR 0167)",
		Run:  func(pass *analysis.Pass) (any, error) { return run(pass, set) },
	}
}

// checker holds what one package's run found; the findings are reported at the end.
type checker struct {
	pass     *analysis.Pass
	useCase  bool
	adr      string
	findings []finding
}

type finding struct {
	pos token.Pos
	msg string
}

func run(pass *analysis.Pass, exempt map[string]bool) (any, error) {
	c := &checker{pass: pass}
	path := pass.Pkg.Path()
	switch {
	case tier.IsUseCase(path):
		c.useCase, c.adr = true, "ADR 0104"
	case tier.IsDomain(path):
		c.adr = "ADR 0167"
	default:
		return nil, nil
	}
	for _, file := range pass.Files {
		if analysisx.InTestFile(c.pass, file.Pos()) {
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				c.checkFunc(fn)
			}
		}
		if c.useCase {
			c.checkNow(file)
		}
	}
	if !c.useCase {
		c.checkConstructors()
	}
	if exempt[path] {
		if len(c.findings) == 0 {
			c.reportStale(path)
		}
		return nil, nil
	}
	for _, f := range c.findings {
		pass.Reportf(f.pos, "%s", f.msg)
	}
	return nil, nil
}

// reportStale reports an exempt package that keeps the contract, at its first non-test file's
// package clause, so that the list holds only the packages still to move.
func (c *checker) reportStale(path string) {
	for _, file := range c.pass.Files {
		if analysisx.InTestFile(c.pass, file.Pos()) {
			continue
		}
		rule.Report(c.pass, file.Name.Pos(), 5,
			strings.TrimPrefix(path, tier.Module)+" is on the exemption list but keeps the call contract",
			"remove it from tools/analyzers/callshape/exempt.txt (ADR 0167)")
		return
	}
}

// checkNow refuses a read of time.Now in a use case.
func (c *checker) checkNow(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if f, isFunc := c.pass.TypesInfo.Uses[sel.Sel].(*types.Func); isFunc && f.Pkg() != nil &&
			f.Pkg().Path() == "time" && f.Name() == "Now" {
			c.add(sel.Pos(), "the use case reads time.Now", "read the clock the wiring gives to New (app.Clock)")
		}
		return true
	})
}

func (c *checker) checkFunc(fn *ast.FuncDecl) {
	if !fn.Name.IsExported() {
		return
	}
	obj, ok := c.pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return
	}
	sig := obj.Signature()
	name := fn.Name.Name
	if sig.Recv() == nil {
		c.checkPackageFunc(fn, sig)
		return
	}
	recv := receiverName(sig.Recv().Type())
	if !ast.IsExported(recv) {
		return
	}
	if !c.useCase {
		if analysisx.TakesContext(sig) {
			c.checkResults(fn, sig)
			c.checkParams(fn, sig)
		}
		return
	}
	c.checkResults(fn, sig)
	if recv != "UseCase" {
		if analysisx.TakesContext(sig) {
			c.report(fn, name+" is a method of "+recv+", an exported type other than UseCase",
				"make the operation a method of UseCase")
		}
		return
	}
	if sig.Results().Len() > 0 && c.isUseCasePtr(sig.Results().At(0).Type()) {
		c.report(fn, name+" returns *UseCase, a builder",
			"make what it sets a parameter of New")
		return
	}
	c.checkParams(fn, sig)
}

// checkPackageFunc refuses an exported function that takes a context; a use case package exports
// no function but New, which returns *UseCase.
func (c *checker) checkPackageFunc(fn *ast.FuncDecl, sig *types.Signature) {
	name := fn.Name.Name
	if isConstructor(name) && takesOptions(sig) {
		c.report(fn, name+" takes functional options",
			"make each dependency a parameter of "+name+"; the wiring passes every one, a test passes its fake")
	}
	if !c.useCase {
		if !isConstructor(name) && analysisx.TakesContext(sig) {
			c.report(fn, name+" is an exported function that takes a context",
				"make the operation a method of a type its constructor New… builds from the dependencies")
		}
		return
	}
	if name != "New" {
		c.report(fn, name+" is an exported function of a use case package",
			"make the operation a method of UseCase, a dependency a parameter of New, and move a helper to the domain module or the driver that needs it")
		return
	}
	if sig.Results().Len() != 1 || !c.isUseCasePtr(sig.Results().At(0).Type()) {
		c.report(fn, "New returns something other than *UseCase",
			"return the use case alone; the wiring builds and checks a dependency that can fail and hands it in")
	}
}

// checkParams refuses more than two arguments after ctx and the scope, and the time or the clock as
// an argument.
func (c *checker) checkParams(fn *ast.FuncDecl, sig *types.Signature) {
	params := sig.Params()
	name := fn.Name.Name
	rest := 0
	for i := range params.Len() {
		p := params.At(i)
		switch {
		case i == 0 && analysisx.IsNamed(p.Type(), "context", "Context"):
			continue
		case i <= 1 && isScope(p.Type()):
			continue
		}
		rest++
		switch {
		case isClient(p.Type()):
			c.report(fn, name+" takes a client ("+types.TypeString(p.Type(), byName)+")",
				"take the client as a parameter of the constructor, which the wiring feeds")
		case p.Name() == "now" && analysisx.IsNamed(p.Type(), "time", "Time"):
			c.report(fn, name+" takes now",
				"read the clock the wiring gives to New (app.Clock)")
		case isClock(p.Type()):
			c.report(fn, name+" takes a clock",
				"take the clock as a parameter of New, fed from app.Clock")
		}
	}
	if rest > 2 {
		c.report(fn, fmt.Sprintf("%s takes %d arguments after ctx and the scope", name, rest),
			"keep at most two required arguments and put the rest, and every optional one, into a typed "+name+"Input")
	}
}

// checkConstructors refuses an exported type with operations (exported methods that take a context)
// that no New or New<Type> builds as *Type alone: the constructor takes every dependency and the
// clock, and the wiring builds and checks what can fail before it hands it in. The use case tier
// needs no such check: only UseCase may have operations there, and New must return *UseCase.
func (c *checker) checkConstructors() {
	scope := c.pass.Pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() || tn.IsAlias() || analysisx.InTestFile(c.pass, tn.Pos()) || !hasOperations(tn.Type()) {
			continue
		}
		var returnsMore *types.Func
		returnsAlone := false
		for _, ctor := range []string{"New", "New" + name} {
			f, ok := scope.Lookup(ctor).(*types.Func)
			if !ok {
				continue
			}
			res := f.Signature().Results()
			switch {
			case res.Len() == 1 && isPtrTo(res.At(0).Type(), tn):
				returnsAlone = true
			case res.Len() > 1 && isPtrTo(res.At(0).Type(), tn):
				returnsMore = f
			}
		}
		switch {
		case returnsAlone:
		case returnsMore != nil:
			c.add(returnsMore.Pos(), returnsMore.Name()+" returns more than *"+name,
				"return *"+name+" alone; the wiring builds and checks a dependency that can fail and hands it in")
		default:
			c.add(tn.Pos(), name+" has operations but no constructor New or New"+name+" that returns *"+name,
				"add the constructor, taking every dependency and the clock")
		}
	}
}

// hasOperations holds for a type whose pointer has an exported method that takes a context.
func hasOperations(t types.Type) bool { return hasContextMethod(types.NewPointer(t), true) }

func methodTakesContext(m *types.Selection) bool {
	sig, ok := m.Type().(*types.Signature)
	return ok && analysisx.TakesContext(sig)
}

// isPtrTo holds for *T, T being the named type tn.
func isPtrTo(t types.Type, tn *types.TypeName) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	return ok && named.Obj() == tn
}

// takesOptions holds for a variadic last parameter of a function type: functional options.
func takesOptions(sig *types.Signature) bool {
	if !sig.Variadic() {
		return false
	}
	last, ok := sig.Params().At(sig.Params().Len() - 1).Type().(*types.Slice)
	if !ok {
		return false
	}
	_, isFunc := last.Elem().Underlying().(*types.Signature)
	return isFunc
}

// isConstructor holds for New and New<Name>: a constructor, not an operation.
func isConstructor(name string) bool {
	rest, ok := strings.CutPrefix(name, "New")
	return ok && (rest == "" || ast.IsExported(rest))
}

// isScope holds for *ent.Scoped, *ent.WorkspaceLocked (the scope that stands for the Workspace lock)
// and *ent.Client.
func isScope(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	return analysisx.IsNamed(ptr.Elem(), entx.Path, "Scoped") || analysisx.IsNamed(ptr.Elem(), entx.Path, "WorkspaceLocked") ||
		analysisx.IsNamed(ptr.Elem(), entx.Path, "Client")
}

// isClient holds for a dependency that does I/O: a type with a method that takes a context, an
// *http.Client, or an *ent.Client outside the scope's slot. What makes it a client is what it can do,
// not the package that declares it, so a value such as an Actor or a message is an argument. A type
// of the generated ent package is an entity or a scope: an entity carries the raw client, but a write
// through it is rule 18's (entityupdate).
func isClient(t types.Type) bool {
	elem := t
	if ptr, ok := t.(*types.Pointer); ok {
		elem = ptr.Elem()
	}
	if analysisx.IsNamed(elem, "net/http", "Client") || analysisx.IsNamed(elem, entx.Path, "Client") {
		return true
	}
	if _, ok := entx.Named(elem); ok {
		return false
	}
	if hasContextMethod(t, false) {
		return true
	}
	_, isPtr := t.(*types.Pointer)
	return !isPtr && !types.IsInterface(t) && hasContextMethod(types.NewPointer(t), false)
}

// hasContextMethod holds for a type with a method that takes a context, an exported one if exported.
func hasContextMethod(t types.Type, exported bool) bool {
	for m := range types.NewMethodSet(t).Methods() {
		if (!exported || m.Obj().Exported()) && methodTakesContext(m) {
			return true
		}
	}
	return false
}

// byName writes a type's package by its name, as the code that uses it does.
func byName(p *types.Package) string { return p.Name() }

// isClock holds for func() time.Time.
func isClock(t types.Type) bool {
	sig, ok := t.Underlying().(*types.Signature)
	return ok && sig.Params().Len() == 0 && sig.Results().Len() == 1 && analysisx.IsNamed(sig.Results().At(0).Type(), "time", "Time")
}

// checkResults refuses a tuple, a second result other than error, named results and a []error.
func (c *checker) checkResults(fn *ast.FuncDecl, sig *types.Signature) {
	res := sig.Results()
	name := fn.Name.Name
	const fix = "return one value, or put two or more into a named result struct"
	switch {
	case res.Len() > 2:
		c.report(fn, fmt.Sprintf("%s returns %d values", name, res.Len()), fix)
	case res.Len() == 2 && !isError(res.At(1).Type()):
		c.report(fn, name+" returns a second value that is not error", fix)
	case res.Len() > 0 && res.At(0).Name() != "":
		c.report(fn, name+" has named results", "drop the names; "+fix)
	}
	for v := range res.Variables() {
		if s, ok := v.Type().(*types.Slice); ok && isError(s.Elem()) {
			c.report(fn, name+" returns []error",
				"return what became of each item as batch.Result (internal/kernel/batch)")
		}
	}
}

func isError(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

func (c *checker) report(fn *ast.FuncDecl, what, fix string) {
	c.add(fn.Name.Pos(), what, fix)
}

func (c *checker) add(pos token.Pos, what, fix string) {
	c.findings = append(c.findings, finding{pos, rule.Message(5, what, fix+" ("+c.adr+")")})
}

// isUseCasePtr holds for *UseCase of the package under analysis.
func (c *checker) isUseCasePtr(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	return ok && named.Obj().Pkg() == c.pass.Pkg && named.Obj().Name() == "UseCase"
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
