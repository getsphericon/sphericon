package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// storeEvent writes the Event row of a projected domain event, on client, the producer's
// transaction: it is the one place the Event log is written. It decodes nothing: the row is
// the event's own Project(), so any event type projects faithfully without this function
// knowing its fields.
//
// It reports whether the row is new. An event whose source id (Envelope.SourceID) is already a
// row is a repeat: a provider's redelivery, a client's retried event id, a job that published
// again. The row stays as it was and the caller publishes nothing more. The insert skips the
// conflict rather than failing on it, because a failed statement would abort the producer's
// whole transaction.
func storeEvent(ctx context.Context, client *ent.Client, env Envelope, ev DomainEvent) (bool, error) {
	p := ev.Project()
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
	// ent reports a conflict skipped by DoNothing as sql.ErrNoRows, which is how a repeat is told
	// from a first write.
	err := create.OnConflictColumns(event.FieldSourceID).DoNothing().Exec(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store event %q: %w", env.Name, err)
	}
	return true, nil
}

// recordProviderOutcome counts a newly stored provider bounce or complaint on the send-outcome
// counter. It runs once the Event's transaction committed, so neither a repeated notice nor a
// rolled-back one counts.
func recordProviderOutcome(ctx context.Context, ev DomainEvent) {
	f, ok := ev.(*EmailDelivery)
	if !ok || f.Provider == "" {
		return
	}
	var status messaging.SendStatus
	switch f.Action {
	case NameEmailBounced:
		status = messaging.SendBounce
	case NameEmailComplained:
		status = messaging.SendComplaint
	default:
		return
	}
	// Provider is the Preset's name (ADR 0037), or "ses" for the SES adapter.
	messaging.RecordSendOutcome(ctx, messaging.Provider(f.Provider), status)
}
