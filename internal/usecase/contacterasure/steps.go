package contacterasure

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

func visitorOf(contactID int64) predicate.Visitor { return visitor.ContactID(contactID) }

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
