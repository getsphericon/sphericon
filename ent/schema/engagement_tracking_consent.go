package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// EngagementTrackingConsent is the derived row of a Destination's Engagement tracking consent
// (ADR 0120): whether the opens and clicks of the messages to one (Workspace, Channel,
// Destination) may be tracked, `granted` or `denied`, and who recorded it. Its truth is the
// evidentiary `<channel>.engagement_tracking_granted` / `_denied` Event published in the same
// transaction; the row is what the send path and the recorder read. Absence means nothing is
// recorded and the Workspace's default decides. It is keyed by Destination, never by Contact, so
// a `denied` survives the Contact's Erasure and a re-import (ADR 0021); Erasure deletes a
// `granted`. When the observation that set it occurred is its consent stamp's (ADR 0154). Only
// internal/domain/contacts/engagementtracking writes it.
type EngagementTrackingConsent struct {
	ent.Schema
}

func (EngagementTrackingConsent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		WithLowerCheck(TableWithEnumChecks("engagement_tracking_consents", EngagementTrackingConsent{}.Fields()), "destination"),
	}
}

func (EngagementTrackingConsent) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "engagement_tracking_consents"}}
}

func (EngagementTrackingConsent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Channel the consent is on.
		field.Enum("channel").
			GoType(channels.Channel("")).
			Immutable(),
		// Normalized Channel-specific Destination: the lower-cased email address, the phone number
		// in E.164, or the Telegram user id a Binding names. On sms the Erasure of the Contact whose
		// phone it was rekeys a `denied` to the number's hash (phone.Hash), as it does an opt-out
		// (ADR 0021): the refusal outlives the person, the number does not.
		field.String("destination").
			NotEmpty(),
		// Whether the Destination agreed to be tracked. A `denied` is replaced only by the person's
		// own grant (ADR 0121); the observations of one Destination are serialized and ordered by
		// its consent stamp (ADR 0154).
		field.Enum("state").
			Values("granted", "denied").
			Annotations(Attribute("the latest admitted observation of the Destination; consentstamps orders and serializes every write of it")),
		// Who recorded the consent: the person on a page reached from a message (page), a member
		// on the Contact card (manual), the customer through the API or an import (imported), an
		// On-site form's checkbox (onsite_form).
		field.Enum("provenance").
			Values("page", "manual", "imported", "onsite_form"),
	}
}

func (EngagementTrackingConsent) Indexes() []ent.Index {
	return []ent.Index{
		// One consent per (Channel, Destination) per Workspace; the send path and the recorder read
		// it by that key.
		index.Fields("workspace_id", "channel", "destination").
			Unique().
			StorageKey("engagement_tracking_consents_ws_channel_dest"),
	}
}
