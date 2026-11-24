package eventsunchecked

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// engagementActions are the opens and clicks the recorder publishes of an Outbound message: an email
// open and click, a click of an SMS's or a Telegram message's Short tracked link.
var engagementActions = slices.Concat([]string{systemevent.EmailOpened}, events.ClickActions)

// DeleteEngagementOf deletes the opens and clicks recorded of the Outbound messages, human and
// machine, every repeat included: each Event of engagementActions that names one of them as its
// Outbound message. It is what a refusal of tracking deletes of its Destination's earlier messages
// (ADR 0120), run by the use case internal/usecase/trackingwithdrawal in the transaction that holds
// the messages' rows. An Event names its message's Contact, or none for a message kept by address,
// which narrows the search to those Contacts' Events (the Contact's index) before the property is
// read. What the Events triggered stays: an Enrollment, a send, a Rule's action. An Erasure that
// comes before the reach-back runs it too (internal/usecase/contacterasure).
func (m *Module) DeleteEngagementOf(ctx context.Context, s *ent.Scoped, msgs []*ent.OutboundMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := lo.Map(msgs, func(m *ent.OutboundMessage, _ int) int64 { return m.ID })
	contacts := lo.Uniq(lo.FilterMap(msgs, func(m *ent.OutboundMessage, _ int) (int64, bool) {
		return lo.FromPtr(m.ContactID), m.ContactID != nil
	}))
	of := []predicate.Event{event.ContactIDIsNil()}
	if len(contacts) > 0 {
		of = append(of, event.ContactIDIn(contacts...))
	}
	_, err := s.Event().Delete().
		Where(event.Or(of...), event.ActionIn(engagementActions...), events.NamesMessageIn(ids)).
		Exec(ctx)
	return err
}
