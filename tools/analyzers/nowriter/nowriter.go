// Package nowriter keeps the domain and the use case tiers off io.Writer (ADR 0104, ADR 0111,
// review rule 5): a package of either tier (tools/analyzers/tier) returns a value or an iterator
// (iter.Seq2), and the driver writes it. It refuses an io.Writer parameter of any function, method,
// function literal or function type, and a struct field that holds one.
package nowriter

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// Analyzer reports an io.Writer in the domain or the use case tier.
var Analyzer = &analysis.Analyzer{
	Name: "nowriter",
	Doc:  "refuses an io.Writer parameter or field in the domain and the use case tiers (ADR 0104, ADR 0111)",
	Run:  run,
}

const fix = "return a value or an iterator (iter.Seq2) and let the driver (the CLI, the API or the job runner) write it (ADR 0111)"

func run(pass *analysis.Pass) (any, error) {
	if !tier.IsCore(pass.Pkg.Path()) {
		return nil, nil
	}
	for _, file := range pass.Files {
		if analysisx.InTestFile(pass, file.Pos()) {
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

// checkParams reports the signature once when one of its parameters is an io.Writer.
func checkParams(pass *analysis.Pass, ft *ast.FuncType, what string) {
	if ft.Params == nil {
		return
	}
	for _, field := range ft.Params.List {
		if isWriter(pass.TypesInfo.TypeOf(field.Type)) {
			rule.Report(pass, ft.Pos(), 5, what, fix)
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
		rule.Report(pass, field.Pos(), 5, "the field "+name+" holds an io.Writer", fix)
	}
}

// isWriter holds for io.Writer, under any alias.
func isWriter(t types.Type) bool { return analysisx.IsNamed(t, "io", "Writer") }
