package entrywrite_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/tools/analyzers/entrywrite"
)

// root is the repository the analyzer checks, from this package's directory.
const root = "../../.."

// TestEveryEntityHasAnOwner reads the ent schema: each entity (a type that embeds ent.Schema)
// names its owner in the map, the map names no entity the schema lacks, and each owner is a
// directory of the repository, so a moved package fails here instead of passing every write.
func TestEveryEntityHasAnOwner(t *testing.T) {
	entities := schemaEntities(t)
	owners := entrywrite.Owners()

	for _, e := range entities {
		assert.Contains(t, owners, e, "entity %s has no owner", e)
	}
	for e, owner := range owners {
		assert.True(t, slices.Contains(entities, e), "owners names %s, which the schema does not declare", e)
		assert.True(t, isDir(filepath.Join(root, owner)), "the owner of %s is no directory: %s", e, owner)
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func schemaEntities(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "ent", "schema", "*.go"))
	require.NoError(t, err)
	var entities []string
	for _, path := range files {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if st, ok := spec.Type.(*ast.StructType); ok && embedsSchema(st) {
				entities = append(entities, spec.Name.Name)
			}
			return false
		})
	}
	require.NotEmpty(t, entities)
	return entities
}

func embedsSchema(st *ast.StructType) bool {
	return slices.ContainsFunc(st.Fields.List, func(f *ast.Field) bool {
		sel, ok := f.Type.(*ast.SelectorExpr)
		if !ok || len(f.Names) > 0 {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "ent" && sel.Sel.Name == "Schema"
	})
}
