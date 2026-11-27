package contacterasure

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps/consentstampsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking/engagementtrackingunchecked"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
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
		denied, err := u.trackingDenied(ctx, s, channels.SMS, number)
		if err != nil {
			return err
		}
		if err := u.stamps.Erase(ctx, s, consentstampsunchecked.EraseInput{
			Channel: channels.SMS, Destination: number, Standing: standing, TrackingDenied: denied,
		}); err != nil {
			return err
		}
	}
	return nil
}

// trackingDenied reports whether a tracking refusal of destination on channel stands, which the
// Erasure keeps with its time.
func (u *UseCase) trackingDenied(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (bool, error) {
	consent, err := u.tracking.Of(ctx, s, channel, destination)
	return consent.State == engagementtracking.ReadingDenied, err
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
	denied, err := u.trackingDenied(ctx, s, channel, destination)
	if err != nil {
		return err
	}
	return u.stamps.Erase(ctx, s, consentstampsunchecked.EraseInput{
		Channel: channel, Destination: destination, Standing: standing, TrackingDenied: denied,
	})
}

// refusedBatch is how many messages withdrawRefused takes at a time, all in the Erasure's
// transaction.
const refusedBatch = 500

// withdrawRefused is a refusal's reach-back (ADR 0120) for each Destination of the subject whose
// tracking refusal stands: the tracking of its messages is withdrawn, and the opens and clicks
// recorded of them, their stamps and the credits their clicks earned are deleted, as the reach-back
// job does. That job finds the messages by their Destination, which anonymizeDelivery blanks, and
// the Events by the message ids EraseSubject strips: an Erasure that comes first does the job's
// work itself, so the outcome does not depend on which runs first (ADR 0113). A run after the job
// finds the messages withdrawn and nothing left to delete. The job, which names the Destination
// (ADR 0021), is deleted while it waits: its refusal stands, since nothing lifts a refusal yet, so
// the work it was queued for is done here. It reads the refusals before eraseTracking rekeys a
// number's to its hash.
func (u *UseCase) withdrawRefused(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
	held, err := u.heldDestinations(ctx, s, t)
	if err != nil {
		return err
	}
	if err := pub.PurgeDestinationJobs(ctx, s.WorkspaceID(), jobkind.TrackingWithdrawal, addressesOf(held)); err != nil {
		return err
	}
	for _, d := range held {
		denied, err := u.trackingDenied(ctx, s, d.channel, d.address)
		if err != nil {
			return err
		}
		if !denied {
			continue
		}
		if err := u.withdrawFrom(ctx, s, d); err != nil {
			return err
		}
	}
	return nil
}

// withdrawFrom withdraws the tracking of the messages to d, a batch at a time, and deletes what
// was recorded of them.
func (u *UseCase) withdrawFrom(ctx context.Context, s *ent.Scoped, d heldDestination) error {
	which := outboundmessages.OfDestination(d.channel, d.address)
	var after int64
	for {
		taken, err := u.messages.WithdrawTracking(ctx, s, outboundmessages.WithdrawalBatch{Messages: which, After: after, Limit: refusedBatch})
		if err != nil || len(taken) == 0 {
			return err
		}
		ids := lo.Map(taken, func(m *ent.OutboundMessage, _ int) int64 { return m.ID })
		if err := u.messages.ForgetEngagement(ctx, s, ids); err != nil {
			return err
		}
		// The copies a Wait for event kept of these Events are not cleared here: the Contact's own
		// Enrollments go with it, and with them their copies.
		if _, err := u.events.DeleteEngagementOf(ctx, s, taken); err != nil {
			return err
		}
		if err := u.credits.ClearCredits(ctx, s, ids); err != nil {
			return err
		}
		if len(taken) < refusedBatch {
			return nil
		}
		after = taken[len(taken)-1].ID
	}
}

// heldDestination is one Destination of the subject on a Channel.
type heldDestination struct {
	channel channels.Channel
	address string
}

// namedDestinations is every Destination of the subject a queued job may name: its addresses, its
// phone and the Telegram users of its chats.
func (u *UseCase) namedDestinations(ctx context.Context, s *ent.Scoped, t *Target) ([]string, error) {
	held, err := u.heldDestinations(ctx, s, t)
	return addressesOf(held), err
}

// addressesOf is the Destinations of held, whatever their Channel.
func addressesOf(held []heldDestination) []string {
	return lo.Map(held, func(d heldDestination, _ int) string { return d.address })
}

// heldDestinations is the subject's Destinations that take an Engagement tracking consent: its
// addresses, its phone and the Telegram users of its chats.
func (u *UseCase) heldDestinations(ctx context.Context, s *ent.Scoped, t *Target) ([]heldDestination, error) {
	var out []heldDestination
	for _, address := range t.Destinations {
		out = append(out, heldDestination{channel: channels.Email, address: address})
	}
	if t.Phone != "" {
		out = append(out, heldDestination{channel: channels.SMS, address: t.Phone})
	}
	if t.ContactID != 0 {
		users, err := u.chats.DestinationsOf(ctx, s, t.ContactID)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			out = append(out, heldDestination{channel: channels.Telegram, address: user})
		}
	}
	return out, nil
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
// the Contact reference, which their foreign key sets null when the Contact row goes. The places
// those messages took under the Frequency cap go before them, found by the Contact the anonymizing
// clears: they name the Destination, and its count restarts (ADR 0178).
func (u *UseCase) anonymizeDelivery(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := u.counts.EraseContact(ctx, s, t.ContactID, t.destinations()); err != nil {
		return err
	}
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
