package localcalendar_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// testingT is a *testing.T or a *rapid.T.
type testingT interface {
	require.TestingT
	Helper()
}

func zone(t testingT, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func schedule(t testingT, recurrence localcalendar.Recurrence, offsetDays int, localTime string) localcalendar.Schedule {
	t.Helper()
	s, err := localcalendar.NewSchedule(recurrence, offsetDays, localTime)
	require.NoError(t, err)
	return s
}

func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

// A local time is the wall-clock time in the zone. One a daylight-saving change skips is the first
// moment after the gap; one that occurs twice is the first of the two (ADR 0126). The transitions
// are tzdata's: Berlin 2027-03-28 and 2027-10-31 at 01:00 UTC, New York 2027-03-14 and 2027-11-07
// at 07:00 and 06:00 UTC.
func TestAnOccurrenceIsTheLocalTimeOnItsDayInTheZone(t *testing.T) {
	cases := []struct {
		name      string
		zone      string
		date      localcalendar.Date
		localTime string
		year      int
		want      time.Time
	}{
		{"summer in Berlin", "Europe/Berlin", localcalendar.NewDate(1990, time.May, 10), "10:00", 2027, utc(2027, time.May, 10, 8, 0)},
		{"winter in New York", "America/New_York", localcalendar.NewDate(1990, time.January, 15), "09:30", 2027, utc(2027, time.January, 15, 14, 30)},
		{"spring-forward gap in Berlin", "Europe/Berlin", localcalendar.NewDate(1990, time.March, 28), "02:30", 2027, utc(2027, time.March, 28, 1, 0)},
		{"fall-back overlap in Berlin", "Europe/Berlin", localcalendar.NewDate(1990, time.October, 31), "02:30", 2027, utc(2027, time.October, 31, 0, 30)},
		{"spring-forward gap in New York", "America/New_York", localcalendar.NewDate(1990, time.March, 14), "02:30", 2027, utc(2027, time.March, 14, 7, 0)},
		{"fall-back overlap in New York", "America/New_York", localcalendar.NewDate(1990, time.November, 7), "01:30", 2027, utc(2027, time.November, 7, 5, 30)},
		{"the gap's own start in New York", "America/New_York", localcalendar.NewDate(1990, time.March, 14), "02:00", 2027, utc(2027, time.March, 14, 7, 0)},
		{"just after the gap in New York", "America/New_York", localcalendar.NewDate(1990, time.March, 14), "03:00", 2027, utc(2027, time.March, 14, 7, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := schedule(t, localcalendar.Yearly, 0, c.localTime)
			loc := zone(t, c.zone)

			got, ok := s.Moment(c.date, c.year, loc)

			require.True(t, ok)
			assert.WithinDuration(t, c.want, got, 0)
		})
	}
}

// 29 February is 28 February in a year without it, so the occurrence stays in its month; an offset
// counts calendar days from that day, and the anniversary year is the year of the day it counts from.
func TestAnOccurrenceCountsItsOffsetFromTheDayInItsAnniversaryYear(t *testing.T) {
	cases := []struct {
		name       string
		date       localcalendar.Date
		offsetDays int
		year       int
		want       time.Time
	}{
		{"29 February in 2027", localcalendar.NewDate(2000, time.February, 29), 0, 2027, utc(2027, time.February, 28, 10, 0)},
		{"29 February in 2028", localcalendar.NewDate(2000, time.February, 29), 0, 2028, utc(2028, time.February, 29, 10, 0)},
		{"a day after 29 February in 2027", localcalendar.NewDate(2000, time.February, 29), 1, 2027, utc(2027, time.March, 1, 10, 0)},
		{"3 days before 2 January", localcalendar.NewDate(1990, time.January, 2), -3, 2027, utc(2026, time.December, 30, 10, 0)},
		{"3 days after 30 December", localcalendar.NewDate(1990, time.December, 30), 3, 2026, utc(2027, time.January, 2, 10, 0)},
		{"a year after", localcalendar.NewDate(1990, time.June, 1), 365, 2026, utc(2027, time.June, 1, 10, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := schedule(t, localcalendar.Yearly, c.offsetDays, "10:00")

			got, ok := s.Moment(c.date, c.year, time.UTC)

			require.True(t, ok)
			assert.WithinDuration(t, c.want, got, 0)
		})
	}
}

// A yearly date's own day is no occurrence: its first is one year after it (decision 13). A once
// date's one occurrence is in its own year.
func TestAnOccurrenceIsOnlyInAnAnniversaryYearOfItsRecurrence(t *testing.T) {
	date := localcalendar.NewDate(2026, time.March, 10)
	held := []struct {
		name       string
		recurrence localcalendar.Recurrence
		year       int
		want       time.Time
	}{
		{"yearly, one year after", localcalendar.Yearly, 2027, utc(2027, time.March, 10, 10, 0)},
		{"once, its own year", localcalendar.Once, 2026, utc(2026, time.March, 10, 10, 0)},
	}
	for _, c := range held {
		t.Run(c.name, func(t *testing.T) {
			s := schedule(t, c.recurrence, 0, "10:00")

			got, ok := s.Moment(date, c.year, time.UTC)

			require.True(t, ok)
			assert.WithinDuration(t, c.want, got, 0)
		})
	}
	none := []struct {
		name       string
		recurrence localcalendar.Recurrence
		year       int
	}{
		{"yearly, its own year", localcalendar.Yearly, 2026},
		{"yearly, a year before", localcalendar.Yearly, 2025},
		{"once, a year after", localcalendar.Once, 2027},
	}
	for _, c := range none {
		t.Run(c.name, func(t *testing.T) {
			s := schedule(t, c.recurrence, 0, "10:00")

			_, ok := s.Moment(date, c.year, time.UTC)

			assert.False(t, ok)
		})
	}
}

// A Wait until's date form waits for the next occurrence at or after the arrival: one equal to the
// arrival is ahead (decision 11), and a yearly date's own day is not its first (decision 13).
func TestTheNextOccurrenceIsTheFirstAtOrAfterTheArrival(t *testing.T) {
	cases := []struct {
		name       string
		recurrence localcalendar.Recurrence
		date       localcalendar.Date
		offsetDays int
		arrival    time.Time
		want       localcalendar.Occurrence
	}{
		{"later this year", localcalendar.Yearly, localcalendar.NewDate(1990, time.May, 10), 0, utc(2027, time.March, 1, 0, 0),
			localcalendar.Occurrence{Year: 2027, At: utc(2027, time.May, 10, 10, 0)}},
		{"arriving at the moment", localcalendar.Yearly, localcalendar.NewDate(1990, time.May, 10), 0, utc(2027, time.May, 10, 10, 0),
			localcalendar.Occurrence{Year: 2027, At: utc(2027, time.May, 10, 10, 0)}},
		{"a minute after this year's", localcalendar.Yearly, localcalendar.NewDate(1990, time.May, 10), 0, utc(2027, time.May, 10, 10, 1),
			localcalendar.Occurrence{Year: 2028, At: utc(2028, time.May, 10, 10, 0)}},
		{"3 days before 2 January, in December", localcalendar.Yearly, localcalendar.NewDate(1990, time.January, 2), -3, utc(2026, time.December, 29, 0, 0),
			localcalendar.Occurrence{Year: 2027, At: utc(2026, time.December, 30, 10, 0)}},
		{"a year after, counted from last year", localcalendar.Yearly, localcalendar.NewDate(1990, time.January, 5), 365, utc(2027, time.January, 1, 0, 0),
			localcalendar.Occurrence{Year: 2026, At: utc(2027, time.January, 5, 10, 0)}},
		{"a yearly date's own day", localcalendar.Yearly, localcalendar.NewDate(2026, time.March, 10), 0, utc(2026, time.March, 10, 9, 0),
			localcalendar.Occurrence{Year: 2027, At: utc(2027, time.March, 10, 10, 0)}},
		{"29 February in a year without it", localcalendar.Yearly, localcalendar.NewDate(2000, time.February, 29), 0, utc(2027, time.January, 1, 0, 0),
			localcalendar.Occurrence{Year: 2027, At: utc(2027, time.February, 28, 10, 0)}},
		{"once, ahead", localcalendar.Once, localcalendar.NewDate(2027, time.January, 15), -3, utc(2027, time.January, 1, 0, 0),
			localcalendar.Occurrence{Year: 2027, At: utc(2027, time.January, 12, 10, 0)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := schedule(t, c.recurrence, c.offsetDays, "10:00")

			got, ok := s.Next(c.date, c.arrival, time.UTC)

			require.True(t, ok)
			assert.Equal(t, c.want.Year, got.Year)
			assert.WithinDuration(t, c.want.At, got.At, 0)
		})
	}
}

// A once date whose occurrence has passed has no next one: the Wait until goes on as past.
func TestAOnceDateHasNoNextOccurrenceAfterItsOwn(t *testing.T) {
	s := schedule(t, localcalendar.Once, -3, "10:00")

	_, ok := s.Next(localcalendar.NewDate(2027, time.January, 15), utc(2027, time.January, 12, 10, 1), time.UTC)

	assert.False(t, ok)
}

var workdays = []localcalendar.Weekday{localcalendar.Monday, localcalendar.Tuesday, localcalendar.Wednesday, localcalendar.Thursday, localcalendar.Friday}

// A Wait until's time-of-day form waits for the next local time on one of its weekdays, every day
// when it names none; arriving at that very moment waits zero (decision 11). 2027-01-15 is a Friday.
func TestTheNextTimeOfDayIsTheFirstOnAChosenWeekdayAtOrAfterTheArrival(t *testing.T) {
	cases := []struct {
		name      string
		zone      string
		localTime string
		weekdays  []localcalendar.Weekday
		arrival   time.Time
		want      time.Time
	}{
		{"the next weekday from a Friday evening", "Europe/Berlin", "09:00", workdays, utc(2027, time.January, 15, 17, 0), utc(2027, time.January, 18, 8, 0)},
		{"a Friday evening that is Saturday in UTC", "America/New_York", "09:00", workdays, utc(2027, time.January, 16, 1, 0), utc(2027, time.January, 18, 14, 0)},
		{"arriving at the time", "Europe/Berlin", "09:00", workdays, utc(2027, time.January, 18, 8, 0), utc(2027, time.January, 18, 8, 0)},
		{"every day", "Europe/Berlin", "09:00", nil, utc(2027, time.January, 15, 17, 0), utc(2027, time.January, 16, 8, 0)},
		{"later the same day", "Europe/Berlin", "09:00", nil, utc(2027, time.January, 15, 7, 0), utc(2027, time.January, 15, 8, 0)},
		{"a minute past the one weekday", "Europe/Berlin", "10:00", []localcalendar.Weekday{localcalendar.Tuesday}, utc(2027, time.January, 19, 9, 1), utc(2027, time.January, 26, 9, 0)},
		{"a time the spring-forward change skips", "Europe/Berlin", "02:30", nil, utc(2027, time.March, 27, 12, 0), utc(2027, time.March, 28, 1, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, err := localcalendar.NewTimeOfDay(c.localTime, c.weekdays)
			require.NoError(t, err)
			loc := zone(t, c.zone)

			got := w.Next(c.arrival, loc)

			assert.WithinDuration(t, c.want, got, 0)
		})
	}
}

// The bounds of decision 6: an offset within ±365 days, a local time HH:MM in whole minutes, the
// weekdays a set of ISO day names.
func TestTheConstructorsRefuseWhatIsOutOfBounds(t *testing.T) {
	schedules := []struct {
		name       string
		recurrence localcalendar.Recurrence
		offsetDays int
		localTime  string
		want       error
	}{
		{"an offset beyond a year after", localcalendar.Yearly, 366, "10:00", localcalendar.ErrOffset},
		{"an offset beyond a year before", localcalendar.Yearly, -366, "10:00", localcalendar.ErrOffset},
		{"an unknown recurrence", "monthly", 0, "10:00", localcalendar.ErrRecurrence},
		{"a one-digit hour", localcalendar.Yearly, 0, "9:00", localcalendar.ErrLocalTime},
		{"24:00", localcalendar.Yearly, 0, "24:00", localcalendar.ErrLocalTime},
		{"a minute past the hour's end", localcalendar.Yearly, 0, "10:60", localcalendar.ErrLocalTime},
		{"seconds", localcalendar.Yearly, 0, "10:00:00", localcalendar.ErrLocalTime},
		{"a sign", localcalendar.Yearly, 0, "+1:00", localcalendar.ErrLocalTime},
	}
	for _, c := range schedules {
		t.Run(c.name, func(t *testing.T) {
			_, err := localcalendar.NewSchedule(c.recurrence, c.offsetDays, c.localTime)

			require.ErrorIs(t, err, c.want)
		})
	}
	timesOfDay := []struct {
		name      string
		localTime string
		weekdays  []localcalendar.Weekday
		want      error
	}{
		{"a repeated weekday", "09:00", []localcalendar.Weekday{localcalendar.Monday, localcalendar.Monday}, localcalendar.ErrRepeatedWeekday},
		{"an unknown weekday", "09:00", []localcalendar.Weekday{"funday"}, localcalendar.ErrWeekday},
		{"a weekday not in lower case", "09:00", []localcalendar.Weekday{"Monday"}, localcalendar.ErrWeekday},
		{"a malformed local time", "9am", nil, localcalendar.ErrLocalTime},
	}
	for _, c := range timesOfDay {
		t.Run(c.name, func(t *testing.T) {
			_, err := localcalendar.NewTimeOfDay(c.localTime, c.weekdays)

			require.ErrorIs(t, err, c.want)
		})
	}
}

// The bounds themselves are inside them.
func TestTheConstructorsTakeTheirBounds(t *testing.T) {
	_, err := localcalendar.NewSchedule(localcalendar.Once, -365, "23:59")
	require.NoError(t, err)
	_, err = localcalendar.NewSchedule(localcalendar.Yearly, 365, "00:00")
	require.NoError(t, err)
	_, err = localcalendar.NewTimeOfDay("00:00", localcalendar.Weekdays)
	require.NoError(t, err)
}

// The zone is the Contact's when the zone database loads it, by the Report day's rule, and the
// Workspace's otherwise, UTC while that is unset.
func TestTheZoneIsTheContactsElseTheWorkspaces(t *testing.T) {
	cases := []struct {
		name      string
		contact   *string
		workspace *string
		want      string
	}{
		{"the Contact's", new("Europe/Berlin"), new("America/New_York"), "Europe/Berlin"},
		{"an unknown Contact zone", new("Mars/Olympus_Mons"), new("America/New_York"), "America/New_York"},
		{"the machine's zone", new("Local"), new("America/New_York"), "America/New_York"},
		{"an empty Contact zone", new(""), new("America/New_York"), "America/New_York"},
		{"no Contact zone", nil, new("America/New_York"), "America/New_York"},
		{"no zone at all", nil, nil, "UTC"},
		{"an unknown Contact zone and no Workspace zone", new("Mars/Olympus_Mons"), nil, "UTC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loc, err := localcalendar.Zone(c.contact, c.workspace)
			require.NoError(t, err)
			got := loc.String()

			assert.Equal(t, c.want, got)
		})
	}
}

// A Workspace's zone is checked when it is written, so one the zone database does not load is a
// fault, never a fallback.
func TestAnUnknownWorkspaceZoneIsAnError(t *testing.T) {
	_, err := localcalendar.Zone(new("Europe/Berlin"), new("Mars/Olympus_Mons"))

	require.Error(t, err)
}

// A datetime Custom field's value is read in the Workspace time zone: one with an offset is moved
// into it, one without and a bare date keep their own date (ADR 0087). Tokyo is UTC+9, Los Angeles
// UTC-8 in January.
func TestAStoredValuesDateIsReadInTheWorkspaceZone(t *testing.T) {
	tokyo, losAngeles := zone(t, "Asia/Tokyo"), zone(t, "America/Los_Angeles")
	jan14, jan15, jan16 := localcalendar.NewDate(2026, time.January, 14), localcalendar.NewDate(2026, time.January, 15), localcalendar.NewDate(2026, time.January, 16)
	cases := []struct {
		name  string
		value any
		east  localcalendar.Date
		west  localcalendar.Date
	}{
		{"an evening in UTC", "2026-01-15T20:00:00Z", jan16, jan15},
		{"an early hour in UTC", "2026-01-15T03:00:00Z", jan15, jan14},
		{"an offset of its own", "2026-01-15T23:00:00-05:00", jan16, jan15},
		{"fractional seconds", "2026-01-15T03:00:00.123456Z", jan15, jan14},
		{"late at night without an offset", "2026-01-15T23:30:00", jan15, jan15},
		{"early in the morning without an offset", "2026-01-15T00:30:00", jan15, jan15},
		{"a bare date", "2026-01-15", jan15, jan15},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			east, eastOK := localcalendar.ValueDate(c.value, tokyo)
			west, westOK := localcalendar.ValueDate(c.value, losAngeles)

			require.True(t, eastOK)
			require.True(t, westOK)
			assert.Equal(t, c.east, east)
			assert.Equal(t, c.west, west)
		})
	}
}

// A value that is no date counts as the field not held.
func TestAValueThatIsNoDateIsNotHeld(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"text", "tomorrow"},
		{"a slash form", "01/15/2026"},
		{"a time alone", "10:00:00"},
		{"no day of the calendar", "2026-02-30"},
		{"empty", ""},
		{"a number", 1736899200.0},
		{"a boolean", true},
		{"null", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := localcalendar.ValueDate(c.value, time.UTC)

			assert.False(t, ok)
		})
	}
}

// A Contact's birthday is a calendar date already: its own year, month and day, whatever the
// location of the instant that carries it.
func TestABirthdayKeepsItsOwnDate(t *testing.T) {
	stored := time.Date(1990, time.May, 10, 0, 0, 0, 0, time.UTC)

	got := localcalendar.DateOf(stored)

	assert.Equal(t, localcalendar.NewDate(1990, time.May, 10), got)
}

// Every part out of bounds is refused at once, each matched by its own refusal, so a save can name
// every field it refuses.
func TestTheConstructorsRefuseEveryPartOutOfBoundsAtOnce(t *testing.T) {
	_, scheduleErr := localcalendar.NewSchedule("monthly", 400, "25:00")
	_, timeOfDayErr := localcalendar.NewTimeOfDay("7pm", []localcalendar.Weekday{localcalendar.Monday, localcalendar.Monday, "someday"})

	require.ErrorIs(t, scheduleErr, localcalendar.ErrRecurrence)
	require.ErrorIs(t, scheduleErr, localcalendar.ErrOffset)
	require.ErrorIs(t, scheduleErr, localcalendar.ErrLocalTime)
	require.ErrorIs(t, timeOfDayErr, localcalendar.ErrLocalTime)
	require.ErrorIs(t, timeOfDayErr, localcalendar.ErrRepeatedWeekday)
	require.ErrorIs(t, timeOfDayErr, localcalendar.ErrWeekday)
}
