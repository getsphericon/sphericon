package automationstep

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// WaitUntil holds the Enrollment until a calendar moment (ADR 0126), which Until says by its form.
// The moment is computed once, when the Enrollment reaches the Step, and kept as the Step's due
// moment.
type WaitUntil struct {
	Until Until
}

// Form is the form of a Wait until's moment: the `form` it is stored and shown under.
type Form string

// The forms of a Wait until's moment.
const (
	FormTimeOfDay Form = "time_of_day"
)

// Until is the moment a Wait until holds the Enrollment until, one type per form: TimeOfDay. A new
// form adds its type, its Form constant and its line in untilDecoders, and a type switch over
// Until that misses it fails the lint (gochecksumtype).
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
