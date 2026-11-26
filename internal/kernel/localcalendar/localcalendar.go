// Package localcalendar is the local calendar of ADR 0126: every calendar moment a date Trigger
// and a Wait until read, computed from a calendar date, a local time and a zone. It touches no
// database and keeps no clock: the caller passes "now".
//
// Its calendar rules: a local time a daylight-saving change skips is the first moment after the
// gap, one that occurs twice is the first of the two, and 29 February is 28 February in a year
// without it.
package localcalendar

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Date is a calendar date, with no time or zone.
type Date struct {
	year  int
	month time.Month
	day   int
}

// NewDate is the calendar date of year, month and day, normalized as time.Date normalizes them.
func NewDate(year int, month time.Month, day int) Date {
	return DateOf(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// DateOf is the calendar date t reads in its own location, never moved into another zone: a
// Contact's `birthday`, a date already, is its own year, month and day.
func DateOf(t time.Time) Date {
	year, month, day := t.Date()
	return Date{year: year, month: month, day: day}
}

// Year is the date's year.
func (d Date) Year() int { return d.year }

// String is the date as YYYY-MM-DD.
func (d Date) String() string { return d.Midnight().Format(time.DateOnly) }

// MonthDay is the date's month and day as one number, month × 100 + day: 14 March is 314. It
// orders the days of a year as the calendar does.
func (d Date) MonthDay() int { return int(d.month)*100 + d.day }

// Midnight is the date's start in UTC, the instant its calendar arithmetic runs on and the value a
// date column holds.
func (d Date) Midnight() time.Time { return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC) }

// AddDays is the date days calendar days after d (before it when negative).
func (d Date) AddDays(days int) Date { return DateOf(d.Midnight().AddDate(0, 0, days)) }

// Weekday is the day of the week the date falls on, by the contract's name.
func (d Date) Weekday() Weekday { return Weekday(strings.ToLower(d.weekday().String())) }

// At is the moment the minute of the day (0 for 00:00 to 1439 for 23:59) reads on the date in loc,
// by the calendar rules of the package: a minute a daylight-saving change skips is the first moment
// after the gap, one that occurs twice the first of the two.
func (d Date) At(minute int, loc *time.Location) time.Time {
	return hhmm{hour: minute / 60, minute: minute % 60}.on(d, loc)
}

// weekday is the day of the week the date falls on.
func (d Date) weekday() time.Weekday { return d.Midnight().Weekday() }

// in is the date's day and month in year; 29 February is 28 February in a year without it.
func (d Date) in(year int) Date {
	if d.month == time.February && d.day == 29 && !isLeap(year) {
		return Date{year: year, month: time.February, day: 28}
	}
	return Date{year: year, month: d.month, day: d.day}
}

func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// isoLayouts are the ISO forms of a datetime Custom field's value (#817): a date-time with an offset
// (RFC 3339, "Z" or "+03:00", fractional seconds allowed), one without an offset and a bare date.
// A slash form such as 01/15/2026 is read month-first by some tools and day-first by others, so it
// is no date.
var isoLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", time.DateOnly}

// IsISODate reports whether s is a date in one of the ISO forms a datetime Custom field's value
// takes.
func IsISODate(s string) bool {
	return slices.ContainsFunc(isoLayouts, func(layout string) bool {
		_, err := time.Parse(layout, s)
		return err == nil
	})
}

// ValueDate is the calendar date of a datetime Custom field's value, read in the Workspace time
// zone (ADR 0087): a value with an offset is moved into that zone, one without an offset or a bare
// date keeps its own date. A value that is no date in an ISO form is not held (false).
func ValueDate(value any, workspace *time.Location) (Date, bool) {
	s, ok := value.(string)
	if !ok {
		return Date{}, false
	}
	zoned, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return DateOf(zoned.In(workspace)), true
	}
	for _, layout := range isoLayouts[1:] {
		local, err := time.Parse(layout, s)
		if err == nil {
			return DateOf(local), true
		}
	}
	return Date{}, false
}

// Zone is the zone a Contact's local times are read in: the Contact's own when the zone database
// loads it, by the Report day's rule, and the Workspace time zone otherwise, UTC while unset.
// Zone(nil, workspace) is the Workspace time zone, in which ValueDate reads a value. A Workspace
// zone the database does not load is an error: it was checked when it was written.
func Zone(contact, workspace *string) (*time.Location, error) {
	fallback, err := reportday.Location(workspace)
	if err != nil {
		return nil, fmt.Errorf("workspace time zone: %w", err)
	}
	own, ok := contactZone(contact)
	if !ok {
		return fallback, nil
	}
	return own, nil
}

// contactZone is the zone a Contact's time_zone names, when the zone database loads it.
func contactZone(name *string) (*time.Location, bool) {
	if name == nil {
		return nil, false
	}
	loc, err := reportday.Load(*name)
	return loc, err == nil
}

// The refusals of a constructor beside the contract's bounds, matched with errors.Is. A bound the
// contract declares, an offset in days (DateOffsetDays) or a local time (LocalTime), is refused in
// the kernel's one form: validation.ErrOutOfRange or validation.ErrPatternMismatch on its field.
var (
	// ErrRecurrence is a recurrence that is no DateRecurrence of the contract.
	ErrRecurrence = errors.New("unknown recurrence")
	// ErrWeekday is a name that is no Weekday of the contract.
	ErrWeekday         = errors.New("unknown weekday")
	ErrRepeatedWeekday = errors.New("repeated weekday")
)

// hhmm is a local time: a time of day in whole minutes.
type hhmm struct {
	hour, minute int
}

// parseLocalTime reads a local time of the contract's shape (LocalTime: HH:MM from 00:00 to
// 23:59), or refuses it on its field (validation.Pattern).
func parseLocalTime(s string) (hhmm, error) {
	if err := validation.Pattern(fieldname.LocalTime, s, bounds.LocalTimePattern); err != nil {
		return hhmm{}, err
	}
	hour, hourErr := strconv.Atoi(s[0:2])
	minute, minuteErr := strconv.Atoi(s[3:5])
	if err := errors.Join(hourErr, minuteErr); err != nil {
		return hhmm{}, fmt.Errorf("local time %q: %w", s, err)
	}
	return hhmm{hour: hour, minute: minute}, nil
}

// on is the moment of the local time on date d in loc, by the calendar rules of the package.
func (t hhmm) on(d Date, loc *time.Location) time.Time {
	wall := time.Date(d.year, d.month, d.day, t.hour, t.minute, 0, 0, time.UTC)
	// Every instant that may read as wall in loc lies within a day of wall read as UTC, and the
	// offsets in effect over those two days are the ones at their ends and middle.
	var first time.Time
	for _, probe := range []time.Time{wall.Add(-24 * time.Hour), wall, wall.Add(24 * time.Hour)} {
		at := readWith(wall, probe, loc)
		reads := at.In(loc)
		if sameWall(reads, wall) && (first.IsZero() || at.Before(first)) {
			first = at
		}
	}
	if !first.IsZero() {
		return first.In(loc)
	}
	// No instant reads as wall: the change skipped it. Read with the offset before the change, it
	// lands after the change, in the zone that begins at the first moment after the gap.
	after := readWith(wall, wall.Add(-24*time.Hour), loc)
	zone := after.In(loc)
	start, _ := zone.ZoneBounds()
	return start.In(loc)
}

// readWith is the instant the wall-clock time wall names under the offset loc has at probe.
func readWith(wall, probe time.Time, loc *time.Location) time.Time {
	_, offset := probe.In(loc).Zone()
	return wall.Add(-time.Duration(offset) * time.Second)
}

func sameWall(t, wall time.Time) bool {
	y, mo, d := t.Date()
	return y == wall.Year() && mo == wall.Month() && d == wall.Day() && t.Hour() == wall.Hour() && t.Minute() == wall.Minute()
}

// Schedule is when a date's occurrences come: its recurrence, an offset in whole days from the
// date and a local time on the day the offset lands on.
type Schedule struct {
	recurrence Recurrence
	offsetDays int
	at         hhmm
}

// NewSchedule is the schedule of the recurrence, offset and local time given, or the refusal of
// every one out of bounds, joined: ErrRecurrence, an offset beyond the contract's DateOffsetDays
// (validation.Range) and a local time not of its LocalTime (validation.Pattern).
func NewSchedule(recurrence Recurrence, offsetDays int, localTime string) (Schedule, error) {
	var errs []error
	if !slices.Contains(Recurrences, recurrence) {
		errs = append(errs, fmt.Errorf("%w: %q", ErrRecurrence, recurrence))
	}
	if err := validation.Range(fieldname.OffsetDays, offsetDays, bounds.DateOffsetDaysMin, bounds.DateOffsetDaysMax); err != nil {
		errs = append(errs, err)
	}
	at, err := parseLocalTime(localTime)
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return Schedule{}, errors.Join(errs...)
	}
	return Schedule{recurrence: recurrence, offsetDays: offsetDays, at: at}, nil
}

// Moment is the moment of date's occurrence in the anniversary year given, in loc. The
// anniversary year is the year of the date the offset counts from, so "3 days before" 2 January
// falls on 30 December of the year before. A yearly date has none in its own year or before; a
// once date has one only in its own year.
func (s Schedule) Moment(date Date, year int, loc *time.Location) (time.Time, bool) {
	switch s.recurrence {
	case Yearly:
		if year <= date.year {
			return time.Time{}, false
		}
	case Once:
		if year != date.year {
			return time.Time{}, false
		}
	}
	day := date.in(year).AddDays(s.offsetDays)
	return s.at.on(day, loc), true
}

// Occurrence is one occurrence of a date: its anniversary year and its moment.
type Occurrence struct {
	Year int
	At   time.Time
}

// Next is date's first occurrence at or after the moment given, in loc: an occurrence at that very
// moment is ahead. A yearly date always has one; a once date has none once its own has passed.
func (s Schedule) Next(date Date, after time.Time, loc *time.Location) (Occurrence, bool) {
	if s.recurrence == Once {
		at, _ := s.Moment(date, date.year, loc)
		return Occurrence{Year: date.year, At: at}, !at.Before(after)
	}
	// An offset of up to a year moves an anniversary year's moment into the calendar year before or
	// after it, and each year's moment is later than the one before.
	local := after.In(loc)
	for year := max(date.year+1, local.Year()-2); ; year++ {
		at, _ := s.Moment(date, year, loc)
		if !at.Before(after) {
			return Occurrence{Year: year, At: at}, true
		}
	}
}

// TimeOfDay is a local time on chosen weekdays, every day when none is chosen.
type TimeOfDay struct {
	at       hhmm
	weekdays [7]bool // by time.Weekday; all false is every day
}

// NewTimeOfDay is the local time on the weekdays named, or the refusal of every part out of bounds,
// joined: a local time not of the contract's LocalTime (validation.Pattern), ErrWeekday and
// ErrRepeatedWeekday.
func NewTimeOfDay(localTime string, weekdays []Weekday) (TimeOfDay, error) {
	var errs []error
	at, err := parseLocalTime(localTime)
	if err != nil {
		errs = append(errs, err)
	}
	var days [7]bool
	for _, name := range weekdays {
		day, ok := weekday(name)
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%w: %q", ErrWeekday, name))
		case days[day]:
			errs = append(errs, fmt.Errorf("%w: %q", ErrRepeatedWeekday, name))
		default:
			days[day] = true
		}
	}
	if len(errs) > 0 {
		return TimeOfDay{}, errors.Join(errs...)
	}
	return TimeOfDay{at: at, weekdays: days}, nil
}

// weekday is the day a Weekday of the contract names: its ISO name is Go's in lower case.
func weekday(name Weekday) (time.Weekday, bool) {
	for day := time.Sunday; day <= time.Saturday; day++ {
		if string(name) == strings.ToLower(day.String()) {
			return day, true
		}
	}
	return 0, false
}

// takes reports whether the time of day comes on the weekday.
func (w TimeOfDay) takes(day time.Weekday) bool {
	return w.weekdays == [7]bool{} || w.weekdays[day]
}

// Next is the first moment of the local time on a chosen weekday at or after the moment given, in
// loc: one at that very moment is ahead.
func (w TimeOfDay) Next(after time.Time, loc *time.Location) time.Time {
	local := after.In(loc)
	today := DateOf(local)
	// The same weekday a week on is the last day the search can reach.
	for days := 0; ; days++ {
		day := today.AddDays(days)
		if !w.takes(day.weekday()) {
			continue
		}
		at := w.at.on(day, loc)
		if !at.Before(after) {
			return at
		}
	}
}
