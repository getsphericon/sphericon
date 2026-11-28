package contactimports

import (
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/customfield"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactsexport"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The headers a CSV export of Contacts writes beyond the contract's names (ADR 0138), which a
// proposal maps by itself so that one Workspace's export imports into another as it is: a Custom
// field's, and each Channel's state.

// ExportedConsentWording is the consent wording a proposal gives once for the Confirmations an export
// carries: the export holds how each was confirmed, not what the person was shown. The Member may
// change it.
const ExportedConsentWording = "Consent given before this file was exported from another Workspace"

// exportedRefusals are the opted-out scopes of an export that a refusal column records, by Channel:
// `everything`, and `broadcasts` on email; an opt-out of one Automation (its scope
// `automation:<id>`) means nothing in another Workspace and is not listed. Off email a person stops
// everything (ADR 0138), so an SMS or Telegram opt-out of broadcasts is recorded as one of
// everything: a refusal comes back no narrower than it was.
func exportedRefusals(channel channels.Channel) []csvimport.RefusalValue {
	if channel == channels.Email {
		return []csvimport.RefusalValue{
			{Value: eligibility.SourceEverything, Refusal: csvimport.RefusalEverything},
			{Value: eligibility.SourceBroadcasts, Refusal: csvimport.RefusalBroadcasts},
		}
	}
	return []csvimport.RefusalValue{
		{Value: eligibility.SourceEverything, Refusal: csvimport.RefusalEverything},
		{Value: eligibility.SourceBroadcasts, Refusal: csvimport.RefusalEverything},
	}
}

// suppressionReasons are the reasons of a Suppression an export writes, each recorded as a
// Suppression (of the manual reason) in the importing Workspace.
var suppressionReasons = []entsuppression.Reason{entsuppression.ReasonBounce, entsuppression.ReasonComplaint, entsuppression.ReasonManual}

// provenances are the provenances of a Confirmation an export writes, each recorded as an imported
// Confirmation.
var provenances = []confirmation.Provenance{
	confirmation.ProvenanceDoubleOptIn, confirmation.ProvenanceGrandfathered,
	confirmation.ProvenanceImported, confirmation.ProvenanceKeyword,
}

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
	// brand is the brand the Confirmations an export carries were given to: the Workspace's name.
	brand string
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
		return csvimport.Column{Kind: csvimport.KindRefusal, Fact: &csvimport.Fact{
			Channel: string(cp.channel),
			Refusals: lo.Map(suppressionReasons, func(r entsuppression.Reason, _ int) csvimport.RefusalValue {
				return csvimport.RefusalValue{Value: string(r), Refusal: csvimport.RefusalSuppression}
			}),
		}}, true
	case contactsexport.PartConfirmation:
		if !lo.Contains(consentChannels, cp.channel) {
			return csvimport.Column{}, false
		}
		return csvimport.Column{Kind: csvimport.KindConsent, Fact: &csvimport.Fact{
			Channel: string(cp.channel),
			Values:  lo.Map(provenances, func(p confirmation.Provenance, _ int) string { return string(p) }),
			// The provenance is how the consent was collected, the closest an export holds to where.
			Evidence: &csvimport.Evidence{
				Wording: csvimport.EvidencePart{Text: ExportedConsentWording},
				Source:  csvimport.EvidencePart{Column: lo.ToPtr(at)},
				Brand:   csvimport.EvidencePart{Text: e.brand},
			},
		}}, true
	case contactsexport.PartOptedOutAt, contactsexport.PartConfirmedAt:
		return csvimport.Column{Kind: csvimport.KindTime, DateFormat: csvimport.DateTimeRFC3339, Fact: &csvimport.Fact{}}, true
	}
	return csvimport.Column{}, false
}

// customField is the column of an export's Custom field key: the Workspace's field of the key, else
// a new field of the key, labelled by it and typed as its sample reads (newFieldType); none when
// no new field may take the key.
func (e exported) customField(key string, cells []string) (csvimport.Column, bool) {
	if t, ok := e.fields[key]; ok {
		col := csvimport.Column{Kind: csvimport.KindCustomField, Key: key}
		if t == customfield.TypeDatetime {
			col.DateFormat = proposedFormat(cells, csvimport.DateTimeRFC3339)
		}
		return col, true
	}
	if contacts.CheckNewCustomFieldKey(key, false) != nil {
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
// number when every non-blank cell reads as one, else a boolean, else a datetime in the one format
// they all read in, else text (also when every cell is blank).
func newFieldType(cells []string, sep csvimport.DecimalSeparator) (customfield.Type, csvimport.DateFormat) {
	filled := lo.Filter(cells, func(c string, _ int) bool { return strings.TrimSpace(c) != "" })
	reads := func(read func(string) csvimport.Reason) bool {
		return len(filled) > 0 && lo.EveryBy(filled, func(c string) bool { return read(c) == "" })
	}
	switch {
	case reads(func(c string) csvimport.Reason { _, r := contacts.ReadNumber(c, sep); return r }):
		return customfield.TypeNumber, ""
	case reads(func(c string) csvimport.Reason { _, r := contacts.ReadBool(c); return r }):
		return customfield.TypeBool, ""
	}
	if f := proposedFormat(filled, ""); f != "" {
		return customfield.TypeDatetime, f
	}
	return customfield.TypeString, ""
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
		col.Fact.Of = lo.ToPtr(of)
	}
}
