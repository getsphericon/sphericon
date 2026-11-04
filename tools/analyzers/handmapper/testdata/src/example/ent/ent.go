// Package ent stands in for the generated ORM package.
package ent

type Contact struct{ Email string }

func (c *Contact) Unwrap() *Contact          { return c }
func (c *Contact) Value(string) (any, error) { return nil, nil }

// Invitation has a loaded edge, as ent's entities do.
type Invitation struct {
	Email string
	Edges InvitationEdges
}

type InvitationEdges struct{ Workspace *Workspace }

func (i *Invitation) Unwrap() *Invitation       { return i }
func (i *Invitation) Value(string) (any, error) { return nil, nil }

type Workspace struct {
	PublicID string
	Name     string
}

func (w *Workspace) Unwrap() *Workspace        { return w }
func (w *Workspace) Value(string) (any, error) { return nil, nil }

// Scoped is not an entity: it has no Unwrap.
type Scoped struct{}
