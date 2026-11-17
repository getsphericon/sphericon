// Package ent stands in for the generated ORM package, with the part of its manifest of roles
// (ent/template/roles.tmpl) that says which types are entities.
package ent

// The vocabulary of the manifest of roles.
const (
	TypeEntity = "entity"
	TypeOther  = "other"
)

// What each type is.
const (
	TypeOf_Contact         = TypeEntity
	TypeOf_Invitation      = TypeEntity
	TypeOf_InvitationEdges = TypeOther
	TypeOf_Workspace       = TypeEntity
	TypeOf_Scoped          = TypeOther
)

type Contact struct{ Email string }

// Invitation has a loaded edge, as ent's entities do.
type Invitation struct {
	Email string
	Edges InvitationEdges
}

type InvitationEdges struct{ Workspace *Workspace }

type Workspace struct {
	PublicID string
	Name     string
}

// Scoped is not an entity: the manifest says so.
type Scoped struct{}
