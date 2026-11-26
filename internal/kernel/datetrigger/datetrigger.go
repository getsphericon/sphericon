// Package datetrigger is the stored shape of a date a Contact holds and when its occurrences come
// (ADR 0126): the field (the Contact's birthday or a datetime Custom field), the recurrence, the
// offset in days and the local time. A date Trigger stores it in its version's Trigger document,
// and a Wait until's date form takes the same model. The calendar it is read on is
// internal/kernel/localcalendar.
package datetrigger

import (
	"maps"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// Values are the kinds of date field, the values of a column that stores one.
func (FieldKind) Values() []string {
	values := make([]string, len(FieldKinds))
	for i, k := range FieldKinds {
		values[i] = string(k)
	}
	return values
}

// Field names the date: the birthday, or a Custom field by its key (empty for the birthday).
type Field struct {
	Kind FieldKind `json:"kind"`
	Key  string    `json:"key,omitempty"`
}

// Date is the date a Contact holds and when its occurrences come: every year or once, OffsetDays
// whole days before (negative) or after it, at LocalTime (HH:MM) in the Contact's zone.
type Date struct {
	Field      Field                    `json:"field"`
	Recurrence localcalendar.Recurrence `json:"recurrence"`
	OffsetDays int                      `json:"offsetDays"`
	LocalTime  string                   `json:"localTime"`
}

// Schedule is when the date's occurrences come, or the calendar's refusal of its recurrence,
// offset or local time.
func (d Date) Schedule() (localcalendar.Schedule, error) {
	return localcalendar.NewSchedule(d.Recurrence, d.OffsetDays, d.LocalTime)
}

// Occurrence is the occurrence of the date in the anniversary year: the year for a yearly date,
// the one occurrence for a once date.
func (d Date) Occurrence(year int) Occurrence {
	if d.Recurrence == localcalendar.Once {
		return Once{}
	}
	return Yearly{Year: year}
}

// Holder is what a Contact holds of the dates: its birthday and its Custom field values.
type Holder struct {
	Birthday     *time.Time
	CustomFields map[string]any
}

// Of is the calendar date the Contact holds in the field, read in the Workspace zone for a datetime
// value (localcalendar.ValueDate), and false when it holds none, or a value that is no date.
func (f Field) Of(h Holder, workspace *time.Location) (localcalendar.Date, bool) {
	switch f.Kind {
	case Birthday:
		if h.Birthday == nil {
			return localcalendar.Date{}, false
		}
		return localcalendar.DateOf(*h.Birthday), true
	case CustomField:
		value, ok := h.CustomFields[f.Key]
		if !ok {
			return localcalendar.Date{}, false
		}
		return localcalendar.ValueDate(value, workspace)
	}
	return localcalendar.Date{}, false
}

// The key of a date (ADR 0126): the calendar date the date scheduler's band finds a Contact by,
// which no Workspace time zone moves. A datetime value's date is read in the Workspace time zone
// (Field.Of), which a Workspace may change without touching a Contact; its key is its date read in
// UTC instead, and the date in any zone is the key or the day either side of it. The band reaches
// past the window far enough to cover that day, so a zone change rewrites no key.

// KeyOf is the key of the date the Contact holds in the field: the birthday itself, a datetime value
// read in UTC (localcalendar.ValueDate); false when it holds none.
func (f Field) KeyOf(h Holder) (localcalendar.Date, bool) {
	return f.Of(h, time.UTC)
}

// Keyed is a date a Contact holds, by its field and its key.
type Keyed struct {
	Field Field
	Key   localcalendar.Date
}

// Keys are the keys of every date the Contact holds, ordered by field: its birthday, and each
// Custom field value that is a date in an ISO form, whatever the field's type. A type change
// rewrites no stored value, so a field made datetime later finds its values keyed already.
func (h Holder) Keys() []Keyed {
	var keys []Keyed
	fields := make([]Field, 0, 1+len(h.CustomFields))
	fields = append(fields, Field{Kind: Birthday})
	for _, key := range slices.Sorted(maps.Keys(h.CustomFields)) {
		fields = append(fields, Field{Kind: CustomField, Key: key})
	}
	for _, f := range fields {
		if key, ok := f.KeyOf(h); ok {
			keys = append(keys, Keyed{Field: f, Key: key})
		}
	}
	return keys
}

// bandMargin is how many calendar days the band of a window reaches past it on each side: an
// occurrence's local day lies within a day of its UTC day, the Workspace zone's date of a value
// within a day of its key, and 29 February counts on 28 February in a common year.
const bandMargin = 3

// maxBandDays is the widest band of a yearly date narrowed by its days: a window longer than a year
// holds every key.
const maxBandDays = 400

// Band is the keys of the dates whose occurrence may fall in a window (Date.Band): the database
// narrows the Contacts to those holding such a key, and the local calendar computes each one's
// moment. A once date's band is a range of keys (KeyRange); a yearly date's is a set of month and
// day (MonthDays, localcalendar.Date.MonthDay), or every key when the window is longer than a year
// (EveryKey). A type switch over it that misses a kind fails the lint (gochecksumtype).
//
//sumtype:decl
type Band interface {
	// Holds reports whether the band holds the key.
	Holds(key localcalendar.Date) bool
	band()
}

// KeyRange is a once date's band: the keys from First to Last, both included.
type KeyRange struct {
	First, Last localcalendar.Date
}

// MonthDays is a yearly date's band over a window of a year at most: the keys of these month and
// days, ascending.
type MonthDays struct {
	Days []int
}

// EveryKey is a yearly date's band over a window longer than a year: every key.
type EveryKey struct{}

func (KeyRange) band()  {}
func (MonthDays) band() {}
func (EveryKey) band()  {}

func (b KeyRange) Holds(key localcalendar.Date) bool {
	return !key.Midnight().Before(b.First.Midnight()) && !key.Midnight().After(b.Last.Midnight())
}

func (b MonthDays) Holds(key localcalendar.Date) bool { return slices.Contains(b.Days, key.MonthDay()) }

func (EveryKey) Holds(localcalendar.Date) bool { return true }

// Band is the band of the window from..to: the days the window spans in UTC, bandMargin each side,
// moved back by the offset.
func (d Date) Band(from, to time.Time) Band {
	first := localcalendar.DateOf(from.UTC()).AddDays(-bandMargin - d.OffsetDays)
	last := localcalendar.DateOf(to.UTC()).AddDays(bandMargin - d.OffsetDays)
	if d.Recurrence == localcalendar.Once {
		return KeyRange{First: first, Last: last}
	}
	if last.Midnight().Sub(first.Midnight()) > maxBandDays*24*time.Hour {
		return EveryKey{}
	}
	var days []int
	for day := first; !day.Midnight().After(last.Midnight()); day = day.AddDays(1) {
		days = append(days, day.MonthDay())
		// 29 February comes on 28 February in a common year.
		if day.MonthDay() == 228 {
			days = append(days, 229)
		}
	}
	slices.Sort(days)
	return MonthDays{Days: slices.Compact(days)}
}
