// Package statemoment enforces rule 7 of docs/standards/review-rules.md (ADR 0109): a state field is a
// machine or a latch, ent/schema declares which (schema.Form), and it is written only through the
// `lifecycle` primitives.
//
//   - A machine field is set or cleared only inside the Write of a `lifecycle.Storage{...}` literal,
//     and that update carries a Where: the compare-and-swap on the state the move leaves.
//   - A latch field is set only inside the closure passed to `lifecycle.Latch`, conditioned on
//     `<Field>IsNil()`, and is never cleared.
//
// The form of a field is read from the constants ent/entc.go generates into the ent package
// (`ent.StateForm<Entity><Field>`), so a new state column is covered by its annotation, not by a list
// kept here. The check covers update and upsert builders, not creation: a row created in its first
// state makes no move. Test code is not read, as by entrywrite: test files, the test harness
// (internal/testhelper, which builds a fixture's state directly) and the end-to-end suite (e2e/). The
// generated ent package is not read either: its Archive, Unarchive and Discard and a Versioned
// definition's CompareAndSwap are compare-and-swaps written by the templates.
package statemoment

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

// lifecyclePath is the package of the machine and latch primitives.
const lifecyclePath = "github.com/getsphericon/sphericon/internal/kernel/lifecycle"

// writerSuffixes name the builders that change an existing row: updates, their scoped wrappers and
// the update side of an upsert, longest first.
var writerSuffixes = []string{
	"ScopedUpdateOne", "ScopedUpsertBulk", "ScopedUpsertOne", "ScopedUpdate", "ScopedUpsert",
	"UpdateOne", "UpsertBulk", "UpsertOne", "Update", "Upsert",
}

// verbs are the builder method prefixes that write a field, longest first. Clear is the only one that
// empties it.
var verbs = []string{"SetNillable", "Set", "Clear", "Update"}

const (
	formMachine = "machine"
	formLatch   = "latch"
)

// Analyzer reports writes of a machine or latch field outside the lifecycle primitives.
var Analyzer = &analysis.Analyzer{
	Name:     "statemoment",
	Doc:      "refuses a write of a machine or latch field (schema.Form) outside the lifecycle primitives",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// write is one builder method that writes a declared state field.
type write struct {
	method string // SetVerifiedAt
	verb   string // Set
	field  string // VerifiedAt
	entity string // SendingDomain
	form   string // machine or latch
}

func run(pass *analysis.Pass) (any, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	if unread(pass.Pkg.Path()) {
		return nil, nil
	}
	fenced := map[*ast.FuncDecl]map[types.Object]bool{}
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel, isSel := n.(*ast.SelectorExpr)
		if !isSel || strings.HasSuffix(pass.Fset.Position(sel.Pos()).Filename, "_test.go") {
			return true
		}
		w, ok := declaredWrite(pass, sel)
		if !ok {
			return true
		}
		switch w.form {
		case formLatch:
			checkLatch(pass, sel, w, stack)
		case formMachine:
			checkMachine(pass, sel, w, stack, fenced)
		}
		return true
	})
	return nil, nil
}

// unreadRoots are the packages, under the module, whose writes are not checked.
var unreadRoots = []string{"ent", "internal/testhelper", "e2e"}

func unread(path string) bool {
	const module = "github.com/getsphericon/sphericon/"
	rel, ok := strings.CutPrefix(path, module)
	if !ok {
		return false
	}
	for _, root := range unreadRoots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func checkLatch(pass *analysis.Pass, sel *ast.SelectorExpr, w write, stack []ast.Node) {
	if w.verb == "Clear" {
		pass.Reportf(sel.Sel.Pos(), "%s", rule.Message(7,
			fmt.Sprintf("%s clears the latch %s.%s, and a latch is never cleared", w.method, w.entity, w.field),
			"a fact recorded once stays recorded: a new fact is a new row, or the field is a machine (schema.Machine)"))
		return
	}
	closure := enclosing(pass, stack, isLatchClosure)
	if closure == nil {
		pass.Reportf(sel.Sel.Pos(), "%s", rule.Message(7,
			fmt.Sprintf("%s writes the latch %s.%s outside lifecycle.Latch", w.method, w.entity, w.field),
			fmt.Sprintf("write it inside lifecycle.Latch(ctx, func(ctx) (bool, error) {...}), conditioned on %sIsNil()", w.field)))
		return
	}
	if !callsPredicate(pass, closure.Body, w.field+"IsNil") {
		pass.Reportf(sel.Sel.Pos(), "%s", rule.Message(7,
			fmt.Sprintf("%s writes the latch %s.%s without %sIsNil() in its condition", w.method, w.entity, w.field, w.field),
			fmt.Sprintf("add %sIsNil() to the update's Where, so a second write misses and lifecycle.Latch answers ErrInvalidTransition", w.field)))
	}
}

func checkMachine(pass *analysis.Pass, sel *ast.SelectorExpr, w write, stack []ast.Node, fenced map[*ast.FuncDecl]map[types.Object]bool) {
	if enclosing(pass, stack, isStorageWrite) == nil {
		pass.Reportf(sel.Sel.Pos(), "%s", rule.Message(7,
			fmt.Sprintf("%s writes the machine field %s.%s outside a lifecycle.Storage Write", w.method, w.entity, w.field),
			"fire a trigger on the entity's machine (lifecycle.New); the compare-and-swap Write of its lifecycle.Storage{...} is the one place the column is set"))
		return
	}
	if chainHasWhere(sel.X) {
		return
	}
	if decl := enclosingDecl(stack); decl != nil {
		roots, seen := fenced[decl]
		if !seen {
			roots = fencedRoots(pass, decl.Body)
			fenced[decl] = roots
		}
		if root := rootObject(pass, sel.X); root != nil && roots[root] {
			return
		}
	}
	pass.Reportf(sel.Sel.Pos(), "%s", rule.Message(7,
		fmt.Sprintf("%s writes the machine field %s.%s without a condition on the state it leaves", w.method, w.entity, w.field),
		"add .Where(...) on the state the move leaves, so the write lands only while the row is still in it"))
}

// declaredWrite reports whether sel names a writing method of an update or upsert builder whose field
// the schema declares a machine or a latch.
func declaredWrite(pass *analysis.Pass, sel *ast.SelectorExpr) (write, bool) {
	_, named, ok := entx.Method(pass.TypesInfo, sel)
	if !ok {
		return write{}, false
	}
	var entity string
	for _, suffix := range writerSuffixes {
		if e, cut := strings.CutSuffix(named.Obj().Name(), suffix); cut && e != "" {
			entity = e
			break
		}
	}
	if entity == "" {
		return write{}, false
	}
	method := sel.Sel.Name
	for _, verb := range verbs {
		field, cut := strings.CutPrefix(method, verb)
		if !cut || field == "" {
			continue
		}
		form := stateForm(named.Obj().Pkg(), entity, field)
		if form == "" {
			return write{}, false
		}
		return write{method: method, verb: verb, field: field, entity: entity, form: form}, true
	}
	return write{}, false
}

// stateForm reads the generated ent.StateForm<Entity><Field>, or "" when the field is no machine or latch.
func stateForm(pkg *types.Package, entity, field string) string {
	c, ok := pkg.Scope().Lookup("StateForm" + entity + field).(*types.Const)
	if !ok || c.Val().Kind() != constant.String {
		return ""
	}
	return constant.StringVal(c.Val())
}

// enclosing returns the innermost function literal on the stack that ok accepts, given its parent nodes.
func enclosing(pass *analysis.Pass, stack []ast.Node, ok func(*analysis.Pass, *ast.FuncLit, []ast.Node) bool) *ast.FuncLit {
	for i := len(stack) - 1; i >= 0; i-- {
		if lit, isLit := stack[i].(*ast.FuncLit); isLit && ok(pass, lit, stack[:i]) {
			return lit
		}
	}
	return nil
}

// isStorageWrite reports whether lit is the Write of a lifecycle.Storage composite literal.
func isStorageWrite(pass *analysis.Pass, _ *ast.FuncLit, parents []ast.Node) bool {
	if len(parents) < 2 {
		return false
	}
	kv, ok := parents[len(parents)-1].(*ast.KeyValueExpr)
	if !ok {
		return false
	}
	key, ok := kv.Key.(*ast.Ident)
	if !ok || key.Name != "Write" {
		return false
	}
	lit, ok := parents[len(parents)-2].(*ast.CompositeLit)
	if !ok {
		return false
	}
	return isLifecycle(pass.TypesInfo.TypeOf(lit), "Storage")
}

// isLatchClosure reports whether lit is an argument of a call to lifecycle.Latch.
func isLatchClosure(pass *analysis.Pass, lit *ast.FuncLit, parents []ast.Node) bool {
	if len(parents) < 1 {
		return false
	}
	call, ok := parents[len(parents)-1].(*ast.CallExpr)
	if !ok || !isLifecycleFunc(pass, call.Fun, "Latch") {
		return false
	}
	for _, arg := range call.Args {
		if arg == lit {
			return true
		}
	}
	return false
}

func isLifecycle(t types.Type, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == lifecyclePath && obj.Name() == name
}

func isLifecycleFunc(pass *analysis.Pass, fun ast.Expr, name string) bool {
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return false
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == lifecyclePath && fn.Name() == name
}

// callsPredicate reports whether body calls a function of an entity package of ent with the name.
func callsPredicate(pass *analysis.Pass, body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return !found
		}
		if fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func); ok && fn.Pkg() != nil &&
			strings.HasPrefix(fn.Pkg().Path(), entx.Path+"/") {
			found = true
		}
		return !found
	})
	return found
}

func enclosingDecl(stack []ast.Node) *ast.FuncDecl {
	for i := len(stack) - 1; i >= 0; i-- {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}

// fencedRoots collects the variables a Where is called on anywhere in the function body, and those
// assigned a builder whose chain carries one.
func fencedRoots(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]bool {
	fenced := map[types.Object]bool{}
	if body == nil {
		return fenced
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok && len(assign.Lhs) == len(assign.Rhs) {
			for i, rhs := range assign.Rhs {
				if id, ok := assign.Lhs[i].(*ast.Ident); ok && chainHasWhere(rhs) {
					if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
						fenced[obj] = true
					}
				}
			}
		}
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
