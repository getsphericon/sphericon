package contactimports

import (
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The refusal, consent, Engagement tracking and time columns (ADR 0138): the old platform's
// refusals and the consent the Workspace collected, each with the time it occurred.

// Refusals of a mapping's refusal, consent, tracking and time columns.
var (
	// ErrValuesRequired means a refusal, consent or tracking column lists no value that means
	// anything: any other value records nothing, so a column of none would record nothing.
	ErrValuesRequired = validation.RefuseField(fieldname.Values, "contactimports: list the values that mean a refusal or consent", i18n.ErrorsContactImportValuesRequired)
	// ErrBroadcastsNotOnSMS means an Unsubscribe of `broadcasts` on SMS, where a person stops
	// everything; email and Telegram keep an opt-out of broadcasts.
	ErrBroadcastsNotOnSMS = validation.RefuseField(fieldname.Values, "contactimports: an opt-out of broadcasts is not recorded on sms", i18n.ErrorsContactImportBroadcastsNotOnSms)
	// ErrEvidencePart means a part of a consent column's evidence is neither given once nor read
	// from a column of the file, or is both.
	ErrEvidencePart = validation.RefuseField(fieldname.Evidence, "contactimports: give each part of the evidence once, or name the column that holds it", i18n.ErrorsContactImportEvidencePart)
	// ErrTimeOf means a time column times no refusal, consent or tracking column, or one another
	// time column times already.
	ErrTimeOf = validation.RefuseField(fieldname.Of, "contactimports: a time column times one refusal, consent or tracking column", i18n.ErrorsContactImportTimeOf)
)

// refusalChannels are the Channels a refusal column may name, consentChannels a consent or
// tracking column's.
var (
	refusalChannels = []channels.Channel{channels.Email, channels.SMS, channels.Telegram}
	consentChannels = []channels.Channel{channels.Email, channels.SMS}
)

// checkedFactColumn is a refusal, consent, tracking or time column as a mapping keeps it, its
// values trimmed, or its refusal. Its pairing with other columns is pairs's.
func checkedFactColumn(col *csvimport.Column) (csvimport.Column, error) {
	in := lo.FromPtr(col.Fact)
	var (
		out csvimport.Fact
		err error
	)
	switch col.Kind {
	case csvimport.KindRefusal:
		out, err = checkedRefusal(in)
	case csvimport.KindConsent:
		out, err = checkedConsent(in)
	case csvimport.KindEngagementTracking:
		out, err = checkedTracking(in)
	case csvimport.KindTime:
		if in.Of == nil {
			return *col, ErrTimeOf
		}
		if col.DateFormat.Layout() == "" {
			return *col, ErrDateFormatRequired
		}
		return csvimport.Column{Kind: col.Kind, DateFormat: col.DateFormat, Fact: &csvimport.Fact{Of: in.Of}}, nil
	case csvimport.KindKey, csvimport.KindAttribute, csvimport.KindCustomField, csvimport.KindNewCustomField, csvimport.KindTags, csvimport.KindIgnore:
		return *col, ErrUnknownTarget
	}
	if err != nil {
		return *col, err
	}
	return csvimport.Column{Kind: col.Kind, Fact: &out}, nil
}

// checkedRefusal is a refusal column's fact: its Channel, and each value non-blank and recording
// an Unsubscribe (of `broadcasts` off SMS only) or a Suppression.
func checkedRefusal(in csvimport.Fact) (csvimport.Fact, error) {
	channel := channels.Channel(in.Channel)
	if !lo.Contains(refusalChannels, channel) {
		return in, ErrUnknownTarget
	}
	if err := validation.Items(fieldname.Values, len(in.Refusals), bounds.ContactImportRefusalValuesMinItems, bounds.ContactImportRefusalValuesMaxItems); err != nil {
		return in, err
	}
	out := csvimport.Fact{Channel: in.Channel}
	for _, v := range in.Refusals {
		value := strings.TrimSpace(v.Value)
		if err := validation.Length(fieldname.Value, value, bounds.ContactImportRefusalValueMinLength, bounds.ContactImportRefusalValueMaxLength); err != nil {
			return in, err
		}
		switch v.Refusal {
		case csvimport.RefusalBroadcasts:
			switch channel {
			case channels.SMS:
				return in, ErrBroadcastsNotOnSMS
			case channels.Email, channels.Telegram:
			}
		case csvimport.RefusalEverything, csvimport.RefusalSuppression:
		default:
			return in, ErrUnknownTarget
		}
		out.Refusals = append(out.Refusals, csvimport.RefusalValue{Value: value, Refusal: v.Refusal})
	}
	return out, nil
}

// checkedConsent is a consent column's fact: its Channel, its values and its evidence, each part
// given once (within the Confirmation's bounds) or read from a column.
func checkedConsent(in csvimport.Fact) (csvimport.Fact, error) {
	if !lo.Contains(consentChannels, channels.Channel(in.Channel)) {
		return in, ErrUnknownTarget
	}
	values, err := checkedValues(in.Values, bounds.ContactImportConsentValuesMinItems, bounds.ContactImportConsentValuesMaxItems)
	if err != nil {
		return in, err
	}
	if in.Evidence == nil {
		return in, ErrEvidencePart
	}
	ev := *in.Evidence
	for _, p := range []struct {
		part  *csvimport.EvidencePart
		limit int
	}{
		{&ev.Wording, bounds.ContactImportEvidenceWordingMaxLength},
		{&ev.Source, bounds.ContactImportEvidenceSourceMaxLength},
		{&ev.Brand, bounds.ContactImportEvidenceBrandMaxLength},
	} {
		p.part.Text = strings.TrimSpace(p.part.Text)
		if (p.part.Text == "") == (p.part.Column == nil) {
			return in, ErrEvidencePart
		}
		if err := validation.MaxLength(fieldname.Evidence, p.part.Text, p.limit); err != nil {
			return in, err
		}
	}
	return csvimport.Fact{Channel: in.Channel, Values: values, Evidence: &ev}, nil
}

// checkedTracking is an Engagement tracking column's fact: its Channel and the values of a grant
// and of a refusal, at least one of them.
func checkedTracking(in csvimport.Fact) (csvimport.Fact, error) {
	if !lo.Contains(consentChannels, channels.Channel(in.Channel)) {
		return in, ErrUnknownTarget
	}
	granted, err := checkedValues(in.Values, 0, bounds.ContactImportTrackingValuesMaxItems)
	if err != nil {
		return in, err
	}
	denied, err := checkedValues(in.Denied, 0, bounds.ContactImportTrackingValuesMaxItems)
	if err != nil {
		return in, err
	}
	if len(granted)+len(denied) == 0 {
		return in, ErrValuesRequired
	}
	return csvimport.Fact{Channel: in.Channel, Values: granted, Denied: denied}, nil
}

// checkedValues are values trimmed, no fewer than least and no more than most, none blank.
func checkedValues(values []string, least, most int) ([]string, error) {
	if len(values) < least {
		return nil, ErrValuesRequired
	}
	if err := validation.Items(fieldname.Values, len(values), 0, most); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, ErrValuesRequired
		}
		out = append(out, v)
	}
	return out, nil
}

// pairs checks what the columns of a mapping name of each other: a time column times a refusal,
// consent or tracking column, no two the same one; an evidence part names a column of the file.
func pairs(columns []csvimport.Column) error {
	timed := map[int]bool{}
	for i := range columns {
		col := &columns[i]
		at := validation.Nested(fieldname.Columns, strconv.Itoa(i))
		switch col.Kind {
		case csvimport.KindTime:
			of := *col.Fact.Of
			if of < 0 || of >= len(columns) || !timesFacts(columns[of].Kind) || timed[of] {
				return ErrTimeOf.On(at)
			}
			timed[of] = true
		case csvimport.KindConsent:
			ev := col.Fact.Evidence
			for _, p := range []csvimport.EvidencePart{ev.Wording, ev.Source, ev.Brand} {
				if p.Column != nil && (*p.Column < 0 || *p.Column >= len(columns)) {
					return ErrEvidencePart.On(at)
				}
			}
		case csvimport.KindKey, csvimport.KindAttribute, csvimport.KindCustomField, csvimport.KindNewCustomField,
			csvimport.KindTags, csvimport.KindIgnore, csvimport.KindRefusal, csvimport.KindEngagementTracking:
			// Names no other column.
		}
	}
	return nil
}

// timesFacts reports whether a time column may time a column of kind.
func timesFacts(kind csvimport.Kind) bool {
	return kind == csvimport.KindRefusal || kind == csvimport.KindConsent || kind == csvimport.KindEngagementTracking
}

// recordsSuppressions reports whether a mapping records a Suppression, which needs
// suppressions:manage.
func recordsSuppressions(mapping csvimport.Mapping) bool {
	return lo.SomeBy(mapping.Columns, func(c csvimport.Column) bool {
		return c.Fact != nil && lo.SomeBy(c.Fact.Refusals, func(v csvimport.RefusalValue) bool { return v.Refusal == csvimport.RefusalSuppression })
	})
}

// mayRecord is nil when role may run a mapping: one recording a Suppression needs
// suppressions:manage (ADR 0138), checked when it is saved and when it is run.
func mayRecord(mapping csvimport.Mapping, role membership.Role) error {
	if !recordsSuppressions(mapping) {
		return nil
	}
	return memberships.Require(role, memberships.PermSuppressionsManage)
}

// refusalFact is a refusal a row records: at the row's Destination on its Channel ("" on
// Telegram, whose Destinations are the Bindings of the Contact the row writes), as it occurred.
type refusalFact struct {
	column      int
	channel     channels.Channel
	refusal     csvimport.Refusal
	destination string
	at          time.Time
}

// pendingFact is a refusal, consent or tracking value a cell named, before its Destination and
// time are known.
type pendingFact struct {
	column  int
	channel channels.Channel
	// refusal is set on a refusal, state on a tracking value; a consent sets neither.
	refusal csvimport.Refusal
	state   engagementtracking.State
}

// matches reports whether cell means value: the cell is the value, or one of the items of the
// cell read as a comma-separated list is, ignoring case and the spaces around them.
func matches(cell, value string) bool {
	if strings.EqualFold(strings.TrimSpace(cell), value) {
		return true
	}
	return lo.SomeBy(strings.Split(cell, ","), func(item string) bool { return strings.EqualFold(strings.TrimSpace(item), value) })
}

// readRefusal notes each refusal the cell means, once however many of its values mean it.
func readRefusal(r *rowState, col csvimport.Column, at int, cell string) {
	fact := lo.FromPtr(col.Fact)
	noted := map[csvimport.Refusal]bool{}
	for _, v := range fact.Refusals {
		if matches(cell, v.Value) && !noted[v.Refusal] {
			noted[v.Refusal] = true
			r.facts = append(r.facts, pendingFact{column: at, channel: channels.Channel(fact.Channel), refusal: v.Refusal})
		}
	}
}

// is reports whether cell is value as a whole, ignoring case and the spaces around it: how a consent
// or tracking value is matched, since only a refusal column reads a list of items.
func is(cell, value string) bool { return strings.EqualFold(strings.TrimSpace(cell), value) }

// readConsent notes the consent the cell means.
func readConsent(r *rowState, col csvimport.Column, at int, cell string) {
	fact := lo.FromPtr(col.Fact)
	if lo.SomeBy(fact.Values, func(v string) bool { return is(cell, v) }) {
		r.facts = append(r.facts, pendingFact{column: at, channel: channels.Channel(fact.Channel)})
	}
}

// readTracking notes the Engagement tracking consent the cell means: a refusal when it means both.
func readTracking(r *rowState, col csvimport.Column, at int, cell string) {
	fact := lo.FromPtr(col.Fact)
	means := func(v string) bool { return is(cell, v) }
	var state engagementtracking.State
	switch {
	case lo.SomeBy(fact.Denied, means):
		state = engagementtracking.Denied
	case lo.SomeBy(fact.Values, means):
		state = engagementtracking.Granted
	default:
		return
	}
	r.facts = append(r.facts, pendingFact{column: at, channel: channels.Channel(fact.Channel), state: state})
}

// settle turns the facts the row's cells noted into what it records, once its keys are read: each
// at the row's Destination on its Channel (dropped and reported when the row has none, but on
// Telegram), dated by its time column (a time blank, unreadable or later than the run dropped and
// reported, the fact then occurring now), a consent with its evidence (dropped and reported when a
// part is blank or too long).
func (rd reading) settle(r *rowState, cells []string) {
	times := map[int]time.Time{}
	for _, f := range r.facts {
		kind := rd.mapping.Columns[f.column].Kind
		destination, reachable := "", true
		switch f.channel {
		case channels.Email:
			destination = lo.FromPtr(r.in.Email)
			reachable = destination != ""
		case channels.SMS:
			destination = lo.FromPtr(r.in.Phone)
			reachable = destination != ""
		case channels.Telegram:
			// Its Destinations are the Bindings of the Contact the row writes, found at the run.
		}
		if !reachable {
			r.drop(f.column, csvimport.ReasonNoDestination)
			continue
		}
		at, ok := times[f.column]
		if !ok {
			at = rd.timeOf(r, f.column, cells)
			times[f.column] = at
		}
		switch kind {
		case csvimport.KindRefusal:
			r.refusals = append(r.refusals, refusalFact{column: f.column, channel: f.channel, refusal: f.refusal, destination: destination, at: at})
		case csvimport.KindConsent:
			rd.consent(r, f, cells, at)
		case csvimport.KindEngagementTracking:
			ob := &engagementtracking.Observation{State: f.state, OccurredAt: at}
			switch f.channel {
			case channels.Email:
				r.in.EngagementTracking.Email = ob
			case channels.SMS:
				r.in.EngagementTracking.SMS = ob
			case channels.Telegram:
				// No tracking column is of Telegram (checkedTracking).
			}
		case csvimport.KindKey, csvimport.KindAttribute, csvimport.KindCustomField, csvimport.KindNewCustomField,
			csvimport.KindTags, csvimport.KindIgnore, csvimport.KindTime:
			// Notes no fact.
		}
	}
}

// consent sets the row's consent on f's Channel, with the evidence its column gives or reads.
func (rd reading) consent(r *rowState, f pendingFact, cells []string, at time.Time) {
	ev := rd.mapping.Columns[f.column].Fact.Evidence
	part := func(p csvimport.EvidencePart, limit int) (string, bool) {
		v := p.Text
		if p.Column != nil {
			v = strings.TrimSpace(cellAt(cells, *p.Column))
		}
		return v, v != "" && len([]rune(v)) <= limit
	}
	wording, okWording := part(ev.Wording, bounds.ConsentWordingMaxLength)
	source, okSource := part(ev.Source, bounds.ConsentSourceMaxLength)
	brand, okBrand := part(ev.Brand, bounds.ConsentBrandMaxLength)
	if !okWording || !okSource || !okBrand {
		r.drop(f.column, csvimport.ReasonNoEvidence)
		return
	}
	evidence := &confirmations.Evidence{Wording: wording, Source: source, Brand: brand, OccurredAt: at}
	switch f.channel {
	case channels.Email:
		r.in.EmailConsent = evidence
	case channels.SMS:
		r.in.SMSConsent = evidence
	case channels.Telegram:
		// No consent column is of Telegram (checkedConsent).
	}
}

// timeOf is when the fact of the column at occurred, as its time column states it; the zero time
// (now) when no column times it or its cell is dropped.
func (rd reading) timeOf(r *rowState, at int, cells []string) time.Time {
	for i := range rd.mapping.Columns {
		col := &rd.mapping.Columns[i]
		if col.Kind != csvimport.KindTime || col.Fact == nil || col.Fact.Of == nil || *col.Fact.Of != at {
			continue
		}
		cell := cellAt(cells, i)
		if strings.TrimSpace(cell) == "" {
			r.drop(i, csvimport.ReasonNoTime)
			return time.Time{}
		}
		t, reason := contacts.ReadMoment(cell, col.DateFormat, rd.zone)
		switch {
		case reason != "":
			r.drop(i, reason)
		case t.After(rd.now):
			r.drop(i, csvimport.ReasonTimeInFuture)
		default:
			return t
		}
		return time.Time{}
	}
	return time.Time{}
}
