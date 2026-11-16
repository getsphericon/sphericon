package contacterasure

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps/consentstampsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions/suppressionsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes/unsubscribesunchecked"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/outboundmessagesunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
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

// eraseConsentStamps leaves of the times the subject's consent observations occurred only the time
// of a stop that still stands (ADR 0154), kept as its opt-out is: by the number's hash on sms, by
// the address on email. It reads the opt-outs before detachOptOuts rekeys them, and comes before
// the other deletions: an observation in flight holds its stamp first and the Contact after, so the
// Erasure, which holds the Contact, waits for it here rather than in the middle of rows that
// observation writes next.
func eraseConsentStamps(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	for _, address := range t.Destinations {
		standing := func(ctx context.Context, source string) (bool, error) {
			_, err := unsubscribes.Get(ctx, s, channels.Email, address, source)
			if ent.IsNotFound(err) {
				return false, nil
			}
			return err == nil, err
		}
		if err := consentstampsunchecked.Erase(ctx, s, channels.Email, address, address, standing); err != nil {
			return err
		}
	}
	standing := func(ctx context.Context, _ string) (bool, error) {
		optOut, err := unsubscribes.SMSOptOut(ctx, s, t.Phone)
		return optOut != nil, err
	}
	return consentstampsunchecked.Erase(ctx, s, channels.SMS, t.Phone, phone.Hash(t.Phone), standing)
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
