package events

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The reads of the Event log (GLOSSARY): the bus owns the Events (ADR 0102), so the app and the
// API list them, filter them and name their distinct actions here. Recording a customer's Event
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
		// The flag belongs to opens and clicks only: a customer's own Event may carry a
		// property of that name.
		q = q.Where(event.Or(event.ActionNotIn(append([]string{systemevent.EmailOpened}, ClickActions...)...), HumanOnly()))
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
