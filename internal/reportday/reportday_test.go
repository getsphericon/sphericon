package reportday_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/reportday"
)

var zones = []string{"UTC", "America/New_York", "Europe/Berlin", "Asia/Kolkata", "Pacific/Auckland", "Australia/Lord_Howe", "America/Sao_Paulo"}

func TestAnUnsetZoneIsUTC(t *testing.T) {
	loc, err := reportday.Location(nil)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, loc)
}

func TestUnknownNamesAreRefused(t *testing.T) {
	for _, name := range []string{"", "Local"} {
		_, err := reportday.Load(name)
		require.ErrorIs(t, err, reportday.ErrUnknownZone, name)
	}
}

func TestADaylightSavingDayIsOneBucket(t *testing.T) {
	loc, err := reportday.Load("America/New_York")
	require.NoError(t, err)

	spring := reportday.Trailing(time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), loc, 7)
	assert.Equal(t, []string{"2026-03-04", "2026-03-05", "2026-03-06", "2026-03-07", "2026-03-08", "2026-03-09", "2026-03-10"}, spring.Dates)
	assert.Equal(t, 7*24*time.Hour-time.Hour, spring.Until.Sub(spring.Since), "the window holds the 23-hour day")

	fall := reportday.Trailing(time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC), loc, 7)
	assert.Equal(t, []string{"2026-10-28", "2026-10-29", "2026-10-30", "2026-10-31", "2026-11-01", "2026-11-02", "2026-11-03"}, fall.Dates)
	assert.Equal(t, 7*24*time.Hour+time.Hour, fall.Until.Sub(fall.Since), "the window holds the 25-hour day")
}

func TestTrailingWindowsAreWholeConsecutiveLocalDays(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc, err := reportday.Load(rapid.SampledFrom(zones).Draw(t, "zone"))
		require.NoError(t, err)
		now := time.Unix(rapid.Int64Range(1_600_000_000, 1_900_000_000).Draw(t, "now"), 0)
		days := rapid.SampledFrom([]int{7, 30, 90}).Draw(t, "days")

		w := reportday.Trailing(now, loc, days)

		require.Len(t, w.Dates, days)
		assert.False(t, now.Before(w.Since), "now is inside the window")
		assert.True(t, now.Before(w.Until), "now is inside the window")
		assert.Equal(t, w.Since.In(loc).Format(reportday.Format), w.Dates[0])
		assert.Equal(t, now.In(loc).Format(reportday.Format), w.Dates[days-1])
		since := w.Since.In(loc)
		assert.Equal(t, 0, since.Hour()+since.Minute(), "the window starts at local midnight")
		for i := 1; i < days; i++ {
			prev, err := time.Parse(reportday.Format, w.Dates[i-1])
			require.NoError(t, err)
			assert.Equal(t, prev.AddDate(0, 0, 1).Format(reportday.Format), w.Dates[i])
		}
	})
}

func TestThePreviousWindowIsAsManyLocalDaysEndingWhereTheWindowBegins(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc, err := reportday.Load(rapid.SampledFrom(zones).Draw(t, "zone"))
		require.NoError(t, err)
		now := time.Unix(rapid.Int64Range(1_600_000_000, 1_900_000_000).Draw(t, "now"), 0)
		days := rapid.SampledFrom([]int{7, 14, 30, 90}).Draw(t, "days")
		w := reportday.Trailing(now, loc, days)

		prev := w.Previous()

		require.Len(t, prev.Dates, days)
		assert.Equal(t, w.Since, prev.Until, "the previous window ends where the window begins")
		since := prev.Since.In(loc)
		assert.Equal(t, 0, since.Hour()+since.Minute(), "the previous window starts at local midnight")
		assert.Equal(t, since.Format(reportday.Format), prev.Dates[0])
		last, err := time.Parse(reportday.Format, prev.Dates[days-1])
		require.NoError(t, err)
		assert.Equal(t, last.AddDate(0, 0, 1).Format(reportday.Format), w.Dates[0], "no day is skipped or repeated between them")
	})
}

func TestAWindowIsNamedByTheReportDaysItBeginsAndEndsOn(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc, err := reportday.Load(rapid.SampledFrom(zones).Draw(t, "zone"))
		require.NoError(t, err)
		now := time.Unix(rapid.Int64Range(1_600_000_000, 1_900_000_000).Draw(t, "now"), 0)
		w := reportday.Trailing(now, loc, rapid.SampledFrom([]int{7, 14, 30, 90}).Draw(t, "days"))

		first, last := w.First(), w.Last()

		assert.Equal(t, w.Since.In(loc).Format(reportday.Format), first, "the day of its first instant")
		assert.Equal(t, now.In(loc).Format(reportday.Format), last, "a trailing window ends today")
	})
}

func TestThePreviousWindowOverADaylightSavingChangeHoldsItsShortDay(t *testing.T) {
	loc, err := reportday.Load("America/New_York")
	require.NoError(t, err)

	prev := reportday.Trailing(time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC), loc, 7).Previous()

	assert.Equal(t, []string{"2026-03-04", "2026-03-05", "2026-03-06", "2026-03-07", "2026-03-08", "2026-03-09", "2026-03-10"}, prev.Dates)
	assert.Equal(t, 7*24*time.Hour-time.Hour, prev.Until.Sub(prev.Since), "the window holds the 23-hour day")
}
