package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// RevisionMixin gives a resource a person edits its revision (ADR 0119): the count of the row's
// writes, starting at 1. internal/app/entclient's Revision hook adds one on every update of a type
// that has it, whoever writes, and an edit names the revision it was made on, so a write over a
// change the editor has not seen is refused rather than lost.
type RevisionMixin struct {
	mixin.Schema
}

func (RevisionMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("revision").
			Annotations(Attribute("the count of the row's writes, added to by every update")).
			Positive().
			Default(1),
	}
}
