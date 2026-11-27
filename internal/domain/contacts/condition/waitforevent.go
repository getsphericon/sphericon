package condition

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// A Wait for event's Condition (ADR 0124, ADR 0180) is of its own place: a Branch path's leaves
// but the test of the Enrollment's own message, its property tests reading the Event the
// Enrollment began with or the one an earlier Wait for event arrived on (ADR 0136). It has no
// subject for the Event the Wait for event awaits, whose properties its matcher's filter tests. It
// reads unknown as no match, as a Rule's does.

// CheckWaitForEvent checks a Wait for event's Condition as a Branch path's is checked: it reads as
// the place's tree, and every leaf names a Contact attribute, a Tag, an Event action or a Custom
// field of the Workspace of s, by a test its type takes. A property test may name any key.
func (m *Module) CheckWaitForEvent(ctx context.Context, s *ent.Scoped, g conditiontree.WaitForEvent) error {
	tree, err := decode(g.Group, waitPlace)
	if err != nil {
		return err
	}
	_, err = (&compiler{s: s, now: m.now(), trigger: json.RawMessage(`{}`)}).group(ctx, tree)
	return err
}

// WaitForEventInput is what Holds reads: the Contact, the Wait for event's Condition, the
// properties of the Event the Enrollment began with (nil for none), which its tests of the
// triggering Event read, and the properties of the Events the Enrollment's earlier Wait for event
// Steps arrived on, by each Step's name, which its tests of the awaited Event read (a name absent
// from it has no copy, ADR 0136).
type WaitForEventInput struct {
	ContactID  int64
	Condition  conditiontree.WaitForEvent
	Properties map[string]any
	Awaited    map[string]map[string]any
}

// Holds reports whether a Wait for event's Condition holds for its Contact, on its live data now:
// false when it fails and when it is unknown (an engagement count of a Destination that is not
// tracked, ADR 0123), so a test the app cannot answer never moves the Enrollment on. The Event's
// properties are a JSON object bound to the statement, which each property test reads (ADR 0180).
// A Condition that does not compile is an error; a failure to read is ErrStorage.
func (m *Module) Holds(ctx context.Context, s *ent.Scoped, in WaitForEventInput) (bool, error) {
	tree, err := decode(in.Condition.Group, waitPlace)
	if err != nil {
		return false, err
	}
	trigger, err := propertiesDocument(in.Properties)
	if err != nil {
		return false, fmt.Errorf("encode the Enrollment's Event properties: %w", err)
	}
	held, err := (&compiler{s: s, now: m.now(), trigger: trigger, awaited: in.Awaited}).group(ctx, tree)
	if err != nil {
		return false, err
	}
	holds, err := s.Contact().Query().Where(contact.ID(in.ContactID), held).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: read the Wait for event's Condition: %w", ErrStorage, err)
	}
	return holds, nil
}
