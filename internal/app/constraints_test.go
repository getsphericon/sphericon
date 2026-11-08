package app_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// root is the repository, from this package's directory.
const root = "../.."

// module is the import path of the repository's Go module.
const module = "github.com/getsphericon/sphericon/"

// TestEveryUniqueConstraintIsDeclaredByItsOwner holds the declarations of ADR 0106 to the migrated
// schema. This package imports every module, the Enterprise Edition's included, so every
// declaration is made by the time the test runs. Each unique index of a table of ours (a primary
// key aside: its id comes from a sequence) is declared exactly once, by the package that owns the
// table in entrywrite's ownership map; no declaration names an index the schema lacks; and each
// declared refusal is asserted with ErrorIs in a test of its owner, which breaks the constraint
// through the module's interface (or, a refusal that never leaves its owner, unexported, is caught
// there with errors.Is and turned into the outcome its tests assert). The tables of river, the job
// queue, are no entity of ours and have no owner here.
func TestEveryUniqueConstraintIsDeclaredByItsOwner(t *testing.T) {
	env := testhelper.Setup(t)
	indexes := testhelper.UniqueIndexes(t, env.SQLDB)
	owners := tableOwners(t)
	declarations := db.Declarations()

	schema := schemaProblems(indexes, owners, declarations)
	refusals := refusalProblems(t, declarations)

	assert.Empty(t, schema)
	assert.Empty(t, refusals)
}

// schemaProblems is what is wrong between the schema's unique indexes (index to table), the owners
// of the tables (table to package) and the declarations: an index of a table without an owner, one
// declared other than once or outside its owner, and a declaration of an index the schema lacks.
func schemaProblems(indexes, ownerOf map[string]string, declarations []db.Declaration) []string {
	byName := map[string][]db.Declaration{}
	for _, d := range declarations {
		byName[d.Constraint] = append(byName[d.Constraint], d)
	}
	var problems []string
	for name, table := range indexes {
		owner, owned := ownerOf[table]
		ds := byName[name]
		switch {
		case !owned && !strings.HasPrefix(table, "river_"):
			problems = append(problems, fmt.Sprintf("the table %s of %s has no owner in entrywrite", table, name))
		case !owned:
		case len(ds) != 1:
			problems = append(problems, fmt.Sprintf("the unique constraint %s of %s is declared %d times; its owner %s declares it once (db.Refusal or db.Fault)", name, table, len(ds), owner))
		case ds[0].Owner != module+owner:
			problems = append(problems, fmt.Sprintf("the unique constraint %s of %s is declared by %s, not by its owner %s", name, table, ds[0].Owner, owner))
		}
	}
	for name, ds := range byName {
		if _, exists := indexes[name]; !exists {
			problems = append(problems, fmt.Sprintf("%s declares %s, which the schema does not have", ds[0].Owner, name))
		}
	}
	slices.Sort(problems)
	return problems
}

// refusalProblems is what is wrong with the declarations themselves: a fault that gives no reason,
// a refusal with no named sentinel, an exported refusal no test of its owner asserts with ErrorIs,
// and an unexported one its owner never catches with errors.Is (a refusal that never leaves its
// owner, which turns it into the outcome its tests assert, such as a recorded reason).
func refusalProblems(t *testing.T, declarations []db.Declaration) []string {
	t.Helper()
	var problems []string
	for _, d := range declarations {
		if d.Refusal == nil {
			if d.Reason == "" {
				problems = append(problems, fmt.Sprintf("the fault %s of %s gives no reason", d.Constraint, d.Owner))
			}
			continue
		}
		dir := filepath.Join(root, strings.TrimPrefix(d.Owner, module))
		sentinel := refusalSentinel(t, dir, d.Constraint)
		switch {
		case sentinel == "":
			problems = append(problems, fmt.Sprintf("%s declares the refusal of %s with no named sentinel", d.Owner, d.Constraint))
		case !ast.IsExported(sentinel) && !calls(t, dir, false, "errors", "Is", sentinel):
			problems = append(problems, fmt.Sprintf("%s declares the unexported refusal %s of %s and never catches it with errors.Is", d.Owner, sentinel, d.Constraint))
		case ast.IsExported(sentinel) && !calls(t, dir, true, "", "ErrorIs", sentinel):
			problems = append(problems, fmt.Sprintf("no test of %s asserts %s with ErrorIs: break %s through the module and assert the refusal", d.Owner, sentinel, d.Constraint))
		}
	}
	return problems
}

// tableOwners maps each table of an ent entity to the package that owns it: the ownership map of
// the entrywrite analyzer (a module of its own, so it is read as source), joined with the Table
// constant ent generates for each entity.
func tableOwners(t *testing.T) map[string]string {
	t.Helper()
	f := parse(t, filepath.Join(root, "tools", "analyzers", "entrywrite", "entrywrite.go"))
	var owners map[string]string
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "owners" {
			return true
		}
		owners = map[string]string{}
		for _, elt := range spec.Values[0].(*ast.CompositeLit).Elts {
			kv := elt.(*ast.KeyValueExpr)
			owners[unquote(t, kv.Key)] = unquote(t, kv.Value)
		}
		return false
	})
	require.NotEmpty(t, owners, "entrywrite declares no owners map")

	out := map[string]string{}
	for entity, owner := range owners {
		pkg := strings.ToLower(entity)
		out[entityTable(t, filepath.Join(root, "ent", pkg, pkg+".go"))] = owner
	}
	return out
}

// entityTable is the Table constant of a generated entity package.
func entityTable(t *testing.T, path string) string {
	t.Helper()
	var table string
	ast.Inspect(parse(t, path), func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if ok && len(spec.Names) == 1 && spec.Names[0].Name == "Table" {
			table = unquote(t, spec.Values[0])
		}
		return table == ""
	})
	require.NotEmpty(t, table, "%s has no Table constant", path)
	return table
}

// refusalSentinel is the name of the sentinel the package in dir declares as the refusal of the
// constraint: the second argument of its db.Refusal call.
func refusalSentinel(t *testing.T, dir, constraint string) string {
	t.Helper()
	var name string
	for _, f := range parseDir(t, dir, false) {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !isSelector(call.Fun, "db", "Refusal") {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && unquote(t, lit) == constraint {
				name = identName(call.Args[1])
			}
			return true
		})
	}
	return name
}

// calls reports whether a file of the package in dir (its tests, or the files that are no test)
// calls pkg.name (any receiver when pkg is empty, as testify's assert and require) with the
// sentinel as its last argument but one or its last: errors.Is(err, target), ErrorIs(t, err, target).
func calls(t *testing.T, dir string, tests bool, pkg, name, sentinel string) bool {
	t.Helper()
	found := false
	for _, f := range parseDir(t, dir, tests) {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return !found
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != name || (pkg != "" && !isSelector(call.Fun, pkg, name)) {
				return !found
			}
			target := call.Args[1]
			if pkg == "" && len(call.Args) > 2 {
				target = call.Args[2]
			}
			if identName(target) == sentinel {
				found = true
			}
			return !found
		})
	}
	return found
}

func parse(t *testing.T, path string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	return f
}

// parseDir parses the Go files of dir: its tests, or the files that are no test.
func parseDir(t *testing.T, dir string, tests bool) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") != tests {
			continue
		}
		files = append(files, parse(t, filepath.Join(dir, name)))
	}
	return files
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg
}

// identName is the name an identifier or a qualified identifier ends in.
func identName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

func unquote(t *testing.T, e ast.Expr) string {
	t.Helper()
	lit, ok := e.(*ast.BasicLit)
	require.True(t, ok, "not a literal: %T", e)
	s, err := strconv.Unquote(lit.Value)
	require.NoError(t, err)
	return s
}
