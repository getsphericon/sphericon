package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// FrequencyCount is one place a marketing message takes under the Frequency cap of one Destination
// (ADR 0137, ADR 0178): one row per Broadcast or Automation message and each Destination it is
// claimed for, written with the claim, whether or not a cap applies there, so a cap set later sees
// the period behind it. A row counts while its message is pending or sent: a message that failed
// or was skipped frees its places, a target the provider drops loses its own row, and the row dies
// with its message. Nothing rewrites it; it is deleted once older than the longest period (seven
// days) and by an Erasure. Only internal/domain/sending/frequencycap writes it.
type FrequencyCount struct {
	ent.Schema
}

func (FrequencyCount) Annotations() []schema.Annotation {
	return []schema.Annotation{TableWithEnumChecks("frequency_counts", FrequencyCount{}.Fields())}
}

func (FrequencyCount) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "frequency_counts"}}
}

func (FrequencyCount) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The message that takes the place.
		field.Int64("outbound_message_id").
			Immutable(),
		// The Channel the place is counted on: the message's.
		field.Enum("channel").
			GoType(channels.Channel("")).
			Immutable(),
		// The Destination the message was claimed for, as its Channel keys it (on email the
		// address, on SMS the number, on Telegram the chat's Destination).
		field.String("destination").
			NotEmpty().
			Immutable(),
		// When the message was counted: the claim's moment, which neither a handed-back claim nor a
		// takeover rewrites.
		field.Time("counted_at").
			Immutable().
			Annotations(Attribute(reasonHistory)),
	}
}

func (FrequencyCount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("outbound_message", OutboundMessage.Type).
			Ref("frequency_counts").
			Field("outbound_message_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (FrequencyCount) Indexes() []ent.Index {
	return []ent.Index{
		// The count of a Destination in a rolling period.
		index.Fields("workspace_id", "channel", "destination", "counted_at").
			StorageKey("frequency_counts_workspace_channel_destination_counted_at"),
		// The rows of a message, which die with it.
		index.Fields("workspace_id", "outbound_message_id").
			StorageKey("frequency_counts_workspace_id_outbound_message_id"),
	}
}
