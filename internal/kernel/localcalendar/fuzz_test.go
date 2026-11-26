package localcalendar_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// hhmm is the local time decision 6 of #1613 takes: HH:MM in whole minutes, 00:00 to 23:59.
var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// A local time comes from a request or an agent: it is taken exactly when it is HH:MM.
func FuzzALocalTimeIsTakenOnlyAsHHMM(f *testing.F) {
	for _, seed := range []string{"09:00", "00:00", "23:59", "24:00", "9:00", "10:60", "10:00:00", "", "+1:00", "1٠:00"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, localTime string) {
		_, err := localcalendar.NewTimeOfDay(localTime, nil)

		taken := err == nil
		want := hhmm.MatchString(localTime)
		assert.Equal(t, want, taken, "%q", localTime)
	})
}

// A datetime Custom field's value comes from ingest: it is held as a date exactly when it is in one
// of the ISO forms ingest types as a datetime.
func FuzzAValueIsHeldAsADateExactlyInAnISOForm(f *testing.F) {
	for _, seed := range []string{"2026-01-15T20:00:00Z", "2026-01-15T23:00:00-05:00", "2026-01-15T23:30:00", "2026-01-15", "01/15/2026", "2026-02-30", "tomorrow", ""} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		_, held := localcalendar.ValueDate(value, time.UTC)

		iso := localcalendar.IsISODate(value)
		assert.Equal(t, iso, held, "%q", value)
	})
}
