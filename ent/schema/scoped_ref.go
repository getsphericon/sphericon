package schema

import "entgo.io/ent/schema"

// ScopedRef marks a plain (non-edge) field that stores the id of another
// Workspace-owned entity, so the generated scoped client (ADR 0017) verifies the
// id belongs to the same Workspace before it is saved. Edges and edge fields are
// detected automatically; only plain columns need this annotation.
type ScopedRef struct {
	// Entity is the schema name of the referenced entity, e.g. "Segment".
	Entity string
	// Unchecked marks an id column that deliberately outlives the referenced row and
	// is therefore not verified (Entity stays empty).
	Unchecked bool
	// Reason says why the column has no foreign key (a tombstone or a snapshot): an unchecked
	// reference states it at the field.
	Reason string
}

func (ScopedRef) Name() string { return "ScopedRef" }

var _ schema.Annotation = ScopedRef{}
