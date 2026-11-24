// Package trackingwithdrawal is the use case of a refusal's reach-back (ADR 0120): once a
// Destination refuses tracking, what was collected of its earlier Outbound messages goes. Each
// message records the withdrawal of its tracking, and the opens and clicks recorded of it (their
// Events, human and machine), its opened and clicked stamps and the credit of every Order credited
// through one of its clicks are deleted. What they did stays: an Automation the Events triggered,
// its Enrollment, a send, a Rule's action, a picked Winner, a Broadcast's frozen figures.
//
// It writes the rows of three owners, each through the owner's function: the Outbound messages
// (outboundmessages), the Events (eventsunchecked, which only a use case imports) and the Orders
// (orders). The messages' rows are taken for their update first, in each batch's transaction, so a
// hit, a stamp's projection or an Order's credit of one of them either commits before the batch
// (and the batch deletes it) or waits and finds the message withdrawn (and records nothing).
package trackingwithdrawal

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/events/eventsunchecked"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Batch is the messages one transaction of the reach-back takes: few enough that a Destination
// with a long history never holds a long lock.
const Batch = 500

// UseCase is the reach-back of a refusal of tracking over the event bus, the Outbound messages and
// the Orders.
type UseCase struct {
	bus      *events.Bus
	messages *outboundmessages.Module
	orders   *orders.Module
	events   *eventsunchecked.Module
	batch    int
}

// New returns the reach-back, batch messages to a transaction (Batch; a test passes 1 to turn
// every loop).
func New(bus *events.Bus, messages *outboundmessages.Module, credits *orders.Module, batch int) *UseCase {
	return &UseCase{bus: bus, messages: messages, orders: credits, events: eventsunchecked.New(), batch: batch}
}

// Refusal is the Destination that refused tracking, in the form its consent is stored in.
type Refusal struct {
	Channel     channels.Channel
	Destination string
}

// Withdraw reaches back from the refusal r to every Outbound message of its Destination in the
// Workspace of s, whatever its status, a batch at a time in id order, each batch a transaction of
// its own whose result persists at once. It is idempotent: a message withdrawn before keeps its
// first moment and has nothing left to delete, so a second run, or one that resumes a run cut
// short, changes nothing more.
func (u *UseCase) Withdraw(ctx context.Context, s *ent.Scoped, r Refusal) error {
	which := outboundmessages.OfDestination(r.Channel, r.Destination)
	var after int64
	for {
		var taken []*ent.OutboundMessage
		err := u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
			var err error
			taken, err = u.messages.WithdrawTracking(ctx, ts, outboundmessages.WithdrawalBatch{Messages: which, After: after, Limit: u.batch})
			if err != nil || len(taken) == 0 {
				return err
			}
			return u.forget(ctx, ts, taken)
		})
		if err != nil {
			return fmt.Errorf("withdraw the tracking of a %s destination: %w", r.Channel, err)
		}
		if len(taken) < u.batch {
			return nil
		}
		after = taken[len(taken)-1].ID
	}
}

// forget deletes what was recorded of the withdrawn messages: their opens and clicks, their stamps
// and the credits their clicks earned.
func (u *UseCase) forget(ctx context.Context, ts *ent.Scoped, msgs []*ent.OutboundMessage) error {
	ids := lo.Map(msgs, func(m *ent.OutboundMessage, _ int) int64 { return m.ID })
	if err := u.messages.ForgetEngagement(ctx, ts, ids); err != nil {
		return err
	}
	if err := u.events.DeleteEngagementOf(ctx, ts, msgs); err != nil {
		return fmt.Errorf("delete the opens and clicks: %w", err)
	}
	return u.orders.ClearCredits(ctx, ts, ids)
}
