package outboundmessages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// The unique constraints of the Outbound messages and their tracked links (ADR 0106): no client can
// break them.
var _ = db.Declare(
	db.Fault("outbound_messages_workspace_id_idempotency_key",
		"a claim and a failed attempt are written with ON CONFLICT DO NOTHING, and judge the row that holds the key"),
	db.Fault("tracked_links_token_key",
		"a short token is drawn at random and written with ON CONFLICT DO NOTHING: a draw that meets one draws again"),
)

// errKeyVanished: an insert under an idempotency key found the key taken, and then no row under it.
var errKeyVanished = errors.New("outboundmessages: the idempotency key was taken, then held no row")

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
		return m.adopt(ctx, s, req, existing)
	}

	onChannel := req.Channel()
	create := s.OutboundMessage().Create().
		SetKind(req.Kind()).
		SetIdempotencyKey(req.Key).
		SetChannel(onChannel).
		SetClaimedAt(m.claimTime())
	if dest != "" {
		create.SetDestination(dest)
	}
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
	req.Ref.stamp(create)
	if req.digest != "" {
		create.SetRequestDigest(req.digest)
	}

	id, err := create.OnConflictColumns(outboundmessage.FieldWorkspaceID, outboundmessage.FieldIdempotencyKey).DoNothing().ID(ctx)
	inserted, err := db.Inserted(err)
	switch {
	case err != nil:
		return nil, nil, err
	case inserted:
		msg, err := s.OutboundMessage().Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		withdrawn, err := withdrawIfStopped(ctx, s, req.Stopped, msg.ID, ownsClaim(msg)...)
		if err != nil || withdrawn {
			return nil, withdrawnResult(withdrawn), err
		}
		return msg, nil, nil
	}
	// Lost the insert race: another attempt claimed this key between our replay
	// check and now. Treat its row exactly like one we found up front.
	other, err := m.find(ctx, s, req.Key)
	if err != nil {
		return nil, nil, err
	}
	if other == nil {
		return nil, nil, fmt.Errorf("%w: %q", errKeyVanished, req.Key)
	}
	if err := sameRequest(other, req); err != nil {
		return nil, nil, err
	}
	if other.Status != outboundmessage.StatusPending {
		r := replayResult(other)
		return nil, &r, nil
	}
	return m.adopt(ctx, s, req, other)
}

// adopt takes over a pending claim another attempt left (takeOver), unless it was left behind
// after the source stopped (withdrawLeftBehind): that one is withdrawn.
func (m *Module) adopt(ctx context.Context, s *ent.Scoped, req Request, existing *ent.OutboundMessage) (*ent.OutboundMessage, *Result, error) {
	withdrawn, err := m.withdrawLeftBehind(ctx, s, req.Stopped, existing)
	if err != nil || withdrawn {
		return nil, withdrawnResult(withdrawn), err
	}
	msg, err := m.takeOver(ctx, s, existing)
	return msg, nil, err
}

// withdrawLeftBehind withdraws a pending claim whose lease has run out when it was written after
// its source stopped (SourceStop): the withdrawal of a claim that raced the stop never finished (the
// attempt died between the claim and the question), and taking it over would send what the stop
// refused. A claim written before the stop goes on, as any claim does. A live claim is left to
// takeOver, which reports it in progress.
func (m *Module) withdrawLeftBehind(ctx context.Context, s *ent.Scoped, stopped SourceStop, claim *ent.OutboundMessage) (bool, error) {
	if stopped == nil || m.now().Sub(claim.ClaimedAt) < m.lease {
		return false, nil
	}
	at, err := stopped(ctx, s)
	if err != nil {
		return false, fmt.Errorf("outbound: ask whether the source stopped: %w", err)
	}
	if at == nil || claim.ClaimedAt.Before(*at) {
		return false, nil
	}
	n, err := s.OutboundMessage().Delete().Where(ownsClaim(claim)...).Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("outbound: withdraw message %d: %w", claim.ID, err)
	}
	if n == 0 {
		return false, ErrInProgress // another attempt took the claim meanwhile: look again later
	}
	return true, nil
}

// withdrawIfStopped asks a message's source, once the message's row is written (a new claim, or a
// recipient's failure), whether it has stopped (Request.Stopped, Failure.Stopped). A stopped
// source gets the row deleted, matched by fence, and the message reports withdrawn: no row is left behind, so the
// Recipient it was for still reads as never claimed. A nil stop takes every message. The deletion
// must land: a claim left behind would be taken over once its lease runs out, unless
// withdrawLeftBehind catches it.
func withdrawIfStopped(ctx context.Context, s *ent.Scoped, stopped SourceStop, id int64, fence ...predicate.OutboundMessage) (bool, error) {
	if stopped == nil {
		return false, nil
	}
	at, err := stopped(ctx, s)
	if err == nil && at == nil {
		return false, nil
	}
	if _, derr := s.OutboundMessage().Delete().Where(append(fence, outboundmessage.ID(id))...).Exec(ctx); derr != nil {
		return false, errors.Join(err, fmt.Errorf("outbound: withdraw message %d: %w", id, derr))
	}
	if err != nil {
		return false, fmt.Errorf("outbound: ask whether the source stopped: %w", err)
	}
	return true, nil
}

// withdrawnResult is the Result of a withdrawn message, nil for one that was not.
func withdrawnResult(withdrawn bool) *Result {
	if !withdrawn {
		return nil
	}
	return &Result{Outcome: Withdrawn}
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
		slog.WarnContext(ctx, "outbound: release claim failed", "message_id", msg.ID, "err", err)
	}
}

// dropClaim deletes the claimed message so the retry of a Held or Deferred send starts clean.
// Best effort: if it fails, the claim's lease runs out and the retry takes over.
func (m *Module) dropClaim(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage) {
	if _, err := s.OutboundMessage().Delete().Where(ownsClaim(msg)...).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "outbound: drop claim failed", "message_id", msg.ID, "err", err)
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
