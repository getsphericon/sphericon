package automationstep

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// WaitForEvent holds the Enrollment until what it Awaits arrives or TimeoutSeconds pass from when
// it reached the Step (ADR 0124), then sends it down Arrived or TimedOut, or down Unknown when it
// awaits an open or a click it cannot know of (AwaitedMessage). Its paths rejoin at the Step after
// it. Name is the key later Steps read the Event it arrived on by (ADR 0136): it belongs to one Step
// id across every version of the Automation.
type WaitForEvent struct {
	Name           string
	TimeoutSeconds int
	Awaits         Awaits
	Arrived        Path
	TimedOut       Path
	Unknown        *Path
}

// Awaits is what a Wait for event waits for, one type per kind: AwaitedEvent and AwaitedMessage. A
// new kind is an arm
// of the contract's WaitForEventAwaits, whose discriminator generates its AwaitsKind
// (awaits_gen.go); it adds its type and its line in awaitsDecoders, and a type switch over Awaits
// that misses it fails the lint (gochecksumtype).
//
//sumtype:decl
type Awaits interface {
	AwaitsKind() AwaitsKind
	// stored is what is awaited as a version stores it.
	stored() Awaits
}

// AwaitedEvent waits for an Event of one of Actions.
type AwaitedEvent struct {
	Actions []string `json:"actions"`
}

func (AwaitedEvent) AwaitsKind() AwaitsKind { return AwaitsEvent }

// stored trims each action, as an Event matcher's action is stored.
func (a AwaitedEvent) stored() Awaits {
	actions := make([]string, len(a.Actions))
	for i, action := range a.Actions {
		actions[i] = strings.TrimSpace(action)
	}
	return AwaitedEvent{Actions: actions}
}

// AwaitedMessage waits for an open or a click (Engagement) of the message the send Step of the id
// Step sent to the Enrollment, with the meaning a Branch's test of that message has (ADR 0122).
type AwaitedMessage struct {
	Step       ID                       `json:"step"`
	Engagement conditiontree.Engagement `json:"engagement"`
}

func (AwaitedMessage) AwaitsKind() AwaitsKind { return AwaitsMessage }

func (a AwaitedMessage) stored() Awaits { return a }

// The places of a Wait for event's paths in its Step's document, which are its fields.
const (
	placeArrived  = "arrived"
	placeTimedOut = "timedOut"
	placeUnknown  = "unknown"
)

func (WaitForEvent) Kind() Kind { return KindWaitForEvent }

// Stored is the Wait for event with what it awaits in its stored form: the walker stores the Steps
// of its paths (Rewrite).
func (w WaitForEvent) Stored() Payload {
	if w.Awaits != nil {
		w.Awaits = w.Awaits.stored()
	}
	return w
}

// Forks are arrived, timed out, then unknown when the Wait for event has it.
func (w WaitForEvent) Forks() []PlacedPath {
	out := []PlacedPath{{Place: placeArrived, Path: w.Arrived}, {Place: placeTimedOut, Path: w.TimedOut}}
	if w.Unknown != nil {
		out = append(out, PlacedPath{Place: placeUnknown, Path: *w.Unknown})
	}
	return out
}

func (w WaitForEvent) WithForks(paths []Path) Fork {
	w.Arrived, w.TimedOut = paths[0], paths[1]
	if w.Unknown != nil {
		unknown := paths[2]
		w.Unknown = &unknown
	}
	return w
}

// awaitsDecoders read the fields of each kind of what is awaited, refusing any field the kind does
// not hold.
var awaitsDecoders = map[AwaitsKind]func([]byte) (Awaits, error){
	AwaitsEvent:   decodeAwaitsAs[AwaitedEvent],
	AwaitsMessage: decodeAwaitsAs[AwaitedMessage],
}

func decodeAwaitsAs[A Awaits](b []byte) (Awaits, error) {
	var a A
	if err := decodeClosed(b, &a); err != nil {
		return nil, err
	}
	return a, nil
}

// waitForEventJSON is a Wait for event as it is stored: what it awaits is an object with its kind
// beside the kind's fields.
type waitForEventJSON struct {
	Name           string          `json:"name"`
	TimeoutSeconds int             `json:"timeoutSeconds"`
	Awaits         json.RawMessage `json:"awaits"`
	Arrived        Path            `json:"arrived"`
	TimedOut       Path            `json:"timedOut"`
	Unknown        *Path           `json:"unknown,omitempty"`
}

// MarshalJSON writes the Wait for event with the kind of what it awaits beside that kind's fields.
func (w WaitForEvent) MarshalJSON() ([]byte, error) {
	if w.Awaits == nil {
		return nil, errors.New("automationstep: a wait for event that awaits nothing")
	}
	b, err := json.Marshal(w.Awaits)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	if fields["kind"], err = json.Marshal(w.Awaits.AwaitsKind()); err != nil {
		return nil, err
	}
	awaits, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	stored := waitForEventJSON{
		Name: w.Name, TimeoutSeconds: w.TimeoutSeconds, Awaits: awaits, Arrived: emptied(w.Arrived), TimedOut: emptied(w.TimedOut),
	}
	if w.Unknown != nil {
		stored.Unknown = new(emptied(*w.Unknown))
	}
	return json.Marshal(stored)
}

// emptied is the path with no Steps written as an empty list, which a path stores.
func emptied(p Path) Path {
	if p.Steps == nil {
		p.Steps = []Step{}
	}
	return p
}

// UnmarshalJSON reads a Wait for event written by MarshalJSON. A kind nobody knows, and a field the
// Wait for event or what it awaits does not hold, are errors: the stored payload is closed.
func (w *WaitForEvent) UnmarshalJSON(b []byte) error {
	var raw waitForEventJSON
	if err := decodeClosed(b, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw.Awaits, &fields); err != nil {
		return fmt.Errorf("awaits: %w", err)
	}
	var kind AwaitsKind
	if err := json.Unmarshal(fields["kind"], &kind); err != nil {
		return fmt.Errorf("awaits kind: %w", err)
	}
	decode, ok := awaitsDecoders[kind]
	if !ok {
		return fmt.Errorf("awaits kind %q is unknown", kind)
	}
	delete(fields, "kind")
	rest, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	a, err := decode(rest)
	if err != nil {
		return fmt.Errorf("awaits %s: %w", kind, err)
	}
	*w = WaitForEvent{Name: raw.Name, TimeoutSeconds: raw.TimeoutSeconds, Awaits: a, Arrived: raw.Arrived, TimedOut: raw.TimedOut, Unknown: raw.Unknown}
	return nil
}

// WaitsForEvents yields each Wait for event of the tree with its Step, in the walker's order (All),
// as a copy the caller may read.
func WaitsForEvents(p Path) iter.Seq2[*Step, *WaitForEvent] {
	return func(yield func(*Step, *WaitForEvent) bool) {
		for _, st := range All(p) {
			w, ok := st.Payload.(WaitForEvent)
			if ok && !yield(st, &w) {
				return
			}
		}
	}
}
