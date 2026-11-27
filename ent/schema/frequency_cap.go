package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// FrequencyCap is one of a Workspace's Frequency caps (ADR 0137): at most max_messages marketing
// messages to one Destination on its Channel per rolling period_minutes, for the numbers of one
// country or, with no country, for every Destination on the Channel. A country's row outranks the
// Channel's for a number of that country, and a row with no maximum removes the cap there. A new
// Workspace starts with the SMS row (three per 1440 minutes); the Workspace's Owners and Admins
// replace the rows as a whole. Only internal/domain/identity/workspaces writes it.
type FrequencyCap struct {
	ent.Schema
}

func (FrequencyCap) Annotations() []schema.Annotation {
	a := WithFormatCheck(TableWithEnumChecks("frequency_caps", FrequencyCap{}.Fields()), "country", bounds.CountryCodePattern)
	// A country takes a cap of its own only on a phone Channel, by the number's country.
	a.Checks["frequency_caps_country_check"] = "country IS NULL OR channel = 'sms'"
	// A cap is a maximum over a period; a row without one removes the cap, and has no period.
	a.Checks["frequency_caps_cap_check"] = "(max_messages IS NULL) = (period_minutes IS NULL)"
	a.Checks["frequency_caps_max_messages_check"] = "max_messages >= 1"
	// A rolling period of one minute to seven days: the count of a Destination is kept no longer.
	a.Checks["frequency_caps_period_minutes_check"] = "period_minutes BETWEEN 1 AND 10080"
	return []schema.Annotation{a}
}

func (FrequencyCap) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "frequency_caps"}}
}

func (FrequencyCap) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Channel the cap counts on.
		field.Enum("channel").
			GoType(channels.Channel("")).
			Immutable(),
		// The ISO 3166-1 alpha-2 country of the numbers the row is for; null for every Destination
		// on the Channel.
		field.String("country").
			Optional().
			Nillable().
			Immutable(),
		// The most marketing messages one Destination receives in the period; null removes the cap.
		field.Int("max_messages").
			Optional().
			Nillable(),
		// The rolling period, in minutes; null when the row removes the cap.
		field.Int("period_minutes").
			Optional().
			Nillable(),
	}
}

func (FrequencyCap) Indexes() []ent.Index {
	return []ent.Index{
		// One cap per Channel and country (one without a country) per Workspace: its NULL country is
		// one value (NullsNotDistinct).
		index.Fields("workspace_id", "channel", "country").
			Unique().
			StorageKey(FrequencyCapIndex),
	}
}

// FrequencyCapIndex is the unique index of a Workspace's Frequency cap per Channel and country.
const FrequencyCapIndex = "frequency_caps_workspace_channel_country"
