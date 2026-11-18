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

// Binding is the record that one Integration can reach one Destination and whose it is (ADR 0075),
// on a Channel whose Destination is no attribute of the Contact: on Telegram a chat, made only by
// what the bot receives (a `/start` with a Start token), never written through an API. Its columns
// beyond the Destination are its Channel's (bindingDestinations). The owner is a Contact or a
// Visitor, the latest owner wins; a Visitor's Binding reaches nobody until a proven Identify hands
// it to the Contact (ADR 0171, ADR 0179). It dies with its owner and its Integration (an
// Integration is archived, never deleted, so only Workspace deletion removes one).
type Binding struct {
	ent.Schema
}

func (Binding) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("bindings", Binding{}.Fields())
	a.Checks["bindings_owner_check"] = ownerCheck
	a.Checks["bindings_destination_check"] = variantCheck("channel", bindingDestinations)
	return []schema.Annotation{a}
}

// bindingDestinations are the columns each Channel of Bindings keeps beside the Destination: a
// Telegram Binding has the chat its messages go to. A Channel added to the channel enum adds its
// entry here.
var bindingDestinations = []variant{
	{value: "telegram", required: []string{"chat_id"}},
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
		// The Channel's address of the person, unique per Integration; opt-outs are keyed by it. On
		// Telegram the user id (`from.id`).
		field.String("destination").
			NotEmpty().
			Immutable(),
		// Telegram's (bindingDestinations): where a message goes, the chat id, kept apart because a
		// private chat's id is not promised to equal the user id.
		field.String("chat_id").
			Optional().
			Nillable().
			NotEmpty(),
		// Exactly one of the two owners (bindings_owner_check).
		field.Int64("contact_id").
			Optional().
			Nillable(),
		field.Int64("visitor_id").
			Optional().
			Nillable(),
		// When the current owner got the Binding: creation, or the latest move to another owner.
		field.Time("bound_at").
			Annotations(Attribute("the latest binding of the Destination; a rebind moves it")),
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
