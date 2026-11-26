package automationstep_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

func at(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	return loc
}

// trialEnds is "3 days before trial_ends_at, at 10:00", once or every year.
func trialEnds(recurrence localcalendar.Recurrence) automationstep.OnDate {
	return automationstep.OnDate{
		Field:      datetrigger.Field{Kind: datetrigger.CustomField, Key: "trial_ends_at"},
		Recurrence: recurrence, OffsetDays: -3, LocalTime: "10:00",
	}
}

// nextBirthday is "until the next birthday, at 10:00".
var nextBirthday = automationstep.OnDate{
	Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, LocalTime: "10:00",
}

func holding(trialEndsAt any) datetrigger.Holder {
	return datetrigger.Holder{CustomFields: map[string]any{"trial_ends_at": trialEndsAt}}
}

func bornOn(year int, month time.Month, day int) datetrigger.Holder {
	birthday := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return datetrigger.Holder{Birthday: &birthday}
}

// A Wait until on a date waits for its next occurrence at or after the arrival, in the Contact's
// zone, when it comes within 366 days (ADR 0126). The arrival is Friday 2027-01-15 12:00 UTC unless
// a case says otherwise; Berlin is UTC+1 in winter and UTC+2 in summer.
func TestAWaitUntilOnADateWaitsForItsNextOccurrence(t *testing.T) {
	friday := at(2027, time.January, 15, 12, 0, 0)
	cases := []struct {
		name    string
		until   automationstep.OnDate
		dates   datetrigger.Holder
		arrival time.Time
		want    time.Time
	}{
		{"3 days before a trial ending ahead", trialEnds(localcalendar.Once), holding("2027-01-20T00:00:00Z"), friday, at(2027, time.January, 17, 9, 0, 0)},
		{"this year's birthday, still ahead", nextBirthday, bornOn(1990, time.March, 1), friday, at(2027, time.March, 1, 9, 0, 0)},
		{"next year's birthday, this year's passed", nextBirthday, bornOn(1990, time.January, 10), friday, at(2028, time.January, 10, 9, 0, 0)},
		{"arriving at the very moment waits zero", nextBirthday, bornOn(1990, time.January, 15), at(2027, time.January, 15, 9, 0, 0), at(2027, time.January, 15, 9, 0, 0)},
		// 2028 is a leap year and 30 October 2028 is in winter time: the occurrence is 366 days and
		// an hour ahead, yet within 366 days on the Contact's calendar.
		{"a year ahead across a leap day and a change of clocks", nextBirthday, bornOn(1990, time.October, 30), at(2027, time.October, 30, 8, 0, 1), at(2028, time.October, 30, 9, 0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			contact := automationstep.Contact{Zone: berlin(t), Workspace: time.UTC, Dates: c.dates}

			due, err := automationstep.Reach(c.until, contact, c.arrival)

			require.NoError(t, err)
			assert.Equal(t, c.want, due.At.UTC())
			assert.Empty(t, due.WentOn)
		})
	}
}

// With no moment ahead within 366 days the Enrollment goes on at once, and why is kept: `past`,
// `no_date` (the field not held, or holding no date) or `beyond_bound`. A yearly field whose value
// lies in the future has its first anniversary more than a year away, so it is `beyond_bound`
// (decision 14 of #1613).
func TestAWaitUntilOnADateGoesOnAtOnceWithItsReason(t *testing.T) {
	friday := at(2027, time.January, 15, 12, 0, 0)
	cases := []struct {
		name  string
		until automationstep.OnDate
		dates datetrigger.Holder
		want  automationstep.WentOn
	}{
		{"a trial that ended", trialEnds(localcalendar.Once), holding("2027-01-10"), automationstep.WentOnPast},
		{"3 days before a trial ending tomorrow", trialEnds(localcalendar.Once), holding("2027-01-16"), automationstep.WentOnPast},
		{"no trial_ends_at", trialEnds(localcalendar.Once), datetrigger.Holder{}, automationstep.WentOnNoDate},
		{"a trial_ends_at that is no date", trialEnds(localcalendar.Once), holding("soon"), automationstep.WentOnNoDate},
		{"no birthday", nextBirthday, datetrigger.Holder{}, automationstep.WentOnNoDate},
		{"a trial ending two years ahead", trialEnds(localcalendar.Once), holding("2029-01-20"), automationstep.WentOnBeyondBound},
		{"a yearly field whose value lies ahead", trialEnds(localcalendar.Yearly), holding("2027-06-01"), automationstep.WentOnBeyondBound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			contact := automationstep.Contact{Zone: berlin(t), Workspace: time.UTC, Dates: c.dates}

			due, err := automationstep.Reach(c.until, contact, friday)

			require.NoError(t, err)
			assert.Equal(t, automationstep.Due{WentOn: c.want}, due)
		})
	}
}

// The time-of-day form always has a moment ahead, within a week: the next weekday at 9:00 from a
// Friday evening is Monday morning.
func TestAWaitUntilAtATimeOfDayAlwaysWaits(t *testing.T) {
	until := automationstep.TimeOfDay{LocalTime: "09:00", Weekdays: []localcalendar.Weekday{localcalendar.Monday, localcalendar.Friday}}
	contact := automationstep.Contact{Zone: berlin(t), Workspace: time.UTC}

	due, err := automationstep.Reach(until, contact, at(2027, time.January, 15, 17, 0, 0))

	require.NoError(t, err)
	assert.Equal(t, at(2027, time.January, 18, 8, 0, 0), due.At.UTC())
}
