package events

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// The unique constraint of the Event log (ADR 0106): one Event per source id.
var _ = db.Declare(
	db.Fault("event_workspace_id_source_id", "storeEvent inserts ON CONFLICT (workspace_id, source_id) DO NOTHING, and an Erasure only clears a source id"),
	db.Fault("event_receipts_receipt_key_key", "claimExternal inserts ON CONFLICT (receipt_key) DO NOTHING so an external fact is consumed once"),
)

// storeEvent writes the Event row of a projected domain event, on client, the producer's
// transaction: it is the one place the Event log is written. It decodes nothing: the row is
// the event's own projection p, so any event type projects faithfully without this function
// knowing its fields.
//
// It reports whether the row is new. An event whose source id (Envelope.SourceID) is already a
// row is a repeat: a provider's redelivery, a client's retried event id, a job that published
// again. The row stays as it was and the caller publishes nothing more. The insert skips the
// conflict rather than failing on it, because a failed statement would abort the producer's
// whole transaction.
func storeEvent(ctx context.Context, client *ent.Client, env Envelope, ev DomainEvent, p Projection) (bool, error) {
	create := client.Event.Create().
		SetWorkspaceID(env.WorkspaceID).
		SetSubjectID(p.Subject).
		SetAction(p.Action).
		SetProperties(p.Properties).
		SetOccurredAt(env.OccurredAt).
		SetBackfill(IsBackfill(ev)).
		SetMachine(p.Machine).
		SetCaptured(p.Captured).
		SetNillableBroadcastID(lo.EmptyableToPtr(p.BroadcastID)).
		SetNillableAutomationID(lo.EmptyableToPtr(p.AutomationID)).
		SetNillableURL(lo.EmptyableToPtr(p.URL)).
		SetNillableVisitorID(lo.EmptyableToPtr(p.VisitorID)).
		SetNillableEmail(lo.EmptyableToPtr(p.Email)).
		SetNillablePhone(lo.EmptyableToPtr(p.Phone)).
		// Envelope.SourceID is never empty: the DedupKey, else the envelope's own id.
		SetSourceID(env.SourceID())
	// The authoritative Contact link, resolved at ingest. 0 ⇒ anonymous (leave null). The
	// producer may publish about a Contact it does not hold (one an Erasure removed in this
	// transaction): the Event is stored unlinked rather than violating the contact_id FK.
	if p.ContactID != 0 {
		linked, err := client.Contact.Query().
			Where(contact.ID(p.ContactID), contact.WorkspaceID(env.WorkspaceID)).Exist(ctx)
		if err != nil {
			return false, fmt.Errorf("resolve contact of event %q: %w", env.Name, err)
		}
		if linked {
			create.SetContactID(p.ContactID)
		}
	}
	// A repeat is told from a first write by the insert its conflict skipped.
	err := create.OnConflictColumns(event.FieldWorkspaceID, event.FieldSourceID).DoNothing().Exec(ctx)
	stored, err := db.Inserted(err)
	if err != nil {
		return false, fmt.Errorf("store event %q: %w", env.Name, err)
	}
	return stored, nil
}
