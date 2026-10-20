package visitors

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/samber/lo"
)

type IdentifyInput struct {
	VisitorID string
	Email     *string
	Phone     *string
	SubjectID *string
	Traits    map[string]any
}

type CollectEventInput struct {
	VisitorID  string
	Action     string
	Properties map[string]any
	OccurredAt *time.Time
	// ClientEventID is the client's own id for the event ("" = none). A repeat of an id
	// in a Workspace is a no-op, so a client can retry a batch it never saw answered.
	ClientEventID string
}

// Identify binds a Visitor to a Contact and asserts that Contact's alias keys
// (subject_id / email / phone). It upserts the Contact by any present alias key,
// auto-creates typed Custom fields from the traits, binds the device, and stitches
// the device's earlier anonymous Events onto the Contact so pre-identify behavior
// becomes visible to segmentation. A newly created Contact emits contact.created.
func Identify(ctx context.Context, bus *events.Bus, s *ent.Scoped, input IdentifyInput) error {
	visitorID := strings.TrimSpace(input.VisitorID)
	if visitorID == "" {
		return errors.New("visitorId is required")
	}

	return bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		res, err := contacts.UpsertIn(ctx, ts, pub, contacts.Attributes{
			SubjectID:    input.SubjectID,
			Email:        input.Email,
			Phone:        input.Phone,
			CustomFields: input.Traits,
		})
		if errors.Is(err, contacts.ErrIdentityRequired) {
			return errors.New("identify requires subjectId, email, or phone")
		}
		if err != nil {
			return err
		}
		c := res.Contact

		vis, err := findOrCreateVisitor(ctx, ts, visitorID)
		if err != nil {
			return err
		}
		if err := ts.Visitor().UpdateOneID(vis.ID).
			SetContactID(c.ID).
			SetLastSeenAt(time.Now()).
			Exec(ctx); err != nil {
			return err
		}

		// Stitch: attach the device's earlier anonymous events onto the Contact.
		if _, err := ts.Event().Update().
			Where(
				event.VisitorID(visitorID),
				event.ContactIDIsNil(),
			).
			SetContactID(c.ID).
			Save(ctx); err != nil {
			return err
		}

		return nil
	})
}

// Collect refuses the whole batch with an *events.Rejection when an event claims a
// reserved action: delivery, consent and Contact lifecycle facts are the system's alone.
func Collect(ctx context.Context, bus *events.Bus, s *ent.Scoped, evts []CollectEventInput) error {
	for _, evt := range evts {
		if events.ReservedAction(evt.Action) {
			return events.CheckAction(evt.Action)
		}
	}
	for _, evt := range evts {
		visitorID := strings.TrimSpace(evt.VisitorID)
		dedupID := ""
		if evt.ClientEventID != "" {
			dedupID = events.DedupKey(events.SourceCollect, s.WorkspaceID(), evt.ClientEventID)
			// Already stored: a retry of an event that arrived. One still on the outbox, or
			// racing this call, is published again and the persist subscriber's unique
			// source_id keeps one Event row; the webhook id is the same key, so a receiver
			// drops the duplicate. Enrolling a Contact is once per
			// Automation, so a trigger evaluated twice in that window enrolls once.
			seen, err := s.Event().Query().Where(event.SourceID(dedupID)).Exist(ctx)
			if err != nil {
				return err
			}
			if seen {
				continue
			}
		}
		// Resolve identity and publish in one transaction: the visitor upsert and the
		// outbox row commit together. The collected event is the customer's own — its
		// action and properties are stored as-is.
		if err := bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
			res, err := resolveIdentity(ctx, ts, visitorID)
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

func resolveIdentity(ctx context.Context, s *ent.Scoped, visitorID string) (*identityResolution, error) {
	vis, err := findOrCreateVisitor(ctx, s, visitorID)
	if err != nil {
		return nil, err
	}
	if vis.ContactID == nil {
		return &identityResolution{}, nil // anonymous
	}
	c, err := s.Contact().Get(ctx, *vis.ContactID)
	if err != nil {
		return &identityResolution{}, nil // contact gone; treat as anonymous
	}
	return &identityResolution{
		contactID: c.ID,
		subjectID: lo.FromPtr(c.SubjectID),
		email:     lo.FromPtr(c.Email),
		phone:     lo.FromPtr(c.Phone),
	}, nil
}

func findOrCreateVisitor(ctx context.Context, s *ent.Scoped, visitorID string) (*ent.Visitor, error) {
	existing, err := s.Visitor().Query().
		Where(visitor.VisitorID(visitorID)).
		First(ctx)
	if err == nil {
		return s.Visitor().UpdateOneID(existing.ID).
			SetLastSeenAt(time.Now()).
			Save(ctx)
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	return s.Visitor().Create().
		SetVisitorID(visitorID).
		SetLastSeenAt(time.Now()).
		Save(ctx)
}
