package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Erasure is one row of a Workspace's Erasures Export stream (ADR 0045, GLOSSARY: Export
// stream): the keys an Erasure matched on, so a warehouse consumer deletes the same rows.
// It is an Enterprise table: only the ee/export bus subscriber writes it (one row per
// `contact.erased` message, deduplicated by the envelope id) and only the ee/export prune job
// removes it, at the instance's Event retention window. The addresses are never stored, only
// their SHA-256, and the contact id is a plain number with no edge: the Contact is gone.
type Erasure struct {
	ent.Schema
}

func (Erasure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "erasures"},
	}
}

func (Erasure) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "erasures"}}
}

func (Erasure) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The bus envelope id: the dedupe key that makes at-least-once redelivery write
		// the row once.
		field.String("erasure_key").
			NotEmpty().
			Immutable(),
		// The erased Contact, which outlives nothing: its row is gone, so the id is not
		// verified. Absent for an address or an anonymous Visitor with no Contact.
		field.Int64("contact_id").
			Optional().
			Nillable().
			Immutable().
			Annotations(ScopedRef{
				Unchecked: true,
				Reason:    "tombstone of an erased Contact: its row is gone, so there is nothing for a key to point at",
			}),
		// The customer's own id for the person; empty when there was none to give.
		field.String("subject_id").
			Default("").
			Immutable(),
		// The anonymous devices erased with the Contact.
		field.JSON("visitor_ids", []string{}).
			Immutable(),
		// SHA-256 (hex) of the lowercased address and of the phone, never the value itself.
		field.String("email_sha256").
			Optional().
			Nillable().
			Immutable(),
		field.String("phone_sha256").
			Optional().
			Nillable().
			Immutable(),
		// When the Erasure happened; the age the prune job reads.
		field.Time("erased_at").
			Immutable(),
	}
}

func (Erasure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "erasure_key").Unique().StorageKey("erasures_workspace_id_erasure_key"),
		// Backs the Erasures stream (ADR 0045): the keyset page over (updated_at, id).
		index.Fields("workspace_id", "updated_at", "id"),
		// Backs the prune by age.
		index.Fields("erased_at"),
	}
}
