// Package testwrites enforces rule 10 of docs/agents/review-rules.md: a test creates rows itself only
// when creating them is the operation under test. A call to an ent `Create` or `CreateBulk` builder
// (the raw client's, a transaction's or a scoped wrapper's) in a `_test.go` file must carry a comment
// `// test writes: <why the write is the subject>` on the line above the statement that holds it, or
// trailing on its line. Generated mocks (`*_gen_test.go`) are not checked. The end-to-end suite sits
// behind the `e2e` build tag, which the lint run does not set, so it is not read here.
package testwrites

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const marker = "test writes:"

const entPath = "github.com/getsphericon/sphericon/ent"

var fix = "add a fixture row (see internal/AGENTS.md); if creating it is the operation under test, write `// " +
	marker + " <reason>` on the line above"

// Analyzer reports an inline creation in a test that gives no reason.
var Analyzer = &analysis.Analyzer{
	Name:     "testwrites",
	Doc:      "refuses an ent Create or CreateBulk in a test without a `test writes:` reason comment",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var creators = map[string]bool{"Create": true, "CreateBulk": true}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call := n.(*ast.CallExpr)
		if !isCreate(pass, call) {
			return true
		}
		file := pass.Fset.Position(call.Pos()).Filename
		if !strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_gen_test.go") {
			return true
		}
		if hasReason(pass, call, stack) {
			return true
		}
		pass.Reportf(call.Pos(), "%s", rule.Message(10,
			"a test creates a row inline and gives no reason that the write is the operation under test", fix))
		return true
	})
	return nil, nil
}

// isCreate reports whether the call is Create or CreateBulk on a type of the ent package.
func isCreate(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !creators[sel.Sel.Name] {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Signature().Recv() == nil {
		return false
	}
	recv := fn.Signature().Recv().Type()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	named, ok := recv.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == entPath
}

// hasReason finds a `test writes: <reason>` comment on the call's line, or ending on the line above
// the call or above the statement that holds it.
func hasReason(pass *analysis.Pass, call *ast.CallExpr, stack []ast.Node) bool {
	file := enclosingFile(pass, call.Pos())
	if file == nil {
		return false
	}
	lines := map[int]bool{pass.Fset.Position(call.Pos()).Line: true}
	for _, node := range slices.Backward(stack) {
		if stmt, ok := node.(ast.Stmt); ok {
			lines[pass.Fset.Position(stmt.Pos()).Line] = true
			break
		}
	}
	for _, cg := range file.Comments {
		_, reason, found := strings.Cut(cg.Text(), marker)
		if !found || strings.TrimSpace(reason) == "" {
			continue
		}
		start, end := pass.Fset.Position(cg.Pos()).Line, pass.Fset.Position(cg.End()).Line
		for l := range lines {
			if end == l-1 || (start <= l && l <= end) {
				return true
			}
		}
	}
	return false
}

func enclosingFile(pass *analysis.Pass, pos token.Pos) *ast.File {
	for _, f := range pass.Files {
		if f.FileStart <= pos && pos <= f.FileEnd {
			return f
		}
	}
	return nil
}
