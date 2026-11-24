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
	"github.com/getsphericon/sphericon/internal/infra/events"
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

// Module holds the Events steps of the use cases that remove Events: a Contact's Erasure and a
// refusal's reach-back. It holds nothing: every call takes the scope of the use case's
// transaction.
type Module struct{}

// New builds the module.
func New() *Module { return &Module{} }

// Subject is whom an Erasure erases the Events of: the Contact (0 for a subject without one), its
// phone and addresses ("" and empty then) and its visitor ids.
type Subject struct {
	ContactID  int64
	Phone      string
	Addresses  []string
	VisitorIDs []string
}

// EraseSubject is the Events part of a Contact's Erasure (ADR 0021): it deletes the subject's
// customer-tracked Events and anonymizes its system Events in place. Events are found by
// contact_id (Identify stitched earlier anonymous Events onto it), by the subject's visitor ids
// (Events not yet stitched), for the Contact's phone by the sms.confirmed, SMS Engagement
// tracking and sms.clicked Events of the number that name no Contact: consent the number gave before
// any Contact had it (by the API or a START), a click of a text sent to the number alone, and
// for its addresses by the Engagement tracking Events of an address that name no Contact, for the
// same reason. Run it inside the Erasure's transaction.
func (m *Module) EraseSubject(ctx context.Context, s *ent.Scoped, subj Subject) error {
	contactID, phone, addresses, visitorIDs := subj.ContactID, subj.Phone, subj.Addresses, subj.VisitorIDs
	var matches []predicate.Event
	if contactID != 0 {
		matches = append(matches, event.ContactID(contactID))
	}
	if phone != "" {
		matches = append(matches, event.And(
			event.ContactIDIsNil(),
			event.ActionIn(systemevent.SmsConfirmed, systemevent.SmsEngagementTrackingGranted, systemevent.SmsEngagementTrackingDenied,
				systemevent.SmsClicked),
			event.Phone(phone)))
	}
	if len(addresses) > 0 {
		matches = append(matches, event.And(
			event.ContactIDIsNil(),
			event.ActionIn(systemevent.EmailEngagementTrackingGranted, systemevent.EmailEngagementTrackingDenied),
			event.EmailIn(addresses...)))
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
		if !events.KeptOnErasure(e.Action) {
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
