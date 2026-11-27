// Package csvimport is the vocabulary a CSV import of Contacts stores (ADR 0138): the mapping
// of its columns, the date formats a column is read in and the reasons a value or a row is not
// written. The ent schema stores these shapes; the contacts module reads cells by them and the
// contactimports module runs the import.
package csvimport

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
)

// Column is what one column of the file is. Key names the alias key of a key column and the Custom
// field of a customField or newCustomField one; Attribute the core attribute; DateFormat how a date
// column writes its dates. Label and FieldType are the display name and the type (a Custom field
// type's value: string, number, bool or datetime) of the field a newCustomField column creates.
type Column struct {
	Kind       Kind       `json:"kind"`
	Key        string     `json:"key,omitempty"`
	Attribute  string     `json:"attribute,omitempty"`
	DateFormat DateFormat `json:"dateFormat,omitempty"`
	Label      string     `json:"label,omitempty"`
	FieldType  string     `json:"fieldType,omitempty"`
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

// DecimalSeparator is the character a file's numbers write their decimals after: the values of
// the contract's ContactImportDecimalSeparator.
type DecimalSeparator string

const (
	DecimalPoint DecimalSeparator = "."
	DecimalComma DecimalSeparator = ","
)

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
)

// Issue is a value or a row an import does not write, or a row it wrote with a warning, and why.
// Column counts from 0, nil for an issue of the whole row; Message says why in words.
type Issue struct {
	Column  *int   `json:"column,omitempty"`
	Reason  Reason `json:"reason"`
	Message string `json:"message"`
}
