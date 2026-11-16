package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// ConsentStamp is when the Consent observations that set the consent states of one (Channel,
// Destination, Sending source) occurred (ADR 0154): the opt-out state (an Unsubscribe, or the
// return that lifted it) and, on the `everything` source, the Confirmation state (a Confirmation,
// or the opt-out that withdrew it). It outlives the Unsubscribe row a return deletes, so a late,
// older stop is compared with the return. A new observation changes a state only when it occurred
// later. The row is also what serializes the observations of its key: each takes it FOR UPDATE
// first. Only internal/domain/contacts/consentstamps writes it. Keyed by the Destination like the
// Unsubscribe, but an sms stamp by the number's hash (lowercase hex SHA-256), never the number, so
// it holds nothing an Erasure must find (ADR 0021).
type ConsentStamp struct {
	ent.Schema
}

// Annotations checks what the comments of the columns say: a bot's number comes with the bot and
// only on a stamped opt-out, and a Confirmation's time only on the `everything` source.
func (ConsentStamp) Annotations() []schema.Annotation {
	a := WithLowerCheck(TableWithEnumChecks("consent_stamps", ConsentStamp{}.Fields()), "destination")
	a.Checks["consent_stamps_sequence_check"] = "(opt_out_integration_id IS NULL) = (opt_out_sequence IS NULL) AND " +
		"(opt_out_occurred_at IS NOT NULL OR opt_out_integration_id IS NULL)"
	a.Checks["consent_stamps_confirmation_check"] = "sending_source = 'everything' OR confirmation_occurred_at IS NULL"
	a.Checks["consent_stamps_sms_hash_check"] = "channel <> 'sms' OR destination ~ '^[0-9a-f]{64}$'"
	return []schema.Annotation{a}
}

func (ConsentStamp) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "consent_stamps"}}
}

func (ConsentStamp) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Enum("channel").
			GoType(channels.Channel("")).
			Immutable(),
		field.String("destination").
			NotEmpty(),
		field.String("sending_source").
			NotEmpty().
			Immutable(),
		// When the observation that set the opt-out state occurred: a stop, or a return that
		// lifted one. Nil until one has.
		field.Time("opt_out_occurred_at").
			Optional().
			Nillable().
			Annotations(Attribute("the occurrence an observation is compared with; no move of a lifecycle reads it")),
		// The Integration whose own sequence numbered that observation (a Telegram bot's
		// update_id), with its number: two observations of one bot are ordered by it.
		field.Int64("opt_out_integration_id").
			Optional().
			Nillable().
			Annotations(ScopedRef{
				Unchecked: true,
				Reason:    "a snapshot of the bot whose sequence numbered the observation; it outlives the Integration, whose ids are never reused",
			}),
		field.Int64("opt_out_sequence").
			Optional().
			Nillable(),
		// When the observation that set the Confirmation state occurred: a Confirmation, or the
		// opt-out of everything that withdrew it. Only on the `everything` source.
		field.Time("confirmation_occurred_at").
			Optional().
			Nillable().
			Annotations(Attribute("the occurrence an observation is compared with; no move of a lifecycle reads it")),
	}
}

func (ConsentStamp) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "channel", "destination", "sending_source").
			Unique().
			StorageKey("consent_stamps_ws_channel_dest_source"),
	}
}
