// Package testwrites enforces rule 10 of docs/standards/review-rules.md: a test creates rows itself only
// when creating them is the operation under test. A call that starts a create through ent (`Create`,
// `CreateBulk`, `MapCreateBulk` of the raw client, a transaction's or a scoped wrapper: a method the
// manifest of roles, read through entx, gives the role Start and that returns a create builder) in a
// `_test.go` file must carry a comment `// test writes: <why the write is the subject>` on the line
// above the statement that holds it, or trailing on its line. Generated files (moq's mocks) are not
// checked. The test harness
// (internal/testhelper) is checked like a test: a helper that creates a row for every test that calls it
// is setup the fixtures should hold, so it names its reason too. The end-to-end suite sits behind the
// `e2e` build tag, which the lint run sets (`.golangci.yml`), so it is read here.
package testwrites

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/reason"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

const marker = "test writes:"

const fix = "add a fixture row (see internal/AGENTS.md); if creating it is the operation under test, write `// " +
	marker + " <reason>` on the line above"

// Analyzer reports an inline creation in a test that gives no reason.
var Analyzer = &analysis.Analyzer{
	Name:     "testwrites",
	Doc:      "refuses an ent Create or CreateBulk in a test without a `test writes:` reason comment",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
	}
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call, isNode := n.(*ast.CallExpr)
		file, inFile := stack[0].(*ast.File)
		if !isNode || !inFile || !isCreate(pass, call) || !inScope(pass, file) || reason.Given(pass.Fset, stack, marker) {
			return true
		}
		rule.Report(pass, call.Pos(), 10,
			"a test creates a row inline and gives no reason that the write is the operation under test", fix)
		return true
	})
	return nil, nil
}

// harness is the import path of the test harness, whose helpers are checked like tests.
const harness = tier.Module + "internal/testhelper"

// inScope reports whether a creation in the file needs a reason: a test file that is not generated,
// or a file of the test harness.
func inScope(pass *analysis.Pass, file *ast.File) bool {
	if ast.IsGenerated(file) {
		return false
	}
	return analysisx.InTestFile(pass, file.FileStart) || pass.Pkg.Path() == harness
}

// isCreate reports whether the call starts a create through ent: a method the manifest gives the
// role Start that returns a create builder, raw or scoped.
func isCreate(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, named, ok := entx.Method(pass.TypesInfo, sel)
	if !ok || entx.RoleOf(named, sel.Sel.Name) != entx.Start || fn.Signature().Results().Len() == 0 {
		return false
	}
	builder, ok := entx.Named(fn.Signature().Results().At(0).Type())
	if !ok {
		return false
	}
	kind := entx.KindOf(builder)
	return kind == entx.Create || kind == entx.ScopedCreate
}
