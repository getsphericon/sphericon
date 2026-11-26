package datetrigger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// zones hold the extremes of the offsets (Kiritimati +14, American Samoa -11; -12 is the zone of
// no place), daylight-saving changes north and south, a half-hour offset and a change at midnight;
// "" is no zone, the Workspace's or UTC.
var zones = []string{"", "UTC", "Pacific/Kiritimati", "Pacific/Pago_Pago", "Europe/Berlin", "America/New_York", "Asia/Kolkata", "Australia/Lord_Howe", "America/Havana"}

// drawZone is a zone name, nil for none.
func drawZone(t *rapid.T, label string) *string {
	name := rapid.SampledFrom(zones).Draw(t, label)
	if name == "" {
		return nil
	}
	return &name
}

// drawHolder is a Contact that holds a date in the field it returns: its birthday, or a Custom
// field value in one of the ISO forms (with an offset of up to 14 hours either way, without one,
// a bare date).
func drawHolder(t *rapid.T) (datetrigger.Holder, datetrigger.Field) {
	instant := time.Unix(rapid.Int64Range(-2_000_000_000, 2_000_000_000).Draw(t, "instant"), 0)
	if rapid.Bool().Draw(t, "birthday") {
		year, month, day := instant.UTC().Date()
		birthday := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return datetrigger.Holder{Birthday: &birthday}, datetrigger.Field{Kind: datetrigger.Birthday}
	}
	offset := time.FixedZone("", rapid.IntRange(-14*4, 14*4).Draw(t, "quarterHours")*15*60)
	layout := rapid.SampledFrom([]string{time.RFC3339Nano, "2006-01-02T15:04:05", time.DateOnly}).Draw(t, "layout")
	value := instant.In(offset).Format(layout)
	return datetrigger.Holder{CustomFields: map[string]any{"renews_at": value, "plan": "pro"}},
		datetrigger.Field{Kind: datetrigger.CustomField, Key: "renews_at"}
}

// occurrenceOf is the moment of the date's occurrence in an anniversary year it has one in, for
// the holder read in its zone and the Workspace's.
func occurrenceOf(t *rapid.T, d datetrigger.Date, h datetrigger.Holder, contactZone, workspaceZone *string) time.Time {
	workspace, err := localcalendar.Zone(nil, workspaceZone)
	require.NoError(t, err)
	held, ok := d.Field.Of(h, workspace)
	require.True(t, ok, "the holder holds a date")
	year := held.Year()
	if d.Recurrence == localcalendar.Yearly {
		year += rapid.IntRange(1, 40).Draw(t, "years")
	}
	loc, err := localcalendar.Zone(contactZone, workspaceZone)
	require.NoError(t, err)
	sched, err := d.Schedule()
	require.NoError(t, err)
	at, ok := sched.Moment(held, year, loc)
	require.True(t, ok, "the year has an occurrence")
	return at
}

// The band of a window holds the key of every date with an occurrence in the window, whatever the
// zones it is read in: the database narrows by the key, the local calendar computes the moment.
func TestTheBandOfAWindowHoldsTheKeyOfEveryOccurrenceInIt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		h, field := drawHolder(t)
		d := datetrigger.Date{
			Field:      field,
			Recurrence: rapid.SampledFrom([]localcalendar.Recurrence{localcalendar.Yearly, localcalendar.Once}).Draw(t, "recurrence"),
			OffsetDays: rapid.IntRange(-365, 365).Draw(t, "offsetDays"),
			LocalTime:  fmt.Sprintf("%02d:%02d", rapid.IntRange(0, 23).Draw(t, "hour"), rapid.IntRange(0, 59).Draw(t, "minute")),
		}
		at := occurrenceOf(t, d, h, drawZone(t, "contactZone"), drawZone(t, "workspaceZone"))
		from := at.Add(-time.Duration(rapid.Int64Range(0, int64(72*time.Hour)).Draw(t, "before")))
		to := at.Add(time.Duration(rapid.Int64Range(0, int64(72*time.Hour)).Draw(t, "after")))

		key, ok := d.Field.KeyOf(h)

		require.True(t, ok)
		assert.True(t, d.Band(from, to).Holds(key), "key %s, occurrence %s", key, at)
	})
}

// The keys of a Contact are its birthday and each Custom field value that is a date, whatever the
// field's type: a value with an offset is keyed by its date in UTC, one without by its own date.
func TestAContactIsKeyedByEachDateItHolds(t *testing.T) {
	birthday := time.Date(1990, time.March, 14, 0, 0, 0, 0, time.UTC)
	h := datetrigger.Holder{Birthday: &birthday, CustomFields: map[string]any{
		"renews_at": "2027-01-01T01:00:00+03:00", "signed_up": "2026-05-01T23:30:00", "ends_on": "2026-06-30",
		"plan": "pro", "seats": 3.0, "note": "2026/01/15",
	}}

	keys := h.Keys()

	assert.Equal(t, []datetrigger.Keyed{
		{Field: datetrigger.Field{Kind: datetrigger.Birthday}, Key: localcalendar.NewDate(1990, time.March, 14)},
		{Field: datetrigger.Field{Kind: datetrigger.CustomField, Key: "ends_on"}, Key: localcalendar.NewDate(2026, time.June, 30)},
		{Field: datetrigger.Field{Kind: datetrigger.CustomField, Key: "renews_at"}, Key: localcalendar.NewDate(2026, time.December, 31)},
		{Field: datetrigger.Field{Kind: datetrigger.CustomField, Key: "signed_up"}, Key: localcalendar.NewDate(2026, time.May, 1)},
	}, keys)
}

// A band longer than a year narrows nothing; a shorter one holds the days of the window, moved
// back by the offset, by their month and day for a yearly date.
func TestABandLongerThanAYearHoldsEveryKey(t *testing.T) {
	yearly := datetrigger.Date{Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, OffsetDays: 3, LocalTime: "10:00"}
	week := yearly.Band(time.Date(2027, time.March, 14, 0, 0, 0, 0, time.UTC), time.Date(2027, time.March, 15, 0, 0, 0, 0, time.UTC))
	outage := yearly.Band(time.Date(2027, time.March, 14, 0, 0, 0, 0, time.UTC), time.Date(2034, time.March, 15, 0, 0, 0, 0, time.UTC))

	days, narrowed := week.MonthDays()
	_, outageNarrowed := outage.MonthDays()

	require.True(t, narrowed)
	assert.Equal(t, []int{308, 309, 310, 311, 312, 313, 314, 315}, days)
	assert.False(t, outageNarrowed)
	assert.True(t, outage.Holds(localcalendar.NewDate(1990, time.September, 1)))
	assert.False(t, week.Holds(localcalendar.NewDate(1990, time.September, 1)))
}

// The occurrence an Enrollment of a yearly date records reads back as its anniversary year, and
// that of a once date as no year.
func TestAnOccurrenceReadsBackAsItsAnniversaryYearOrOnce(t *testing.T) {
	for name, c := range map[string]struct {
		recurrence localcalendar.Recurrence
		year       int
		wantYear   int
		wantYearly bool
	}{
		"a yearly date's anniversary":  {recurrence: localcalendar.Yearly, year: 2027, wantYear: 2027, wantYearly: true},
		"a yearly date's first year":   {recurrence: localcalendar.Yearly, year: 1, wantYear: 1, wantYearly: true},
		"a once date's one occurrence": {recurrence: localcalendar.Once, year: 2027, wantYear: 0, wantYearly: false},
	} {
		t.Run(name, func(t *testing.T) {
			occurrence := datetrigger.Date{Recurrence: c.recurrence}.Occurrence(c.year)

			year, yearly := datetrigger.YearOf(occurrence)

			assert.Equal(t, c.wantYear, year)
			assert.Equal(t, c.wantYearly, yearly)
		})
	}
}

// Every anniversary year a yearly date records reads back as itself.
func TestEveryAnniversaryYearReadsBack(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		want := rapid.IntRange(1, 9999).Draw(t, "year")

		year, yearly := datetrigger.YearOf(datetrigger.Date{Recurrence: localcalendar.Yearly}.Occurrence(want))

		assert.Equal(t, want, year)
		assert.True(t, yearly)
	})
}
