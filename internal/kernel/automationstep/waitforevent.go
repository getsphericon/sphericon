package automationstep

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
)

// WaitForEvent holds the Enrollment until what it Awaits arrives or TimeoutSeconds pass from when
// it reached the Step (ADR 0124), then sends it down Arrived or TimedOut. Its paths rejoin at the
// Step after it. Name is the key later Steps read the Event it arrived on by (ADR 0136): it belongs
// to one Step id across every version of the Automation.
type WaitForEvent struct {
	Name           string
	TimeoutSeconds int
	Awaits         Awaits
	Arrived        Path
	TimedOut       Path
}

// Awaits is what a Wait for event waits for, one type per kind: AwaitedEvent. A new kind is an arm
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

// AwaitedEvent waits for an Event one of Matchers matches (ADR 0124, ADR 0180): of the matcher's
// action, its properties passing the matcher's filter. Condition, when set, must hold for the
// Contact, on its live data and the Event the Enrollment began with, when that Event is handled:
// false and unknown leave the Enrollment waiting.
type AwaitedEvent struct {
	Matchers  []eventmatcher.Matcher      `json:"matchers"`
	Condition *conditiontree.WaitForEvent `json:"condition,omitempty"`
}

func (AwaitedEvent) AwaitsKind() AwaitsKind { return AwaitsEvent }

// stored trims each matcher's action, as an Event matcher's action is stored.
func (a AwaitedEvent) stored() Awaits {
	matchers := make([]eventmatcher.Matcher, len(a.Matchers))
	for i, m := range a.Matchers {
		matchers[i] = eventmatcher.Matcher{Action: strings.TrimSpace(m.Action), Filter: m.Filter}
	}
	return AwaitedEvent{Matchers: matchers, Condition: a.Condition}
}

// Actions are the Event actions the matchers name, each once, in their order.
func (a AwaitedEvent) Actions() []string {
	var out []string
	for _, m := range a.Matchers {
		if !slices.Contains(out, m.Action) {
			out = append(out, m.Action)
		}
	}
	return out
}

// The places of a Wait for event's paths in its Step's document, which are its fields.
const (
	placeArrived  = "arrived"
	placeTimedOut = "timedOut"
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

func (w WaitForEvent) Forks() []PlacedPath {
	return []PlacedPath{{Place: placeArrived, Path: w.Arrived}, {Place: placeTimedOut, Path: w.TimedOut}}
}

func (w WaitForEvent) WithForks(paths []Path) Fork {
	w.Arrived, w.TimedOut = paths[0], paths[1]
	return w
}

// awaitsDecoders read the fields of each kind of what is awaited, refusing any field the kind does
// not hold.
var awaitsDecoders = map[AwaitsKind]func([]byte) (Awaits, error){
	AwaitsEvent: decodeAwaitsAs[AwaitedEvent],
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
	return json.Marshal(waitForEventJSON{
		Name: w.Name, TimeoutSeconds: w.TimeoutSeconds, Awaits: awaits, Arrived: emptied(w.Arrived), TimedOut: emptied(w.TimedOut),
	})
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
	*w = WaitForEvent{Name: raw.Name, TimeoutSeconds: raw.TimeoutSeconds, Awaits: a, Arrived: raw.Arrived, TimedOut: raw.TimedOut}
	return nil
}

// WaitsForEvents yields each Wait for event of the tree with its Step, in the walker's order (All).
func WaitsForEvents(p Path) iter.Seq2[*Step, WaitForEvent] {
	return func(yield func(*Step, WaitForEvent) bool) {
		for _, st := range All(p) {
			w, ok := st.Payload.(WaitForEvent)
			if ok && !yield(st, w) {
				return
			}
		}
	}
}
