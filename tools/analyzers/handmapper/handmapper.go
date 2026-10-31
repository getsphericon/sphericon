// Package handmapper enforces rule 1 of docs/standards/review-rules.md in both directions of the API
// contract, each of which goverter generates.
//
// Output: an ent entity becomes an API resource only through the generated goverter converter. A
// function that takes an ent entity and returns a type of a generated API package (gen/...) is a
// hand-written mapper, unless it is a helper the package registers with `goverter:extend` or a
// `goverter:map ... | helper` line, or a function that maps through a converter method and only
// attaches the fields the converter ignores.
//
// Input: a request becomes a module's input only through the generated goverter Inputs of its
// surface (resources/inputs.go). A struct literal of a module input type in a /site or /api handler
// package is a hand-written mapping. A field the request does not carry (the actor) is a
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
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

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
	if isHandlerPackage(pass.Pkg.Path()) {
		reportInputLiterals(pass, insp)
	}
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
		pass.Reportf(fn.Name.Pos(), "%s", rule.Message(1,
			fn.Name.Name+" maps an ent entity to an API resource by hand",
			"add a method to the surface's goverter Converter (or register a goverter:extend helper) and call it"))
	})
	return nil, nil
}

// handlerPackages are the packages of the /site and /api handlers, which hand a module what the
// request says. Their subpackages are not handlers: resources holds the goverter Inputs and its
// extend helpers, the one place a module input is built by hand (a oneOf is beyond goverter).
var handlerPackages = []string{"/internal/api/site", "/internal/api/external"}

func isHandlerPackage(path string) bool {
	for _, p := range handlerPackages {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}

// reportInputLiterals reports each struct literal of a module input type in a handler package.
func reportInputLiterals(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || skippedFile(pass, lit.Pos()) {
			return
		}
		named, ok := types.Unalias(pass.TypesInfo.TypeOf(lit)).(*types.Named)
		if !ok || !isModuleInput(pass.Pkg, named) {
			return
		}
		name := named.Obj().Pkg().Name() + "." + named.Obj().Name()
		pass.Reportf(lit.Pos(), "%s", rule.Message(1,
			name+" is built by hand from the request",
			"add a method to the surface's goverter Inputs (resources/inputs.go), call it, and assign after it only the fields it ignores"))
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
