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
	"errors"
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
	bus *events.Bus
}

// New builds the module over the domain-event bus (transactional publish). Reads
// and identity resolution go through the Workspace-scoped client each call receives.
func New(bus *events.Bus) *Module {
	return &Module{bus: bus}
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
}

// Ingest attaches each Event to an existing Contact by stable identity (an unknown
// identity stays anonymous; a Contact is never created) and publishes the whole batch
// atomically: either every Event is accepted or none is.
func (m *Module) Ingest(ctx context.Context, s *ent.Scoped, inputs []Input) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		for _, in := range inputs {
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

// ErrInvalid means an Event lacks its subject id or its action.
var ErrInvalid = errors.New("eventlog: subject id and action are required")

// IngestEach ingests each Event in its own transaction, so an invalid or failing item
// does not affect the others (Ingest, by contrast, is all-or-nothing). The returned
// errors are parallel to inputs; nil means the Event was accepted.
func (m *Module) IngestEach(ctx context.Context, s *ent.Scoped, inputs []Input) []error {
	errs := make([]error, len(inputs))
	for i, in := range inputs {
		if strings.TrimSpace(in.SubjectID) == "" || strings.TrimSpace(in.Action) == "" {
			errs[i] = ErrInvalid
			continue
		}
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
