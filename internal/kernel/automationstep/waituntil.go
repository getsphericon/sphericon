package automationstep

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// WaitUntil holds the Enrollment until a calendar moment (ADR 0126), which Until says by its form.
// The moment is computed once, when the Enrollment reaches the Step, and kept as the Step's due
// moment.
type WaitUntil struct {
	Until Until
}

// Until is the moment a Wait until holds the Enrollment until, one type per form: TimeOfDay and
// OnDate. A new form is an arm of the contract's WaitUntilMoment, whose discriminator generates its
// Form (form_gen.go); it adds its type and its line in untilDecoders, and a type
// switch over Until that misses it fails the lint (gochecksumtype).
//
//sumtype:decl
type Until interface {
	Form() Form
	// stored is the moment as a version stores it.
	stored() Until
}

// TimeOfDay is the next moment LocalTime comes on one of Weekdays, every day when there are none,
// in the Contact's zone.
type TimeOfDay struct {
	// LocalTime is the local time of day, HH:MM.
	LocalTime string `json:"localTime"`
	// Weekdays are the days the local time counts on; none is every day.
	Weekdays []localcalendar.Weekday `json:"weekdays,omitempty"`
}

func (TimeOfDay) Form() Form { return FormTimeOfDay }

func (t TimeOfDay) stored() Until {
	if len(t.Weekdays) == 0 {
		t.Weekdays = nil // every day is stored as no weekday
	}
	return t
}

// Calendar is the time of day as the local calendar computes it, or its refusal: a malformed local
// time, an unknown or a repeated weekday.
func (t TimeOfDay) Calendar() (localcalendar.TimeOfDay, error) {
	return localcalendar.NewTimeOfDay(t.LocalTime, t.Weekdays)
}

// OnDate is the next occurrence of a date the Contact holds, in the date Trigger's model (ADR 0126):
// its birthday or a datetime Custom field, every year or once, OffsetDays before or after it, at
// LocalTime in the Contact's zone.
type OnDate struct {
	datetrigger.Date
}

func (OnDate) Form() Form { return FormDate }

func (d OnDate) stored() Until { return d }

func (WaitUntil) Kind() Kind { return KindWaitUntil }

// Stored is the Wait until with its moment in its stored form.
func (w WaitUntil) Stored() Payload {
	if w.Until == nil {
		return w
	}
	return WaitUntil{Until: w.Until.stored()}
}

// untilDecoders read the fields of each form's moment, refusing any field the form does not hold.
var untilDecoders = map[Form]func([]byte) (Until, error){
	FormTimeOfDay: decodeUntilAs[TimeOfDay],
	FormDate:      decodeUntilAs[OnDate],
}

func decodeUntilAs[U Until](b []byte) (Until, error) {
	var u U
	if err := decodeClosed(b, &u); err != nil {
		return nil, err
	}
	return u, nil
}

// decodeClosed decodes b into v, refusing a field v does not hold.
func decodeClosed(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// waitUntilJSON is a Wait until as it is stored: its moment, an object with its form beside the
// form's fields.
type waitUntilJSON struct {
	Until json.RawMessage `json:"until"`
}

// MarshalJSON writes the Wait until with its moment's form beside the moment's fields.
func (w WaitUntil) MarshalJSON() ([]byte, error) {
	if w.Until == nil {
		return nil, errors.New("automationstep: a wait until without a moment")
	}
	b, err := json.Marshal(w.Until)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	if fields["form"], err = json.Marshal(w.Until.Form()); err != nil {
		return nil, err
	}
	until, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return json.Marshal(waitUntilJSON{Until: until})
}

// UnmarshalJSON reads a Wait until written by MarshalJSON. A form nobody knows, and a field the
// Wait until or its form does not hold, are errors: the stored payload is closed.
func (w *WaitUntil) UnmarshalJSON(b []byte) error {
	var raw waitUntilJSON
	if err := decodeClosed(b, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw.Until, &fields); err != nil {
		return fmt.Errorf("until: %w", err)
	}
	var form Form
	if err := json.Unmarshal(fields["form"], &form); err != nil {
		return fmt.Errorf("until form: %w", err)
	}
	decode, ok := untilDecoders[form]
	if !ok {
		return fmt.Errorf("until form %q is unknown", form)
	}
	delete(fields, "form")
	rest, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	u, err := decode(rest)
	if err != nil {
		return fmt.Errorf("until %s: %w", form, err)
	}
	*w = WaitUntil{Until: u}
	return nil
}

// WentOn is why an Enrollment went on at once from a Wait until instead of waiting on it (ADR
// 0126), decided when it reached the Step and kept on the Step's record.
type WentOn string

// The reasons an Enrollment goes on at once from a Wait until on a date.
const (
	// WentOnPast: the date's moment has passed, and a once date has no other.
	WentOnPast WentOn = "past"
	// WentOnNoDate: the Contact holds no date in the field, or a value that is no date.
	WentOnNoDate WentOn = "no_date"
	// WentOnBeyondBound: the moment lies more than BoundDays ahead.
	WentOnBeyondBound WentOn = "beyond_bound"
)

// Values is every reason, as an ent enum column takes them.
func (WentOn) Values() []string {
	return []string{string(WentOnPast), string(WentOnNoDate), string(WentOnBeyondBound)}
}

// BoundDays is how far ahead, in days of the Contact's calendar, a Wait until waits at most: a
// moment further away goes on at once as WentOnBeyondBound (ADR 0126).
const BoundDays = 366

// Contact is what a Wait until reads of the Contact that reaches it: the zone its local times are
// read in (localcalendar.Zone), the Workspace's zone a datetime value's date is read in, and the
// dates it holds.
type Contact struct {
	Zone, Workspace *time.Location
	Dates           datetrigger.Holder
}

// Due is what a Step that holds the Enrollment decides for one that reaches it, computed once, then
// kept: it waits until a moment (WaitsUntil: a wait's delay, a Wait for event's deadline, a Wait
// until's moment), a Wait until goes on at once for a reason (GoesOn, ADR 0126), or a Wait for event
// on an open or a click goes on at once down the path it already knows of (GoesDown, ADR 0124). A
// type switch over it that misses a kind fails the lint (gochecksumtype).
//
//sumtype:decl
type Due interface {
	due()
}

// WaitsUntil holds the Enrollment on the Step until At.
type WaitsUntil struct {
	At time.Time
}

// GoesOn lets the Enrollment go on at once, for Reason.
type GoesOn struct {
	Reason WentOn
}

// GoesDown lets the Enrollment go on at once down the path Path of the Step it reached; Deadline is
// the Step's deadline all the same, which its record keeps.
type GoesDown struct {
	Path     ID
	Deadline time.Time
}

func (WaitsUntil) due() {}
func (GoesOn) due()     {}
func (GoesDown) due()   {}

// Reach is what the Wait until's moment u decides for the Contact c reaching it at now, computed
// once, then kept (ADR 0126): the first moment at or after now, one equal to now waiting zero
// (decision 11 of #1613). A time of day always has one within a week. A date goes on at once when
// the Contact holds no date in its field, when its moment has passed, or when the moment lies
// more than BoundDays ahead on the Contact's calendar. An error is a moment out of its bounds, which
// its save refuses.
func Reach(u Until, c Contact, now time.Time) (Due, error) {
	switch u := u.(type) {
	case TimeOfDay:
		w, err := u.Calendar()
		if err != nil {
			return nil, err
		}
		return WaitsUntil{At: w.Next(now, c.Zone)}, nil
	case OnDate:
		date, held := u.Field.Of(c.Dates, c.Workspace)
		if !held {
			return GoesOn{Reason: WentOnNoDate}, nil
		}
		schedule, err := u.Schedule()
		if err != nil {
			return nil, err
		}
		next, ok := schedule.Next(date, now, c.Zone)
		if !ok {
			return GoesOn{Reason: WentOnPast}, nil
		}
		if next.At.After(now.In(c.Zone).AddDate(0, 0, BoundDays)) {
			return GoesOn{Reason: WentOnBeyondBound}, nil
		}
		return WaitsUntil{At: next.At}, nil
	case nil:
		return nil, errors.New("automationstep: a wait until without a moment")
	}
	return nil, fmt.Errorf("automationstep: the moment %T has no due moment", u)
}

// OnDates yields each Wait until of the tree that waits on a date, with its Step, in the walker's
// order (All): the one walk every reader of a version's date fields takes.
func OnDates(p Path) iter.Seq2[*Step, OnDate] {
	return func(yield func(*Step, OnDate) bool) {
		for _, st := range All(p) {
			w, ok := st.Payload.(WaitUntil)
			if !ok {
				continue
			}
			if d, ok := w.Until.(OnDate); ok && !yield(st, d) {
				return
			}
		}
	}
}
