// Package visitors resolves the anonymous visitors of a customer site to Contacts: it tells which
// Contact a tracked visitor id is, once an identify call has linked them.
package visitors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/samber/lo"
)

type CollectEventInput struct {
	VisitorID  string
	Action     string
	Properties map[string]any
	OccurredAt *time.Time
	// ClientEventID is the client's own id for the event ("" = none). A repeat of an id
	// in a Workspace is a no-op, so a client can retry a batch it never saw answered.
	ClientEventID string
}

// SeeIn returns the Visitor visitorID, marked seen now, and creates it when it has not been seen,
// inside the transaction s is bound to: the device Identify resolves.
func SeeIn(ctx context.Context, s *ent.Scoped, visitorID string, now time.Time) (*ent.Visitor, error) {
	return findOrCreateVisitor(ctx, s, visitorID, now)
}

// EnsureIn returns the Visitor visitorID, creating it when it has not been seen; a Visitor that
// exists is left as it is. A chat started on its behalf binds to it
// (internal/domain/sending/bindings).
func EnsureIn(ctx context.Context, s *ent.Scoped, visitorID string, now time.Time) (*ent.Visitor, error) {
	// A Visitor already there is left as it is, and read below like a new one.
	err := s.Visitor().Create().SetVisitorID(visitorID).SetLastSeenAt(now).
		OnConflictColumns(visitor.FieldWorkspaceID, visitor.FieldVisitorID).
		DoNothing().
		Exec(ctx)
	_, err = db.Inserted(err)
	if err != nil {
		return nil, fmt.Errorf("create visitor: %w", err)
	}
	return s.Visitor().Query().Where(visitor.VisitorID(visitorID)).Only(ctx)
}

// The unique constraint of the Visitor's table (ADR 0106): one Visitor per Visitor id, which
// EnsureIn and SeeIn insert ON CONFLICT on.
var _ = db.Declare(
	db.Fault("visitors_workspace_id_visitor_id", "a Visitor is inserted ON CONFLICT (workspace_id, visitor_id), and its Visitor id never changes"),
)

// BindIn binds the Visitor id to the Contact contactID and marks it seen now, inside the
// transaction s is bound to: the device Identify resolved.
func BindIn(ctx context.Context, s *ent.Scoped, id, contactID int64, now time.Time) error {
	return s.Visitor().UpdateOneID(id).
		SetContactID(contactID).
		SetLastSeenAt(now).
		Exec(ctx)
}

// Collect refuses the whole batch with events.ErrRejected when an event claims a
// reserved action: delivery, consent and Contact lifecycle facts are the system's alone.
func Collect(ctx context.Context, bus *events.Bus, s *ent.Scoped, evts []CollectEventInput, now time.Time) error {
	for _, evt := range evts {
		if events.ReservedAction(evt.Action) {
			return events.CheckAction(evt.Action)
		}
	}
	check := orders.NewCheck(s)
	for _, evt := range evts {
		if err := check.Check(ctx, evt.Action, evt.Properties); err != nil {
			return err
		}
	}
	for _, evt := range evts {
		visitorID := strings.TrimSpace(evt.VisitorID)
		dedupID := ""
		if evt.ClientEventID != "" {
			dedupID = events.DedupKey(events.SourceCollect, s.WorkspaceID(), evt.ClientEventID)
			// Already stored: a retry of an event that arrived, skipped before it touches the
			// visitor. One racing this call is a repeat Publish drops when it finds the Event's
			// row, so it is reacted to once.
			seen, err := s.Event().Query().Where(event.SourceID(dedupID)).Exist(ctx)
			if err != nil {
				return err
			}
			if seen {
				continue
			}
		}
		// Resolve identity and publish in one transaction: the visitor upsert and the
		// event commit together. The collected event is the customer's own — its
		// action and properties are stored as-is.
		if err := bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
			res, err := resolveIdentity(ctx, ts, visitorID, now)
			if err != nil {
				return err
			}
			var occurred time.Time
			if evt.OccurredAt != nil {
				occurred = *evt.OccurredAt
			}
			return pub.Publish(ctx, &events.CollectedEvent{
				WorkspaceID: ts.WorkspaceID(),
				ContactID:   res.contactID,
				VisitorID:   visitorID,
				SubjectID:   res.subjectID,
				Email:       res.email,
				Phone:       res.phone,
				Action:      evt.Action,
				Properties:  evt.Properties,
				OccurredAt:  occurred,
				DedupID:     dedupID,
			})
		}); err != nil {
			return err
		}
	}
	return nil
}

// identityResolution is the Contact a device resolves to at event-ingest time. A
// zero contactID means anonymous (the device is not yet identified); the event is
// recorded with a null contact_id and stitched onto a Contact at the next Identify.
type identityResolution struct {
	contactID int64
	subjectID string
	email     string
	phone     string
}

func resolveIdentity(ctx context.Context, s *ent.Scoped, visitorID string, now time.Time) (*identityResolution, error) {
	vis, err := findOrCreateVisitor(ctx, s, visitorID, now)
	if err != nil {
		return nil, err
	}
	if vis.ContactID == nil {
		return &identityResolution{}, nil // anonymous
	}
	c, err := s.Contact().Get(ctx, *vis.ContactID)
	if ent.IsNotFound(err) {
		return &identityResolution{}, nil // contact gone; treat as anonymous
	}
	if err != nil {
		return nil, err
	}
	return &identityResolution{
		contactID: c.ID,
		subjectID: lo.FromPtr(c.SubjectID),
		email:     lo.FromPtr(c.Email),
		phone:     lo.FromPtr(c.Phone),
	}, nil
}

// findOrCreateVisitor creates the Visitor visitorID seen now, or marks the one there seen now: one
// statement, so two batches of a new device do not race to create it.
func findOrCreateVisitor(ctx context.Context, s *ent.Scoped, visitorID string, now time.Time) (*ent.Visitor, error) {
	id, err := s.Visitor().Create().
		SetVisitorID(visitorID).
		SetLastSeenAt(now).
		OnConflictColumns(visitor.FieldWorkspaceID, visitor.FieldVisitorID).
		Update(func(u *ent.VisitorScopedUpsert) { u.UpdateLastSeenAt().UpdateUpdatedAt() }).
		ID(ctx)
	if err != nil {
		return nil, err
	}
	return s.Visitor().Get(ctx, id)
}
