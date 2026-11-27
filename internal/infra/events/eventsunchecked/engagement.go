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
// comes before the reach-back runs it too (internal/usecase/contacterasure). It returns the ids of
// the Events it deleted, so that the reach-back deletes what was copied of them (a Wait for event's
// kept Event, ADR 0136): the messages' rows are taken for their update, so no open or click of them
// is recorded between the read of the ids and their deletion.
func (m *Module) DeleteEngagementOf(ctx context.Context, s *ent.Scoped, msgs []*ent.OutboundMessage) ([]int64, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	ids := lo.Map(msgs, func(m *ent.OutboundMessage, _ int) int64 { return m.ID })
	contacts := lo.Uniq(lo.FilterMap(msgs, func(m *ent.OutboundMessage, _ int) (int64, bool) {
		return lo.FromPtr(m.ContactID), m.ContactID != nil
	}))
	of := []predicate.Event{event.ContactIDIsNil()}
	if len(contacts) > 0 {
		of = append(of, event.ContactIDIn(contacts...))
	}
	where := []predicate.Event{event.Or(of...), event.ActionIn(engagementActions...), events.NamesMessageIn(ids)}
	engaged, err := s.Event().Query().Where(where...).IDs(ctx)
	if err != nil || len(engaged) == 0 {
		return nil, err
	}
	// Deleted by the same predicate, not by the ids read: a message's repeats may be many more
	// than a statement takes parameters.
	if _, err := s.Event().Delete().Where(where...).Exec(ctx); err != nil {
		return nil, err
	}
	return engaged, nil
}
