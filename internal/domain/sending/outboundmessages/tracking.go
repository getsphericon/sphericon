package outboundmessages

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
)

// IsTracked is whether an Outbound message is tracked (ADR 0120): it went out tracked and its
// tracking was not withdrawn since. It is the one definition every reader uses (the recorder, the
// rates, a Broadcast's figures, the export, the Winner, the Link identity token, a Branch's open
// test); Tracked and NotTracked are the same in SQL. It speaks of a sent message: one that was
// never rendered has decided nothing.
func IsTracked(msg *ent.OutboundMessage) bool {
	return msg.Tracked && msg.TrackingWithdrawnAt == nil
}

// Tracked is IsTracked as a predicate: the messages that went out tracked and whose tracking was
// not withdrawn.
func Tracked() predicate.OutboundMessage {
	return outboundmessage.And(outboundmessage.Tracked(true), outboundmessage.TrackingWithdrawnAtIsNil())
}

// NotTracked is Tracked's complement: the messages that went out untracked or whose tracking was
// withdrawn since.
func NotTracked() predicate.OutboundMessage {
	return outboundmessage.Or(outboundmessage.Tracked(false), outboundmessage.TrackingWithdrawnAtNotNil())
}

// decideTracking decides whether the claimed message msg goes out tracked (ADR 0120) and records
// it on the row before the provider is called: its surface tracks (channel.tracks), every target's
// effective Engagement tracking consent is to track, and, for an attempt that takes over a claim, the
// attempt before it rendered tracked and no withdrawal is recorded since. A message of several
// targets (a Contact's Telegram chats, one rendered message) is tracked only when all of them are.
// A render only ever lowers the row's value: nothing gives a message its tracking back. A claim
// taken over meanwhile is ErrInProgress.
func (m *Module) decideTracking(ctx context.Context, s *ent.Scoped, req Request, ch channel, msg *ent.OutboundMessage, targets []target) (bool, error) {
	tracked := ch.tracks(req) && IsTracked(msg)
	if tracked && msg.Destination == "" && len(targets) > 0 {
		// A Channel of Bindings resolves its Destination after the claim. It is on the row before
		// the consent is read, so a refusal recorded meanwhile, whose withdrawal finds the messages
		// by their Destination, reaches this one; the send records the target that accepted.
		n, err := s.OutboundMessage().Update().Where(ownsClaim(msg)...).SetDestination(targets[0].dest).Save(ctx)
		if err != nil {
			return false, fmt.Errorf("outbound: record the destination of message %d: %w", msg.ID, err)
		}
		if n == 0 {
			return false, ErrInProgress // the claim was taken over while we worked
		}
	}
	for _, t := range targets {
		if !tracked {
			break
		}
		consent, err := m.consents.Of(ctx, s, req.Channel(), t.dest)
		if err != nil {
			return false, err
		}
		tracked = consent.Effective == engagementtracking.Track
	}
	if tracked == msg.Tracked {
		return tracked, nil
	}
	n, err := s.OutboundMessage().Update().Where(ownsClaim(msg)...).SetTracked(tracked).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("outbound: record whether message %d is tracked: %w", msg.ID, err)
	}
	if n == 0 {
		return false, ErrInProgress // the claim was taken over while we worked
	}
	return tracked, nil
}
