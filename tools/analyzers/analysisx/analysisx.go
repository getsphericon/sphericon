// Package analysisx holds the plumbing the analyzers of this module share: the inspector a pass
// requires, whether a node stands in a test file, and the questions they ask of a type or a call
// (is it this named type, does it take a context, does it call this function).
package analysisx

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

// Inspector is the result of inspect.Analyzer, which the analyzer of the pass requires.
func Inspector(pass *analysis.Pass) (*inspector.Inspector, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	return insp, nil
}

// InTestFile reports whether pos stands in a `_test.go` file.
func InTestFile(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.File(pos).Name(), "_test.go")
}

// SkippedFiles tells whether a position stands in a test file or a generated one, the files an
// analyzer of product code does not read. Each file of the pass is classified once.
func SkippedFiles(pass *analysis.Pass) func(token.Pos) bool {
	skipped := make(map[*token.File]bool, len(pass.Files))
	for _, f := range pass.Files {
		tf := pass.Fset.File(f.FileStart)
		skipped[tf] = strings.HasSuffix(tf.Name(), "_test.go") || ast.IsGenerated(f)
	}
	return func(pos token.Pos) bool { return skipped[pass.Fset.File(pos)] }
}

// IsNamed reports whether t, under any alias, is the named type name of the package at path.
func IsNamed(t types.Type, path, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == path && obj.Name() == name
}

// TakesContext reports whether a parameter of sig is a context.Context: what makes a function of
// the core an operation.
func TakesContext(sig *types.Signature) bool {
	for v := range sig.Params().Variables() {
		if IsNamed(v.Type(), "context", "Context") {
			return true
		}
	}
	return false
}

// Calls reports whether call calls the function or method name of the package at path, a generic
// one through any instance.
func Calls(info *types.Info, call *ast.CallExpr, path, name string) bool {
	f, ok := typeutil.Callee(info, call).(*types.Func)
	return ok && f.Pkg() != nil && f.Pkg().Path() == path && f.Name() == name
}
