package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// RevisionMixin gives a resource a person edits its revision (ADR 0119), starting at 1.
// internal/app/entclient's Revision hook adds one on every update of a type that has it, whoever
// writes, unless the update writes Bookkeeping columns alone; an edit names the revision it was made
// on, so a write over a change the editor has not seen is refused rather than lost.
type RevisionMixin struct {
	mixin.Schema
}

func (RevisionMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("revision").
			Annotations(Attribute("moved by every update of what an editor shows")).
			Positive().
			Default(1),
	}
}

// Bookkeeping marks a column of a type with a revision that the system keeps for itself and no
// editor shows or sends: a reading of a provider, a notice's moment, a feed position. An update that
// writes such columns alone leaves the revision as it was (ADR 0119), so a background job does not
// refuse the save of a person who has the resource open; one that writes any other column moves it.
// A column a person also writes, or that an editor shows, is not one.
type Bookkeeping struct{}

func (Bookkeeping) Name() string { return "Bookkeeping" }

var _ schema.Annotation = Bookkeeping{}
