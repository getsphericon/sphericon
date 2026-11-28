// Package csvimport is the vocabulary a CSV import of Contacts stores (ADR 0138): the mapping
// of its columns, the date formats a column is read in and the reasons a value or a row is not
// written. The ent schema stores these shapes; the contacts module reads cells by them and the
// contactimports module runs the import.
package csvimport

import "github.com/getsphericon/sphericon/internal/kernel/channels"

// Kind is what a column of an import is: the kinds of the contract's ContactImportColumn union. A
// kind added by a later column (consent, refusal, time, a new Custom field) is a constant here and
// a reader of the contactimports module.
type Kind string

const (
	// KindKey fills an alias key (Column.Key: subjectId, email or phone).
	KindKey Kind = "key"
	// KindAttribute writes a core attribute (Column.Attribute).
	KindAttribute Kind = "attribute"
	// KindCustomField writes a Custom field the Workspace has (Column.Key).
	KindCustomField Kind = "customField"
	// KindNewCustomField writes a Custom field the import creates when it runs (Column.Key, Label
	// and FieldType), read in the type the Member chose.
	KindNewCustomField Kind = "newCustomField"
	// KindTags adds the Tags its cell names.
	KindTags Kind = "tags"
	// KindIgnore is not read.
	KindIgnore Kind = "ignore"
	// KindRefusal records the refusals its cells name on a Channel (Fact.Channel), at the row's
	// Destination there: an Unsubscribe or a Suppression (Fact.Refusals).
	KindRefusal Kind = "refusal"
	// KindConsent records an imported Confirmation on a Channel (Fact.Channel) for a row whose
	// cell is one of Fact.Values, with Fact.Evidence.
	KindConsent Kind = "consent"
	// KindEngagementTracking records the Engagement tracking consent its cells name on a Channel
	// (Fact.Channel): granted by Fact.Values, denied by Fact.Denied (ADR 0120).
	KindEngagementTracking Kind = "engagementTracking"
	// KindTime states when the fact of the refusal, consent or Engagement tracking column Fact.Of
	// occurred, read in Column.DateFormat (ADR 0154).
	KindTime Kind = "time"
)

// Refusal is what a value of a refusal column records: the values of the contract's
// ContactImportRefusal.
type Refusal string

const (
	// RefusalEverything records an Unsubscribe of `everything`, `via: imported`.
	RefusalEverything Refusal = "unsubscribeEverything"
	// RefusalBroadcasts records an Unsubscribe of `broadcasts` (email only), `via: imported`.
	RefusalBroadcasts Refusal = "unsubscribeBroadcasts"
	// RefusalSuppression records a Suppression of the `manual` reason.
	RefusalSuppression Refusal = "suppression"
)

// RefusalValue is a cell value of a refusal column and the refusal it records. A cell records it
// when the cell is the value, or one of the cell's items read as a list (a CSV record, as an export
// writes several opt-out scopes) is, ignoring case and the spaces around it.
type RefusalValue struct {
	Value   string  `json:"value"`
	Refusal Refusal `json:"refusal"`
}

// EvidencePart is one part of a consent column's evidence: the text given once for every row, or
// the column (counted from 0) whose cell holds it on each row. Exactly one is set.
type EvidencePart struct {
	Text   string `json:"text,omitempty"`
	Column *int   `json:"column,omitempty"`
}

// Evidence is the evidence of the consent a consent column records: the wording the person
// agreed to, where it was collected and the brand (ADR 0077).
type Evidence struct {
	Wording EvidencePart `json:"wording"`
	Source  EvidencePart `json:"source"`
	Brand   EvidencePart `json:"brand"`
}

// Column is what one column of the file is. Key names the alias key of a key column and the Custom
// field of a customField or newCustomField one; Attribute the core attribute; DateFormat how a date
// or time column writes its dates. Label and FieldType are the display name and the type (a Custom
// field type's value: string, number, bool or datetime) of the field a newCustomField column
// creates.
type Column struct {
	Kind       Kind       `json:"kind"`
	Key        string     `json:"key,omitempty"`
	Attribute  string     `json:"attribute,omitempty"`
	DateFormat DateFormat `json:"dateFormat,omitempty"`
	Label      string     `json:"label,omitempty"`
	FieldType  string     `json:"fieldType,omitempty"`
	// Fact is what a refusal, consent, Engagement tracking or time column records; nil on a
	// column of another kind.
	Fact *Fact `json:"fact,omitempty"`
}

// Fact is what a refusal, consent, Engagement tracking or time column records.
type Fact struct {
	// Channel is the Channel of a refusal, consent or Engagement tracking column: email or sms,
	// and telegram for a refusal.
	Channel channels.Channel `json:"channel,omitempty"`
	// Refusals are the values of a refusal column, each with what it records.
	Refusals []RefusalValue `json:"refusals,omitempty"`
	// Values mean consent in a consent column, and a grant in an Engagement tracking one.
	Values []string `json:"values,omitempty"`
	// Denied mean a refusal of tracking in an Engagement tracking column.
	Denied []string `json:"denied,omitempty"`
	// Evidence is a consent column's evidence.
	Evidence *Evidence `json:"evidence,omitempty"`
	// Of is the refusal, consent or Engagement tracking column a time column times, from 0.
	Of *int `json:"of,omitempty"`
}

// Mapping is how an import reads its file: a Column per column of the file, in its order, and the
// choices that hold for every row.
type Mapping struct {
	Columns []Column `json:"columns"`
	// DecimalSeparator is DecimalPoint or DecimalComma.
	DecimalSeparator DecimalSeparator `json:"decimalSeparator"`
	// DefaultCountry is the ISO 3166-1 alpha-2 country a phone without its country code is read
	// in when its row names none; "" for none.
	DefaultCountry string `json:"defaultCountry,omitempty"`
	// Tag is given to every row; "" for none.
	Tag string `json:"tag,omitempty"`
}

// DecimalSeparator is the character a file's numbers write their decimals after, by name: the
// values of the contract's ContactImportDecimalSeparator.
type DecimalSeparator string

const (
	DecimalPoint DecimalSeparator = "point"
	DecimalComma DecimalSeparator = "comma"
)

// Char is the character the separator names, "" for none of them.
func (d DecimalSeparator) Char() string {
	switch d {
	case DecimalPoint:
		return "."
	case DecimalComma:
		return ","
	}
	return ""
}

// DateFormat is how a column writes its dates: the values of the contract's ContactImportDateFormat.
type DateFormat string

const (
	DateISO          DateFormat = "yyyy-mm-dd"
	DateDMYSlash     DateFormat = "dd/mm/yyyy"
	DateDMYDot       DateFormat = "dd.mm.yyyy"
	DateDMYDash      DateFormat = "dd-mm-yyyy"
	DateMDYSlash     DateFormat = "mm/dd/yyyy"
	DateMDYDot       DateFormat = "mm.dd.yyyy"
	DateMDYDash      DateFormat = "mm-dd-yyyy"
	DateTimeRFC3339  DateFormat = "rfc3339"
	dateFormatAbsent DateFormat = ""
)

// DateFormats are the formats a date column may be read in, in the order a proposal tries them.
var DateFormats = []DateFormat{
	DateISO, DateTimeRFC3339,
	DateDMYSlash, DateDMYDot, DateDMYDash,
	DateMDYSlash, DateMDYDot, DateMDYDash,
}

// Layout is the format's layout for time.Parse, "" for a value that is no format. A day or a month
// may be written with one digit.
func (f DateFormat) Layout() string {
	switch f {
	case DateISO:
		return "2006-1-2"
	case DateDMYSlash:
		return "2/1/2006"
	case DateDMYDot:
		return "2.1.2006"
	case DateDMYDash:
		return "2-1-2006"
	case DateMDYSlash:
		return "1/2/2006"
	case DateMDYDot:
		return "1.2.2006"
	case DateMDYDash:
		return "1-2-2006"
	case DateTimeRFC3339:
		return "2006-01-02T15:04:05Z07:00"
	case dateFormatAbsent:
	}
	return ""
}

// Reason is why a value or a row of an import is not written: the values of the contract's
// ContactImportIssueReason.
type Reason string

const (
	// ReasonNoKey: the row names no alias key, so it is refused.
	ReasonNoKey Reason = "noKey"
	// ReasonCellCount: the row has another number of cells than the header, so it is refused.
	ReasonCellCount Reason = "cellCount"
	// ReasonRefused: the Contact intake refused the row; the Issue's message says why.
	ReasonRefused Reason = "refused"

	ReasonTooLong     Reason = "tooLong"
	ReasonNotNumber   Reason = "notNumber"
	ReasonNotBoolean  Reason = "notBoolean"
	ReasonNotDate     Reason = "notDate"
	ReasonNotPhone    Reason = "notPhone"
	ReasonNotEmail    Reason = "notEmail"
	ReasonNotTimeZone Reason = "notTimeZone"
	ReasonNotCountry  Reason = "notCountry"
	ReasonNotLocale   Reason = "notLocale"
	ReasonNotImageURL Reason = "notImageUrl"
	ReasonNotTag      Reason = "notTag"

	// ReasonKeysNameTwoContacts: the row's keys named two Contacts and it was written to the one
	// their rank picks; the row is reported, not refused.
	ReasonKeysNameTwoContacts Reason = "keysNameTwoContacts"

	// ReasonNoTime: a time column's cell is blank on a row whose fact it times; the fact is
	// recorded as occurring now.
	ReasonNoTime Reason = "noTime"
	// ReasonTimeInFuture: a time column's cell is later than the run; the fact is recorded as
	// occurring now.
	ReasonTimeInFuture Reason = "timeInFuture"
	// ReasonNoDestination: a refusal, consent or tracking value on a row that has no Destination
	// on its Channel; nothing is recorded for it.
	ReasonNoDestination Reason = "noDestination"
	// ReasonNoEvidence: a consent value on a row whose evidence cell is blank or too long; no
	// consent is recorded for it.
	ReasonNoEvidence Reason = "noEvidence"
	// ReasonRefusalNotRecorded: a refusal the row names could not be recorded; the message says
	// why.
	ReasonRefusalNotRecorded Reason = "refusalNotRecorded"
)

// Issue is a value or a row an import does not write, or a row it wrote with a warning, and why.
// Column counts from 0, nil for an issue of the whole row; Message says why in words.
type Issue struct {
	Column  *int   `json:"column,omitempty"`
	Reason  Reason `json:"reason"`
	Message string `json:"message"`
}
