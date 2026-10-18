package external_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handlerScopes returns every scope string the /api handlers check through
// auth.HasScope, read from the handler package source (string literals and
// package-level string constants), so the list cannot drift from the code.
func handlerScopes(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		require.NoError(t, err)
		files = append(files, f)
	}

	consts := map[string]string{}
	var calls []*ast.CallExpr
	{
		for _, file := range files {
			ast.Inspect(file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.ValueSpec:
					for i, name := range n.Names {
						if i >= len(n.Values) {
							continue
						}
						if lit, ok := n.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if v, err := strconv.Unquote(lit.Value); err == nil {
								consts[name.Name] = v
							}
						}
					}
				case *ast.CallExpr:
					if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "HasScope" && len(n.Args) == 2 {
						calls = append(calls, n)
					}
				}
				return true
			})
		}
	}

	var scopes []string
	for _, call := range calls {
		switch arg := call.Args[1].(type) {
		case *ast.BasicLit:
			v, err := strconv.Unquote(arg.Value)
			require.NoError(t, err)
			scopes = append(scopes, v)
		case *ast.Ident:
			v, ok := consts[arg.Name]
			require.Truef(t, ok, "unresolved scope constant %s", arg.Name)
			scopes = append(scopes, v)
		default:
			t.Fatalf("unsupported HasScope argument at %s", fset.Position(call.Pos()))
		}
	}
	slices.Sort(scopes)
	return slices.Compact(scopes)
}

func TestExternalEveryCheckedScopeIsGrantable(t *testing.T) {
	scopes := handlerScopes(t)
	require.NotEmpty(t, scopes)

	env := testhelper.Setup(t)
	// The minter holds every scope under test: a token can only grant what it has.
	c := env.ExternalScoped(t, append(slices.Clone(scopes), "tokens:write")...)

	for _, scope := range scopes {
		t.Run(scope, func(t *testing.T) {
			res, err := c.AuthTokensCreate(context.Background(), &externalapi.CreateApiTokenInput{
				Name:   "scope-" + scope,
				Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScope(scope)},
			})
			require.NoError(t, err)
			assert.IsTypef(t, &externalapi.CreateApiTokenResponse{}, res, "scope %q rejected at token creation", scope)
		})
	}
}

func TestOAuthScopesAreInTheTokenScopeEnum(t *testing.T) {
	enum := map[string]bool{}
	for _, s := range (externalapi.ApiTokenScope("")).AllValues() {
		enum[string(s)] = true
	}
	for _, s := range oauthserver.SupportedScopes() {
		assert.Truef(t, enum[s], "OAuth scope %q is not an ApiTokenScope", s)
	}
}

// The workspace UI offers every token scope: its option list must be the enum.
func TestApiKeysUIOffersEveryTokenScope(t *testing.T) {
	src, err := os.ReadFile("../../../src/pages/workspace/ApiKeysSection.tsx")
	require.NoError(t, err)
	text := string(src)
	start := strings.Index(text, "SCOPE_OPTIONS = [")
	require.GreaterOrEqual(t, start, 0)
	block := text[start : start+strings.Index(text[start:], "]")]

	for _, s := range (externalapi.ApiTokenScope("")).AllValues() {
		assert.Containsf(t, block, "'"+string(s)+"'", "ApiKeysSection is missing scope %s", s)
	}
}
