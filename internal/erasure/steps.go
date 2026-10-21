package erasure

import (
	"context"
	"encoding/json"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// idChunk bounds how many ids go into one IN (...) list, well below the driver's
// bind-parameter limit.
const idChunk = 10_000

// anonymousKeep lists the Event properties an anonymized system Event keeps: the
// non-personal delivery facts the complaint and bounce rates are computed from
// (ADR 0011). Everything else a system Event carries (urls, provider and message ids,
// provenance) is dropped.
var anonymousKeep = []string{"sendingDomain", "bounceKind"}

// anonymized reports whether an Event action is a reserved system action that
// Erasure keeps as an anonymous row. Every other action is customer-tracked and is
// deleted; marketing.confirmed is deleted too, as consent proof for a person who is
// gone (ADR 0021).
func anonymized(action string) bool {
	switch action {
	case events.NameEmailSent, events.NameEmailOpened, events.NameEmailClicked,
		events.NameEmailBounced, events.NameEmailComplained, events.NameEmailUnsubscribed,
		events.NameContactCreated:
		return true
	}
	return false
}

// cancelBroadcastWork has the Broadcast delivery module cancel the Contact's unsent
// recipients; a Broadcast left with nothing pending settles.
func cancelBroadcastWork(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return broadcasts.CancelPending(ctx, s, t.ContactID, purger)
}

// cancelAutomationWork has the Automation module cancel the queued step calls and trigger
// evaluations that name the Contact or its runs (a failure's response excerpt sits on the
// run, which eraseRuns deletes).
func cancelAutomationWork(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return automations.CancelPending(ctx, s, t.ContactID, purger)
}

// purgeOutbox deletes the domain-event outbox rows that name the subject; the bus is the
// only owner of that table, and the purge shares Erasure's transaction.
func purgeOutbox(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return purger.PurgeOutbox(ctx, s.WorkspaceID(), t.ContactID, t.Destinations)
}

// purgeWebhookDeliveries deletes the queued webhook deliveries whose payload names the
// subject; the event envelope's owner knows that payload shape.
func purgeWebhookDeliveries(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return purger.PurgeWebhookJobs(ctx, s.WorkspaceID(), t.ContactID, t.Destinations)
}

func visitorOf(contactID int64) predicate.Visitor { return visitor.ContactID(contactID) }

// eraseEvents deletes the subject's customer-tracked Events and anonymizes its
// system Events in place. Events are found by contact_id (Identify stitched earlier
// anonymous Events onto it) and by the subject's visitor ids (Events not yet stitched).
func eraseEvents(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	var matches []predicate.Event
	if t.ContactID != 0 {
		matches = append(matches, event.ContactID(t.ContactID))
	}
	if len(t.VisitorIDs) > 0 {
		matches = append(matches, event.VisitorIDIn(t.VisitorIDs...))
	}
	if len(matches) == 0 {
		return nil
	}
	match := event.Or(matches...)
	rows, err := s.Event().Query().Where(match).Select(event.FieldID, event.FieldAction, event.FieldProperties).All(ctx)
	if err != nil {
		return err
	}

	var doomed []int64
	// Anonymized Events grouped by the properties they keep, so each distinct property
	// set is one bulk update.
	keptBy := map[string][]int64{}
	kept := map[string]map[string]any{}
	for _, e := range rows {
		if !anonymized(e.Action) {
			doomed = append(doomed, e.ID)
			continue
		}
		props := map[string]any{}
		for _, k := range anonymousKeep {
			if v, ok := e.Properties[k]; ok {
				props[k] = v
			}
		}
		key, err := json.Marshal(props) // map keys marshal sorted: a stable group key
		if err != nil {
			return err
		}
		keptBy[string(key)] = append(keptBy[string(key)], e.ID)
		kept[string(key)] = props
	}

	for _, ids := range lo.Chunk(doomed, idChunk) {
		if _, err := s.Event().Delete().Where(event.IDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
	}
	for key, ids := range keptBy {
		for _, chunk := range lo.Chunk(ids, idChunk) {
			if _, err := s.Event().Update().Where(event.IDIn(chunk...)).
				ClearContactID().ClearVisitorID().ClearEmail().ClearPhone().ClearSubjectID().
				SetProperties(kept[key]).
				Save(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func eraseVisitors(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	match := visitor.VisitorIDIn(t.VisitorIDs...)
	if t.ContactID != 0 {
		match = visitor.Or(visitorOf(t.ContactID), match)
	}
	_, err := s.Visitor().Delete().Where(match).Exec(ctx)
	return err
}

// eraseConfirmations deletes the Confirmation; losing it fails safe (re-confirmation).
func eraseConfirmations(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	_, err := s.Confirmation().Delete().Where(confirmation.ContactID(t.ContactID)).Exec(ctx)
	return err
}

func eraseRuns(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	_, err := s.AutomationRun().Delete().Where(automationrun.ContactID(t.ContactID)).Exec(ctx)
	return err
}

// detachOptOuts keeps the Unsubscribe and Suppression rows, which are keyed by
// Destination and keep honoring the refusal, and only clears their Contact reference.
func detachOptOuts(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if _, err := s.Unsubscribe().Update().Where(unsubscribe.ContactID(t.ContactID)).ClearContactID().Save(ctx); err != nil {
		return err
	}
	return suppression.DetachContact(ctx, s, t.ContactID)
}

// anonymizeDelivery turns the subject's delivery records into anonymous rows: they keep
// their status and timing, so reports keep their totals, but no longer name anyone.
func anonymizeDelivery(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if _, err := s.OutboundMessage().Update().Where(outboundmessage.ContactID(t.ContactID)).
		ClearContactID().ClearDestination().Save(ctx); err != nil {
		return err
	}
	if _, err := s.BroadcastRecipient().Update().Where(broadcastrecipient.ContactID(t.ContactID)).
		ClearContactID().Save(ctx); err != nil {
		return err
	}
	// A transactional send to the address may never have had a Contact: it is found by
	// destination.
	if len(t.Destinations) == 0 {
		return nil
	}
	_, err := s.OutboundMessage().Update().Where(outboundmessage.DestinationIn(t.Destinations...)).
		ClearContactID().ClearDestination().Save(ctx)
	return err
}

// eraseContact removes the Contact itself; its Tag links and Custom field values go
// with the row.
func eraseContact(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if t.ContactID == 0 {
		return nil
	}
	return s.Contact().DeleteOneID(t.ContactID).Exec(ctx)
}

// publishErased emits the PII-free contact.erased Event in the erasure's transaction,
// once per Erasure. The subject id travels in the message for the webhook delivery
// and is not stored on the Event (events.ContactErased.Project).
func publishErased(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
	return pub.Publish(ctx, &events.ContactErased{
		WorkspaceID:    s.WorkspaceID(),
		SubjectID:      t.SubjectID,
		IdentifierKind: t.IdentifierKind,
		RequesterKind:  t.Requester.Kind,
		RequesterID:    t.Requester.ID,
		ContactID:      t.ContactID,
		VisitorIDs:     t.VisitorIDs,
		EmailHash:      Hash(firstDestination(t)),
		PhoneHash:      Hash(t.Phone),
	})
}
