// Package eventsunchecked holds the Events part of a Contact's Erasure: deleting the subject's
// customer-tracked Events and anonymizing its system Events. It is a step of a Contact's Erasure
// (ADR 0021), which harms a live Contact when it runs alone: the use case
// internal/usecase/contacterasure runs each owner's step in its order, in the one transaction of
// the Erasure; only a use case may import this one (ADR 0110). The events module does not import
// it.
package eventsunchecked

import (
	"context"
	"encoding/json"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// eraseChunk bounds how many ids go into one IN (...) list of an Erasure, well below the driver's
// bind-parameter limit.
const eraseChunk = 10_000

// anonymousKeep lists the Event properties an anonymized system Event keeps: the
// non-personal delivery facts the complaint and bounce rates are computed from
// (ADR 0011). Everything else a system Event carries (urls, provider and message ids,
// provenance) is dropped, and so are the columns that copy them (url, broadcast_id,
// automation_id) and the source id, which may name the destination or the provider's reference
// (an email notice's address, an SMS report's SID); machine and captured stay, facts about the
// send and not the person.
var anonymousKeep = []string{"sendingDomain", systemevent.BounceKindProperty}

// anonymized reports whether an Event action is a reserved system action that
// Erasure keeps as an anonymous row. Every other action is customer-tracked and is
// deleted; marketing.confirmed and sms.confirmed are deleted too, as consent proof for a person
// who is gone (ADR 0021), and so is telegram.received, the person's own words. The other telegram.*
// System events are anonymized like email's, bound and unbound included: they say a Contact's
// chat came and went, and with the Contact and the Destination dropped they are counts. sms.sent
// and sms.clicked are anonymized like email.sent and email.clicked, sms.delivered and
// sms.undelivered like email's delivery notices, and sms.unsubscribed like email.unsubscribed.
func anonymized(action string) bool {
	switch action {
	case systemevent.EmailSent, systemevent.EmailOpened, systemevent.EmailClicked,
		systemevent.EmailDelivered, systemevent.EmailDelayed, systemevent.EmailDropped,
		systemevent.EmailBounced, systemevent.EmailComplained, systemevent.EmailUnsubscribed,
		systemevent.ContactCreated,
		systemevent.TelegramSent, systemevent.TelegramClicked, systemevent.TelegramUnsubscribed,
		systemevent.TelegramBound, systemevent.TelegramUnbound,
		systemevent.SmsSent, systemevent.SmsDelivered, systemevent.SmsUndelivered, systemevent.SmsClicked, systemevent.SmsUnsubscribed:
		return true
	}
	return false
}

// EraseSubject is the Events part of a Contact's Erasure (ADR 0021): it deletes the subject's
// customer-tracked Events and anonymizes its system Events in place. Events are found by
// contact_id (Identify stitched earlier anonymous Events onto it), by the subject's visitor ids
// (Events not yet stitched) and, for the Contact's phone, by the sms.confirmed Events of the number
// that name no Contact: consent the number gave before any Contact had it (by the API or a START).
// contactID is 0 and phone "" for a subject without a Contact. Run it inside the Erasure's
// transaction.
func EraseSubject(ctx context.Context, s *ent.Scoped, contactID int64, phone string, visitorIDs []string) error {
	var matches []predicate.Event
	if contactID != 0 {
		matches = append(matches, event.ContactID(contactID))
	}
	if phone != "" {
		matches = append(matches, event.And(
			event.ContactIDIsNil(), event.Action(systemevent.SmsConfirmed), event.Phone(phone)))
	}
	if len(visitorIDs) > 0 {
		matches = append(matches, event.VisitorIDIn(visitorIDs...))
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
	type keptGroup struct {
		props map[string]any
		ids   []int64
	}
	kept := map[string]*keptGroup{}
	for _, e := range rows {
		if !anonymized(e.Action) {
			doomed = append(doomed, e.ID)
			continue
		}
		props := lo.PickByKeys(e.Properties, anonymousKeep)
		key, err := json.Marshal(props) // map keys marshal sorted: a stable group key
		if err != nil {
			return err
		}
		g, ok := kept[string(key)]
		if !ok {
			g = &keptGroup{props: props}
			kept[string(key)] = g
		}
		g.ids = append(g.ids, e.ID)
	}

	for _, ids := range lo.Chunk(doomed, eraseChunk) {
		if _, err := s.Event().Delete().Where(event.IDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
	}
	for _, g := range kept {
		for _, chunk := range lo.Chunk(g.ids, eraseChunk) {
			if _, err := s.Event().Update().Where(event.IDIn(chunk...)).
				ClearContactID().ClearVisitorID().ClearEmail().ClearPhone().ClearSubjectID().
				ClearURL().ClearBroadcastID().ClearAutomationID().ClearSourceID().
				SetProperties(g.props).
				Save(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
