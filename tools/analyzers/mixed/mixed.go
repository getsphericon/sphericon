// Package mixed names the packages that hold API handlers next to their domain module, which the
// analyzers about handlers (entrywrite, handmapper) check only inside the methods of the handler types.
package mixed

import (
	"go/ast"
	"slices"
	"strings"
)

// packages are the mixed packages, by path inside the repository, with the receiver types that are
// handlers. ee/staff holds the handlers of the Staff console's Enterprise sections and of the service
// API with their domain module (*Module); a new handler type there is added here.
var packages = map[string][]string{
	"ee/staff": {"Handlers", "ServiceHandlers", "SecurityHandler", "ServiceSecurityHandler", "Surface", "ServiceSurface"},
}

// Receivers is the set of handler receiver types of the package at path, and whether it is mixed at
// all. The path is matched by its end, so a test data package (example/ee/staff) stands in for it.
func Receivers(path string) (recvs []string, ok bool) {
	for pkg, names := range packages {
		if path == pkg || strings.HasSuffix(path, "/"+pkg) {
			return names, true
		}
	}
	return nil, false
}

// EnclosingReceiver is the name of the receiver type, without its pointer, of the method that holds
// the node on top of the stack, or "" outside a method.
func EnclosingReceiver(stack []ast.Node) string {
	var decl *ast.FuncDecl
	for _, n := range slices.Backward(stack) {
		if d, ok := n.(*ast.FuncDecl); ok {
			decl = d
			break
		}
	}
	if decl == nil || decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	t := decl.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
