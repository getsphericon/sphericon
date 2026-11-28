package contacts_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
)

// A number written with either separator reads back as the number it writes, whatever its sign
// and digits; the other separator is no number.
func TestANumberReadsWithItsSeparator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		whole := rapid.Int64Range(0, 1_000_000_000).Draw(t, "whole")
		cents := rapid.IntRange(0, 99).Draw(t, "cents")
		sep := rapid.SampledFrom([]csvimport.DecimalSeparator{csvimport.DecimalPoint, csvimport.DecimalComma}).Draw(t, "sep")
		sign := rapid.SampledFrom([]string{"", "-", "+"}).Draw(t, "sign")
		cell := fmt.Sprintf("%s%d%s%02d", sign, whole, sep, cents)
		want, err := strconv.ParseFloat(strings.Replace(cell, string(sep), ".", 1), 64)
		require.NoError(t, err)

		got, reason := contacts.ReadNumber(cell, sep)
		other := map[csvimport.DecimalSeparator]csvimport.DecimalSeparator{csvimport.DecimalPoint: csvimport.DecimalComma, csvimport.DecimalComma: csvimport.DecimalPoint}[sep]
		_, otherReason := contacts.ReadNumber(cell, other)

		assert.Empty(t, reason)
		assert.InDelta(t, want, got, 0)
		assert.Equal(t, csvimport.ReasonNotNumber, otherReason)
	})
}

func TestTextThatIsNoNumberIsNotRead(t *testing.T) {
	for _, cell := range []string{"", "abc", "1.2.3", "1e5", "1 000", "--1", "+", ".", "Inf", "NaN", "0x10"} {
		_, reason := contacts.ReadNumber(cell, csvimport.DecimalPoint)
		assert.Equal(t, csvimport.ReasonNotNumber, reason, cell)
	}
}

// Each spelling of a boolean reads in any case; the rest is no boolean.
func TestABooleanReadsInEverySpellingAndCase(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pair := rapid.SampledFrom([][2]string{{"true", "false"}, {"yes", "no"}, {"1", "0"}, {"t", "f"}, {"y", "n"}}).Draw(t, "pair")
		upper := rapid.Bool().Draw(t, "upper")
		cased := func(s string) string {
			if upper {
				return strings.ToUpper(s)
			}
			return s
		}

		yes, yesReason := contacts.ReadBool(" " + cased(pair[0]) + " ")
		no, noReason := contacts.ReadBool(cased(pair[1]))

		assert.Empty(t, yesReason)
		assert.Empty(t, noReason)
		assert.True(t, yes)
		assert.False(t, no)
	})
	_, reason := contacts.ReadBool("maybe")
	assert.Equal(t, csvimport.ReasonNotBoolean, reason)
}

// A date written in any of the date formats reads back as that date, and a format with the day and
// the month the other way round does not read a day past the twelfth.
func TestADateReadsInItsFormat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		day := time.Date(rapid.IntRange(1900, 2100).Draw(t, "year"), time.Month(rapid.IntRange(1, 12).Draw(t, "month")), rapid.IntRange(13, 28).Draw(t, "day"), 0, 0, 0, 0, time.UTC)
		formats := map[csvimport.DateFormat]string{
			csvimport.DateISO:      "2006-01-02",
			csvimport.DateDMYSlash: "02/01/2006", csvimport.DateDMYDot: "02.01.2006", csvimport.DateDMYDash: "02-01-2006",
			csvimport.DateMDYSlash: "01/02/2006", csvimport.DateMDYDot: "01.02.2006", csvimport.DateMDYDash: "01-02-2006",
		}
		swapped := map[csvimport.DateFormat]csvimport.DateFormat{
			csvimport.DateISO:      csvimport.DateMDYSlash,
			csvimport.DateDMYSlash: csvimport.DateMDYSlash, csvimport.DateDMYDot: csvimport.DateMDYDot, csvimport.DateDMYDash: csvimport.DateMDYDash,
			csvimport.DateMDYSlash: csvimport.DateDMYSlash, csvimport.DateMDYDot: csvimport.DateDMYDot, csvimport.DateMDYDash: csvimport.DateDMYDash,
		}
		f := rapid.SampledFrom([]csvimport.DateFormat{csvimport.DateISO, csvimport.DateDMYSlash, csvimport.DateDMYDot, csvimport.DateDMYDash, csvimport.DateMDYSlash, csvimport.DateMDYDot, csvimport.DateMDYDash}).Draw(t, "format")
		cell := day.Format(formats[f])

		got, reason := contacts.ReadDate(cell, f, time.UTC)
		stored, storedReason := contacts.ReadDatetime(cell, f)
		_, swappedReason := contacts.ReadDate(cell, swapped[f], time.UTC)

		assert.Empty(t, reason)
		assert.Equal(t, day, got)
		assert.Empty(t, storedReason)
		assert.Equal(t, day.Format(time.DateOnly), stored)
		assert.True(t, contacts.TypeReads(customfield.TypeDatetime, stored), "a datetime field reads %q", stored)
		assert.Equal(t, csvimport.ReasonNotDate, swappedReason)
	})
}

// A date and time keeps its offset as a datetime field's value, and as a date it is the date of
// its instant in the Workspace's time zone.
func TestADateAndTimeIsDatedInTheWorkspaceZone(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	birthday, reason := contacts.ReadDate("2024-03-04T20:00:00Z", csvimport.DateTimeRFC3339, tokyo)
	value, valueReason := contacts.ReadDatetime("2024-03-04T20:00:00-05:00", csvimport.DateTimeRFC3339)

	assert.Empty(t, reason)
	assert.Equal(t, time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC), birthday)
	assert.Empty(t, valueReason)
	assert.Equal(t, "2024-03-04T20:00:00-05:00", value)
}

// The time a consent fact occurred is an instant: a date and time at its offset, a date alone at
// the start of that day in the Workspace's time zone.
func TestAMomentIsAnInstantDatedInTheWorkspaceZone(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	stamped, stampedReason := contacts.ReadMoment("2024-03-04T20:00:00-05:00", csvimport.DateTimeRFC3339, tokyo)
	day, dayReason := contacts.ReadMoment("04/03/2024", csvimport.DateDMYSlash, tokyo)
	_, badReason := contacts.ReadMoment("yesterday", csvimport.DateISO, tokyo)

	assert.Empty(t, stampedReason)
	assert.True(t, time.Date(2024, 3, 5, 1, 0, 0, 0, time.UTC).Equal(stamped))
	assert.Empty(t, dayReason)
	assert.True(t, time.Date(2024, 3, 3, 15, 0, 0, 0, time.UTC).Equal(day))
	assert.Equal(t, csvimport.ReasonNotDate, badReason)
}

// RFC 3339 writes a date alone as its full-date (as a datetime Custom field holds one), so a column
// in RFC 3339 reads a date alone as a date, in the Workspace's time zone as ISO reads it.
func TestARFC3339ColumnReadsADateAlone(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	value, valueReason := contacts.ReadDatetime("2024-03-04", csvimport.DateTimeRFC3339)
	birthday, birthdayReason := contacts.ReadDate("2024-03-04", csvimport.DateTimeRFC3339, tokyo)
	stamped, stampedReason := contacts.ReadMoment("2024-03-04", csvimport.DateTimeRFC3339, tokyo)

	assert.Empty(t, valueReason)
	assert.Equal(t, "2024-03-04", value)
	assert.Empty(t, birthdayReason)
	assert.Equal(t, time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC), birthday)
	assert.Empty(t, stampedReason)
	assert.True(t, time.Date(2024, 3, 4, 0, 0, 0, 0, tokyo).Equal(stamped))
}

func TestADateNotInItsFormatIsNotRead(t *testing.T) {
	for _, c := range []struct {
		cell   string
		format csvimport.DateFormat
	}{
		{"31/02/2024", csvimport.DateDMYSlash},
		{"2024-03-04", csvimport.DateDMYSlash},
		{"04.03.24", csvimport.DateDMYDot},
		{"2024-3-4", csvimport.DateTimeRFC3339},
		{"yesterday", csvimport.DateISO},
		{"2024-03-04", ""},
	} {
		_, reason := contacts.ReadDate(c.cell, c.format, time.UTC)
		assert.Equal(t, csvimport.ReasonNotDate, reason, c.cell)
	}
}

func TestAPhoneReadsWithTheRowsCountry(t *testing.T) {
	international, reason := contacts.ReadPhone("+44 20 7946 0958", "")
	national, nationalReason := contacts.ReadPhone("020 7946 0958", "GB")
	_, noCountry := contacts.ReadPhone("020 7946 0958", "")
	_, garbage := contacts.ReadPhone("call me", "GB")

	assert.Empty(t, reason)
	assert.Equal(t, "+442079460958", international)
	assert.Empty(t, nationalReason)
	assert.Equal(t, "+442079460958", national)
	assert.Equal(t, csvimport.ReasonNotPhone, noCountry)
	assert.Equal(t, csvimport.ReasonNotPhone, garbage)
}

func TestAnEmailReadsAsTheIntakeTakesIt(t *testing.T) {
	address, reason := contacts.ReadEmail(" Alice@Example.COM ")
	_, notAddress := contacts.ReadEmail("alice at example")

	assert.Empty(t, reason)
	assert.Equal(t, "alice@example.com", address)
	assert.Equal(t, csvimport.ReasonNotEmail, notAddress)
}

// A Tags cell is one CSV record of names: a quoted name keeps its comma, blanks are left out.
func TestATagsCellIsOneRecordOfNames(t *testing.T) {
	names, reason := contacts.ReadTags(`vip, "Mailchimp, March",, newsletter`)
	_, tooLong := contacts.ReadTags(strings.Repeat("x", 101))

	assert.Empty(t, reason)
	assert.Equal(t, []string{"vip", "Mailchimp, March", "newsletter"}, names)
	assert.Equal(t, csvimport.ReasonNotTag, tooLong)
}

// Whatever its type, a Custom field's value as read is one its type reads, or none with a reason.
func TestACustomFieldValueIsOfItsType(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		typ := rapid.SampledFrom([]customfield.Type{customfield.TypeNumber, customfield.TypeBool, customfield.TypeDatetime, customfield.TypeString}).Draw(t, "type")
		cell := rapid.SampledFrom([]string{"42", "4,2", "yes", "2024-03-04", "04/03/2024", "2024-03-04T10:00:00Z", "hello"}).Draw(t, "cell")
		format := rapid.SampledFrom(csvimport.DateFormats).Draw(t, "format")

		value, reason := contacts.ReadCustomField(typ, cell, csvimport.DecimalComma, format)
		read := reason == "" && contacts.TypeReads(typ, value)
		dropped := reason != "" && value == nil

		assert.True(t, read || dropped, "%s %q: %v %q", typ, cell, value, reason)
	})
}

func FuzzAReaderReturnsAValueOrAReason(f *testing.F) {
	for _, seed := range []string{"", "42", "4,2", "true", "2024-03-04", "04/03/2024", "+44 20 7946 0958", "a@b.co", `"x,y",z`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, cell string) {
		_, numberReason := contacts.ReadNumber(cell, csvimport.DecimalComma)
		_, dateReason := contacts.ReadDate(cell, csvimport.DateDMYSlash, time.UTC)
		datetime, datetimeReason := contacts.ReadDatetime(cell, csvimport.DateTimeRFC3339)
		tags, tagsReason := contacts.ReadTags(cell)

		assert.Contains(t, []csvimport.Reason{"", csvimport.ReasonNotNumber}, numberReason)
		assert.Contains(t, []csvimport.Reason{"", csvimport.ReasonNotDate}, dateReason)
		assert.True(t, datetimeReason != "" || contacts.TypeReads(customfield.TypeDatetime, datetime), "%q reads as %q", cell, datetime)
		assert.True(t, tagsReason == "" || tags == nil)
	})
}
