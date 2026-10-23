package schema_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// forbidigoUpdatePattern reads the entity-level Update rule from .golangci.yml, the
// pattern lint actually runs (ADR 0017).
func forbidigoUpdatePattern(t *testing.T) *regexp.Regexp {
	t.Helper()
	raw, err := os.ReadFile("../../.golangci.yml")
	require.NoError(t, err)
	var cfg struct {
		Linters struct {
			Settings struct {
				Forbidigo struct {
					Forbid []struct {
						Pattern string `yaml:"pattern"`
						Pkg     string `yaml:"pkg"`
					} `yaml:"forbid"`
				} `yaml:"forbidigo"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	for _, f := range cfg.Linters.Settings.Forbidigo.Forbid {
		if strings.Contains(f.Pkg, "sphericon/ent") {
			return regexp.MustCompile(f.Pattern)
		}
	}
	require.FailNow(t, "no forbidigo rule for the ent package in .golangci.yml")
	return nil
}

// updateReceivers splits the methods named Update in the generated ent package into
// entity-level ones (an entity's own Update returns its `<Entity>UpdateOne` builder)
// and every other receiver (<Entity>Client, <Entity>Scoped, upsert builders).
func updateReceivers(t *testing.T) (entities, others []string) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir("../../ent")
	require.NoError(t, err)
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join("..", "..", "ent", name), nil, 0)
		require.NoError(t, err)
		files = append(files, f)
	}

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Update" {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			recv, ok := star.X.(*ast.Ident)
			if !ok {
				continue
			}
			if returnsUpdateOne(fn, recv.Name) {
				entities = append(entities, recv.Name)
			} else {
				others = append(others, recv.Name)
			}
		}
	}
	return entities, others
}

func returnsUpdateOne(fn *ast.FuncDecl, recv string) bool {
	res := fn.Type.Results
	if res == nil || len(res.List) != 1 {
		return false
	}
	rs, ok := res.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := rs.X.(*ast.Ident)
	return ok && id.Name == recv+"UpdateOne"
}

// The forbidigo rule must match `ent.<Entity>.Update` for every generated entity and
// nothing else named Update (the <Entity>Client, <Entity>Scoped and upsert builders).
// Both sets come from the generated ent package itself, so a new entity whose name
// ends in Client, Scoped, One or Bulk cannot slip through the pattern unnoticed.
func TestForbidigoUpdatePatternCoversEveryEntity(t *testing.T) {
	re := forbidigoUpdatePattern(t)
	entities, others := updateReceivers(t)

	// Guards against the AST walk silently finding nothing.
	require.GreaterOrEqual(t, len(entities), 25)
	require.Contains(t, entities, "OAuthClient")
	require.Contains(t, others, "TagClient")
	require.Contains(t, others, "TagScoped")
	require.Contains(t, others, "TagUpsertOne")
	require.Contains(t, others, "TagUpsertBulk")

	for _, name := range entities {
		require.Truef(t, re.MatchString("ent."+name+".Update"), "pattern misses entity %s", name)
	}
	for _, name := range others {
		require.Falsef(t, re.MatchString("ent."+name+".Update"), "pattern wrongly matches %s", name)
	}
}
