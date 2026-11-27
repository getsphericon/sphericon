package contactsexport

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
)

// Column is a choice of columns of the export, the contract's ContactCsvColumn: a key or a core
// attribute is one column under its own name, the others several (Header and ChannelHeader).
type Column string

// The columns, as the contract names them.
const (
	ColumnSubjectID       Column = "subjectId"
	ColumnEmail           Column = "email"
	ColumnPhone           Column = "phone"
	ColumnFirstName       Column = "firstName"
	ColumnLastName        Column = "lastName"
	ColumnTimeZone        Column = "timeZone"
	ColumnCountry         Column = "country"
	ColumnLocale          Column = "locale"
	ColumnBirthday        Column = "birthday"
	ColumnCity            Column = "city"
	ColumnAvatar          Column = "avatar"
	ColumnSource          Column = "source"
	ColumnCreatedAt       Column = "createdAt"
	ColumnUpdatedAt       Column = "updatedAt"
	ColumnCustomFields    Column = "customFields"
	ColumnTags            Column = "tags"
	ColumnEmailChannel    Column = "emailChannel"
	ColumnSMSChannel      Column = "smsChannel"
	ColumnTelegramChannel Column = "telegramChannel"
)

// The headers of the export that are not a column's own name: a Custom field's is its key after
// CustomFieldPrefix, a Channel's six are the Channel and a part (ChannelHeader). The CSV import maps
// each by itself (ADR 0138).
const (
	// CustomFieldPrefix begins the header of a Custom field, followed by its key.
	CustomFieldPrefix = "customFields."
	// PartState is the Destination's state as a Broadcast meets it (eligibility.State): may_receive,
	// needs_confirmation, opted_out, suppressed, or "" with no Destination.
	PartState = "state"
	// PartOptedOutScopes is the scopes the Destination opted out of, one CSV record (csvfile.List):
	// everything, broadcasts, automation:<id>.
	PartOptedOutScopes = "optedOutScopes"
	// PartOptedOutAt is when the Destination last opted out: the latest of its scopes.
	PartOptedOutAt = "optedOutAt"
	// PartSuppressionReason is the reason of the Suppression standing on the Destination.
	PartSuppressionReason = "suppressionReason"
	// PartConfirmation is the provenance of the Confirmation the Destination holds.
	PartConfirmation = "confirmation"
	// PartConfirmedAt is when that consent was given.
	PartConfirmedAt = "confirmedAt"
)

// CustomFieldHeader is the header of the Custom field key.
func CustomFieldHeader(key string) string { return CustomFieldPrefix + key }

// ChannelHeader is the header of a part of a Channel's state: "email.state".
func ChannelHeader(ch channels.Channel, part string) string { return string(ch) + "." + part }

// stamp is how a time is written: RFC 3339 in UTC.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// attribute is a one-column choice: its value of a Contact.
type attribute func(c *ent.Contact) string

// attributes are the choices of one column, the header their own name.
var attributes = map[Column]attribute{
	ColumnSubjectID: func(c *ent.Contact) string { return lo.FromPtr(c.SubjectID) },
	ColumnEmail:     func(c *ent.Contact) string { return lo.FromPtr(c.Email) },
	ColumnPhone:     func(c *ent.Contact) string { return lo.FromPtr(c.Phone) },
	ColumnFirstName: func(c *ent.Contact) string { return lo.FromPtr(c.FirstName) },
	ColumnLastName:  func(c *ent.Contact) string { return lo.FromPtr(c.LastName) },
	ColumnTimeZone:  func(c *ent.Contact) string { return lo.FromPtr(c.TimeZone) },
	ColumnCountry:   func(c *ent.Contact) string { return lo.FromPtr(c.Country) },
	ColumnLocale:    func(c *ent.Contact) string { return lo.FromPtr(c.Locale) },
	ColumnBirthday: func(c *ent.Contact) string {
		if c.Birthday == nil {
			return ""
		}
		return c.Birthday.Format(time.DateOnly)
	},
	ColumnCity:      func(c *ent.Contact) string { return lo.FromPtr(c.City) },
	ColumnAvatar:    func(c *ent.Contact) string { return lo.FromPtr(c.Avatar) },
	ColumnSource:    func(c *ent.Contact) string { return string(c.Source) },
	ColumnCreatedAt: func(c *ent.Contact) string { return stamp(c.CreatedAt) },
	ColumnUpdatedAt: func(c *ent.Contact) string { return stamp(c.UpdatedAt) },
	ColumnTags: func(c *ent.Contact) string {
		return csvfile.List(lo.Map(c.Edges.Tags, func(t *ent.Tag, _ int) string { return t.Name }))
	},
}

// channelOf is the Channel of a Channel's choice.
var channelOf = map[Column]channels.Channel{
	ColumnEmailChannel:    channels.Email,
	ColumnSMSChannel:      channels.SMS,
	ColumnTelegramChannel: channels.Telegram,
}

// known reports whether the contract has the column.
func (c Column) known() bool {
	switch c {
	case ColumnSubjectID, ColumnEmail, ColumnPhone, ColumnFirstName, ColumnLastName, ColumnTimeZone,
		ColumnCountry, ColumnLocale, ColumnBirthday, ColumnCity, ColumnAvatar, ColumnSource,
		ColumnCreatedAt, ColumnUpdatedAt, ColumnTags, ColumnCustomFields,
		ColumnEmailChannel, ColumnSMSChannel, ColumnTelegramChannel:
		return true
	}
	return false
}

// cell is one column of the file: its header and its value of a Contact with its Channel states.
type cell struct {
	header string
	value  func(c *ent.Contact, states eligibility.ContactStates) string
}

// layout is the file's columns, fixed when it starts: the Custom fields are the Workspace's then.
type layout struct {
	cells  []cell
	states bool
}

// newLayout lays out the columns chosen, in their order.
func newLayout(ctx context.Context, s *ent.Scoped, cols []Column) (layout, error) {
	var l layout
	for _, col := range cols {
		switch col {
		case ColumnSubjectID, ColumnEmail, ColumnPhone, ColumnFirstName, ColumnLastName, ColumnTimeZone,
			ColumnCountry, ColumnLocale, ColumnBirthday, ColumnCity, ColumnAvatar, ColumnSource,
			ColumnCreatedAt, ColumnUpdatedAt, ColumnTags:
			attr := attributes[col]
			l.cells = append(l.cells, cell{header: string(col), value: func(c *ent.Contact, _ eligibility.ContactStates) string { return attr(c) }})
		case ColumnEmailChannel, ColumnSMSChannel, ColumnTelegramChannel:
			l.states = true
			l.cells = append(l.cells, channelCells(channelOf[col])...)
		case ColumnCustomFields:
			var keys []string
			if err := s.CustomField().Query().Order(ent.Asc(customfield.FieldKey)).Select(customfield.FieldKey).Scan(ctx, &keys); err != nil {
				return layout{}, err
			}
			for _, key := range keys {
				l.cells = append(l.cells, cell{header: CustomFieldHeader(key), value: func(c *ent.Contact, _ eligibility.ContactStates) string {
					return fieldValue(c.CustomFields[key])
				}})
			}
		}
	}
	return l, nil
}

func (l layout) header() []string {
	return lo.Map(l.cells, func(c cell, _ int) string { return c.header })
}

func (l layout) needsStates() bool { return l.states }

func (l layout) row(c *ent.Contact, states eligibility.ContactStates) []string {
	return lo.Map(l.cells, func(cl cell, _ int) string { return cl.value(c, states) })
}

// stateOn is a Contact's state on the Channel.
func stateOn(states eligibility.ContactStates, ch channels.Channel) eligibility.ChannelState {
	switch ch {
	case channels.Email:
		return states.Email
	case channels.SMS:
		return states.SMS
	case channels.Telegram:
		return states.Telegram
	}
	return eligibility.ChannelState{}
}

// channelCells are the six columns of a Channel's state.
func channelCells(ch channels.Channel) []cell {
	part := func(name string, value func(st eligibility.ChannelState) string) cell {
		return cell{header: ChannelHeader(ch, name), value: func(_ *ent.Contact, states eligibility.ContactStates) string {
			return value(stateOn(states, ch))
		}}
	}
	return []cell{
		part(PartState, func(st eligibility.ChannelState) string { return string(st.State) }),
		part(PartOptedOutScopes, func(st eligibility.ChannelState) string {
			return csvfile.List(lo.Map(st.OptOuts, func(o eligibility.OptOut, _ int) string { return o.Scope }))
		}),
		part(PartOptedOutAt, func(st eligibility.ChannelState) string {
			if len(st.OptOuts) == 0 {
				return ""
			}
			latest := slices.MaxFunc(st.OptOuts, func(a, b eligibility.OptOut) int { return a.At.Compare(b.At) })
			return stamp(latest.At)
		}),
		part(PartSuppressionReason, func(st eligibility.ChannelState) string {
			if st.Suppression == nil {
				return ""
			}
			return string(st.Suppression.Reason)
		}),
		part(PartConfirmation, func(st eligibility.ChannelState) string {
			if st.Confirmation == nil {
				return ""
			}
			return string(st.Confirmation.Provenance)
		}),
		part(PartConfirmedAt, func(st eligibility.ChannelState) string {
			if st.Confirmation == nil {
				return ""
			}
			return stamp(st.Confirmation.At)
		}),
	}
}

// fieldValue is a Custom field's value as text: a string as it is, a number in its shortest form,
// a boolean as true or false, anything else as its JSON, none as "".
func fieldValue(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
