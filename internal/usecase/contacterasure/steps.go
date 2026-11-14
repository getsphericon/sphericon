package contacterasure

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions/suppressionsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes/unsubscribesunchecked"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/outboundmessagesunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// detachOptOuts keeps the Unsubscribe and active Suppression rows, which are keyed by
// Destination and keep honoring the refusal; their Contact reference is cleared by its foreign key
// when the Contact row goes. A lifted Suppression honors nothing, so it goes with the Contact. On
// sms the rows keep refusing the number by its hash, never by the number itself (ADR 0021): the
// phone leaves only its hash.
func detachOptOuts(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := unsubscribesunchecked.EraseSMS(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	return suppressionsunchecked.EraseContact(ctx, s, t.ContactID, t.Phone)
}

// anonymizeDelivery turns the subject's delivery records into anonymous rows: they keep
// their status and timing, so reports keep their totals, but no longer name anyone. The
// Contact's Outbound messages go first, then the messages to its addresses and number, which a
// transactional send may have made without a Contact. Its Broadcast recipients name it only by
// the Contact reference, which their foreign key sets null when the Contact row goes.
func anonymizeDelivery(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := outboundmessagesunchecked.AnonymizeContact(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	return outboundmessagesunchecked.AnonymizeDestinations(ctx, s, t.destinations())
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
		EmailHash:      hash(firstDestination(t)),
		PhoneHash:      hash(t.Phone),
	})
}
