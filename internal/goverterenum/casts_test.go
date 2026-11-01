package goverterenum_test

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"testing"

	"github.com/jmattheis/goverter/enum"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// The packages that map enums between ent, the modules and the contracts: each surface's
// converters, and the handlers that call them.
var apiPackages = []string{
	"github.com/getsphericon/sphericon/internal/api/site/resources",
	"github.com/getsphericon/sphericon/internal/api/external/resources",
	"github.com/getsphericon/sphericon/internal/api/staff/resources",
	"github.com/getsphericon/sphericon/ee/staff/resources",
	"github.com/getsphericon/sphericon/internal/api/site",
	"github.com/getsphericon/sphericon/internal/api/external",
	"github.com/getsphericon/sphericon/internal/api/staff",
	"github.com/getsphericon/sphericon/ee/staff",
}

// An enum crosses into another enum only through a converter's enum method, whose switch goverter
// checks member by member. A plain conversion compiles whatever the value sets are, in a handler
// as in a converter, and goverter writes one itself when it cannot see an enum's members: when
// the converter's file does not import the enum's package, goverter reads the package through
// ent's export data, without its constants. Add an enum method to the surface's Converter
// (response) or Inputs (request), `goverter:enum:transform value`, in a file that imports the
// package, and call it.
func TestAPIConvertsNoEnumByCast(t *testing.T) {
	require.Empty(t, enumCasts(t), "enums converted by a plain conversion")
}

// enumCasts lists every conversion between two enums in the API packages, sorted.
func enumCasts(t *testing.T) []string {
	t.Helper()
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax,
		Tests: false,
	}, apiPackages...)
	require.NoError(t, err)

	var casts []cast
	for _, p := range pkgs {
		require.Empty(t, p.Errors, p.PkgPath)
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 || !p.TypesInfo.Types[call.Fun].IsType() {
					return true
				}
				from, to := p.TypesInfo.TypeOf(call.Args[0]), p.TypesInfo.TypeOf(call)
				if types.Identical(from, to) {
					return true
				}
				casts = append(casts, cast{p.Fset.Position(call.Pos()).String(), named(from), named(to)})
				return true
			})
		}
	}

	enums := enumTypes(t, casts)
	var offenders []string
	for _, c := range casts {
		if enums[c.from] && enums[c.to] {
			offenders = append(offenders, fmt.Sprintf("%s: %s -> %s", c.pos, c.from, c.to))
		}
	}
	slices.Sort(offenders)
	return offenders
}

// cast is a conversion between two types, named by qualified name.
type cast struct{ pos, from, to string }

// named is the qualified name of a named type, or of the one a pointer points to; "" otherwise.
func named(t types.Type) string {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return ""
	}
	return n.Obj().Pkg().Path() + "." + n.Obj().Name()
}

// enumTypes reports which of the named types are enums, reading each one's own package in full:
// the converters' view of a package they reach through another's export data lacks its constants.
func enumTypes(t *testing.T, casts []cast) map[string]bool {
	t.Helper()
	byPkg := map[string][]string{}
	for _, c := range casts {
		for _, q := range []string{c.from, c.to} {
			if q == "" {
				continue
			}
			i := len(q) - len(q[lastDot(q)+1:]) - 1
			byPkg[q[:i]] = append(byPkg[q[:i]], q[i+1:])
		}
	}
	paths := make([]string, 0, len(byPkg))
	for p := range byPkg {
		paths = append(paths, p)
	}
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes}, paths...)
	require.NoError(t, err)

	out := map[string]bool{}
	for _, p := range pkgs {
		for _, name := range byPkg[p.PkgPath] {
			obj, ok := p.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			if n, ok := obj.Type().(*types.Named); ok {
				_, isEnum := enum.Detect(n)
				out[p.PkgPath+"."+name] = isEnum
			}
		}
	}
	return out
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
