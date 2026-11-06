package contacterasure

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/capturedmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

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

// cancelRuleWork has the Rule module cancel the queued evaluations that name the Contact.
func cancelRuleWork(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return rules.CancelPending(ctx, s, t.ContactID, purger)
}

// eraseFirings deletes the Contact's Rule Firings.
func eraseFirings(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return rules.EraseFirings(ctx, s, t.ContactID)
}

// purgeEventJobs deletes the queued event consumer jobs whose envelope names the subject; the
// bus inserted them, and the purge shares Erasure's transaction.
func purgeEventJobs(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return purger.PurgeEventJobs(ctx, s.WorkspaceID(), t.ContactID, t.destinations())
}

// purgeWebhookDeliveries deletes the queued webhook deliveries whose payload names the
// subject; the event envelope's owner knows that payload shape.
func purgeWebhookDeliveries(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return purger.PurgeWebhookJobs(ctx, s.WorkspaceID(), t.ContactID, t.destinations())
}

func visitorOf(contactID int64) predicate.Visitor { return visitor.ContactID(contactID) }

// eraseShortLinks has the Outbound message module delete the short links of the Contact's
// messages, before anonymizeDelivery clears the contact_id they are found by.
func eraseShortLinks(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return outboundmessages.EraseShortLinks(ctx, s, t.ContactID)
}

// eraseTelegram has the Binding module delete the Bindings and Start tokens of the Contact and of
// its Visitors, so it runs before eraseVisitors. Telegram opt-outs are not here: they are keyed by
// the user id and stay (detachOptOuts).
func eraseTelegram(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return bindings.EraseSubject(ctx, s, t.ContactID, t.VisitorIDs)
}

// eraseEvents has the bus, which stores the Events, delete the subject's customer-tracked Events
// and anonymize its system Events in place.
func eraseEvents(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return events.EraseSubject(ctx, s, t.ContactID, t.Phone, t.VisitorIDs)
}

func eraseVisitors(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return visitors.EraseSubject(ctx, s, t.ContactID, t.VisitorIDs)
}

// eraseConfirmations deletes the Confirmation and its evidence; losing it fails safe
// (re-confirmation).
func eraseConfirmations(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return confirmations.EraseContact(ctx, s, t.ContactID, t.Phone)
}

func eraseRuns(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return automations.EraseRuns(ctx, s, t.ContactID)
}

// detachOptOuts keeps the Unsubscribe and active Suppression rows, which are keyed by
// Destination and keep honoring the refusal, and only clears their Contact reference. A
// lifted Suppression honors nothing, so it goes with the Contact. On sms the rows keep refusing
// the number by its hash, never by the number itself (ADR 0021): the phone leaves only its hash.
func detachOptOuts(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := unsubscribes.EraseSMS(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	if err := unsubscribes.DetachContact(ctx, s, t.ContactID); err != nil {
		return err
	}
	return suppressions.EraseContact(ctx, s, t.ContactID, t.Phone)
}

// anonymizeDelivery turns the subject's delivery records into anonymous rows: they keep
// their status and timing, so reports keep their totals, but no longer name anyone. The
// Contact's Outbound messages and Broadcast recipients go first, then the messages to its
// addresses and number, which a transactional send may have made without a Contact.
func anonymizeDelivery(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := outboundmessages.AnonymizeContact(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	if err := broadcasts.DetachRecipients(ctx, s, t.ContactID); err != nil {
		return err
	}
	return outboundmessages.AnonymizeDestinations(ctx, s, t.destinations())
}

// eraseCapturedMessages deletes the Captured messages (Test inbox, ADR 0058) addressed to the
// subject's Destinations.
func eraseCapturedMessages(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return capturedmessages.EraseTo(ctx, s, t.Destinations)
}

// eraseContact removes the Contact itself; its Tag links and Custom field values go
// with the row.
func eraseContact(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return contacts.DeleteIn(ctx, s, t.ContactID)
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
