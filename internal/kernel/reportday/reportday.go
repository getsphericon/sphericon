// Package reportday is the Report day of ADR 0057: a calendar day in the Workspace time zone.
// It loads the zone and cuts a trailing window of whole local days, so every report counts days
// the same way and a 23- or 25-hour day at a daylight-saving change is one bucket.
package reportday

import (
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
)

// ErrUnknownZone is a name the zone database does not know.
var ErrUnknownZone = errors.New("unknown time zone")

// Format is the layout of a Report day's label.
const Format = "2006-01-02"

// Name is the zone name a Workspace's timezone column stands for: UTC while unset.
func Name(name *string) string {
	if name == nil {
		return "UTC"
	}
	return *name
}

// Location is the zone named by a Workspace's timezone column; an unset one (nil) is UTC.
func Location(name *string) (*time.Location, error) {
	if name == nil {
		return time.UTC, nil
	}
	return Load(*name)
}

// Load is the zone an IANA name stands for. The empty name and "Local" (the machine's zone,
// which no Workspace may depend on) are refused like any unknown name.
func Load(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, fmt.Errorf("%w: %q", ErrUnknownZone, name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownZone, name)
	}
	return loc, nil
}

// Window is a run of whole Report days: Since is the first instant of the first day, Until the
// first instant after the last (exclusive), and Dates one label per day, oldest first.
type Window struct {
	Loc          *time.Location
	Since, Until time.Time
	Dates        []string
}

// Trailing is the last days Report days in loc, the one holding now included.
func Trailing(now time.Time, loc *time.Location, days int) Window {
	y, m, d := now.In(loc).Date()
	return span(loc, y, m, d-(days-1), days)
}

// Previous is the window of as many Report days just before w, ending where w begins: the period
// a report compares w with.
func (w Window) Previous() Window {
	days := len(w.Dates)
	y, m, d := w.Since.In(w.Loc).Date()
	return span(w.Loc, y, m, d-days, days)
}

// First is the label of w's first Report day, the one a report names its period from.
func (w Window) First() string { return w.Dates[0] }

// Last is the label of w's last Report day, included.
func (w Window) Last() string { return w.Dates[len(w.Dates)-1] }

// span is the days Report days in loc from the calendar date y-m-first on; time.Date normalizes a
// first day outside the month.
func span(loc *time.Location, y int, m time.Month, first, days int) Window {
	w := Window{
		Loc:   loc,
		Since: time.Date(y, m, first, 0, 0, 0, 0, loc),
		Until: time.Date(y, m, first+days, 0, 0, 0, 0, loc),
	}
	// The labels walk calendar dates in UTC, where every day has 24 hours: adding a day to a
	// local instant would skip or repeat a date across a daylight-saving change.
	civil := time.Date(y, m, first, 0, 0, 0, 0, time.UTC)
	for range days {
		w.Dates = append(w.Dates, civil.Format(Format))
		civil = civil.AddDate(0, 0, 1)
	}
	return w
}

// Label is the Report-day label (YYYY-MM-DD) of a timestamp column in loc, for a query that counts
// by Report day. The zone is a bound argument, never spliced into the statement.
func Label(column string, loc *time.Location) sql.Querier {
	// raw sql: ent has no AT TIME ZONE expression; the zone is a bound argument
	return sql.ExprFunc(func(b *sql.Builder) {
		b.WriteString("to_char(").WriteString(column).WriteString(" AT TIME ZONE ").Arg(loc.String()).WriteString(", 'YYYY-MM-DD')")
	})
}
