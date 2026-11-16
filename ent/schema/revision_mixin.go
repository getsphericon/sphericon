package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// RevisionMixin gives a resource a person edits its revision (ADR 0119), starting at 1.
// internal/app/entclient's Revision hook adds one on every update of a type that has it that changes
// something, whoever writes; an edit names the revision it was made on, so a write over a change the
// editor has not seen is refused rather than lost. State a job writes lives in a table of its own
// beside the row (ADR 0168), so a job never moves it.
type RevisionMixin struct {
	mixin.Schema
}

func (RevisionMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64(RevisionColumn).
			Annotations(Attribute("moved by every update of what an editor shows")).
			Positive().
			// The first revision is the application's rule, set by ent on every insert: a Go default,
			// which ent does not copy into the column, so a row written past ent names its own.
			DefaultFunc(firstRevision),
	}
}

// Annotations declare the kind the mixin gives: with a revision.
func (RevisionMixin) Annotations() []schema.Annotation {
	return []schema.Annotation{Revised{}}
}

// firstRevision is the revision a resource is created at (ADR 0119).
func firstRevision() int64 { return 1 }
