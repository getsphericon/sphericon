package contactimports

import (
	"context"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
)

// reading is what reading a file's rows needs beside the rows: the mapping, the type of each
// Custom field a column writes and the Workspace's time zone, which a date and time is dated in.
type reading struct {
	mapping csvimport.Mapping
	header  []string
	fields  map[string]customfield.Type
	zone    *time.Location
}

// readingOf is the reading of imp's file by mapping, for the Workspace of s.
func readingOf(ctx context.Context, s *ent.Scoped, header []string, mapping csvimport.Mapping) (reading, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return reading{}, err
	}
	zone, err := localcalendar.Zone(nil, ws.Timezone)
	if err != nil {
		return reading{}, err
	}
	keys := lo.FilterMap(mapping.Columns, func(c csvimport.Column, _ int) (string, bool) {
		return c.Key, c.Kind == csvimport.KindCustomField
	})
	defined, err := s.CustomField().Query().Where(customfield.KeyIn(keys...)).All(ctx)
	if err != nil {
		return reading{}, err
	}
	fields := lo.SliceToMap(defined, func(f *ent.CustomField) (string, customfield.Type) { return f.Key, f.Type })
	return reading{mapping: mapping, header: header, fields: fields, zone: zone}, nil
}

// readRow is a row of the file as read: the intake row it writes, the values it drops and, when it
// is refused before the intake sees it, why.
type readRow struct {
	in      contacts.IntakeInput
	dropped []csvimport.Issue
	refusal *csvimport.Issue
}

// rowState is a row being read: its intake row, the values it drops so far and its phone cell,
// which is read last, in the country the row names.
type rowState struct {
	in       contacts.IntakeInput
	dropped  []csvimport.Issue
	phone    string
	phoneAt  int
	hasPhone bool
	reading  reading
}

// drop records that the value of column at could not be read, and why.
func (r *rowState) drop(at int, reason csvimport.Reason) {
	r.dropped = append(r.dropped, issue(&at, reason, r.reading.header))
}

// cellReader reads one cell of a column of its kind into the row; a cell it cannot read it drops
// (rowState.drop). A kind a later column adds (consent, refusal, time, a new Custom field) is a
// reader of this table.
type cellReader func(r *rowState, col csvimport.Column, at int, cell string)

// cellReaders are the readers of the kinds of column; an ignored column reads nothing.
var cellReaders = map[csvimport.Kind]cellReader{
	csvimport.KindKey:         readKey,
	csvimport.KindAttribute:   readAttribute,
	csvimport.KindCustomField: readCustomField,
	csvimport.KindTags:        readTags,
	csvimport.KindIgnore:      func(*rowState, csvimport.Column, int, string) {},
}

// row reads the cells of a row: a row with another number of cells than the header is refused as
// it is, a blank cell is no value and changes nothing, every other cell is read by its column's
// reader, the phone last in the country the row names (else the import's default), and a row that
// names no alias key is refused. The Tag for every row is added to every row.
func (rd reading) row(cells []string) readRow {
	if len(cells) != len(rd.mapping.Columns) {
		refusal := issue(nil, csvimport.ReasonCellCount, rd.header)
		return readRow{refusal: &refusal}
	}
	state := &rowState{reading: rd}
	for at, col := range rd.mapping.Columns {
		cell := cells[at]
		if strings.TrimSpace(cell) == "" {
			continue
		}
		read, ok := cellReaders[col.Kind]
		if !ok {
			continue
		}
		read(state, col, at, cell)
	}
	if state.hasPhone {
		country := lo.CoalesceOrEmpty(lo.FromPtr(state.in.Country.Ptr()), rd.mapping.DefaultCountry)
		if v, reason := contacts.ReadPhone(state.phone, country); reason == "" {
			state.in.Phone = &v
		} else {
			state.drop(state.phoneAt, reason)
		}
	}
	if rd.mapping.Tag != "" {
		state.in.Tags = append(state.in.Tags, rd.mapping.Tag)
	}
	out := readRow{in: state.in, dropped: state.dropped}
	if state.in.SubjectID == nil && state.in.Email == nil && state.in.Phone == nil {
		refusal := issue(nil, csvimport.ReasonNoKey, rd.header)
		out.refusal = &refusal
	}
	return out
}

// readKey fills an alias key: a subject ID as written, an email read as the intake takes it, a
// phone read last.
func readKey(r *rowState, col csvimport.Column, at int, cell string) {
	switch col.Key {
	case fieldname.SubjectID:
		v := strings.TrimSpace(cell)
		r.in.SubjectID = &v
	case fieldname.Email:
		v, reason := contacts.ReadEmail(cell)
		if reason != "" {
			r.drop(at, reason)
			return
		}
		r.in.Email = &v
	case fieldname.Phone:
		r.phone, r.phoneAt, r.hasPhone = cell, at, true
	}
}

// attributeReaders read a cell of a text attribute, by the contract's name of the attribute.
var attributeReaders = map[string]struct {
	read  func(string) (string, csvimport.Reason)
	field func(*contacts.Core) *patch.Field[string]
}{
	fieldname.FirstName: {contacts.ReadText, func(c *contacts.Core) *patch.Field[string] { return &c.FirstName }},
	fieldname.LastName:  {contacts.ReadText, func(c *contacts.Core) *patch.Field[string] { return &c.LastName }},
	fieldname.City:      {contacts.ReadText, func(c *contacts.Core) *patch.Field[string] { return &c.City }},
	fieldname.TimeZone:  {contacts.ReadTimeZone, func(c *contacts.Core) *patch.Field[string] { return &c.TimeZone }},
	fieldname.Country:   {contacts.ReadCountry, func(c *contacts.Core) *patch.Field[string] { return &c.Country }},
	fieldname.Locale:    {contacts.ReadLocale, func(c *contacts.Core) *patch.Field[string] { return &c.Locale }},
	fieldname.Avatar:    {contacts.ReadImageURL, func(c *contacts.Core) *patch.Field[string] { return &c.Avatar }},
}

// readAttribute writes a core attribute: the birthday a date in the column's format, any other its
// reader's value.
func readAttribute(r *rowState, col csvimport.Column, at int, cell string) {
	if col.Attribute == fieldname.Birthday {
		v, reason := contacts.ReadDate(cell, col.DateFormat, r.reading.zone)
		if reason != "" {
			r.drop(at, reason)
			return
		}
		r.in.Birthday = patch.Set(v)
		return
	}
	a, ok := attributeReaders[col.Attribute]
	if !ok {
		return
	}
	v, reason := a.read(cell)
	if reason != "" {
		r.drop(at, reason)
		return
	}
	*a.field(&r.in.Core) = patch.Set(v)
}

// readCustomField writes a Custom field the Workspace has, read by its type.
func readCustomField(r *rowState, col csvimport.Column, at int, cell string) {
	t, ok := r.reading.fields[col.Key]
	if !ok {
		return
	}
	v, reason := contacts.ReadCustomField(t, cell, r.reading.mapping.DecimalSeparator, col.DateFormat)
	if reason != "" {
		r.drop(at, reason)
		return
	}
	if r.in.CustomFields == nil {
		r.in.CustomFields = map[string]any{}
	}
	r.in.CustomFields[col.Key] = v
}

// readTags adds the Tags a cell names.
func readTags(r *rowState, _ csvimport.Column, at int, cell string) {
	names, reason := contacts.ReadTags(cell)
	if reason != "" {
		r.drop(at, reason)
		return
	}
	r.in.Tags = append(r.in.Tags, names...)
}

// reasonMessages word each reason for the Member.
var reasonMessages = map[csvimport.Reason]i18n.ID{
	csvimport.ReasonNoKey:               i18n.ContactImportReasonNoKey,
	csvimport.ReasonCellCount:           i18n.ContactImportReasonCellCount,
	csvimport.ReasonRefused:             i18n.ContactImportReasonRefused,
	csvimport.ReasonTooLong:             i18n.ContactImportReasonTooLong,
	csvimport.ReasonNotNumber:           i18n.ContactImportReasonNotNumber,
	csvimport.ReasonNotBoolean:          i18n.ContactImportReasonNotBoolean,
	csvimport.ReasonNotDate:             i18n.ContactImportReasonNotDate,
	csvimport.ReasonNotPhone:            i18n.ContactImportReasonNotPhone,
	csvimport.ReasonNotEmail:            i18n.ContactImportReasonNotEmail,
	csvimport.ReasonNotTimeZone:         i18n.ContactImportReasonNotTimeZone,
	csvimport.ReasonNotCountry:          i18n.ContactImportReasonNotCountry,
	csvimport.ReasonNotLocale:           i18n.ContactImportReasonNotLocale,
	csvimport.ReasonNotImageURL:         i18n.ContactImportReasonNotImageUrl,
	csvimport.ReasonNotTag:              i18n.ContactImportReasonNotTag,
	csvimport.ReasonKeysNameTwoContacts: i18n.ContactImportReasonKeysNameTwoContacts,
}

// issue is a value of the column at (nil for the whole row) not written for reason, worded with
// the column's name in header.
func issue(at *int, reason csvimport.Reason, header []string) csvimport.Issue {
	column := ""
	if at != nil && *at < len(header) {
		column = header[*at]
	}
	return csvimport.Issue{Column: at, Reason: reason, Message: i18n.T(reasonMessages[reason], map[string]any{"Column": column})}
}

// refusedIssue is the row refused by the Contact intake, worded with the intake's own message.
func refusedIssue(shown string) csvimport.Issue {
	return csvimport.Issue{Reason: csvimport.ReasonRefused, Message: i18n.T(reasonMessages[csvimport.ReasonRefused], map[string]any{"Detail": shown})}
}
