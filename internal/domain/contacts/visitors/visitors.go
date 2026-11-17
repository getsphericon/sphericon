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
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/samber/lo"
)

// Module resolves Visitors and collects their Events. The bus opens the transaction each collected
// Event is published in; now is the Instance clock a Visitor is marked seen by.
type Module struct {
	bus *events.Bus
	now func() time.Time
}

// New builds the module.
func New(bus *events.Bus, now func() time.Time) *Module { return &Module{bus: bus, now: now} }

// CollectEventInput is one Event a tracker sent.
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
func (m *Module) SeeIn(ctx context.Context, s *ent.Scoped, visitorID string) (*ent.Visitor, error) {
	return findOrCreateVisitor(ctx, s, visitorID, m.now())
}

// EnsureIn returns the Visitor visitorID, creating it when it has not been seen; a Visitor that
// exists is left as it is. A chat started on its behalf binds to it
// (internal/domain/sending/bindings).
func (m *Module) EnsureIn(ctx context.Context, s *ent.Scoped, visitorID string) (*ent.Visitor, error) {
	if err := checkVisitorID(visitorID); err != nil {
		return nil, err
	}
	// A Visitor already there is left as it is, and read below like a new one.
	err := s.Visitor().Create().SetVisitorID(visitorID).SetLastSeenAt(m.now()).
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
func (m *Module) BindIn(ctx context.Context, s *ent.Scoped, id, contactID int64) error {
	return s.Visitor().UpdateOneID(id).
		SetContactID(contactID).
		SetLastSeenAt(m.now()).
		Exec(ctx)
}

// Collect refuses the whole batch when an event is outside the contract's bounds (its visitor id,
// action or client event id), and with events.ErrRejected when one claims a reserved action:
// delivery, consent and Contact lifecycle facts are the system's alone. Every Visitor of the batch
// is marked seen at the one time the batch arrived.
func (m *Module) Collect(ctx context.Context, s *ent.Scoped, evts []CollectEventInput) error {
	now := m.now()
	for _, evt := range evts {
		if err := checkEvent(evt); err != nil {
			return err
		}
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
		if err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
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

// checkVisitorID refuses a Visitor id outside the contract's bounds (bounds.VisitorID).
func checkVisitorID(visitorID string) error {
	return validation.Length(fieldname.VisitorID, visitorID, bounds.VisitorIDMinLength, bounds.VisitorIDMaxLength)
}

// checkEvent refuses a collected event outside the contract's bounds: its visitor id, its action
// (bounds.CollectEventAction) and its client event id when it has one (bounds.ClientEventID).
func checkEvent(evt CollectEventInput) error {
	if err := checkVisitorID(strings.TrimSpace(evt.VisitorID)); err != nil {
		return err
	}
	if err := validation.Length(fieldname.Action, evt.Action, bounds.CollectEventActionMinLength, bounds.CollectEventActionMaxLength); err != nil {
		return err
	}
	if evt.ClientEventID == "" {
		return nil
	}
	return validation.Length(fieldname.ClientEventID, evt.ClientEventID, bounds.ClientEventIDMinLength, bounds.ClientEventIDMaxLength)
}

// findOrCreateVisitor creates the Visitor visitorID seen now, or marks the one there seen now: one
// statement, so two batches of a new device do not race to create it.
func findOrCreateVisitor(ctx context.Context, s *ent.Scoped, visitorID string, now time.Time) (*ent.Visitor, error) {
	if err := checkVisitorID(visitorID); err != nil {
		return nil, err
	}
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
