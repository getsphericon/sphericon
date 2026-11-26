package datetrigger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

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
