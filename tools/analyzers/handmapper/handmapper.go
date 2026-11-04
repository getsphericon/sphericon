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
// of every surface, internal/server (which renders the refusals) and the handler types' methods of ee/staff. A handler
// that loops over the entities a module returned takes no entity, so only the literal shows it. A
// value handed to a converter method is mapped by goverter, and a fact decided from a field (a
// comparison, a negation) copies none.
//
// Input: a request becomes a module's input only through the generated goverter Inputs of its
// surface (resources/inputs.go). A struct literal of a module input type in a handler package (/site,
// /api, /staff, and the methods of the handler types of ee/staff, which holds its domain module too)
// is a hand-written mapping. A field the request does not carry (the actor) is a
// `goverter:ignore` with its reason, and the handler assigns it after the call; an assignment is not
// a literal and is not reported.
//
// Test files are not API code and are skipped, and so are generated files.
package handmapper

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/mixed"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
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
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	via := viaConverter(pass, insp)
	recvs, isMixed := mixed.Receivers(pass.Pkg.Path())
	if isMixed || isHandlerPackage(pass.Pkg.Path()) {
		reportInputLiterals(pass, insp, recvs)
	}
	reported := map[*ast.FuncDecl]bool{}
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn, isNode := n.(*ast.FuncDecl)
		if !isNode {
			return
		}
		if strings.HasSuffix(pass.Fset.File(fn.Pos()).Name(), "_test.go") || generated(pass, fn) || registered[fn.Name.Name] {
			return
		}
		obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
		if !ok {
			return
		}
		sig, isSig := obj.Type().(*types.Signature)
		if !isSig {
			return
		}
		if !anyType(sig.Params(), isEntity) || !anyType(sig.Results(), isResource) {
			return
		}
		if via[obj] {
			return
		}
		reported[fn] = true
		pass.Reportf(fn.Name.Pos(), "%s", rule.Message(1,
			fn.Name.Name+" maps an ent entity to an API resource by hand",
			"add a method to the surface's goverter Converter (or register a goverter:extend helper) and call it"))
	})
	if isMixed || isLiteralPackage(pass.Pkg.Path()) {
		reportEntityLiterals(pass, insp, recvs, func(fn *ast.FuncDecl) bool {
			return reported[fn] || registered[fn.Name.Name]
		}, via)
	}
	return nil, nil
}

// literalPackages are the packages that build API types from what a module returned: the handlers
// of every surface (/collect too, which takes no module input) and internal/server, which renders
// the refusals with the contract's problem types. The resources subpackages are the goverter homes,
// whose registered helpers read entity fields by design.
var literalPackages = append([]string{"/internal/server", "/internal/api/collect"}, handlerPackages...)

func isLiteralPackage(path string) bool {
	for _, p := range literalPackages {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}

// reportEntityLiterals reports each composite literal of a generated API type that reads a field of an
// ent entity in one of its elements: a resource mapped by hand, whatever the enclosing function takes
// (a handler that loops over the entities a module returned takes none). A value handed to a converter
// method, or to a function of the package that maps through one, is not read by hand; a function
// literal is a function of its own (its literals are visited on their own), and so is a nested literal
// of an API type. In a mixed package (recvs not nil) only the methods of the handler types are read,
// and a function the function-level check already reported (skip) is not reported twice.
func reportEntityLiterals(pass *analysis.Pass, insp *inspector.Inspector, recvs []string, skip func(*ast.FuncDecl) bool, via map[types.Object]bool) {
	insp.WithStack([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !push || skippedFile(pass, lit.Pos()) {
			return true
		}
		if recvs != nil && !slices.Contains(recvs, mixed.EnclosingReceiver(stack)) {
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
		pass.Reportf(lit.Pos(), "%s", rule.Message(1,
			qualified(named)+" is built by hand from the fields of "+qualified(entity),
			"add a method to the surface's goverter Converter (a list method for a slice of entities; a value it cannot see is a `goverter:map ... | helper`) and call it"))
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

// handlerPackages are the packages of the /site, /api and core /staff handlers, which hand a module
// what the request says. Their subpackages are not handlers: resources holds the goverter Inputs and
// its extend helpers, the one place a module input is built by hand (a oneOf is beyond goverter). The
// handlers of the Enterprise Staff and service APIs share ee/staff with their domain module: that
// package is mixed (tools/analyzers/mixed), and only its handler types' methods are checked.
var handlerPackages = []string{"/internal/api/site", "/internal/api/external", "/internal/api/staff"}

func isHandlerPackage(path string) bool {
	for _, p := range handlerPackages {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}

// reportInputLiterals reports each struct literal of a module input type in a handler package; in a
// mixed package (recvs not nil), only those inside a method of one of the handler types recvs.
func reportInputLiterals(pass *analysis.Pass, insp *inspector.Inspector, recvs []string) {
	insp.WithStack([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !push || skippedFile(pass, lit.Pos()) {
			return true
		}
		if recvs != nil && !slices.Contains(recvs, mixed.EnclosingReceiver(stack)) {
			return true
		}
		named, ok := types.Unalias(pass.TypesInfo.TypeOf(lit)).(*types.Named)
		if !ok || !isModuleInput(pass.Pkg, named) {
			return true
		}
		name := named.Obj().Pkg().Name() + "." + named.Obj().Name()
		pass.Reportf(lit.Pos(), "%s", rule.Message(1,
			name+" is built by hand from the request",
			"add a method to the surface's goverter Inputs (resources/inputs.go), call it, and assign after it only the fields it ignores"))
		return true
	})
}

// isModuleInput is a module's input: a struct type named ...Input declared in a package under
// internal/ other than the one checked. The name is the modules' own convention for what an
// operation takes (automations.CreateInput, apitokens.Input, eventlog.Input), so a new module is
// covered the day it is written, where a list of types would go stale. A type of a generated API
// package (siteapi.SiteCreateAutomationInput) is the request itself, and a type the handler package
// declares is its own.
func isModuleInput(self *types.Package, n *types.Named) bool {
	pkg := n.Obj().Pkg()
	if pkg == nil || pkg == self || isResource(n) || !strings.HasSuffix(n.Obj().Name(), "Input") {
		return false
	}
	if !strings.HasPrefix(pkg.Path(), "internal/") && !strings.Contains(pkg.Path(), "/internal/") {
		return false
	}
	_, isStruct := n.Underlying().(*types.Struct)
	return isStruct
}

func skippedFile(pass *analysis.Pass, pos token.Pos) bool {
	if strings.HasSuffix(pass.Fset.File(pos).Name(), "_test.go") {
		return true
	}
	for _, f := range pass.Files {
		if f.FileStart <= pos && pos < f.FileEnd {
			return ast.IsGenerated(f)
		}
	}
	return false
}

// viaConverter is the set of functions of the package that map through a goverter converter: they
// call a method of a type named ...Converter or ...ConverterImpl, or call such a function of the
// package (a wrapper that loads what the mapper needs and delegates). Such a function attaches only
// what the converter cannot see, each field a `goverter:ignore` with its reason in the converter.
func viaConverter(pass *analysis.Pass, insp *inspector.Inspector) map[types.Object]bool {
	calls := map[types.Object][]types.Object{}
	via := map[types.Object]bool{}
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn, isNode := n.(*ast.FuncDecl)
		if !isNode {
			return
		}
		self := pass.TypesInfo.Defs[fn.Name]
		if fn.Body == nil || self == nil {
			return
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
	})
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
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
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

func generated(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	for _, f := range pass.Files {
		if f.Pos() <= fn.Pos() && fn.Pos() < f.End() {
			for _, cg := range f.Comments {
				for _, c := range cg.List {
					if strings.HasPrefix(c.Text, "// Code generated") && strings.HasSuffix(c.Text, "DO NOT EDIT.") {
						return true
					}
				}
			}
		}
	}
	return false
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

// isEntity is a generated ent entity: a type of the ent package with Unwrap and Value methods
// (the client, the scope and the builders have not).
func isEntity(n *types.Named) bool {
	pkg := n.Obj().Pkg()
	if pkg == nil || (pkg.Path() != "ent" && !strings.HasSuffix(pkg.Path(), "/ent")) {
		return false
	}
	ms := types.NewMethodSet(types.NewPointer(n))
	return ms.Lookup(pkg, "Unwrap") != nil && ms.Lookup(pkg, "Value") != nil
}

// isResource is a type of a generated API package (gen/site, gen/external, ...).
func isResource(n *types.Named) bool {
	pkg := n.Obj().Pkg()
	return pkg != nil && (strings.HasPrefix(pkg.Path(), "gen/") || strings.Contains(pkg.Path(), "/gen/"))
}
