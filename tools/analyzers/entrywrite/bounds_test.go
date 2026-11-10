package entrywrite_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/tools/analyzers/entrywrite"
)

// boundsFile is the generated package of the contract's bounds (cmd/boundgen, ADR 0107).
var boundsFile = filepath.Join(root, "internal", "kernel", "bounds", "bounds_gen.go")

// TestEveryBoundIsCheckedByItsOwner reads the generated bounds and their Owners: each bound is used
// by the non-test code of every owner of its field, the package that owns the entity's table in
// the ownership map, or the package the bound names when no column holds the value. A bound no
// owner checks is one only ogen applies, which is what the generated bounds are there to end.
func TestEveryBoundIsCheckedByItsOwner(t *testing.T) {
	owners := boundOwners(t)
	require.NotEmpty(t, owners, "the generated bounds name no owner")
	tables := entrywrite.Owners()

	for bound, named := range owners {
		for _, owner := range named {
			pkg := ownerPackage(tables, owner)
			assert.Truef(t, usedIn(t, filepath.Join(root, pkg), bound),
				"no non-test code of %s (the owner of %s) uses bounds.%s: check the write against it", pkg, owner, bound)
		}
	}
}

// ownerPackage is the package an owner names: the owner of the entity's table, or the package
// itself when no column holds the value.
func ownerPackage(tables map[string]string, owner string) string {
	if pkg, isEntity := tables[owner]; isEntity {
		return pkg
	}
	return owner
}

// boundOwners reads the Owners map literal of the generated bounds: each bound and what owns it.
func boundOwners(t *testing.T) map[string][]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), boundsFile, nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	owners := map[string][]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "Owners" || len(spec.Values) != 1 {
			return true
		}
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		require.True(t, ok, "Owners is a map literal")
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			require.True(t, ok)
			owners[unquote(t, kv.Key)] = values(t, kv.Value)
		}
		return false
	})
	return owners
}

func values(t *testing.T, e ast.Expr) []string {
	t.Helper()
	lit, ok := e.(*ast.CompositeLit)
	require.True(t, ok, "an owner list is a slice literal")
	out := make([]string, 0, len(lit.Elts))
	for _, el := range lit.Elts {
		out = append(out, unquote(t, el))
	}
	return out
}

func unquote(t *testing.T, e ast.Expr) string {
	t.Helper()
	lit, ok := e.(*ast.BasicLit)
	require.True(t, ok, "a string literal")
	s, err := strconv.Unquote(lit.Value)
	require.NoError(t, err)
	return s
}

// usedIn reports whether a source file directly in dir mentions the bound: a non-test Go file
// mentions bounds.<bound>, a TypeScript file of a package boundgen writes bounds for mentions the
// constant itself (its generated bounds.gen.ts is not read).
func usedIn(t *testing.T, dir, bound string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "the owner %s is no directory", dir)
	for _, e := range entries {
		ref, ok := referenceIn(e, bound)
		if !ok {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		// A whole name: a bound whose name another's begins with is not used by the other.
		mentions := regexp.MustCompile(regexp.QuoteMeta(ref) + `\b`)
		if mentions.Match(src) {
			return true
		}
	}
	return false
}

// referenceIn is how a source file names the bound, and false for a file that is not read.
func referenceIn(e os.DirEntry, bound string) (string, bool) {
	name := e.Name()
	switch {
	case e.IsDir(), strings.HasSuffix(name, "_test.go"), strings.HasSuffix(name, ".test.ts"),
		strings.HasSuffix(name, ".gen.ts"):
		return "", false
	case strings.HasSuffix(name, ".go"):
		return "bounds." + bound, true
	case strings.HasSuffix(name, ".ts"):
		return bound, true
	}
	return "", false
}
