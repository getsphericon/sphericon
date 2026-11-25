package localcalendar_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// zones hold daylight-saving changes north and south, a half-hour offset, a half-hour change and a
// change at midnight.
var zones = []string{"UTC", "Europe/Berlin", "America/New_York", "Asia/Kolkata", "Pacific/Auckland", "Australia/Lord_Howe", "America/Santiago", "America/Havana"}

var weekdayNames = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

func drawLocalTime(t *rapid.T) (hour, minute int, localTime string) {
	hour = rapid.IntRange(0, 23).Draw(t, "hour")
	minute = rapid.IntRange(0, 59).Draw(t, "minute")
	return hour, minute, fmt.Sprintf("%02d:%02d", hour, minute)
}

func drawMoment(t *rapid.T) time.Time {
	return time.Unix(rapid.Int64Range(1_600_000_000, 1_900_000_000).Draw(t, "moment"), 0)
}

// chosen reports whether a time of day of the weekdays named comes on day: every day when none is
// named.
func chosen(weekdays []string, day time.Weekday) bool {
	return len(weekdays) == 0 || lo.Contains(weekdays, strings.ToLower(day.String()))
}

// readsAs reports whether t reads as the local time in its location, or is the first moment after a
// change that skipped it.
func readsAs(t time.Time, hour, minute int) bool {
	start, _ := t.ZoneBounds()
	return (t.Hour() == hour && t.Minute() == minute) || t.Equal(start)
}

func TestTheNextTimeOfDayIsTheLocalTimeOnAChosenDayWithinAWeek(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc := zone(t, rapid.SampledFrom(zones).Draw(t, "zone"))
		arrival := drawMoment(t)
		hour, minute, localTime := drawLocalTime(t)
		weekdays := rapid.SliceOfDistinct(rapid.SampledFrom(weekdayNames), func(s string) string { return s }).Draw(t, "weekdays")
		w, err := localcalendar.NewTimeOfDay(localTime, weekdays)
		require.NoError(t, err)

		got := w.Next(arrival, loc)

		local := got.In(loc)
		again := w.Next(got, loc)
		assert.False(t, got.Before(arrival), "never before the arrival")
		assert.True(t, got.Before(arrival.Add(8*24*time.Hour)), "within the week")
		assert.True(t, chosen(weekdays, local.Weekday()), "on a chosen weekday: %s", local)
		assert.True(t, readsAs(local, hour, minute), "at the local time: %s", local)
		assert.True(t, again.Equal(got), "the moment itself is the next one")
	})
}

func TestTheNextYearlyOccurrenceIsTheFirstAtOrAfterTheArrival(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc := zone(t, rapid.SampledFrom(zones).Draw(t, "zone"))
		arrival := drawMoment(t)
		_, _, localTime := drawLocalTime(t)
		date := localcalendar.NewDate(rapid.IntRange(1900, 2030).Draw(t, "year"), time.Month(rapid.IntRange(1, 12).Draw(t, "month")), rapid.IntRange(1, 31).Draw(t, "day"))
		s := schedule(t, localcalendar.Yearly, rapid.IntRange(-365, 365).Draw(t, "offsetDays"), localTime)

		got, ok := s.Next(date, arrival, loc)

		require.True(t, ok)
		moment, momentOK := s.Moment(date, got.Year, loc)
		previous, previousOK := s.Moment(date, got.Year-1, loc)
		require.True(t, momentOK)
		assert.True(t, got.At.Equal(moment), "the anniversary year's own moment")
		assert.False(t, got.At.Before(arrival), "never before the arrival")
		assert.Greater(t, got.Year, date.Year(), "never in the date's own year")
		assert.True(t, !previousOK || previous.Before(arrival), "the year before is not ahead")
	})
}
