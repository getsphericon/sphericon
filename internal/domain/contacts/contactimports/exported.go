package contactimports

import (
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/membership"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactsexport"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The headers a CSV export of Contacts writes beyond the contract's names (ADR 0138), which a
// proposal maps by itself so that one Workspace's export imports into another as it is: a Custom
// field's, and each Channel's state.

// exportedRefusals are the opted-out scopes of an export that a refusal column records, by Channel:
// `everything`, and `broadcasts` where an opt-out keeps that scope (email, Telegram); an opt-out of
// one Automation (its scope `automation:<id>`) means nothing in another Workspace and is not
// listed. On SMS a person stops everything (ADR 0138), so an SMS opt-out of broadcasts is recorded
// as one of everything: a refusal comes back no narrower than it was.
func exportedRefusals(channel channels.Channel) []csvimport.RefusalValue {
	broadcasts := csvimport.RefusalBroadcasts
	switch channel {
	case channels.SMS:
		broadcasts = csvimport.RefusalEverything
	case channels.Email, channels.Telegram:
	}
	return []csvimport.RefusalValue{
		{Value: eligibility.SourceEverything, Refusal: csvimport.RefusalEverything},
		{Value: eligibility.SourceBroadcasts, Refusal: broadcasts},
	}
}

// suppressionReasons are the reasons of a Suppression an export writes, every reason the schema has,
// each recorded as a Suppression (of the manual reason) in the importing Workspace.
var suppressionReasons = entsuppression.Reason("").Values()

// provenances are the provenances of a Confirmation an export writes, every provenance the schema
// has, each recorded as an imported Confirmation.
var provenances = confirmation.Provenance("").Values()

// channelPart is a header of a Channel's state: the Channel and the part of its state.
type channelPart struct {
	channel channels.Channel
	part    string
}

// channelParts are the headers of the Channels' states an export writes.
var channelParts = func() map[string]channelPart {
	parts := []string{
		contactsexport.PartState, contactsexport.PartOptedOutScopes, contactsexport.PartOptedOutAt,
		contactsexport.PartSuppressionReason, contactsexport.PartConfirmation, contactsexport.PartConfirmedAt,
	}
	out := map[string]channelPart{}
	for _, ch := range refusalChannels {
		for _, p := range parts {
			out[contactsexport.ChannelHeader(ch, p)] = channelPart{channel: ch, part: p}
		}
	}
	return out
}()

// timedPart is the part of a Channel's state whose time a header of the export states, by that
// header's part.
var timedPart = map[string]string{
	contactsexport.PartOptedOutAt:  contactsexport.PartOptedOutScopes,
	contactsexport.PartConfirmedAt: contactsexport.PartConfirmation,
}

// exported proposes the columns of an export's own headers.
type exported struct {
	// fields are the types of the Workspace's Custom fields, by key.
	fields map[string]customfield.Type
	// sep is the decimal separator the proposal reads numbers with.
	sep csvimport.DecimalSeparator
	// role is the Role of the import's Member: a column it may not save (a new Custom field without
	// custom_fields:manage, a Suppression without suppressions:manage) is not proposed.
	role membership.Role
}

// column is the column proposed for header at, whose sample cells are cells (read back as written,
// csvfile.Unguarded), and whether the header is one of an export's. A time column's Of is resolved
// by times once every column is proposed.
func (e exported) column(header string, at int, cells []string) (csvimport.Column, bool) {
	header = strings.TrimSpace(header)
	if key, ok := strings.CutPrefix(header, contactsexport.CustomFieldPrefix); ok {
		return e.customField(strings.TrimSpace(key), cells)
	}
	cp, ok := channelParts[header]
	if !ok {
		return csvimport.Column{}, false
	}
	switch cp.part {
	case contactsexport.PartOptedOutScopes:
		return csvimport.Column{Kind: csvimport.KindRefusal, Fact: &csvimport.Fact{Channel: string(cp.channel), Refusals: exportedRefusals(cp.channel)}}, true
	case contactsexport.PartSuppressionReason:
		if !memberships.Allows(e.role, memberships.PermSuppressionsManage) {
			return csvimport.Column{}, false
		}
		return csvimport.Column{Kind: csvimport.KindRefusal, Fact: &csvimport.Fact{
			Channel: string(cp.channel),
			Refusals: lo.Map(suppressionReasons, func(r string, _ int) csvimport.RefusalValue {
				return csvimport.RefusalValue{Value: r, Refusal: csvimport.RefusalSuppression}
			}),
		}}, true
	case contactsexport.PartConfirmation:
		if !lo.Contains(consentChannels, cp.channel) {
			return csvimport.Column{}, false
		}
		return csvimport.Column{Kind: csvimport.KindConsent, Fact: &csvimport.Fact{
			Channel: string(cp.channel),
			Values:  slices.Clone(provenances),
			// The provenance is how the consent was collected, the closest an export holds to where.
			// The export holds neither the wording the person was shown nor the brand they agreed to:
			// both are left blank, which the save refuses until the Member gives them, so no proof of
			// consent is made up.
			Evidence: &csvimport.Evidence{Source: csvimport.EvidencePart{Column: new(at)}},
		}}, true
	case contactsexport.PartOptedOutAt, contactsexport.PartConfirmedAt:
		return csvimport.Column{Kind: csvimport.KindTime, DateFormat: csvimport.DateTimeRFC3339, Fact: &csvimport.Fact{}}, true
	}
	return csvimport.Column{}, false
}

// customField is the column of an export's Custom field key: the Workspace's field of the key, else
// a new field of the key, labelled by it and typed as its sample reads (newFieldType); none when
// no new field may take the key or the Member's Role may not create one.
func (e exported) customField(key string, cells []string) (csvimport.Column, bool) {
	if t, ok := e.fields[key]; ok {
		col := csvimport.Column{Kind: csvimport.KindCustomField, Key: key}
		if t == customfield.TypeDatetime {
			col.DateFormat = proposedFormat(cells, csvimport.DateTimeRFC3339)
		}
		return col, true
	}
	if !memberships.Allows(e.role, memberships.PermCustomFieldsManage) || contacts.CheckNewCustomFieldKey(key, false) != nil {
		return csvimport.Column{}, false
	}
	label, err := validation.Name(key)
	if err != nil {
		return csvimport.Column{}, false
	}
	t, format := newFieldType(cells, e.sep)
	return csvimport.Column{Kind: csvimport.KindNewCustomField, Key: key, Label: label, FieldType: string(t), DateFormat: format}, true
}

// newFieldType is the type a new Custom field is proposed in from the sample of its column: a
// number when every non-blank cell reads as one and none is a code (codeLike), else a boolean, else
// a datetime in the one format they all read in, else text (also when every cell is blank).
func newFieldType(cells []string, sep csvimport.DecimalSeparator) (customfield.Type, csvimport.DateFormat) {
	filled := lo.Filter(cells, func(c string, _ int) bool { return strings.TrimSpace(c) != "" })
	reads := func(read func(string) csvimport.Reason) bool {
		return len(filled) > 0 && lo.EveryBy(filled, func(c string) bool { return read(c) == "" })
	}
	switch {
	case !lo.SomeBy(filled, codeLike) && reads(func(c string) csvimport.Reason { _, r := contacts.ReadNumber(c, sep); return r }):
		return customfield.TypeNumber, ""
	case reads(func(c string) csvimport.Reason { _, r := contacts.ReadBool(c); return r }):
		return customfield.TypeBool, ""
	}
	if f := proposedFormat(filled, ""); f != "" {
		return customfield.TypeDatetime, f
	}
	return customfield.TypeString, ""
}

// codeLike reports whether a cell that may read as a number is a code whose characters count: a
// plus sign before it (a dialling code), or a zero before another digit (a postal code, `02134`),
// which a number would lose.
func codeLike(cell string) bool {
	c := strings.TrimSpace(cell)
	return strings.HasPrefix(c, "+") || (len(c) > 1 && c[0] == '0' && c[1] >= '0' && c[1] <= '9')
}

// times points each time column of an export at the column whose part it times (by header), and
// ignores one whose column is not mapped to a refusal or consent.
func times(columns []csvimport.Column, header []string) {
	at := map[string]int{}
	for i, h := range header {
		if _, seen := at[strings.TrimSpace(h)]; !seen {
			at[strings.TrimSpace(h)] = i
		}
	}
	for i := range columns {
		col := &columns[i]
		if col.Kind != csvimport.KindTime || col.Fact == nil || col.Fact.Of != nil {
			continue
		}
		cp := channelParts[strings.TrimSpace(header[i])]
		of, ok := at[contactsexport.ChannelHeader(cp.channel, timedPart[cp.part])]
		if !ok || !timesFacts(columns[of].Kind) {
			*col = csvimport.Column{Kind: csvimport.KindIgnore}
			continue
		}
		col.Fact.Of = new(of)
	}
}
