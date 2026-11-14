// Package entx holds what the analyzers share about the generated ent package: finding the method a
// selector calls on one of its types, and reading the manifest of roles the generator declares there
// (ent/template/roles.tmpl): what a type is, the entity it belongs to, the role of a method and the
// state field a method writes. Nothing here guesses from a name: a type or a method the manifest does
// not describe has no kind and no role, and ent/schema/roles_test.go keeps the manifest complete.
package entx

import (
	"go/ast"
	"go/constant"
	"go/types"
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
	named, ok := Named(fn.Signature().Recv().Type())
	return fn, named, ok
}

// Named returns the named type of the ent package t is or points to.
func Named(t types.Type) (*types.Named, bool) {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != Path {
		return nil, false
	}
	return named, true
}

// Role is what a method does to rows (RoleOf_<Type>_<Method>).
type Role int

const (
	// Unlisted: the manifest names no role for the method (a method of ent's own types that starts
	// neither a read nor a write).
	Unlisted Role = iota
	// Read starts a read of rows.
	Read
	// Start starts a write: it returns the builder of one, or writes at once.
	Start
	// Write carries on a write already started.
	Write
	// None reads and writes no row.
	None
)

var roles = map[string]Role{"RoleRead": Read, "RoleStart": Start, "RoleWrite": Write, "RoleNone": None}

// Kind is what a type is (TypeOf_<Type>).
type Kind int

const (
	// NoKind: the manifest does not describe the type.
	NoKind Kind = iota
	Entity
	Client
	Query
	Create
	Update
	Delete
	Upsert
	Wrapper
	ScopedCreate
	ScopedUpdate
	ScopedDelete
	ScopedUpsert
	Pointers
	Other
)

var kinds = map[string]Kind{
	"TypeEntity": Entity, "TypeClient": Client, "TypeQuery": Query,
	"TypeCreate": Create, "TypeUpdate": Update, "TypeDelete": Delete, "TypeUpsert": Upsert,
	"TypeWrapper": Wrapper, "TypeScopedCreate": ScopedCreate, "TypeScopedUpdate": ScopedUpdate,
	"TypeScopedDelete": ScopedDelete, "TypeScopedUpsert": ScopedUpsert,
	"TypePointers": Pointers, "TypeOther": Other,
}

// Builds reports whether a type of the kind builds a write: a create, an update, a delete, an upsert
// (the raw builders and the scoped ones) or a Versioned definition's pointers.
func (k Kind) Builds() bool {
	switch k {
	case Create, Update, Delete, Upsert, ScopedCreate, ScopedUpdate, ScopedDelete, ScopedUpsert, Pointers:
		return true
	case NoKind, Entity, Client, Query, Wrapper, Other:
		return false
	}
	return false
}

// value reads the string constant name of the package.
func value(pkg *types.Package, name string) (string, bool) {
	c, ok := pkg.Scope().Lookup(name).(*types.Const)
	if !ok || c.Val().Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(c.Val()), true
}

// word maps the value of an entry to the vocabulary constant of the package that holds it.
func word[T comparable](pkg *types.Package, entry string, vocabulary map[string]T) T {
	var zero T
	v, ok := value(pkg, entry)
	if !ok {
		return zero
	}
	for name, w := range vocabulary {
		if wv, ok := value(pkg, name); ok && wv == v {
			return w
		}
	}
	return zero
}

// KindOf is what the manifest says the type is.
func KindOf(named *types.Named) Kind {
	return word(named.Obj().Pkg(), "TypeOf_"+named.Obj().Name(), kinds)
}

// RoleOf is the role the manifest gives the method of the type.
func RoleOf(named *types.Named, method string) Role {
	return word(named.Obj().Pkg(), "RoleOf_"+named.Obj().Name()+"_"+method, roles)
}

// EntityOf is the entity the type belongs to (`TagScopedCreate` is `Tag`, the scope is its
// Workspace), or "" for a type of no entity.
func EntityOf(named *types.Named) string {
	e, _ := value(named.Obj().Pkg(), "EntityOf_"+named.Obj().Name())
	return e
}

// StateWrite returns the machine or latch field a method of an update or upsert builder sets or
// clears (SetsState_ and ClearsState_ of the manifest).
func StateWrite(named *types.Named, method string) (field string, clears, ok bool) {
	key := named.Obj().Name() + "_" + method
	if f, ok := value(named.Obj().Pkg(), "SetsState_"+key); ok {
		return f, false, true
	}
	if f, ok := value(named.Obj().Pkg(), "ClearsState_"+key); ok {
		return f, true, true
	}
	return "", false, false
}
