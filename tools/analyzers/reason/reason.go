// Package reason finds the comment that excuses a construct an analyzer otherwise reports: a
// `<marker> <why>` comment on the construct's line, or ending on the line above it or above the
// statement that holds it. rawsql (`raw sql:`) and testwrites (`test writes:`) read it.
package reason

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// Given reports whether a comment carrying marker and a reason after it stands on the line of the
// innermost node of stack, or ends on the line above that node or above the innermost statement
// that holds it. stack is the traversal stack of inspector.WithStack: the *ast.File first, the node
// last. A marker with nothing after it gives no reason.
func Given(fset *token.FileSet, stack []ast.Node, marker string) bool {
	file, isFile := stack[0].(*ast.File)
	if !isFile {
		return false
	}
	lines := map[int]bool{fset.Position(stack[len(stack)-1].Pos()).Line: true}
	for _, node := range slices.Backward(stack) {
		if stmt, ok := node.(ast.Stmt); ok {
			lines[fset.Position(stmt.Pos()).Line] = true
			break
		}
	}
	for _, cg := range file.Comments {
		_, why, found := strings.Cut(cg.Text(), marker)
		if !found || strings.TrimSpace(why) == "" {
			continue
		}
		start, end := fset.Position(cg.Pos()).Line, fset.Position(cg.End()).Line
		for l := range lines {
			if end == l-1 || (start <= l && l <= end) {
				return true
			}
		}
	}
	return false
}
