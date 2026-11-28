package contacts

import (
	"encoding/csv"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The readers turn the text of a CSV import's cell into the value of a Contact's field (ADR 0138),
// one per field type, each in the format the Member chose for its column. Each returns a value or
// the reason it read none (a csvimport.Reason, "" when it read one); a value one cannot read is
// dropped from its row and reported, never guessed. A blank cell is no value: the caller leaves
// it out before a reader sees it, so a blank cell changes nothing.

// ReadText is a text attribute (a first or last name, a city): the cell trimmed, refused when it is
// longer than the contract takes.
func ReadText(cell string) (string, csvimport.Reason) {
	v, ok := text(cell)
	if !ok {
		return "", csvimport.ReasonTooLong
	}
	return v, ""
}

// ReadTimeZone is an IANA time zone name.
func ReadTimeZone(cell string) (string, csvimport.Reason) {
	return readWith(cell, timeZoneName, csvimport.ReasonNotTimeZone)
}

// ReadCountry is an ISO 3166-1 alpha-2 country code, in upper case.
func ReadCountry(cell string) (string, csvimport.Reason) {
	return readWith(cell, countryCode, csvimport.ReasonNotCountry)
}

// ReadLocale is a BCP 47 language tag, in its canonical form.
func ReadLocale(cell string) (string, csvimport.Reason) {
	return readWith(cell, localeTag, csvimport.ReasonNotLocale)
}

// ReadImageURL is an https address of an image (an avatar).
func ReadImageURL(cell string) (string, csvimport.Reason) {
	return readWith(cell, avatarURL, csvimport.ReasonNotImageURL)
}

func readWith(cell string, read func(string) (string, bool), reason csvimport.Reason) (string, csvimport.Reason) {
	v, ok := read(cell)
	if !ok {
		return "", reason
	}
	return v, ""
}

// ReadNumber is a number written with sep as its decimal separator: an optional sign,
// digits and at most one separator, no grouping and no exponent. A number too large to hold is
// refused, as is anything else.
func ReadNumber(cell string, sep csvimport.DecimalSeparator) (float64, csvimport.Reason) {
	s := strings.TrimSpace(cell)
	if s == "" || strings.ContainsAny(s, "eE_ ") {
		return 0, csvimport.ReasonNotNumber
	}
	other := lo.Ternary(sep == csvimport.DecimalComma, csvimport.DecimalPoint, csvimport.DecimalComma)
	if strings.Contains(s, string(other)) || strings.Count(s, string(sep)) > 1 {
		return 0, csvimport.ReasonNotNumber
	}
	s = strings.Replace(s, string(sep), string(csvimport.DecimalPoint), 1)
	digits := strings.TrimLeft(s, "+-")
	if len(s)-len(digits) > 1 || digits == "" || digits == "." || strings.ContainsFunc(digits, func(r rune) bool { return r != '.' && (r < '0' || r > '9') }) {
		return 0, csvimport.ReasonNotNumber
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) {
		return 0, csvimport.ReasonNotNumber
	}
	return v, ""
}

// booleans are the spellings ReadBool takes, in lower case.
var booleans = map[string]bool{
	"true": true, "false": false,
	"yes": true, "no": false,
	"1": true, "0": false,
	"t": true, "f": false,
	"y": true, "n": false,
}

// ReadBool is a boolean written true/false, yes/no, 1/0, t/f or y/n, in any case.
func ReadBool(cell string) (bool, csvimport.Reason) {
	v, ok := booleans[strings.ToLower(strings.TrimSpace(cell))]
	if !ok {
		return false, csvimport.ReasonNotBoolean
	}
	return v, ""
}

// ReadDate is a calendar date written in format f, as midnight UTC of that date (how a birthday is
// held). A date and time (rfc3339) gives its date in loc, the Workspace's time zone.
func ReadDate(cell string, f csvimport.DateFormat, loc *time.Location) (time.Time, csvimport.Reason) {
	t, dated, reason := parseDate(cell, f)
	if reason != "" {
		return time.Time{}, reason
	}
	if !dated {
		t = t.In(loc)
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), ""
}

// ReadMoment is when a consent fact occurred, written in format f (ADR 0154): a date and time
// (rfc3339) at its offset, a date alone at the start of that day in loc, the Workspace's time zone.
func ReadMoment(cell string, f csvimport.DateFormat, loc *time.Location) (time.Time, csvimport.Reason) {
	t, dated, reason := parseDate(cell, f)
	if reason != "" {
		return time.Time{}, reason
	}
	if dated {
		year, month, day := t.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, loc), ""
	}
	return t, ""
}

// ReadDatetime is the value of a datetime Custom field written in format f, in an ISO form the
// field's type reads (TypeReads): a date as `2006-01-02`, which is read in the Workspace time zone
// as every value without an offset is, and a date and time (rfc3339) with its offset.
func ReadDatetime(cell string, f csvimport.DateFormat) (string, csvimport.Reason) {
	t, dated, reason := parseDate(cell, f)
	if reason != "" {
		return "", reason
	}
	if dated {
		return t.Format(time.DateOnly), ""
	}
	return t.Format(time.RFC3339), ""
}

// parseDate reads cell in format f: the date it writes and whether it is a date alone. RFC 3339
// writes a date alone as its full-date (`2006-01-02`), as a datetime Custom field holds one, so
// an rfc3339 column reads one too.
func parseDate(cell string, f csvimport.DateFormat) (time.Time, bool, csvimport.Reason) {
	layout := f.Layout()
	if layout == "" {
		return time.Time{}, false, csvimport.ReasonNotDate
	}
	cell = strings.TrimSpace(cell)
	dated := f != csvimport.DateTimeRFC3339
	t, err := time.Parse(layout, cell)
	if err != nil && !dated {
		t, err = time.Parse(time.DateOnly, cell)
		dated = true
	}
	if err != nil || t.Year() < 1 {
		return time.Time{}, false, csvimport.ReasonNotDate
	}
	return t, dated, ""
}

// ReadPhone is a phone number in E.164 (ADR 0075): one written without its country code is read in
// country, the row's or the import's default, and refused when there is none.
func ReadPhone(cell, country string) (string, csvimport.Reason) {
	v, err := phone.Normalize(cell, country)
	if err != nil {
		return "", csvimport.ReasonNotPhone
	}
	return v, ""
}

// ReadEmail is an email address as the Contact intake takes it: one bare address, in its Email
// case.
func ReadEmail(cell string) (string, csvimport.Reason) {
	v, err := validation.Email(FieldEmail, cell)
	if err != nil {
		return "", csvimport.ReasonNotEmail
	}
	return v, ""
}

// ReadTags are the Tag names of a cell, written as one CSV record (comma-separated, a name holding
// a comma quoted, spaces after a comma allowed), each trimmed; a cell no CSV reader reads is refused; blank names are left out, and a name longer than a Tag takes
// refuses the cell.
func ReadTags(cell string) ([]string, csvimport.Reason) {
	r := csv.NewReader(strings.NewReader(cell))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	record, err := r.Read()
	if err != nil {
		return nil, csvimport.ReasonNotTag
	}
	var names []string
	for _, raw := range record {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		name, err := validation.Name(raw)
		if err != nil {
			return nil, csvimport.ReasonNotTag
		}
		names = append(names, name)
	}
	return names, ""
}

// ReadCustomField is the value of a Custom field of type t: a number written with sep, a boolean,
// a datetime in format f, or a string as written (trimmed). The value is of the JSON type the
// field's type reads (TypeReads).
func ReadCustomField(t customfield.Type, cell string, sep csvimport.DecimalSeparator, f csvimport.DateFormat) (any, csvimport.Reason) {
	switch t {
	case customfield.TypeNumber:
		return asAny(ReadNumber(cell, sep))
	case customfield.TypeBool:
		return asAny(ReadBool(cell))
	case customfield.TypeDatetime:
		return asAny(ReadDatetime(cell, f))
	case customfield.TypeString:
		// A string is the cell as written, below.
	}
	return strings.TrimSpace(cell), ""
}

func asAny[T any](v T, reason csvimport.Reason) (any, csvimport.Reason) {
	if reason != "" {
		return nil, reason
	}
	return v, ""
}
