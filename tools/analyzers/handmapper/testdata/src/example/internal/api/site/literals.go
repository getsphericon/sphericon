package site

import (
	"strings"

	"example/ent"
	"example/gen/site"
)

// identities stands in for a module call that hands the handler entities.
func identities() []*ent.Contact { return nil }

// A handler that takes no entity but loops over the ones a module returned.
func list() site.ContactList {
	rows := identities()
	items := make([]site.ContactResource, len(rows))
	for i, row := range rows {
		items[i] = site.ContactResource{Email: strings.ToLower(row.Email)} // want `rule 1: site.ContactResource is built by hand from the fields of ent.Contact; fix: .*goverter Converter`
	}
	return site.ContactList{Items: items}
}

// A field of a loaded edge is a field of an entity.
func lookup(inv *ent.Invitation) any {
	return &site.InvitationLookup{WorkspaceName: inv.Edges.Workspace.Name, Email: inv.Email} // want `rule 1: site.InvitationLookup is built by hand from the fields of ent.Invitation`
}

// refused stands in for a module's error that carries entities.
type refused struct{ Workspaces []*ent.Workspace }

// Inside a callback (lo.Map), and the outer literal is not reported a second time.
func problem(e *refused) site.DeleteProblem {
	return site.DeleteProblem{
		Detail: "sole owner",
		Workspaces: each(e.Workspaces, func(w *ent.Workspace) site.SoleOwnedWorkspace {
			return site.SoleOwnedWorkspace{ID: site.WorkspaceID(w.PublicID), Name: w.Name} // want `rule 1: site.SoleOwnedWorkspace is built by hand from the fields of ent.Workspace`
		}),
	}
}

func each[T, R any](xs []T, f func(T) R) []R {
	out := make([]R, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

// Converter stands in for the surface's goverter Converter.
type Converter interface {
	Contacts(source []*ent.Contact) []site.ContactResource
	Workspace(source *ent.Workspace) site.SoleOwnedWorkspace
}

var mapper Converter

// The converter maps the entities; the literal only wraps what it returns.
func listViaConverter() site.ContactList {
	return site.ContactList{Items: mapper.Contacts(identities())}
}

// An entity's edge handed to the converter is no field read by hand.
func problemViaConverter(inv *ent.Invitation) site.DeleteProblem {
	return site.DeleteProblem{Workspaces: []site.SoleOwnedWorkspace{mapper.Workspace(inv.Edges.Workspace)}}
}

// A fact decided from an entity's field (a comparison, a negation) is no field copied by hand.
func passkeys(c *ent.Contact, fresh bool) any {
	return site.PasskeyList{Items: mapper.Contacts(identities()), PasswordSet: c.Email != "" && !fresh}
}

// user stands in for a domain module's struct, which is no entity.
type user struct {
	ID   int64
	Name string
}

func usedBy(u user) site.UsedBy { return site.UsedBy{ID: u.ID, Name: u.Name} }

// The rule reads the literal only: a field copied into a variable first is not seen (a known miss).
func viaVariable() site.ContactResource {
	email := identities()[0].Email
	return site.ContactResource{Email: email}
}
