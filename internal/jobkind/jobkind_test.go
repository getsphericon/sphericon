package jobkind_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/jobkind"
)

// declaredKeys names the package-level variables of jobkind.go initialised with a Key
// literal, read from the source so a new kind cannot be forgotten by this test.
func declaredKeys(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "jobkind.go", nil, 0)
	require.NoError(t, err)
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, value := range vs.Values {
				lit, ok := value.(*ast.CompositeLit)
				if !ok {
					continue
				}
				if ident, ok := lit.Type.(*ast.Ident); ok && ident.Name == "Key" {
					names = append(names, vs.Names[i].Name)
				}
			}
		}
	}
	return names
}

// Every kind declared with a purge key is listed in Purged, and each declares its
// argument: a kind added without being listed fails here.
func TestEveryDeclaredKeyIsPurgedAndNamesItsArgument(t *testing.T) {
	declared := declaredKeys(t)
	require.NotEmpty(t, declared)
	assert.Len(t, jobkind.Purged, len(declared), "Purged must list every declared Key: %v", declared)

	kinds := lo.Map(jobkind.Purged, func(k jobkind.Key, _ int) string { return k.Kind })
	assert.Len(t, lo.Uniq(kinds), len(kinds), "a kind is listed twice")
	for _, k := range jobkind.Purged {
		assert.NotEmpty(t, k.Arg, "kind %q has no declared purge key", k.Kind)
	}
}
