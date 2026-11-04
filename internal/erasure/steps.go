package erasure

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/starttoken"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/optout"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// idChunk bounds how many ids go into one IN (...) list, well below the driver's
// bind-parameter limit.
const idChunk = 10_000

// anonymousKeep lists the Event properties an anonymized system Event keeps: the
// non-personal delivery facts the complaint and bounce rates are computed from
// (ADR 0011). Everything else a system Event carries (urls, provider and message ids,
// provenance) is dropped, and so are the columns that copy them (url, broadcast_id,
// automation_id); machine and captured stay, facts about the send and not the person.
var anonymousKeep = []string{"sendingDomain", "bounceKind"}

// anonymized reports whether an Event action is a reserved system action that
// Erasure keeps as an anonymous row. Every other action is customer-tracked and is
// deleted; marketing.confirmed and sms.confirmed are deleted too, as consent proof for a person
// who is gone (ADR 0021), and so is telegram.received, the person's own words. The other telegram.*
// System events are anonymized like email's, bound and unbound included: they say a Contact's
// chat came and went, and with the Contact and the Destination dropped they are counts. sms.sent
// and sms.clicked are anonymized like email.sent and email.clicked, and sms.delivered and
// sms.undelivered like email's delivery notices.
func anonymized(action string) bool {
	switch action {
	case events.NameEmailSent, events.NameEmailOpened, events.NameEmailClicked,
		events.NameEmailDelivered, events.NameEmailDelayed, events.NameEmailDropped,
		events.NameEmailBounced, events.NameEmailComplained, events.NameEmailUnsubscribed,
		events.NameContactCreated,
		events.NameTelegramSent, events.NameTelegramClicked, events.NameTelegramUnsubscribed,
		events.NameTelegramBound, events.NameTelegramUnbound,
		events.NameSmsSent, events.NameSmsDelivered, events.NameSmsUndelivered, events.NameSmsClicked:
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

// cancelRuleWork has the Rule module cancel the queued evaluations that name the Contact.
func cancelRuleWork(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return rules.CancelPending(ctx, s, t.ContactID, purger)
}

// eraseFirings deletes the Contact's Rule Firings.
func eraseFirings(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	return rules.EraseFirings(ctx, s, t.ContactID)
}

// purgeOutbox deletes the domain-event outbox rows that name the subject; the bus is the
// only owner of that table, and the purge shares Erasure's transaction.
func purgeOutbox(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	return purger.PurgeOutbox(ctx, s.WorkspaceID(), t.ContactID, t.destinations())
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

// eraseShortLinks deletes the short links of the Contact's Outbound messages, Telegram's and
// SMS's (ADR 0083): a link names its message, and following it would record a click of the person.
// The messages themselves stay as anonymous rows (anonymizeDelivery), which clears the contact_id
// the links are found by, so this runs before it. A Test send's links name no message and carry
// only sample data: they are not the person's.
func eraseShortLinks(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if t.ContactID == 0 {
		return nil
	}
	_, err := s.TrackedLink().Delete().Where(
		trackedlink.HasOutboundMessageWith(outboundmessage.ContactID(t.ContactID))).Exec(ctx)
	return err
}

// eraseTelegram deletes the Telegram data of the subject: the Bindings and Start tokens of the
// Contact and of its Visitors (a Visitor's are owned by the Visitor row, which Erasure deletes
// later), so it runs before eraseVisitors. Telegram opt-outs are not here: they are keyed by the
// user id and stay (detachOptOuts).
func eraseTelegram(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	var owners []predicate.Visitor
	if t.ContactID != 0 {
		owners = append(owners, visitorOf(t.ContactID))
	}
	if len(t.VisitorIDs) > 0 {
		owners = append(owners, visitor.VisitorIDIn(t.VisitorIDs...))
	}
	var bindings []predicate.Binding
	var tokens []predicate.StartToken
	if t.ContactID != 0 {
		bindings = append(bindings, binding.ContactID(t.ContactID))
		tokens = append(tokens, starttoken.ContactID(t.ContactID))
	}
	if len(owners) > 0 {
		visitorIDs, err := s.Visitor().Query().Where(visitor.Or(owners...)).IDs(ctx)
		if err != nil {
			return err
		}
		if len(visitorIDs) > 0 {
			bindings = append(bindings, binding.VisitorIDIn(visitorIDs...))
			tokens = append(tokens, starttoken.VisitorIDIn(visitorIDs...))
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	if _, err := s.Binding().Delete().Where(binding.Or(bindings...)).Exec(ctx); err != nil {
		return err
	}
	_, err := s.StartToken().Delete().Where(starttoken.Or(tokens...)).Exec(ctx)
	return err
}

// eraseEvents deletes the subject's customer-tracked Events and anonymizes its
// system Events in place. Events are found by contact_id (Identify stitched earlier
// anonymous Events onto it), by the subject's visitor ids (Events not yet stitched) and, for
// the Contact's phone, by the sms.confirmed Events of the number that name no Contact: consent
// the number gave before any Contact had it (by the API or a START).
func eraseEvents(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	var matches []predicate.Event
	if t.ContactID != 0 {
		matches = append(matches, event.ContactID(t.ContactID))
	}
	if t.Phone != "" {
		matches = append(matches, event.And(
			event.ContactIDIsNil(), event.Action(events.NameSmsConfirmed), event.Phone(t.Phone)))
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

	var doomed, reported []int64
	// Anonymized Events grouped by the properties they keep, so each distinct property
	// set is one bulk update.
	keptBy := map[string][]int64{}
	kept := map[string]map[string]any{}
	for _, e := range rows {
		if !anonymized(e.Action) {
			doomed = append(doomed, e.ID)
			continue
		}
		if e.Action == events.NameSmsDelivered || e.Action == events.NameSmsUndelivered {
			reported = append(reported, e.ID)
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
				ClearURL().ClearBroadcastID().ClearAutomationID().
				SetProperties(kept[key]).
				Save(ctx); err != nil {
				return err
			}
		}
	}
	// An SMS delivery report's source id is the provider's key of the report (ingest.Tx.DedupKey),
	// which names the message in Twilio's log; the dedupe it served is over once the outbox is purged.
	for _, chunk := range lo.Chunk(reported, idChunk) {
		if _, err := s.Event().Update().Where(event.IDIn(chunk...)).ClearSourceID().Save(ctx); err != nil {
			return err
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

// eraseConfirmations deletes the Confirmation and its evidence; losing it fails safe
// (re-confirmation). On sms the Confirmation of the Contact's phone goes too when it names no
// Contact: it was recorded before any Contact had the number (by the API or a START).
func eraseConfirmations(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	match := confirmation.ContactID(t.ContactID)
	if t.Phone != "" {
		match = confirmation.Or(match, confirmation.And(
			confirmation.ChannelEQ(confirmation.ChannelSms), confirmation.DestinationEQ(t.Phone)))
	}
	_, err := s.Confirmation().Delete().Where(match).Exec(ctx)
	return err
}

func eraseRuns(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	_, err := s.AutomationRun().Delete().Where(automationrun.ContactID(t.ContactID)).Exec(ctx)
	return err
}

// detachOptOuts keeps the Unsubscribe and active Suppression rows, which are keyed by
// Destination and keep honoring the refusal, and only clears their Contact reference. A
// lifted Suppression honors nothing, so it goes with the Contact. On sms the rows keep refusing
// the number by its hash, never by the number itself (ADR 0021): the phone leaves only its hash.
func detachOptOuts(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := optout.EraseSMS(ctx, s, t.ContactID, t.Phone); err != nil {
		return err
	}
	if _, err := s.Unsubscribe().Update().Where(unsubscribe.ContactID(t.ContactID)).ClearContactID().Save(ctx); err != nil {
		return err
	}
	return suppression.EraseContact(ctx, s, t.ContactID, t.Phone)
}

// anonymizeDelivery turns the subject's delivery records into anonymous rows: they keep
// their status and timing, so reports keep their totals, but no longer name anyone.
func anonymizeDelivery(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if err := forgetTexts(ctx, s, t); err != nil {
		return err
	}
	if _, err := s.OutboundMessage().Update().Where(outboundmessage.ContactID(t.ContactID)).
		ClearContactID().ClearDestination().ClearSubject().Save(ctx); err != nil {
		return err
	}
	if _, err := s.BroadcastRecipient().Update().Where(broadcastrecipient.ContactID(t.ContactID)).
		ClearContactID().Save(ctx); err != nil {
		return err
	}
	// A transactional send to the address may never have had a Contact: it is found by
	// destination.
	dests := t.destinations()
	if len(dests) == 0 {
		return nil
	}
	_, err := s.OutboundMessage().Update().Where(outboundmessage.DestinationIn(dests...)).
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
func forgetTexts(ctx context.Context, s *ent.Scoped, t *Target) error {
	if t.ContactID == 0 {
		return nil
	}
	texts := []predicate.OutboundMessage{outboundmessage.ContactID(t.ContactID)}
	if t.Phone != "" {
		texts = append(texts, outboundmessage.Destination(t.Phone))
	}
	which := outboundmessage.And(outboundmessage.ChannelEQ(outboundmessage.ChannelSms), outboundmessage.Or(texts...))
	if _, err := s.OutboundMessage().Update().Where(which).ClearProviderMessageID().Save(ctx); err != nil {
		return err
	}
	if t.Phone == "" {
		return nil
	}
	digits := strings.TrimPrefix(t.Phone, "+")
	quoting, err := s.OutboundMessage().Query().Where(which, outboundmessage.ReasonContains(digits)).All(ctx)
	if err != nil {
		return err
	}
	for _, m := range quoting {
		reason := strings.ReplaceAll(strings.ReplaceAll(lo.FromPtr(m.Reason), t.Phone, erasedNumber), digits, erasedNumber)
		if err := s.OutboundMessage().UpdateOneID(m.ID).SetReason(reason).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// eraseCapturedMessages deletes the Captured messages (Test inbox, ADR 0058) addressed to the
// subject's Destinations: they hold the rendered mail, so unlike a delivery record there is nothing
// to keep. Those kept by an archived Test inbox go too.
func eraseCapturedMessages(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
	if len(t.Destinations) == 0 {
		return nil
	}
	_, err := s.CapturedMessage().Delete().Where(capturedmessage.RecipientIn(t.Destinations...)).Exec(ctx)
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
