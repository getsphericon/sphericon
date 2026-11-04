// Package eventlog is the one place a customer's Events enter and are summarized
// (ADR 0016, use-case modules). Ingest resolves each Event's identity to an existing
// Contact (ADR 0002) and publishes the batch in a single transaction; Actions is the
// workspace's distinct-action query. The /site and /api handlers are thin adapters
// over it and no longer duplicate either piece.
//
// Publishing an Event writes its row in Ingest's transaction; what reacts to it
// (Automations, Rules, webhooks) runs asynchronously, so Ingest is accept-then-process.
package eventlog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Module is the Events ingest and read module.
type Module struct {
	bus  *events.Bus
	live LivePolicy
}

// LivePolicy says how old a live Event may be: an older one is history and arrives as a
// Backfill, never as a live Event that could enroll an Automation. Now is the clock the
// age is judged against.
type LivePolicy struct {
	Window time.Duration
	Now    func() time.Time
}

// New builds the module over the domain-event bus (transactional publish). Reads
// and identity resolution go through the Workspace-scoped client each call receives.
func New(bus *events.Bus, live LivePolicy) *Module {
	return &Module{bus: bus, live: live}
}

// Input is one customer Event to ingest. Action and Properties are the customer's
// own and are stored as-is; the alias keys (SubjectID, Email, Phone) identify the
// Contact it belongs to, if one exists.
type Input struct {
	SubjectID  string
	Email      *string
	Phone      *string
	Action     string
	OccurredAt time.Time // zero => publish time
	Properties map[string]any
	// ClientEventID is the client's own id for the Event ("" = none). A repeat of an id in
	// a Workspace is a no-op, so a client can resend a load it never saw answered.
	ClientEventID string
	// Backfill records the Event as history: no Automation enrolls on it and no webhook
	// endpoint receives it.
	Backfill bool
}

// validate refuses an Event the module must not publish: one without an identity or an
// action, one that claims a reserved action, a Backfill without a time that has passed,
// and a live Event old enough to be history.
func (m *Module) validate(in Input) error {
	if strings.TrimSpace(in.SubjectID) == "" {
		return events.Reject("subject id and action are required")
	}
	if err := events.CheckAction(in.Action); err != nil {
		return err
	}
	now := m.live.Now()
	switch {
	case in.Backfill && in.OccurredAt.IsZero():
		return events.Reject("a backfilled event needs occurredAt")
	case in.Backfill && in.OccurredAt.After(now):
		return events.Reject("occurredAt of a backfilled event must not be in the future")
	case !in.Backfill && !in.OccurredAt.IsZero() && in.OccurredAt.Before(now.Add(-m.live.Window)):
		return events.Reject(fmt.Sprintf("occurredAt is more than %d hours ago: send history with backfill", int(m.live.Window.Hours())))
	}
	return nil
}

// Ingest attaches each Event to an existing Contact by stable identity (an unknown
// identity stays anonymous; a Contact is never created) and publishes the whole batch
// atomically: either every Event is accepted or none is. An Event that fails validate
// refuses the whole batch with an events.ErrRejected.
func (m *Module) Ingest(ctx context.Context, s *ent.Scoped, inputs []Input) error {
	for _, in := range inputs {
		if err := m.validate(in); err != nil {
			return err
		}
	}
	check := orders.NewCheck(s)
	for _, in := range inputs {
		if err := check.Check(ctx, in.Action, in.Properties); err != nil {
			return err
		}
	}
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		for _, in := range inputs {
			// A client event id is the Event's source id. A repeat of it, inside this batch or
			// from an earlier request, is the same Event: Publish finds its row and publishes
			// nothing, so it is recorded and reacted to once.
			dedupID := ""
			if in.ClientEventID != "" {
				dedupID = events.DedupKey(events.SourceAPI, ts.WorkspaceID(), in.ClientEventID)
			}
			contactID, err := contacts.ResolveID(ctx, ts, in.SubjectID, in.Email, in.Phone)
			if err != nil {
				return err
			}
			collected := &events.CollectedEvent{
				WorkspaceID: ts.WorkspaceID(),
				ContactID:   contactID,
				SubjectID:   in.SubjectID,
				Action:      in.Action,
				OccurredAt:  in.OccurredAt,
				Properties:  in.Properties,
				DedupID:     dedupID,
				Backfill:    in.Backfill,
			}
			if in.Email != nil {
				collected.Email = *in.Email
			}
			if in.Phone != nil {
				collected.Phone = *in.Phone
			}
			if err := pub.Publish(ctx, collected); err != nil {
				return err
			}
		}
		return nil
	})
}

// IngestEach ingests each Event in its own transaction, so an invalid or failing item
// does not affect the others (Ingest, by contrast, is all-or-nothing). The returned
// errors are parallel to inputs; nil means the Event was accepted.
func (m *Module) IngestEach(ctx context.Context, s *ent.Scoped, inputs []Input) []error {
	errs := make([]error, len(inputs))
	for i, in := range inputs {
		errs[i] = m.Ingest(ctx, s, []Input{in})
	}
	return errs
}

// Actions returns the workspace's distinct Event actions, sorted.
func (m *Module) Actions(ctx context.Context, s *ent.Scoped) ([]string, error) {
	return s.Event().Query().
		Order(ent.Asc(event.FieldAction)).
		GroupBy(event.FieldAction).
		Strings(ctx)
}

// Filter narrows the Events List returns; a zero field does not filter.
type Filter struct {
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

// List returns one page of the workspace's Events matching f, newest first (id breaks ties).
func (m *Module) List(ctx context.Context, s *ent.Scoped, f Filter, p pagination.Params) (pagination.Page[*ent.Event], error) {
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
		q = q.Where(event.Or(event.ActionNotIn(append([]string{events.NameEmailOpened}, events.ClickActions...)...), events.HumanOnly()))
	}
	return pagination.List(ctx, p, q.Count, func(ctx context.Context, limit, offset int) ([]*ent.Event, error) {
		return q.Clone().Order(ent.Desc(event.FieldCreatedAt), ent.Desc(event.FieldID)).
			Limit(limit).Offset(offset).All(ctx)
	})
}

// ListActions returns one page of the distinct Event actions, ascending.
func (m *Module) ListActions(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[string], error) {
	distinct := func() *ent.EventQuery {
		return s.Event().Query().Unique(true)
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return distinct().Select(event.FieldAction).Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]string, error) {
			return distinct().Order(ent.Asc(event.FieldAction)).Limit(limit).Offset(offset).
				Select(event.FieldAction).Strings(ctx)
		})
}

// Subjects is the subject each of the Events' emails kept, by Outbound message id: the one the
// recipient received, merge tags filled. An Event that names no message, or whose message kept no
// subject (sent before it was kept, or erased with the Contact), has no entry.
func (m *Module) Subjects(ctx context.Context, s *ent.Scoped, evs []*ent.Event) (map[int64]string, error) {
	ids := make([]int64, 0, len(evs))
	for _, e := range evs {
		if id, ok := OutboundMessageID(e); ok {
			ids = append(ids, id)
		}
	}
	subjects := map[int64]string{}
	if len(ids) == 0 {
		return subjects, nil
	}
	rows, err := s.OutboundMessage().Query().
		Where(outboundmessage.IDIn(ids...), outboundmessage.SubjectNotNil()).
		Select(outboundmessage.FieldID, outboundmessage.FieldSubject).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		subjects[r.ID] = *r.Subject
	}
	return subjects, nil
}

// OutboundMessageID is the Outbound message an email Event is about, from the id the send or the
// Delivery notice stamped on it; false for any other Event, whose own property of that name is
// the customer's.
func OutboundMessageID(e *ent.Event) (int64, bool) {
	if !strings.HasPrefix(e.Action, "email.") {
		return 0, false
	}
	switch id := e.Properties["outboundMessageId"].(type) {
	case float64:
		return int64(id), id > 0
	case int64:
		return id, id > 0
	}
	return 0, false
}
