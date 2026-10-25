package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CapturedHeader is one header line of a Captured message, in the order it was built.
type CapturedHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CapturedMessage is a message a Test inbox kept instead of sending it out (ADR 0058): the
// one place the rendered content of a send lives, since an Outbound message holds none. It is
// owned by the Workspace and by the Integration (the Test inbox) it was captured through, and
// dies with the Integration (review rule 8: CASCADE for a row that dies with its owner) and with
// the Workspace (Workspace deletion removes it first). It belongs to the Outbound message of the
// send when there is one; a Test send has none, and the message stays when the Outbound message
// goes (SET NULL).
type CapturedMessage struct {
	ent.Schema
}

func (CapturedMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("captured_messages", CapturedMessage{}.Fields()),
	}
}

func (CapturedMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "captured_messages"}}
}

func (CapturedMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Test inbox the message was captured through.
		field.Int64("integration_id").
			Immutable(),
		// The Outbound message of the send; nil for a Test send, which records none.
		field.Int64("outbound_message_id").
			Optional().
			Nillable(),
		// Which surface asked for the send; a Test send of a Broadcast is its own surface.
		field.Enum("surface").
			Values("broadcast", "automation", "transactional", "test").
			Immutable(),
		// The Sending source of a marketing send; nil for transactional (it carries none).
		field.String("sending_source").
			Optional().
			Nillable().
			Immutable(),
		// Normalized (lower-cased) recipient address.
		field.String("recipient").
			NotEmpty().
			Immutable(),
		field.String("from_email").
			NotEmpty().
			Immutable(),
		field.String("from_name").
			Optional().
			Nillable().
			Immutable(),
		field.String("subject").
			Immutable(),
		// The parts as built; either may be empty.
		field.Text("html").
			Immutable(),
		field.Text("text").
			Immutable(),
		// The header lines the message would carry on the wire, in order.
		field.JSON("headers", []CapturedHeader{}).
			Immutable(),
		// The URLs of the links of the HTML part, in order, tracker links as the recipient would get them.
		field.JSON("links", []string{}).
			Immutable(),
		// True when the From is not on a verified Sending domain of the Workspace: the message would
		// not pass the Sending-domain gate at a provider that delivers outside.
		field.Bool("from_unverified").
			Default(false).
			Immutable(),
	}
}

func (CapturedMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).
			Ref("captured_messages").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
		edge.To("outbound_message", OutboundMessage.Type).
			Field("outbound_message_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (CapturedMessage) Indexes() []ent.Index {
	return []ent.Index{
		// The list, newest first.
		index.Fields("workspace_id", "created_at", "id"),
		// Erasure and the Privacy request reach the messages by recipient.
		index.Fields("workspace_id", "recipient"),
		// The keys that need a lookup by the referenced row: its delete or its cascade.
		index.Fields("integration_id").
			StorageKey("captured_messages_integration_id"),
		index.Fields("outbound_message_id").
			StorageKey("captured_messages_outbound_message_id"),
	}
}
