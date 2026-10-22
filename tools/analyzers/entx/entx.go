// Package entx holds what the analyzers share about the generated ent package: finding the method a
// selector calls on one of its types and naming the entity a builder or wrapper belongs to.
package entx

import (
	"go/ast"
	"go/types"
	"strings"
)

// Path is the import path of the generated ent package.
const Path = "github.com/getsphericon/sphericon/ent"

// Method returns the method a selector names and the named type it is declared on, when that type
// belongs to the ent package (the raw client, a transaction, a builder, the scoped client or one of
// its wrappers). The pointer of a pointer receiver is unwrapped.
func Method(info *types.Info, sel *ast.SelectorExpr) (*types.Func, *types.Named, bool) {
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Signature().Recv() == nil {
		return nil, nil, false
	}
	recv := fn.Signature().Recv().Type()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	named, ok := recv.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != Path {
		return nil, nil, false
	}
	return fn, named, true
}

// typeSuffixes are what ent appends to an entity's name for its client, builders and scoped wrappers,
// longest first, so that one cut leaves the entity (OAuthClientClient is OAuthClient, not OAuth).
var typeSuffixes = []string{
	"ScopedCreateBulk", "ScopedUpdateOne", "ScopedDeleteOne", "ScopedUpsertBulk", "ScopedUpsertOne",
	"ScopedCreate", "ScopedUpdate", "ScopedDelete", "ScopedUpsert",
	"CreateBulk", "UpdateOne", "DeleteOne", "UpsertBulk", "UpsertOne",
	"Create", "Update", "Delete", "Upsert", "Scoped", "Client",
}

// Entity is the entity a type of the ent package belongs to (`TagScopedCreate` is `Tag`), or the
// type's own name when it carries none of the suffixes.
func Entity(named *types.Named) string {
	name := named.Obj().Name()
	for _, suffix := range typeSuffixes {
		if entity, ok := strings.CutSuffix(name, suffix); ok && entity != "" {
			return entity
		}
	}
	return name
}
