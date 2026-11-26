package outboundmessages

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// decideTracking decides whether the claimed message msg goes out tracked (ADR 0120): its surface
// tracks (channel.tracks), its tracking was not withdrawn, every target's effective Engagement
// tracking consent is to track and, on a retry, the attempt before it decided tracked. A message
// of several targets (a Contact's Telegram chats, one rendered message) is tracked only when all of
// them are. The decision of the first render is the message's fact, which recordTracking writes
// once: a retry reads it and records nothing, and one that renders untracked over a tracked
// decision does so because a refusal or the default withdrew the Destination's consent, whose
// withdrawal reaches this message too. A claim taken over meanwhile is ErrInProgress.
func (m *Module) decideTracking(ctx context.Context, s *ent.Scoped, req Request, ch channel, msg *ent.OutboundMessage, targets []target) (bool, error) {
	tracked := ch.tracks(req) && msg.TrackingWithdrawnAt == nil && (msg.TrackingDecidedAt == nil || msg.Tracked)
	if tracked && msg.Destination == "" && len(targets) > 0 {
		// A Channel of Bindings claims with no Destination: its first target is on the row before
		// the consent is read, so a refusal recorded meanwhile, whose withdrawal finds the messages
		// by their Destination, reaches this one; the send records the target that accepted. A
		// message rendered once for several chats keeps every one of them, so a refusal of any
		// reaches it and its clicks are checked against each.
		update := s.OutboundMessage().Update().Where(ownsClaim(msg)...).SetDestination(targets[0].dest)
		if len(targets) > 1 {
			update.SetDestinations(lo.Map(targets, func(t target, _ int) string { return t.dest }))
		}
		n, err := update.Save(ctx)
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
	return tracked, nil
}

// recordTracking records on the claimed message msg what its first render decided, with the moment
// it decided, before the provider is called: once, so a retry, which reads the decision, records
// nothing. A claim taken over meanwhile is ErrInProgress.
func (m *Module) recordTracking(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage, tracked bool) error {
	if msg.TrackingDecidedAt != nil {
		return nil
	}
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := s.OutboundMessage().Update().
			Where(append(ownsClaim(msg), outboundmessage.TrackingDecidedAtIsNil())...).
			SetTrackingDecidedAt(m.now()).
			SetTracked(tracked).
			Save(ctx)
		return n > 0, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return ErrInProgress // the claim was taken over while we worked
	}
	if err != nil {
		return fmt.Errorf("outbound: record whether message %d is tracked: %w", msg.ID, err)
	}
	return nil
}

// renderDecided renders msg for dest as decided (tracked) and records the decision once the render
// succeeds: a message whose render failed decides nothing. A content error is the content's own.
func (m *Module) renderDecided(ctx context.Context, s *ent.Scoped, req Request, ch channel, msg *ent.OutboundMessage, dest string, ws *ent.Workspace, tracked bool) (any, error) {
	rendered, err := ch.render(ctx, s, req, msg.ID, dest, ws, tracked)
	if err != nil {
		return nil, err
	}
	return rendered, m.recordTracking(ctx, s, msg, tracked)
}
