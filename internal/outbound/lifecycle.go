package outbound

import (
	"context"
	"errors"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// newLifecycle is the Outbound message's transition table (ADR 0046): a trigger is the status it
// moves to, and only pending leaves. sent, skipped and failed are final.
func newLifecycle(storage lifecycle.Storage[outboundmessage.Status]) *stateless.StateMachine {
	m := lifecycle.New(storage)
	m.Configure(outboundmessage.StatusPending).
		Permit(outboundmessage.StatusSent, outboundmessage.StatusSent).
		Permit(outboundmessage.StatusSkipped, outboundmessage.StatusSkipped).
		Permit(outboundmessage.StatusFailed, outboundmessage.StatusFailed)
	return m
}

// move takes msg from its status to `to` through the table. The write is the compare-and-swap:
// it matches the row only in the status the machine read AND under fence (the claim token, or
// for MarkFailed the expired lease), so a stale attempt matches nothing and gets
// lifecycle.ErrInvalidTransition. set adds the move's other columns.
func move(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage, fence []predicate.OutboundMessage, to outboundmessage.Status, set func(*ent.OutboundMessageScopedUpdate)) error {
	m := newLifecycle(lifecycle.Storage[outboundmessage.Status]{
		Read: func(context.Context) (outboundmessage.Status, error) { return msg.Status, nil },
		Write: func(ctx context.Context, from, to outboundmessage.Status) (bool, error) {
			upd := s.OutboundMessage().Update().
				Where(outboundmessage.ID(msg.ID), outboundmessage.StatusEQ(from)).
				Where(fence...).
				SetStatus(to)
			if set != nil {
				set(upd)
			}
			n, err := upd.Save(ctx)
			return n > 0, err
		},
	})
	return m.FireCtx(ctx, to)
}

// moveOrStale is move for an attempt that holds the claim: losing the fence means the claim
// was taken over, which callers report as ErrInProgress.
func moveOrStale(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage, to outboundmessage.Status, set func(*ent.OutboundMessageScopedUpdate)) error {
	err := move(ctx, s, msg, claimFence(msg), to, set)
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return ErrInProgress
	}
	return err
}
