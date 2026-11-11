package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ownerCheck is the invariant of an owned row: exactly one of the Contact and the Visitor owns it.
const ownerCheck = "(contact_id IS NULL) <> (visitor_id IS NULL)"

// Binding is the record that one Integration can reach one Destination and whose it is (ADR 0075):
// a Telegram chat, made only by what the bot receives (a `/start` with a Start token), never
// written through an API. The owner is a Contact or a Visitor, the latest owner wins, and it dies
// with its owner and its Integration (an Integration is archived, never deleted, so only Workspace
// deletion removes one).
type Binding struct {
	ent.Schema
}

func (Binding) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("bindings", Binding{}.Fields())
	a.Checks["bindings_owner_check"] = ownerCheck
	return []schema.Annotation{a}
}

func (Binding) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "bindings"}}
}

func (Binding) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("integration_id").
			Immutable(),
		field.Enum("channel").
			Values("telegram").
			Immutable(),
		// The channel's address of the person: the Telegram user id (`from.id`). Opt-outs are
		// keyed by it.
		field.String("destination").
			NotEmpty().
			Immutable(),
		// Where a message goes: the Telegram chat id, kept apart because a private chat's id is not
		// promised to equal the user id.
		field.String("chat_id").
			NotEmpty(),
		// Exactly one of the two owners (bindings_owner_check).
		field.Int64("contact_id").
			Optional().
			Nillable(),
		field.Int64("visitor_id").
			Optional().
			Nillable(),
		// Whether telegram.bound was recorded for the Contact the Binding reaches. A Contact's own
		// Binding is announced when it is made; a Visitor's when Identify resolves the Visitor.
		field.Bool("announced").
			Annotations(Attribute("whether the bound event was published; it carries a dedup id, and a rebind resets the flag")).
			Default(false),
		// When the current owner got the Binding: creation, or the latest move to another owner.
		field.Time("bound_at").
			Annotations(Attribute("the latest binding of the chat; a rebind moves it")),
	}
}

func (Binding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).
			Ref("bindings").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
		edge.From("contact", Contact.Type).
			Ref("bindings").
			Field("contact_id").
			Unique(),
		edge.From("visitor", Visitor.Type).
			Ref("bindings").
			Field("visitor_id").
			Unique(),
	}
}

func (Binding) Indexes() []ent.Index {
	return []ent.Index{
		// One Binding per Destination of an Integration; the `/start` upsert finds it here.
		index.Fields("workspace_id", "integration_id", "destination").
			Unique().
			StorageKey("bindings_workspace_id_integration_destination"),
		// A Contact's Bindings and the cascade of its delete (a key's index is whole).
		index.Fields("workspace_id", "contact_id").
			StorageKey("bindings_workspace_id_contact_id"),
		index.Fields("workspace_id", "visitor_id").
			StorageKey("bindings_workspace_id_visitor_id"),
	}
}
