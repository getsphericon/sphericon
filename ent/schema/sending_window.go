package schema

import (
	"strings"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// SendingWindow is one of a Workspace's Sending windows (ADR 0134): the hours of each day of the
// week, in the recipient's local time, in which a marketing message on its Channel may go out, for
// the numbers of one country or, with no country, for every number on the Channel. A country's row
// outranks the Channel's for a number of that country. Only SMS has a window (email and Telegram
// are outside it), and it never fails to open in a week. A new Workspace starts with the SMS row
// (8:00 to 20:00 every day); the Workspace's Owners and Admins replace the rows as a whole. Only
// internal/domain/identity/workspaces writes it.
type SendingWindow struct {
	ent.Schema
}

// SendingWindowDays are the days of the week a window's columns are named after, Monday first (the
// contract's Weekday): <day>_opens and <day>_closes, the minutes of the day it opens and closes,
// both null on a day it stays closed.
var SendingWindowDays = func() []string {
	days := make([]string, len(localcalendar.Weekdays))
	for i, d := range localcalendar.Weekdays {
		days[i] = string(d)
	}
	return days
}()

func (SendingWindow) Annotations() []schema.Annotation {
	a := WithFormatCheck(TableWithEnumChecks("sending_windows", SendingWindow{}.Fields()), "country", bounds.CountryCodePattern)
	// Email and Telegram are outside the Sending window (ADR 0134): only a phone Channel has one,
	// and a country's rows are a phone Channel's anyway.
	a.Checks["sending_windows_phone_channel_check"] = "channel = 'sms'"
	open := make([]string, 0, len(SendingWindowDays))
	for _, day := range SendingWindowDays {
		opens, closes := day+"_opens", day+"_closes"
		// A day is closed (both null) or open from a minute of the day to a later one, midnight
		// at its end at most.
		a.Checks["sending_windows_"+day+"_check"] = "(" + opens + " IS NULL) = (" + closes + " IS NULL) AND " +
			opens + " >= 0 AND " + opens + " < " + closes + " AND " + closes + " <= 1440"
		open = append(open, opens+" IS NOT NULL")
	}
	// A window that never opens in a week would hold its messages for ever.
	a.Checks["sending_windows_opens_check"] = strings.Join(open, " OR ")
	return []schema.Annotation{a}
}

func (SendingWindow) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "sending_windows"}}
}

func (SendingWindow) Fields() []ent.Field {
	fields := make([]ent.Field, 0, 3+2*len(SendingWindowDays))
	fields = append(fields,
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Channel the window holds.
		field.Enum("channel").
			GoType(channels.Channel("")).
			Immutable(),
		// The ISO 3166-1 alpha-2 country of the numbers the row is for; null for every number on
		// the Channel.
		field.String("country").
			Optional().
			Nillable().
			Immutable(),
	)
	for _, day := range SendingWindowDays {
		fields = append(fields,
			// The minute of the day the window opens on that day; null when it stays closed.
			field.Int(day+"_opens").
				Optional().
				Nillable(),
			// The minute of the day it closes, after it opens (1440 is midnight at the day's end).
			field.Int(day+"_closes").
				Optional().
				Nillable(),
		)
	}
	return fields
}

func (SendingWindow) Indexes() []ent.Index {
	return []ent.Index{
		// One window per Channel and country (one without a country) per Workspace: its NULL
		// country is one value (NullsNotDistinct).
		index.Fields("workspace_id", "channel", "country").
			Unique().
			StorageKey(SendingWindowIndex),
	}
}

// SendingWindowIndex is the unique index of a Workspace's Sending window per Channel and country.
const SendingWindowIndex = "sending_windows_workspace_channel_country"
