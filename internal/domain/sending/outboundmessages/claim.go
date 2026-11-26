package outboundmessages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// The unique constraints of the Outbound messages and their tracked links (ADR 0106): no client can
// break them.
var _ = db.Declare(
	db.Fault("outbound_messages_workspace_id_idempotency_key",
		"a claim, a Skipped decided before the claim and a failed attempt are written with ON CONFLICT DO NOTHING, and judge the row that holds the key"),
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

// claim takes the right to send under req.Key to targets: it inserts the pending row before
// the provider is called (claimNew). If a row already exists (existing, from Send's replay
// check, or one that appears in a race) it is replayed when final, taken over when
// its lease has expired, and otherwise reported as ErrInProgress. A claim taken over counts nothing
// new: its rows have held its places since it was written (ADR 0178).
func (m *Module) claim(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, existing *ent.OutboundMessage, targets []target) (*ent.OutboundMessage, *Result, error) {
	if existing == nil {
		id, holder, err := m.claimNew(ctx, s, req, dest, g, targets)
		if err != nil {
			return nil, nil, err
		}
		if holder == nil {
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
		existing = holder
	}
	if existing.Status != outboundmessage.StatusPending {
		r := replayResult(existing)
		return nil, &r, nil
	}
	return m.adopt(ctx, s, req, existing)
}

// claimNew writes the new claim under req.Key in one transaction with its count (ADR 0178): a
// marketing message counts at the Destination of each of targets, and the transaction holds the lock
// of each of them a Frequency cap holds (frequencycap.Module.Within, whose rule decides). A message
// that does not count (recipientLimitsOf) takes no lock and writes no row. id and holder are
// insert's; the source's stop is asked once the transaction commits (claim), so a stop committed
// meanwhile finds the row.
func (m *Module) claimNew(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, targets []target) (id int64, holder *ent.OutboundMessage, err error) {
	limits, err := recipientLimitsOf(req.Kind())
	if err != nil {
		return 0, nil, err
	}
	in := frequencycap.Claim{Channel: req.Channel()}
	if limits.counts {
		in.Destinations = lo.Map(targets, func(t target, _ int) string { return t.dest })
	}
	err = m.caps.Within(ctx, s, in, func(l *ent.DestinationLocked) error {
		var werr error
		id, holder, werr = m.writeClaim(ctx, l, req, dest, g, in)
		return werr
	})
	return id, holder, err
}

// writeClaim inserts the claim under req.Key and, when it is new, counts it at in's Destinations, in
// the transaction that holds their locks: it takes the scope that stands for them, so no claim is
// written without them. An insert that loses the key writes no row: holder is judged as one found up
// front.
func (m *Module) writeClaim(ctx context.Context, l *ent.DestinationLocked, req Request, dest string, g gateResult, in frequencycap.Claim) (int64, *ent.OutboundMessage, error) {
	ls := l.Scope()
	id, holder, err := m.insert(ctx, ls, req, m.newMessage(ls, req, dest, g))
	if err != nil || holder != nil || len(in.Destinations) == 0 {
		return id, holder, err
	}
	return id, nil, m.caps.Count(ctx, l, frequencycap.CountInput{MessageID: id, Channel: in.Channel, Destinations: in.Destinations})
}

// skipUnclaimed records a send Skipped before any claim (no Destination, Send-eligibility) as a
// decided Outbound message under req.Key, with the rules of a claim: a pending row already under the
// key (existing, or one an insert loses to) is adopted first, or reported in progress, and then
// Skipped; an insert that loses the key to a decided row replays that row; and the source's stop is
// asked once the row is written (Request.Stopped), so a stopped source gains no Skipped row.
func (m *Module) skipUnclaimed(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, existing *ent.OutboundMessage, reason skipreason.Reason) (Result, error) {
	if existing == nil {
		create := m.newMessage(s, req, dest, g).SetStatus(outboundmessage.StatusSkipped).SetSkippedReason(reason)
		id, holder, err := m.insert(ctx, s, req, create)
		if err != nil {
			return Result{}, err
		}
		if holder == nil {
			withdrawn, err := withdrawIfStopped(ctx, s, req.Stopped, id, outboundmessage.StatusEQ(outboundmessage.StatusSkipped))
			if err != nil || withdrawn {
				return lo.FromPtr(withdrawnResult(withdrawn)), err
			}
			return Result{Outcome: Skipped, SkippedReason: reason, MessageID: id}, nil
		}
		existing = holder
	}
	if existing.Status != outboundmessage.StatusPending {
		return replayResult(existing), nil
	}
	msg, replay, err := m.adopt(ctx, s, req, existing)
	if err != nil || replay != nil {
		return lo.FromPtr(replay), err
	}
	return m.skip(ctx, s, msg, reason)
}

// newMessage is the new Outbound message of req under its key, as a claim and a Skipped decided
// before the claim both write it: dest is the Request's own Destination ("" for none), g what the
// holds resolved.
func (m *Module) newMessage(s *ent.Scoped, req Request, dest string, g gateResult) *ent.OutboundMessageScopedCreate {
	create := s.OutboundMessage().Create().
		SetKind(req.Kind()).
		SetIdempotencyKey(req.Key).
		SetChannel(req.Channel()).
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
	if req.source() != "" {
		create.SetSendingSource(req.source())
	}
	if contactID := req.contactID(); contactID != 0 {
		create.SetContactID(contactID)
	}
	req.Ref.stamp(create)
	req.Waited.stamp(create)
	if req.digest != "" {
		create.SetRequestDigest(req.digest)
	}
	return create
}

// insert writes create under req.Key unless a row already holds the key: id is the row written, and
// holder nil; or holder is the row that holds the key (another attempt wrote it between Send's replay
// check and now), which the caller judges exactly like one found up front.
func (m *Module) insert(ctx context.Context, s *ent.Scoped, req Request, create *ent.OutboundMessageScopedCreate) (id int64, holder *ent.OutboundMessage, err error) {
	id, err = create.OnConflictColumns(outboundmessage.FieldWorkspaceID, outboundmessage.FieldIdempotencyKey).DoNothing().ID(ctx)
	inserted, err := db.Inserted(err)
	switch {
	case err != nil:
		return 0, nil, err
	case inserted:
		return id, nil, nil
	}
	holder, err = m.find(ctx, s, req.Key)
	if err != nil {
		return 0, nil, err
	}
	if holder == nil {
		return 0, nil, fmt.Errorf("%w: %q", errKeyVanished, req.Key)
	}
	if err := sameRequest(holder, req); err != nil {
		return 0, nil, err
	}
	return 0, holder, nil
}

// adopt takes over a pending claim another attempt left (takeOver), unless it was left behind
// after the source stopped (withdrawLeftBehind): that one is withdrawn.
func (m *Module) adopt(ctx context.Context, s *ent.Scoped, req Request, existing *ent.OutboundMessage) (*ent.OutboundMessage, *Result, error) {
	withdrawn, err := m.withdrawLeftBehind(ctx, s, req.Stopped, existing)
	if err != nil || withdrawn {
		return nil, withdrawnResult(withdrawn), err
	}
	msg, err := m.takeOver(ctx, s, req, existing)
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
// any retry that finds a live claim, gets ErrInProgress. The claim takes the wait of req, the
// attempt that now holds it.
func (m *Module) takeOver(ctx context.Context, s *ent.Scoped, req Request, existing *ent.OutboundMessage) (*ent.OutboundMessage, error) {
	if m.now().Sub(existing.ClaimedAt) < m.lease {
		return nil, ErrInProgress
	}
	u := s.OutboundMessage().Update().
		Where(ownsClaim(existing)...).
		SetClaimedAt(m.claimTime())
	req.Waited.restamp(u)
	n, err := u.Save(ctx)
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

// skip records the claimed message Skipped with its reason.
func (m *Module) skip(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage, reason skipreason.Reason) (Result, error) {
	// A lost fence means the claim was taken over while we worked.
	if err := moveOrStale(ctx, s, msg, outboundmessage.StatusSkipped, func(u *ent.OutboundMessageScopedUpdate) { u.SetSkippedReason(reason) }); err != nil {
		return Result{}, err
	}
	return Result{Outcome: Skipped, SkippedReason: reason, MessageID: msg.ID}, nil
}

// fail records the claimed message Failed with what it failed with.
func (m *Module) fail(ctx context.Context, s *ent.Scoped, msg *ent.OutboundMessage, failure string) (Result, error) {
	if err := moveOrStale(ctx, s, msg, outboundmessage.StatusFailed, func(u *ent.OutboundMessageScopedUpdate) { u.SetFailure(failure) }); err != nil {
		return Result{}, err
	}
	return Result{Outcome: Failed, Failure: failure, MessageID: msg.ID}, nil
}
