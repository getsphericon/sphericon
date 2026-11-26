package automationstep_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

var reachZones = []string{"UTC", "Europe/Berlin", "America/New_York", "Asia/Kolkata", "Pacific/Auckland", "Australia/Lord_Howe"}

func drawBirthdayWait(t *rapid.T) automationstep.OnDate {
	return automationstep.OnDate{
		Field:      datetrigger.Field{Kind: datetrigger.Birthday},
		Recurrence: localcalendar.Yearly,
		OffsetDays: rapid.IntRange(-365, 365).Draw(t, "offset"),
		LocalTime:  fmt.Sprintf("%02d:%02d", rapid.IntRange(0, 23).Draw(t, "hour"), rapid.IntRange(0, 59).Draw(t, "minute")),
	}
}

// A yearly date the Contact holds from before the arrival's year always has its next occurrence
// at or after the arrival and within the bound, so the Enrollment waits for it: a birthday never
// goes on at once.
func TestAYearlyDateFromAnEarlierYearAlwaysWaitsWithinTheBound(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc, err := time.LoadLocation(rapid.SampledFrom(reachZones).Draw(t, "zone"))
		require.NoError(t, err)
		arrival := time.Unix(rapid.Int64Range(1_700_000_000, 1_900_000_000).Draw(t, "arrival"), 0)
		born := time.Date(rapid.IntRange(1920, arrival.Year()-2).Draw(t, "year"), time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"), 0, 0, 0, 0, time.UTC)
		contact := automationstep.Contact{Zone: loc, Workspace: time.UTC, Dates: datetrigger.Holder{Birthday: &born}}

		due, err := automationstep.Reach(drawBirthdayWait(t), contact, arrival)

		require.NoError(t, err)
		assert.Empty(t, due.WentOn)
		assert.False(t, due.At.Before(arrival), "never before the arrival: %s", due.At)
		assert.False(t, due.At.After(arrival.In(loc).AddDate(0, 0, automationstep.BoundDays)), "within the bound: %s", due.At)
	})
}

// Whatever the Contact holds, a Wait until on a date either waits for a moment at or after the
// arrival within the bound, or goes on at once with a reason: never both, never neither.
func TestAWaitUntilOnADateWaitsOrGoesOnNeverBoth(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		loc, err := time.LoadLocation(rapid.SampledFrom(reachZones).Draw(t, "zone"))
		require.NoError(t, err)
		arrival := time.Unix(rapid.Int64Range(1_700_000_000, 1_900_000_000).Draw(t, "arrival"), 0)
		until := drawBirthdayWait(t)
		until.Field = datetrigger.Field{Kind: datetrigger.CustomField, Key: "renews_at"}
		until.Recurrence = rapid.SampledFrom([]localcalendar.Recurrence{localcalendar.Yearly, localcalendar.Once}).Draw(t, "recurrence")
		value := time.Unix(rapid.Int64Range(1_500_000_000, 2_000_000_000).Draw(t, "value"), 0).UTC().Format(time.RFC3339)
		contact := automationstep.Contact{Zone: loc, Workspace: time.UTC, Dates: datetrigger.Holder{CustomFields: map[string]any{"renews_at": value}}}

		due, err := automationstep.Reach(until, contact, arrival)

		require.NoError(t, err)
		assert.NotEqual(t, due.At.IsZero(), due.WentOn == "", "a moment or a reason: %+v", due)
		assert.False(t, !due.At.IsZero() && due.At.Before(arrival), "never before the arrival: %s", due.At)
		assert.False(t, due.At.After(arrival.In(loc).AddDate(0, 0, automationstep.BoundDays)), "within the bound: %s", due.At)
	})
}
