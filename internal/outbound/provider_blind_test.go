package outbound_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Outbound send path asks a sender what it says about itself (ADR 0058); it never
// compares a provider's name. This fails when a non-test file of the package names a
// provider constant (messaging.ProviderSMTP, integration.ProviderSes, ...) or compares a
// Provider field, so a new provider needs no change to the send path.
func TestSendPathDoesNotCompareAProviderName(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	assert.Empty(t, providerNameUses(t, files))
}

// providerNameUses lists where the non-test files among files name a provider.
func providerNameUses(t *testing.T, files []string) []string {
	t.Helper()
	var uses []string
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if strings.HasPrefix(n.Sel.Name, "Provider") && n.Sel.Name != "ProviderMessageID" {
					uses = append(uses, fset.Position(n.Pos()).String()+" names a provider: "+n.Sel.Name)
				}
			case *ast.BinaryExpr:
				uses = append(uses, comparedProviders(fset, n)...)
			}
			return true
		})
	}
	return uses
}

// comparedProviders reports a comparison of a Provider field.
func comparedProviders(fset *token.FileSet, n *ast.BinaryExpr) []string {
	var uses []string
	for _, side := range []ast.Expr{n.X, n.Y} {
		sel, ok := side.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "Provider" && (n.Op == token.EQL || n.Op == token.NEQ) {
			uses = append(uses, fset.Position(n.Pos()).String()+" compares a provider's name")
		}
	}
	return uses
}
