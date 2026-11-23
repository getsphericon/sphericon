package events

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// The reads of the Event log (GLOSSARY): the bus owns the Events (ADR 0102), so the app and the
// API list them, filter them and name their distinct actions and recent property keys here. Recording a customer's Event
// is the Event recording, internal/domain/contacts/eventrecording.

// LogFilter narrows the Events ListLog returns; a zero field does not filter.
type LogFilter struct {
	Action string
	// ContactID selects a Contact's activity by the stable identity link, which includes
	// anonymous Events stitched onto the Contact at Identify (ADR 0002).
	ContactID *int64
	// Email matches case-insensitively: Contact emails are stored as entered, but collect
	// ingestion lowercases Event emails, so an exact match would miss tracked Events.
	Email string
	// IncludeMachine lists the opens and clicks made by software too (ADR 0053); by default
	// they stay in the log and out of the page.
	IncludeMachine bool
}

// ListLog returns one page of the Workspace's Events matching f, newest first (id breaks ties).
func ListLog(ctx context.Context, s *ent.Scoped, f LogFilter, p pagination.Params) (pagination.Page[*ent.Event], error) {
	q := s.Event().Query()
	if f.Action != "" {
		q = q.Where(event.ActionEQ(f.Action))
	}
	if f.ContactID != nil {
		q = q.Where(event.ContactID(*f.ContactID))
	}
	if f.Email != "" {
		q = q.Where(event.EmailEqualFold(f.Email))
	}
	if !f.IncludeMachine {
		// The machine column is the projection's (Projection.Machine), set by an open or a click
		// alone: a customer's property of that name never reaches it.
		q = q.Where(HumanOnly())
	}
	return pagination.List(ctx, p, q.Order(ent.Desc(event.FieldCreatedAt), ent.Desc(event.FieldID)))
}

// DistinctActions returns the Workspace's distinct Event actions, sorted.
func DistinctActions(ctx context.Context, s *ent.Scoped) ([]string, error) {
	return s.Event().Query().
		Order(ent.Asc(event.FieldAction)).
		GroupBy(event.FieldAction).
		Strings(ctx)
}

// propertyKeysSample is how many of an action's most recent Events PropertyKeys reads.
const propertyKeysSample = 50

// PropertyKeys returns the top-level property keys of the Workspace's most recent Events of the
// action (the propertyKeysSample latest by occurred_at, which the action's index serves), distinct
// and sorted: the suggestions of the Enrollment key picker (ADR 0165). Property keys have no
// catalogue, so this is a hint drawn from what was recorded, never a check.
func PropertyKeys(ctx context.Context, s *ent.Scoped, action string) ([]string, error) {
	recent, err := s.Event().Query().
		Where(event.Action(action)).
		Order(ent.Desc(event.FieldOccurredAt), ent.Desc(event.FieldID)).
		Limit(propertyKeysSample).
		Select(event.FieldProperties).
		All(ctx)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for _, e := range recent {
		for key := range e.Properties {
			keys[key] = true
		}
	}
	return slices.Sorted(maps.Keys(keys)), nil
}

// OutboundMessageID is the Outbound message an email Event is about, from the id the send or the
// Delivery notice stamped on it; false for any other Event, whose own property of that name is
// the customer's.
func OutboundMessageID(e *ent.Event) (int64, bool) {
	if !strings.HasPrefix(e.Action, "email.") {
		return 0, false
	}
	switch id := e.Properties[outboundMessageIDProperty].(type) {
	case float64:
		return int64(id), id > 0
	case int64:
		return id, id > 0
	}
	return 0, false
}
