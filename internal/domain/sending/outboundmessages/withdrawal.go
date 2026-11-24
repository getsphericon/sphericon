package outboundmessages

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The withdrawal of a message's tracking (ADR 0120): a refusal of its Destination, or the
// Workspace's default turned to `dont_track`, records on the message when its tracking was
// withdrawn. The message is not tracked from then on (IsTracked), and nothing gives it its tracking
// back. A withdrawal takes the rows of the messages it reaches for their update, so a hit on one
// (whose recorder reads the message FOR SHARE in its publish's transaction) either commits its
// Event first, which a refusal then deletes, or waits and finds the message withdrawn; a stamp's
// projection that waits finds the withdrawal on the row and stamps nothing (StampOpened).

// WithdrawalBatch is one batch of a withdrawal: the messages of the Workspace it reaches, the
// cursor it continues from and how many it takes at most.
type WithdrawalBatch struct {
	// Messages narrows the messages the withdrawal reaches: a Destination's (OfDestination), or
	// those of every Destination without a grant.
	Messages predicate.OutboundMessage
	// After is the cursor: only messages whose id is above it (0 to start).
	After int64
	// Limit is the most messages the batch takes.
	Limit int
}

// OfDestination is the messages sent, or being sent, to destination on channel, as the message
// stores it: the form a Destination's Engagement tracking consent is stored in. A message rendered
// once for several Destinations (a Telegram message to a Contact's chats) is of each of them.
func OfDestination(channel channels.Channel, destination string) predicate.OutboundMessage {
	return outboundmessage.And(outboundmessage.ChannelEQ(channel), outboundmessage.Or(
		outboundmessage.Destination(destination),
		predicate.OutboundMessage(func(s *sql.Selector) {
			s.Where(sqljson.ValueContains(outboundmessage.FieldDestinations, destination))
		}),
	))
}

// WithdrawTracking takes up to in.Limit of the Workspace's messages that in.Messages matches and
// whose id is above in.After, in id order, and holds their rows FOR UPDATE until the transaction s
// is bound to ends. It records the withdrawal, now, on those not withdrawn already, whatever their
// status (a claimed one renders untracked on its retry), and returns every message it took,
// withdrawn now or before, as it was before the batch. updated_at moves with the record, so the
// message export stream delivers the withdrawn messages again. Run it inside a bus transaction; the
// next batch starts after the last id returned, and a batch shorter than in.Limit is the last.
func (m *Module) WithdrawTracking(ctx context.Context, s *ent.Scoped, in WithdrawalBatch) ([]*ent.OutboundMessage, error) {
	taken, err := s.OutboundMessage().Query().
		Where(in.Messages, outboundmessage.IDGT(in.After)).
		Order(outboundmessage.ByID()).
		Limit(in.Limit).
		ForUpdate().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbound: take the messages to withdraw: %w", err)
	}
	ids := idsOf(taken)
	if len(ids) == 0 {
		return nil, nil
	}
	now := m.now()
	err = lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := s.OutboundMessage().Update().
			Where(outboundmessage.IDIn(ids...), outboundmessage.TrackingWithdrawnAtIsNil()).
			SetTrackingWithdrawnAt(now).
			Save(ctx)
		return n > 0, err
	})
	// Every message of the batch withdrawn before is the idempotent case: nothing to record.
	if err != nil && !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil, fmt.Errorf("outbound: record the withdrawal of tracking: %w", err)
	}
	return taken, nil
}

// ForgetEngagement clears the opened and clicked stamps of the messages ids names whose tracking was
// withdrawn: what a refusal deletes of them (ADR 0120). A message still tracked keeps its stamps,
// so a call on the wrong messages takes nothing.
func (m *Module) ForgetEngagement(ctx context.Context, s *ent.Scoped, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.OutboundMessage().Update().
		Where(
			outboundmessage.IDIn(ids...), outboundmessage.TrackingWithdrawnAtNotNil(),
			outboundmessage.Or(outboundmessage.OpenedAtNotNil(), outboundmessage.ClickedAtNotNil()),
		).
		ClearOpenedAt().
		ClearClickedAt().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("outbound: forget the engagement of withdrawn messages: %w", err)
	}
	return nil
}

func idsOf(msgs []*ent.OutboundMessage) []int64 {
	return lo.Map(msgs, func(m *ent.OutboundMessage, _ int) int64 { return m.ID })
}
