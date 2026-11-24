package contacterasure

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps/consentstampsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking/engagementtrackingunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// detachOptOuts keeps the Unsubscribe and active Suppression rows, which are keyed by
// Destination and keep honoring the refusal; their Contact reference is cleared by its foreign key
// when the Contact row goes. A lifted Suppression honors nothing, so it goes with the Contact. On
// sms the rows keep refusing the number by its hash, never by the number itself (ADR 0021): the
// phone leaves only its hash.
func (u *UseCase) detachOptOuts(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := u.optOuts.EraseSMS(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	return u.suppressions.EraseContact(ctx, s, t.ContactID, t.Phone)
}

// eraseConsentStamps leaves of the times the subject's consent observations occurred only the time
// of a stop that still stands and of a tracking refusal (ADR 0154, ADR 0120), kept as its opt-out is: by the address on email, the
// user id on Telegram, the number's hash on sms. It reads the opt-outs and the Bindings before
// detachOptOuts rekeys the ones and the Contact's row takes the others, and comes before the other
// deletions: an observation in flight holds its stamp first and the Contact after, so the Erasure,
// which holds the Contact, waits for it here rather than in the middle of rows that observation
// writes next.
func (u *UseCase) eraseConsentStamps(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	for _, address := range t.Destinations {
		if err := u.eraseStampsOf(ctx, s, channels.Email, address); err != nil {
			return err
		}
	}
	if t.ContactID != 0 {
		users, err := u.chats.DestinationsOf(ctx, s, t.ContactID)
		if err != nil {
			return err
		}
		for _, user := range users {
			if err := u.eraseStampsOf(ctx, s, channels.Telegram, user); err != nil {
				return err
			}
		}
	}
	// The Contact's phone, and the numbers it had before whose opt-outs still name it.
	numbers, err := u.unsubs.SMSNumbersOf(ctx, s, t.ContactID)
	if err != nil {
		return err
	}
	if t.Phone != "" {
		numbers = append(numbers, t.Phone)
	}
	for _, number := range lo.Uniq(numbers) {
		standing := func(ctx context.Context, _ string) (bool, error) {
			optOut, err := u.unsubs.SMSOptOut(ctx, s, number)
			return optOut != nil, err
		}
		if err := u.stamps.Erase(ctx, s, consentstampsunchecked.EraseInput{
			Channel: channels.SMS, Destination: number, Standing: standing, TrackingDenied: u.trackingDenied(s, channels.SMS, number),
		}); err != nil {
			return err
		}
	}
	return nil
}

// trackingDenied reports whether a tracking refusal of destination on channel stands, which the
// Erasure keeps with its time.
func (u *UseCase) trackingDenied(s *ent.Scoped, channel channels.Channel, destination string) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		consent, err := u.tracking.Of(ctx, s, channel, destination)
		return consent.State == engagementtracking.ReadingDenied, err
	}
}

// eraseStampsOf erases the stamps of destination on channel but the time of a stop whose opt-out,
// kept by the Destination, still stands, and that of a tracking refusal the Erasure keeps.
func (u *UseCase) eraseStampsOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) error {
	standing := func(ctx context.Context, source string) (bool, error) {
		_, err := u.unsubs.Get(ctx, s, unsubscribes.Key{Channel: channel, Destination: destination, Source: source})
		if ent.IsNotFound(err) {
			return false, nil
		}
		return err == nil, err
	}
	return u.stamps.Erase(ctx, s, consentstampsunchecked.EraseInput{
		Channel: channel, Destination: destination, Standing: standing, TrackingDenied: u.trackingDenied(s, channel, destination),
	})
}

// eraseTracking deletes the tracking grants of the subject's addresses, phone and Telegram users
// and keeps their refusals, the phone's by its hash (ADR 0120). It comes after eraseConsentStamps,
// which reads the refusals to keep their times, and before the Visitors and the Contact go, whose
// Bindings name its Telegram users.
func (u *UseCase) eraseTracking(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	var users []string
	if t.ContactID != 0 {
		var err error
		if users, err = u.chats.DestinationsOf(ctx, s, t.ContactID); err != nil {
			return err
		}
	}
	held := []engagementtrackingunchecked.EraseInput{
		{Channel: channels.Email, Destinations: t.Destinations},
		{Channel: channels.SMS, Destinations: lo.Compact([]string{t.Phone})},
		{Channel: channels.Telegram, Destinations: users},
	}
	for _, in := range held {
		if err := u.trackingSteps.Erase(ctx, s, in); err != nil {
			return err
		}
	}
	return nil
}

// anonymizeDelivery turns the subject's delivery records into anonymous rows: they keep
// their status and timing, so reports keep their totals, but no longer name anyone. The
// Contact's Outbound messages go first, then the messages to its addresses and number, which a
// transactional send may have made without a Contact. Its Broadcast recipients name it only by
// the Contact reference, which their foreign key sets null when the Contact row goes.
func (u *UseCase) anonymizeDelivery(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := u.outbound.AnonymizeContact(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	return u.outbound.AnonymizeDestinations(ctx, s, t.destinations())
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
