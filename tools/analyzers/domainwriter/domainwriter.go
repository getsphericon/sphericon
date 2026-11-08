// Package domainwriter keeps the domain tier off io.Writer (ADR 0111, review rule 5): a package
// under internal/domain or ee/domain returns a value or an iterator (iter.Seq2), and the driver
// writes it. It refuses an io.Writer parameter of any function, method, function literal or
// function type, and a struct field that holds one.
package domainwriter

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

// Analyzer reports an io.Writer in the domain tier.
var Analyzer = &analysis.Analyzer{
	Name: "domainwriter",
	Doc:  "refuses an io.Writer parameter or field in internal/domain and ee/domain (ADR 0111)",
	Run:  run,
}

const module = "github.com/getsphericon/sphericon/"

const fix = "return a value or an iterator (iter.Seq2) and let the driver (the CLI, the API or the job runner) write it (ADR 0111)"

func run(pass *analysis.Pass) (any, error) {
	if !isDomain(pass.Pkg.Path()) {
		return nil, nil
	}
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		// owned are the signatures of declared functions and literals, reported by their owner.
		owned := map[*ast.FuncType]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				owned[n.Type] = true
				checkParams(pass, n.Type, n.Name.Name+" takes an io.Writer")
			case *ast.FuncLit:
				owned[n.Type] = true
				checkParams(pass, n.Type, "a function literal takes an io.Writer")
			case *ast.FuncType:
				if !owned[n] {
					checkParams(pass, n, "a function type takes an io.Writer")
				}
			case *ast.StructType:
				checkFields(pass, n)
			}
			return true
		})
	}
	return nil, nil
}

// isDomain holds for a package of either domain tier.
func isDomain(path string) bool {
	return strings.HasPrefix(path, module+"internal/domain/") || strings.HasPrefix(path, module+"ee/domain/")
}

// checkParams reports the signature once when one of its parameters is an io.Writer.
func checkParams(pass *analysis.Pass, ft *ast.FuncType, what string) {
	if ft.Params == nil {
		return
	}
	for _, field := range ft.Params.List {
		if isWriter(pass.TypesInfo.TypeOf(field.Type)) {
			pass.Reportf(ft.Pos(), "%s", rule.Message(5, what, fix))
			return
		}
	}
}

func checkFields(pass *analysis.Pass, st *ast.StructType) {
	for _, field := range st.Fields.List {
		if !isWriter(pass.TypesInfo.TypeOf(field.Type)) {
			continue
		}
		name := "embedded"
		if len(field.Names) > 0 {
			name = field.Names[0].Name
		}
		pass.Reportf(field.Pos(), "%s", rule.Message(5, "the field "+name+" holds an io.Writer", fix))
	}
}

// isWriter holds for io.Writer, under any alias.
func isWriter(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "io" && named.Obj().Name() == "Writer"
}
