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

// FieldKind is the kind of date a Contact holds.
type FieldKind string

// The kinds of date field.
const (
	// Birthday is the Contact's date of birth, a calendar date already.
	Birthday FieldKind = "birthday"
	// CustomField is a datetime Custom field of the Workspace, named by its key.
	CustomField FieldKind = "customField"
)

// Values are the kinds of date field, the values of a column that stores one.
func (FieldKind) Values() []string { return []string{string(Birthday), string(CustomField)} }

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

// Occurrence is the mark an occurrence is stored under (ADR 0126): its anniversary year for a
// yearly date, OnceMark for a once date, which no year equals.
func (d Date) Occurrence(year int) int {
	if d.Recurrence == localcalendar.Once {
		return OnceMark
	}
	return year
}

// OnceMark is the occurrence of a once date: one per Contact and Automation, ever. No anniversary
// year is 0, so a switch between yearly and once starts its own key.
const OnceMark = 0

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
	fields := []Field{{Kind: Birthday}}
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
// moment. A once date's band is a range of keys; a yearly date's is a set of month and day
// (localcalendar.Date.MonthDay), or every key when the window is longer than a year.
type Band struct {
	once        bool
	first, last localcalendar.Date
	monthDays   []int
}

// Band is the band of the window from..to: the days the window spans in UTC, bandMargin each side,
// moved back by the offset.
func (d Date) Band(from, to time.Time) Band {
	first := localcalendar.DateOf(from.UTC()).AddDays(-bandMargin - d.OffsetDays)
	last := localcalendar.DateOf(to.UTC()).AddDays(bandMargin - d.OffsetDays)
	if d.Recurrence == localcalendar.Once {
		return Band{once: true, first: first, last: last}
	}
	if last.Midnight().Sub(first.Midnight()) > maxBandDays*24*time.Hour {
		return Band{}
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
	return Band{monthDays: slices.Compact(days)}
}

// Range is a once date's band, the keys from first to last, both included; false for a yearly
// date.
func (b Band) Range() (first, last localcalendar.Date, ok bool) {
	return b.first, b.last, b.once
}

// MonthDays is a yearly date's band, the month and day of its keys; false when it holds every key,
// and for a once date.
func (b Band) MonthDays() ([]int, bool) {
	return b.monthDays, b.monthDays != nil
}

// Holds reports whether the band holds the key.
func (b Band) Holds(key localcalendar.Date) bool {
	if b.once {
		return !key.Midnight().Before(b.first.Midnight()) && !key.Midnight().After(b.last.Midnight())
	}
	return b.monthDays == nil || slices.Contains(b.monthDays, key.MonthDay())
}
