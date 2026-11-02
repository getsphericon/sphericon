package outbound

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/logging"
)

// find loads the Outbound message recorded under (workspace, key), or nil.
func (m *Module) find(ctx context.Context, s *ent.Scoped, key string) (*ent.OutboundMessage, error) {
	msg, err := s.OutboundMessage().Query().
		Where(outboundmessage.IdempotencyKey(key)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return msg, err
}

// claim takes the right to send under req.Key: it inserts the pending row before
// the provider is called. If a row already exists (existing, from Send's replay
// check, or one that appears in a race) it is replayed when final, taken over when
// its lease has expired, and otherwise reported as ErrInProgress.
func (m *Module) claim(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, existing *ent.OutboundMessage) (*ent.OutboundMessage, *Result, error) {
	if existing != nil {
		msg, err := m.takeOver(ctx, s, existing)
		return msg, nil, err
	}

	create := s.OutboundMessage().Create().
		SetKind(req.Kind).
		SetIdempotencyKey(req.Key).
		SetChannel(outboundmessage.Channel(req.Channel())).
		SetDestination(dest).
		SetClaimedAt(m.claimTime())
	if m.tracker != nil {
		create.SetAppOrigin(m.tracker.Origin())
	}
	if g.as.sendingDomain != "" {
		create.SetSendingDomain(g.as.sendingDomain)
	}
	if req.Source != "" {
		create.SetSendingSource(req.Source)
	}
	if contactID := req.contactID(); contactID != 0 {
		create.SetContactID(contactID)
	}
	ref := req.Ref
	if ref.BroadcastID != 0 {
		create.SetBroadcastID(ref.BroadcastID)
	}
	if ref.BroadcastRecipient != 0 {
		create.SetBroadcastRecipientID(ref.BroadcastRecipient)
	}
	if ref.AutomationID != 0 {
		create.SetAutomationID(ref.AutomationID)
	}
	if ref.AutomationRunID != 0 {
		create.SetAutomationRunID(ref.AutomationRunID)
	}
	if ref.AutomationStep != nil {
		create.SetAutomationStep(*ref.AutomationStep)
	}
	if ref.TemplateID != 0 {
		create.SetTemplateID(ref.TemplateID)
	}

	msg, err := create.Save(ctx)
	if err == nil {
		return msg, nil, nil
	}
	if !ent.IsConstraintError(err) {
		return nil, nil, err
	}
	// Lost the insert race: another attempt claimed this key between our replay
	// check and now. Treat its row exactly like one we found up front.
	other, ferr := m.find(ctx, s, req.Key)
	if ferr != nil {
		return nil, nil, ferr
	}
	if other == nil {
		return nil, nil, err
	}
	if other.Status != outboundmessage.StatusPending {
		r := replayResult(other)
		return nil, &r, nil
	}
	msg, terr := m.takeOver(ctx, s, other)
	return msg, nil, terr
}

// takeOver adopts a pending claim, but only when its lease has expired (a crashed
// or abandoned attempt). The update is conditional on the claim being exactly the
// one we read, so of several racing retries exactly one wins; everyone else, and
// any retry that finds a live claim, gets ErrInProgress.
func (m *Module) takeOver(ctx context.Context, s *ent.Scoped, existing *ent.OutboundMessage) (*ent.OutboundMessage, error) {
	if m.now().Sub(existing.ClaimedAt) < m.lease {
		return nil, ErrInProgress
	}
	n, err := s.OutboundMessage().Update().
		Where(ownsClaim(existing)...).
		SetClaimedAt(m.claimTime()).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrInProgress
	}
	return s.OutboundMessage().Get(ctx, existing.ID)
}

// claimTime is the module's "now" at the precision the database stores (microseconds), so a
// claim token read back from the row compares equal to the one we wrote.
func (m *Module) claimTime() time.Time { return m.now().Truncate(time.Microsecond) }

// ownsClaim is the fence for every write an attempt makes to its own claim: the row must
// still be pending AND still carry the exact claimed_at token the attempt holds. An
// attempt that outlived its lease and was taken over matches nothing, so it can never
// record a result, release a claim or delete a row that now belongs to someone else.
func ownsClaim(msg *ent.OutboundMessage) []predicate.OutboundMessage {
	return append([]predicate.OutboundMessage{
		outboundmessage.ID(msg.ID),
		outboundmessage.StatusEQ(outboundmessage.StatusPending),
	}, claimFence(msg)...)
}

// claimFence is the claim token alone: the status condition of a move comes from the lifecycle.
func claimFence(msg *ent.OutboundMessage) []predicate.OutboundMessage {
	return []predicate.OutboundMessage{outboundmessage.ClaimedAtEQ(msg.ClaimedAt)}
}

// release hands the claim back after a definite provider failure (the call
// returned), so the next attempt may retry immediately instead of waiting out the
// lease. Best effort: if it fails, the lease expires on its own.
func (m *Module) release(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage) {
	if _, err := s.OutboundMessage().Update().
		Where(ownsClaim(msg)...).
		SetClaimedAt(time.Unix(0, 0)).
		Save(ctx); err != nil {
		logging.FromContext(ctx).Warn("outbound: release claim failed", "message_id", msg.ID, "err", err)
	}
}

// dropClaim deletes the claimed message so the retry of a Held or Deferred send starts clean.
// Best effort: if it fails, the claim's lease runs out and the retry takes over.
func (m *Module) dropClaim(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage) {
	if _, err := s.OutboundMessage().Delete().Where(ownsClaim(msg)...).Exec(ctx); err != nil {
		logging.FromContext(ctx).Warn("outbound: drop claim failed", "message_id", msg.ID, "err", err)
	}
}

// finish records a final, non-sent status with its reason.
func (m *Module) finish(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage, status outboundmessage.Status, outcome Outcome, reason string) (Result, error) {
	// A lost fence means the claim was taken over while we worked.
	if err := moveOrStale(ctx, s, msg, status, func(u *ent.OutboundMessageScopedUpdate) { u.SetReason(reason) }); err != nil {
		return Result{}, err
	}
	return Result{Outcome: outcome, Reason: reason, MessageID: msg.ID}, nil
}
