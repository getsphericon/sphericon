// Package eventlog is the one place a customer's Events enter and are summarized
// (ADR 0016, use-case modules). Ingest resolves each Event's identity to an existing
// Contact (ADR 0002) and publishes the batch in a single transaction; Actions is the
// workspace's distinct-action query. The /site and /api handlers are thin adapters
// over it and no longer duplicate either piece.
//
// The persist subscriber writes the Event rows asynchronously from the outbox, so
// Ingest is accept-then-process.
package eventlog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Module is the Events ingest and read module.
type Module struct {
	bus        *events.Bus
	liveWindow time.Duration
	now        func() time.Time
}

// New builds the module over the domain-event bus (transactional publish). Reads
// and identity resolution go through the Workspace-scoped client each call receives.
// liveWindow is how old a live Event may be: an older one is history and arrives as a
// Backfill, never as a live Event that could enroll an Automation.
func New(bus *events.Bus, liveWindow time.Duration, now func() time.Time) *Module {
	return &Module{bus: bus, liveWindow: liveWindow, now: now}
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

// DedupKey is the Event's source_id for a client event id: source_id is unique across
// Workspaces, the client's id only within one.
func DedupKey(workspaceID int64, clientEventID string) string {
	return fmt.Sprintf("api:%d:%s", workspaceID, clientEventID)
}

// Rejection is why an Event was refused. Its message is worded for the caller.
type Rejection struct{ Reason string }

func (r *Rejection) Error() string { return r.Reason }

// validate refuses an Event the module must not publish: one without an identity or an
// action, one that claims a reserved action, a Backfill without a time that has passed,
// and a live Event old enough to be history.
func (m *Module) validate(in Input) error {
	switch {
	case strings.TrimSpace(in.SubjectID) == "" || strings.TrimSpace(in.Action) == "":
		return &Rejection{"subject id and action are required"}
	case events.ReservedAction(in.Action):
		return &Rejection{fmt.Sprintf("action %q is reserved for the system", in.Action)}
	}
	now := m.now()
	switch {
	case in.Backfill && in.OccurredAt.IsZero():
		return &Rejection{"a backfilled event needs occurredAt"}
	case in.Backfill && in.OccurredAt.After(now):
		return &Rejection{"occurredAt of a backfilled event must not be in the future"}
	case !in.Backfill && !in.OccurredAt.IsZero() && in.OccurredAt.Before(now.Add(-m.liveWindow)):
		return &Rejection{fmt.Sprintf("occurredAt is more than %d hours ago: send history with backfill", int(m.liveWindow.Hours()))}
	}
	return nil
}

// Ingest attaches each Event to an existing Contact by stable identity (an unknown
// identity stays anonymous; a Contact is never created) and publishes the whole batch
// atomically: either every Event is accepted or none is. An Event that fails validate
// refuses the whole batch with a *Rejection.
func (m *Module) Ingest(ctx context.Context, s *ent.Scoped, inputs []Input) error {
	for _, in := range inputs {
		if err := m.validate(in); err != nil {
			return err
		}
	}
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		for _, in := range inputs {
			dedupID := ""
			if in.ClientEventID != "" {
				dedupID = DedupKey(ts.WorkspaceID(), in.ClientEventID)
				// Already stored: a resend of an Event that arrived. One still on the outbox
				// is published again and the persist subscriber's unique source_id keeps
				// one row.
				seen, err := ts.Event().Query().Where(event.SourceID(dedupID)).Exist(ctx)
				if err != nil {
					return err
				}
				if seen {
					continue
				}
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
