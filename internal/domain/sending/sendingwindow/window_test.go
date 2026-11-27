package sendingwindow_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingwindow"
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

func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

// everyDay is a window open from opens to closes, in minutes of the day, every day of the week.
func everyDay(opens, closes int) sendingwindow.Window {
	hours := lo.Map(localcalendar.Weekdays, func(day localcalendar.Weekday, _ int) workspaces.OpenHours {
		return workspaces.OpenHours{Day: day, OpensAt: opens, ClosesAt: closes}
	})
	return sendingwindow.NewWindow(hours)
}

// 2027-01-15 is a Friday; New York is five hours behind UTC then, Honolulu ten and Tokyo nine ahead.
func TestTheWindowOpensWhenItIsOpenInEveryZone(t *testing.T) {
	window := everyDay(8*60, 20*60)
	cases := []struct {
		name  string
		zones []string
		at    time.Time
		want  time.Time
	}{
		{"open in its one zone", []string{"America/New_York"}, utc(2027, time.January, 15, 14, 0), utc(2027, time.January, 15, 14, 0)},
		{"22:00 in its one zone waits for 8:00 there", []string{"America/New_York"}, utc(2027, time.January, 16, 3, 0), utc(2027, time.January, 16, 13, 0)},
		{"open in one zone waits for the other", []string{"America/New_York", "Pacific/Honolulu"}, utc(2027, time.January, 15, 14, 0), utc(2027, time.January, 15, 18, 0)},
		{"a zone that opens while another closes waits for the overlap", []string{"America/New_York", "Asia/Tokyo"}, utc(2027, time.January, 15, 3, 0), utc(2027, time.January, 15, 23, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			zones := lo.Map(c.zones, func(name string, _ int) *time.Location { return zone(t, name) })

			got, ok := window.Opens(c.at, zones)

			require.True(t, ok)
			assert.WithinDuration(t, c.want, got, 0)
		})
	}
}

// From 9:00 to 17:00, New York and Tokyo are never open at once.
func TestAWindowNeverOpenInEveryZoneAtOnceOpensNone(t *testing.T) {
	window := everyDay(9*60, 17*60)
	zones := []*time.Location{zone(t, "America/New_York"), zone(t, "Asia/Tokyo")}

	_, ok := window.Opens(utc(2027, time.January, 15, 3, 0), zones)

	assert.False(t, ok)
}

// A window open on workdays only, read on a Friday evening, opens on Monday morning.
func TestAWindowClosedOnSomeDaysOpensOnTheNextDayItHas(t *testing.T) {
	workdays := lo.Map([]localcalendar.Weekday{localcalendar.Monday, localcalendar.Tuesday, localcalendar.Wednesday,
		localcalendar.Thursday, localcalendar.Friday}, func(day localcalendar.Weekday, _ int) workspaces.OpenHours {
		return workspaces.OpenHours{Day: day, OpensAt: 9 * 60, ClosesAt: 17 * 60}
	})
	window := sendingwindow.NewWindow(workdays)

	got, ok := window.Opens(utc(2027, time.January, 15, 17, 0), []*time.Location{zone(t, "Europe/Berlin")})

	require.True(t, ok)
	assert.WithinDuration(t, utc(2027, time.January, 18, 8, 0), got, 0)
}

// zones hold daylight-saving changes north and south, half-hour offsets and a change at midnight.
var zones = []string{"UTC", "Europe/Berlin", "America/New_York", "Pacific/Honolulu", "Asia/Kolkata", "Asia/Tokyo",
	"Pacific/Auckland", "Australia/Lord_Howe", "America/Santiago", "America/Havana"}

// openInAll is the model of a window: open at t when, in every zone, t reads as a minute inside the
// day's hours.
func openInAll(hours []workspaces.OpenHours, zones []*time.Location, t time.Time) bool {
	for _, loc := range zones {
		local := t.In(loc)
		minute := local.Hour()*60 + local.Minute()
		day := localcalendar.DateOf(local).Weekday()
		if !lo.ContainsBy(hours, func(h workspaces.OpenHours) bool {
			return h.Day == day && h.OpensAt <= minute && minute < h.ClosesAt
		}) {
			return false
		}
	}
	return true
}

// firstOpenMinute is the model's first moment from at on, minute by minute, at which the window is
// open in every zone within a week, and whether there is one.
func firstOpenMinute(hours []workspaces.OpenHours, zones []*time.Location, at time.Time) (time.Time, bool) {
	if openInAll(hours, zones, at) {
		return at, true
	}
	for t := at.Truncate(time.Minute).Add(time.Minute); !t.After(at.Add(7 * 24 * time.Hour)); t = t.Add(time.Minute) {
		if openInAll(hours, zones, t) {
			return t, true
		}
	}
	return time.Time{}, false
}

func drawHours(t *rapid.T) []workspaces.OpenHours {
	days := rapid.SliceOfNDistinct(rapid.SampledFrom(localcalendar.Weekdays), 1, 7, func(d localcalendar.Weekday) localcalendar.Weekday { return d }).Draw(t, "days")
	return lo.Map(days, func(day localcalendar.Weekday, _ int) workspaces.OpenHours {
		opens := rapid.IntRange(0, 1439).Draw(t, "opens")
		closes := rapid.IntRange(opens+1, 1440).Draw(t, "closes")
		return workspaces.OpenHours{Day: day, OpensAt: opens, ClosesAt: closes}
	})
}

// The moment the window opens is the model's first minute open in every zone, and none is found
// exactly when the model finds none within the week.
func TestTheWindowOpensAtTheFirstMinuteOpenInEveryZone(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hours := drawHours(t)
		names := rapid.SliceOfNDistinct(rapid.SampledFrom(zones), 1, 3, func(z string) string { return z }).Draw(t, "zones")
		locs := lo.Map(names, func(name string, _ int) *time.Location { return zone(t, name) })
		at := time.Unix(rapid.Int64Range(1_700_000_000, 1_900_000_000).Draw(t, "at"), 0)

		got, ok := sendingwindow.NewWindow(hours).Opens(at, locs)

		want, found := firstOpenMinute(hours, locs, at)
		assert.Equal(t, found, ok)
		assert.True(t, want.Equal(got), "opens at %s, the model at %s", got, want)
	})
}
