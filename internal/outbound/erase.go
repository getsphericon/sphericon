package outbound

import (
	"context"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// The Outbound message parts of a Contact's Erasure (ADR 0021). Each runs inside the Erasure's
// transaction (s comes from it); contactID is 0 for a subject without a Contact.

// EraseShortLinks deletes the short links of the Contact's Outbound messages, Telegram's and
// SMS's (ADR 0083): a link names its message, and following it would record a click of the person.
// The messages themselves stay as anonymous rows (AnonymizeContact), which clears the contact_id
// the links are found by, so this runs before it. A Test send's links name no message and carry
// only sample data: they are not the person's.
func EraseShortLinks(ctx context.Context, s *ent.Scoped, contactID int64) error {
	if contactID == 0 {
		return nil
	}
	_, err := s.TrackedLink().Delete().Where(
		trackedlink.HasOutboundMessageWith(outboundmessage.ContactID(contactID))).Exec(ctx)
	return err
}

// AnonymizeContact turns the Contact's delivery records into anonymous rows: they keep their
// status and timing, so reports keep their totals, but no longer name anyone. Its SMS messages
// first forget what they hold besides their destination (forgetTexts); phone is the Contact's
// number, "" when it has none.
func AnonymizeContact(ctx context.Context, s *ent.Scoped, contactID int64, phone string) error {
	if err := forgetTexts(ctx, s, contactID, phone); err != nil {
		return err
	}
	_, err := s.OutboundMessage().Update().Where(outboundmessage.ContactID(contactID)).
		ClearContactID().ClearDestination().ClearSubject().Save(ctx)
	return err
}

// AnonymizeDestinations turns the delivery records to the subject's addresses and phone number
// into anonymous rows like AnonymizeContact: a transactional send to the address may never have
// had a Contact, so it is found by destination.
func AnonymizeDestinations(ctx context.Context, s *ent.Scoped, destinations []string) error {
	if len(destinations) == 0 {
		return nil
	}
	_, err := s.OutboundMessage().Update().Where(outboundmessage.DestinationIn(destinations...)).
		ClearContactID().ClearDestination().ClearSubject().Save(ctx)
	return err
}

// erasedNumber stands for the number where a provider's words on an erased Contact's text quote it.
const erasedNumber = "[erased]"

// forgetTexts drops what an SMS Outbound message of the subject holds besides its destination: the
// provider's reference (Twilio's SID), which names the message, its number and its text in the
// provider's log, and the number where the provider's words on a refused text quote it (Twilio's
// 21211 does). A delivery report that arrives later finds no message and records nothing. It runs
// before the rows lose the Contact and the number they are found by.
func forgetTexts(ctx context.Context, s *ent.Scoped, contactID int64, phone string) error {
	if contactID == 0 {
		return nil
	}
	texts := []predicate.OutboundMessage{outboundmessage.ContactID(contactID)}
	if phone != "" {
		texts = append(texts, outboundmessage.Destination(phone))
	}
	which := outboundmessage.And(outboundmessage.ChannelEQ(channels.SMS), outboundmessage.Or(texts...))
	if _, err := s.OutboundMessage().Update().Where(which).ClearProviderMessageID().Save(ctx); err != nil {
		return err
	}
	if phone == "" {
		return nil
	}
	digits := strings.TrimPrefix(phone, "+")
	quoting, err := s.OutboundMessage().Query().Where(which, outboundmessage.ReasonContains(digits)).All(ctx)
	if err != nil {
		return err
	}
	for _, m := range quoting {
		reason := strings.ReplaceAll(strings.ReplaceAll(lo.FromPtr(m.Reason), phone, erasedNumber), digits, erasedNumber)
		if err := s.OutboundMessage().UpdateOneID(m.ID).SetReason(reason).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
