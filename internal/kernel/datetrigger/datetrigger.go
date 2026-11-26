// Package datetrigger is the stored shape of a date a Contact holds and when its occurrences come
// (ADR 0126): the field (the Contact's birthday or a datetime Custom field), the recurrence, the
// offset in days and the local time. A date Trigger stores it in its version's Trigger document,
// and a Wait until's date form takes the same model. The calendar it is read on is
// internal/kernel/localcalendar.
package datetrigger

import (
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

// YearOf is the anniversary year an occurrence is stored under, the inverse of Date.Occurrence, and
// false for OnceMark, which is no year.
func YearOf(occurrence int) (int, bool) {
	return occurrence, occurrence != OnceMark
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
