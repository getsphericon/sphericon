// Package handmapper enforces rule 1 of docs/standards/review-rules.md in both directions of the API
// contract, each of which goverter generates.
//
// Output: an ent entity becomes an API resource only through the generated goverter converter. A
// function that takes an ent entity and returns a type of a generated API package (gen/...) is a
// hand-written mapper, unless it is a helper the package registers with `goverter:extend` or a
// `goverter:map ... | helper` line, or a function that maps through a converter method and only
// attaches the fields the converter ignores. Whatever the function takes, a composite literal of a
// generated API type that copies a field of an ent entity (directly, through a conversion or a helper
// call, or through a loaded edge) is a hand-written mapping too, in the API packages: the handlers
// of every surface (ee/api/staff too), and internal/server, which renders the refusals. A handler
// that loops over the entities a module returned takes no entity, so only the literal shows it. A
// value handed to a converter method is mapped by goverter, and a fact decided from a field (a
// comparison, a negation) copies none.
//
// Input: a request becomes a module's input only through the generated goverter Inputs of its
// surface (resources/inputs.go). A struct literal of a module input type in a handler package (/site,
// /api, /staff and the Enterprise /staff and /service in ee/api/staff) is a hand-written mapping. A field the request does not carry (the actor) is a
// `goverter:ignore` with its reason, and the handler assigns it after the call; an assignment is not
// a literal and is not reported.
//
// Test files are not API code and are skipped, and so are generated files.
package handmapper

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// Analyzer reports a hand-written ent-to-resource mapper and a hand-built module input in a handler.
var Analyzer = &analysis.Analyzer{
	Name:     "handmapper",
	Doc:      "refuses a hand-written mapping between the API types and ent entities or module inputs outside goverter",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var (
	extendLine = regexp.MustCompile(`goverter:extend\s+(.+)`)
	// pipeLine is a mapping through a named function: `goverter:map . Steps | automationSteps`.
	pipeLine = regexp.MustCompile(`goverter:\w+\s.*\|\s*(\w+)\s*$`)
)

func run(pass *analysis.Pass) (any, error) {
	registered := registeredHelpers(pass)
	skipped := skippedFiles(pass)
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
	}
	via := viaConverter(pass, insp)
	if isOneOf(pass.Pkg.Path(), handlerPackages) {
		reportInputLiterals(pass, insp, skipped)
	}
	reported := map[*ast.FuncDecl]bool{}
	for fn := range inspector.All[*ast.FuncDecl](insp) {
		if skipped(fn.Pos()) || registered[fn.Name.Name] {
			continue
		}
		obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
		if !ok {
			continue
		}
		sig := obj.Signature()
		if !anyType(sig.Results(), isResource) || !anyType(sig.Params(), isEntity) {
			continue
		}
		if via[obj] {
			continue
		}
		reported[fn] = true
		rule.Report(pass, fn.Name.Pos(), 1,
			fn.Name.Name+" maps an ent entity to an API resource by hand",
			"add a method to the surface's goverter Converter (or register a goverter:extend helper) and call it")
	}
	if isOneOf(pass.Pkg.Path(), literalPackages) {
		reportEntityLiterals(pass, insp, skipped, func(fn *ast.FuncDecl) bool {
			return reported[fn] || registered[fn.Name.Name]
		}, via)
	}
	return nil, nil
}

// literalPackages are the packages that build API types from what a module returned: the handlers
// of every surface (/collect too, which takes no module input) and internal/server, which renders
// the refusals with the contract's problem types. The resources subpackages are the goverter homes,
// whose registered helpers read entity fields by design.
var literalPackages = append([]string{"internal/server", "internal/api/collect"}, handlerPackages...)

// isOneOf holds for the package at path when it is one of the module's packages rels, not one below it.
func isOneOf(path string, rels []string) bool {
	rel, ok := strings.CutPrefix(path, tier.Module)
	return ok && slices.Contains(rels, rel)
}

// reportEntityLiterals reports each composite literal of a generated API type that reads a field of an
// ent entity in one of its elements: a resource mapped by hand, whatever the enclosing function takes
// (a handler that loops over the entities a module returned takes none). A value handed to a converter
// method, or to a function of the package that maps through one, is not read by hand; a function
// literal is a function of its own (its literals are visited on their own), and so is a nested literal
// of an API type. A function the function-level check already reported (skip) is not reported twice.
func reportEntityLiterals(pass *analysis.Pass, insp *inspector.Inspector, skipped func(token.Pos) bool, skip func(*ast.FuncDecl) bool, via map[types.Object]bool) {
	insp.WithStack([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !push || skipped(lit.Pos()) {
			return true
		}
		if fn := enclosingFunc(stack); fn != nil && skip(fn) {
			return true
		}
		named, ok := types.Unalias(pass.TypesInfo.TypeOf(lit)).(*types.Named)
		if !ok || !isResource(named) {
			return true
		}
		entity := entityFieldRead(pass, lit, via)
		if entity == nil {
			return true
		}
		rule.Report(pass, lit.Pos(), 1,
			qualified(named)+" is built by hand from the fields of "+qualified(entity),
			"add a method to the surface's goverter Converter (a list method for a slice of entities; a value it cannot see is a `goverter:map ... | helper`) and call it")
		return true
	})
}

// decision are the operators whose result is a fact decided from their operands (a comparison, a
// logical operator), not a copy of them: `PasswordSet: u.PasswordHash != ""` maps no field.
var decision = map[token.Token]bool{
	token.EQL: true, token.NEQ: true, token.LSS: true, token.LEQ: true, token.GTR: true, token.GEQ: true,
	token.LAND: true, token.LOR: true,
}

// entityFieldRead is the entity whose field an element of lit copies, or nil: a field read directly or
// through a conversion or a helper call, but not one a comparison or a negation decides a fact from. Of
// a chain through a loaded edge (inv.Edges.Workspace.Name) it is the entity the chain starts from.
func entityFieldRead(pass *analysis.Pass, lit *ast.CompositeLit, via map[types.Object]bool) *types.Named {
	var found *types.Named
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}
		ast.Inspect(elt, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CompositeLit:
				named, ok := types.Unalias(pass.TypesInfo.TypeOf(n)).(*types.Named)
				return !ok || !isResource(named)
			case *ast.CallExpr:
				return !mapsThroughConverter(pass, n, via)
			case *ast.BinaryExpr:
				return !decision[n.Op]
			case *ast.UnaryExpr:
				return n.Op != token.NOT
			case *ast.SelectorExpr:
				if s := pass.TypesInfo.Selections[n]; s != nil && s.Kind() == types.FieldVal {
					if named, ok := deref(s.Recv()).(*types.Named); ok && isEntity(named) {
						found = named
					}
				}
			}
			return true
		})
	}
	return found
}

// mapsThroughConverter is a call of a converter method, or of a function of the package that maps
// through one (viaConverter).
func mapsThroughConverter(pass *analysis.Pass, call *ast.CallExpr, via map[types.Object]bool) bool {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return via[pass.TypesInfo.Uses[f]]
	case *ast.SelectorExpr:
		if s := pass.TypesInfo.Selections[f]; s != nil && isConverter(s.Recv()) {
			return true
		}
		return via[pass.TypesInfo.Uses[f.Sel]]
	}
	return false
}

func enclosingFunc(stack []ast.Node) *ast.FuncDecl {
	for _, n := range slices.Backward(stack) {
		if d, ok := n.(*ast.FuncDecl); ok {
			return d
		}
	}
	return nil
}

func deref(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

func qualified(n *types.Named) string { return n.Obj().Pkg().Name() + "." + n.Obj().Name() }

// handlerPackages are the packages of the /site, /api, core /staff and Enterprise /staff and /service
// handlers, which hand a module what the request says. Their subpackages are not handlers: resources
// holds the goverter Inputs and its extend helpers, the one place a module input is built by hand (a
// oneOf is beyond goverter).
var handlerPackages = []string{"internal/api/site", "internal/api/external", "internal/api/staff", "ee/api/staff"}

// reportInputLiterals reports each struct literal of a module input type in a handler package.
func reportInputLiterals(pass *analysis.Pass, insp *inspector.Inspector, skipped func(token.Pos) bool) {
	for lit := range inspector.All[*ast.CompositeLit](insp) {
		if skipped(lit.Pos()) {
			continue
		}
		named, ok := types.Unalias(pass.TypesInfo.TypeOf(lit)).(*types.Named)
		if !ok || !isModuleInput(pass.Pkg, named) {
			continue
		}
		rule.Report(pass, lit.Pos(), 1,
			qualified(named)+" is built by hand from the request",
			"add a method to the surface's goverter Inputs (resources/inputs.go), call it, and assign after it only the fields it ignores")
	}
}

// isModuleInput is a module's input: a struct type named ...Input declared in a package of the domain
// or the use case tier (tier.IsCore), of either edition. The name is the modules' own convention for
// what an operation takes (automations.CreateInput, apitokens.Input, eventrecording.Input), so a new
// module is covered the day it is written, where a list of types would go stale. A type of a
// generated API package (siteapi.SiteCreateAutomationInput) is the request itself, and a type the
// handler package declares is its own.
func isModuleInput(self *types.Package, n *types.Named) bool {
	pkg := n.Obj().Pkg()
	if pkg == nil || pkg == self || !tier.IsCore(pkg.Path()) || !strings.HasSuffix(n.Obj().Name(), "Input") {
		return false
	}
	_, isStruct := n.Underlying().(*types.Struct)
	return isStruct
}

// skippedFiles tells whether a position stands in a file that is not API code: a test file or a
// generated one. Each file of the pass is classified once.
func skippedFiles(pass *analysis.Pass) func(token.Pos) bool {
	skipped := make(map[*token.File]bool, len(pass.Files))
	for _, f := range pass.Files {
		tf := pass.Fset.File(f.FileStart)
		skipped[tf] = strings.HasSuffix(tf.Name(), "_test.go") || ast.IsGenerated(f)
	}
	return func(pos token.Pos) bool { return skipped[pass.Fset.File(pos)] }
}

// viaConverter is the set of functions of the package that map through a goverter converter: they
// call a method of a type named ...Converter or ...ConverterImpl, or call such a function of the
// package (a wrapper that loads what the mapper needs and delegates). Such a function attaches only
// what the converter cannot see, each field a `goverter:ignore` with its reason in the converter.
func viaConverter(pass *analysis.Pass, insp *inspector.Inspector) map[types.Object]bool {
	calls := map[types.Object][]types.Object{}
	via := map[types.Object]bool{}
	for fn := range inspector.All[*ast.FuncDecl](insp) {
		self := pass.TypesInfo.Defs[fn.Name]
		if fn.Body == nil || self == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var callee types.Object
			switch f := call.Fun.(type) {
			case *ast.Ident:
				callee = pass.TypesInfo.Uses[f]
			case *ast.SelectorExpr:
				callee = pass.TypesInfo.Uses[f.Sel]
				if s := pass.TypesInfo.Selections[f]; s != nil && isConverter(s.Recv()) {
					via[self] = true
				}
			}
			if callee != nil && callee.Pkg() == pass.Pkg {
				calls[self] = append(calls[self], callee)
			}
			return true
		})
	}
	for changed := true; changed; {
		changed = false
		for fn, callees := range calls {
			if via[fn] {
				continue
			}
			for _, c := range callees {
				if via[c] {
					via[fn], changed = true, true
					break
				}
			}
		}
	}
	return via
}

func isConverter(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if !ok {
		return false
	}
	name := named.Obj().Name()
	return strings.HasSuffix(name, "Converter") || strings.HasSuffix(name, "ConverterImpl")
}

// registeredHelpers names the functions the package lists in a `goverter:extend` comment.
func registeredHelpers(pass *analysis.Pass) map[string]bool {
	names := map[string]bool{}
	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if !strings.Contains(c.Text, "goverter:") {
					continue
				}
				if m := pipeLine.FindStringSubmatch(c.Text); m != nil {
					names[m[1]] = true
				}
				m := extendLine.FindStringSubmatch(c.Text)
				if m == nil {
					continue
				}
				for name := range strings.FieldsSeq(m[1]) {
					names[name[strings.LastIndex(name, ":")+1:]] = true
				}
			}
		}
	}
	return names
}

func anyType(t *types.Tuple, pred func(*types.Named) bool) bool {
	for v := range t.Variables() {
		if mentions(v.Type(), pred, 0) {
			return true
		}
	}
	return false
}

// mentions looks through pointers, slices, maps and type arguments for a named type.
func mentions(t types.Type, pred func(*types.Named) bool, depth int) bool {
	if depth > 6 {
		return false
	}
	switch t := t.(type) {
	case *types.Pointer:
		return mentions(t.Elem(), pred, depth+1)
	case *types.Slice:
		return mentions(t.Elem(), pred, depth+1)
	case *types.Array:
		return mentions(t.Elem(), pred, depth+1)
	case *types.Map:
		return mentions(t.Key(), pred, depth+1) || mentions(t.Elem(), pred, depth+1)
	case *types.Named:
		if pred(t) {
			return true
		}
		args := t.TypeArgs()
		for arg := range args.Types() {
			if mentions(arg, pred, depth+1) {
				return true
			}
		}
	}
	return false
}

// isEntity is a generated ent entity, as the manifest of roles of the ent package says (entx).
func isEntity(n *types.Named) bool {
	_, ok := entx.Named(n)
	return ok && entx.KindOf(n) == entx.Entity
}

// isResource is a type of a generated API package (gen/site, gen/external, ...).
func isResource(n *types.Named) bool {
	pkg := n.Obj().Pkg()
	return pkg != nil && strings.HasPrefix(pkg.Path(), tier.Module+"gen/")
}
