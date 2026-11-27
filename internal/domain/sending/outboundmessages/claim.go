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
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingwindow"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
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

// claimed is a claim Send goes on with: the claimed message and the targets it goes to.
type claimed struct {
	msg     *ent.OutboundMessage
	targets []target
}

// claim takes the right to send under req.Key to targets: it inserts the pending row before
// the provider is called (claimNew), with the targets under their Frequency caps, or decides the
// send Skipped or deferred when none is (a Result). If a row already exists (existing, from Send's replay
// check, or one that appears in a race) it is replayed when final, taken over when
// its lease has expired, and otherwise reported as ErrInProgress. A claim taken over counts nothing
// new and is not capped again: its rows have held its places since it was written (ADR 0178), and it
// goes only to those of targets it holds a place at (placedTargets).
func (m *Module) claim(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, existing *ent.OutboundMessage, targets []target) (claimed, *Result, error) {
	if existing == nil {
		n, err := m.claimNew(ctx, s, req, dest, g, targets)
		if err != nil {
			return claimed{}, nil, err
		}
		if n.holder == nil {
			return m.decidedNew(ctx, s, req, n)
		}
		existing = n.holder
	}
	if existing.Status != outboundmessage.StatusPending {
		r := replayResult(existing)
		return claimed{}, &r, nil
	}
	msg, replay, err := m.adopt(ctx, s, req, existing)
	if err != nil || replay != nil {
		return claimed{}, replay, err
	}
	placed, err := m.placedTargets(ctx, s, req, msg, targets)
	return claimed{msg: msg, targets: placed}, nil, err
}

// placedTargets is the targets a claim taken over goes to: for a message that counts, those it
// holds a place at, so a retry never reaches a Destination the cap dropped when the claim was
// written; all of them for one that does not count, or for a claim that holds no place at all
// (written before the count was).
func (m *Module) placedTargets(ctx context.Context, s *ent.Scoped, req Request, msg *ent.OutboundMessage, targets []target) ([]target, error) {
	limits, err := recipientLimitsOf(req.Kind())
	if err != nil || !limits.counts {
		return targets, err
	}
	places, err := m.caps.PlacesOf(ctx, s, msg.ID)
	if err != nil || len(places) == 0 {
		return targets, err
	}
	return lo.Filter(targets, func(t target, _ int) bool { return lo.Contains(places, t.dest) }), nil
}

// decidedNew is what claimNew decided once its transaction committed: a send deferred over the cap
// wrote no row, and its Deferral is the Result; for a new row the source's stop is asked first, so a
// stop committed meanwhile finds the row and withdraws it; else the claim goes on to its targets, or
// the Skipped decided under the locks is the Result.
func (m *Module) decidedNew(ctx context.Context, s *ent.Scoped, req Request, n newRow) (claimed, *Result, error) {
	if !n.until.IsZero() {
		return claimed{}, &Result{Outcome: Deferral, Cause: deferralcause.FrequencyCap, Until: n.until}, nil
	}
	if n.skip != "" {
		res, err := decidedSkipped(ctx, s, req, n.id, n.skip)
		return claimed{}, &res, err
	}
	msg, err := s.OutboundMessage().Get(ctx, n.id)
	if err != nil {
		return claimed{}, nil, err
	}
	withdrawn, err := withdrawIfStopped(ctx, s, req.Stopped, msg.ID, ownsClaim(msg)...)
	if err != nil || withdrawn {
		return claimed{}, withdrawnResult(withdrawn), err
	}
	return claimed{msg: msg, targets: n.targets}, nil, nil
}

// newRow is the row claimNew wrote under req.Key: a claim to targets, or, with skip, a Skipped
// decided under the locks; or, with holder, none, since holder held the key (insert); or, with
// until, none, since the send waits over the cap until then.
type newRow struct {
	id      int64
	holder  *ent.OutboundMessage
	targets []target
	skip    skipreason.Reason
	until   time.Time
}

// claimNew writes the new row under req.Key in one transaction with its count (ADR 0178): a
// marketing message is checked against the Frequency cap of the Destination of each of targets, under
// the lock of each of them a cap holds (frequencycap.Module.Within, whose rule decides). A target
// over its cap is dropped, as an ineligible one is; the claim goes to the rest and counts at each of
// them. When none is left, the send is decided there (overCap), Skipped or deferred, so two sends
// cannot both take a Destination's last place; a Deferral writes no row. A message that does not count (recipientLimitsOf) takes no lock, is not
// capped and writes no count row. The source's stop is asked once the transaction commits
// (decidedNew), so a stop committed meanwhile finds the row.
func (m *Module) claimNew(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, targets []target) (newRow, error) {
	limits, err := recipientLimitsOf(req.Kind())
	if err != nil {
		return newRow{}, err
	}
	in := frequencycap.Claim{Channel: req.Channel()}
	if limits.counts {
		in.Destinations = destinationsOf(targets)
	}
	var n newRow
	err = m.caps.Within(ctx, s, in, func(l *ent.DestinationLocked, sts frequencycap.Standings) error {
		room := lo.Filter(targets, func(t target, _ int) bool { return !sts.Over(t.dest) })
		if len(room) == 0 {
			var werr error
			n, werr = m.decideOverCap(ctx, l.Scope(), req, dest, g, sts, targets)
			return werr
		}
		count := frequencycap.CountInput{Channel: in.Channel}
		if limits.counts {
			count.Destinations = destinationsOf(room)
		}
		id, holder, werr := m.writeClaim(ctx, l, req, dest, g, count)
		n = newRow{id: id, holder: holder, targets: room}
		return werr
	})
	return n, err
}

// decideOverCap decides, under the locks, a send none of whose targets has a place under its
// Frequency cap (overCap): a Skipped is written under req.Key as a decided row; a Deferral writes
// none, unless another attempt wrote a row under the key since Send's replay check, which is judged
// as one found up front.
func (m *Module) decideOverCap(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, sts frequencycap.Standings, targets []target) (newRow, error) {
	over, err := overCap(req, sts, targets, m.now())
	if err != nil {
		return newRow{}, err
	}
	if !over.until.IsZero() {
		holder, err := m.find(ctx, s, req.Key)
		if err != nil || holder == nil {
			return newRow{until: over.until}, err
		}
		return newRow{holder: holder}, sameRequest(holder, req)
	}
	id, holder, err := m.insert(ctx, s, req, m.skippedMessage(s, req, dest, g, over.skip))
	return newRow{id: id, holder: holder, skip: over.skip}, err
}

// capOutcome is what the Frequency cap makes of a send none of whose targets has a place: Skipped
// with skip, or deferred until a place frees (until).
type capOutcome struct {
	skip  skipreason.Reason
	until time.Time
}

// overCap decides a send none of whose targets has a place under its Frequency cap (ADR 0137, ADR
// 0178), by the surface that asked, at now: the one place the cap's Outcome is chosen. Its switch
// has no default, so a new kind of Outbound message fails the exhaustive linter until it says what
// the cap does to it.
func overCap(req Request, sts frequencycap.Standings, targets []target, now time.Time) (capOutcome, error) {
	switch kind := req.Kind(); kind {
	case outboundmessage.KindBroadcast:
		// A Broadcast Recipient over the cap is Skipped, so a Broadcast is not delivered days late.
		return capOutcome{skip: skipreason.FrequencyCap}, nil
	case outboundmessage.KindAutomation:
		// An Automation send waits at its step until a place frees, so the sequence keeps its
		// order; for one period at most from when its wait there began, then it is Skipped and the
		// Enrollment moves on.
		if until := placeFrees(sts, targets, req.Waited.Since, now); until.After(now) {
			return capOutcome{until: until}, nil
		}
		return capOutcome{skip: skipreason.FrequencyCap}, nil
	case outboundmessage.KindTransactional, outboundmessage.KindConfirmation:
		// Neither counts, so neither is ever over a cap (recipientLimitsOf).
		return capOutcome{}, fmt.Errorf("outbound: a message of kind %q is never over a Frequency cap", kind)
	}
	return capOutcome{}, fmt.Errorf("outbound: no Frequency cap Outcome for kind %q", req.Kind())
}

// placeFrees is when a send whose wait began at since (zero: it begins at now) has a place
// at one of targets, each over its cap: the earliest of their release moments, each no later than
// one period of its cap after the wait began. A moment not after now is the bound reached.
func placeFrees(sts frequencycap.Standings, targets []target, since, now time.Time) time.Time {
	if since.IsZero() {
		since = now
	}
	var until time.Time
	for _, t := range targets {
		st := sts[t.dest]
		at := since.Add(st.Cap.Period)
		// A cap no place frees under (no release moment) waits to its bound.
		if !st.Release.IsZero() && st.Release.Before(at) {
			at = st.Release
		}
		if until.IsZero() || at.Before(until) {
			until = at
		}
	}
	return until
}

// destinationsOf is the Destination of each of targets.
func destinationsOf(targets []target) []string {
	return lo.Map(targets, func(t target, _ int) string { return t.dest })
}

// writeClaim inserts the claim under req.Key and, when it is new, counts it at in's Destinations, in
// the transaction that holds their locks: it takes the scope that stands for them, so no claim is
// written without them. An insert that loses the key writes no row: holder is judged as one found up
// front.
func (m *Module) writeClaim(ctx context.Context, l *ent.DestinationLocked, req Request, dest string, g gateResult, in frequencycap.CountInput) (int64, *ent.OutboundMessage, error) {
	ls := l.Scope()
	id, holder, err := m.insert(ctx, ls, req, m.newMessage(ls, req, dest, g))
	if err != nil || holder != nil || len(in.Destinations) == 0 {
		return id, holder, err
	}
	in.MessageID = id
	return id, nil, m.caps.Count(ctx, l, in)
}

// skipUnclaimed records a send Skipped before any claim (no Destination, Send-eligibility) as a
// decided Outbound message under req.Key, with the rules of a claim: a pending row already under the
// key (existing, or one an insert loses to) is adopted first, or reported in progress, and then
// Skipped; an insert that loses the key to a decided row replays that row; and the source's stop is
// asked once the row is written (Request.Stopped), so a stopped source gains no Skipped row.
func (m *Module) skipUnclaimed(ctx context.Context, s *ent.Scoped, req Request, dest string, g gateResult, existing *ent.OutboundMessage, reason skipreason.Reason) (Result, error) {
	if existing == nil {
		id, holder, err := m.insert(ctx, s, req, m.skippedMessage(s, req, dest, g, reason))
		if err != nil {
			return Result{}, err
		}
		if holder == nil {
			return decidedSkipped(ctx, s, req, id, reason)
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

// decidedSkipped is the Result of a Skipped row written before any claim, once written: the source's
// stop is asked (Request.Stopped), and a stopped source has the row withdrawn.
func decidedSkipped(ctx context.Context, s *ent.Scoped, req Request, id int64, reason skipreason.Reason) (Result, error) {
	withdrawn, err := withdrawIfStopped(ctx, s, req.Stopped, id, outboundmessage.StatusEQ(outboundmessage.StatusSkipped))
	if err != nil || withdrawn {
		return lo.FromPtr(withdrawnResult(withdrawn)), err
	}
	return Result{Outcome: Skipped, SkippedReason: reason, MessageID: id}, nil
}

// skippedMessage is the new Outbound message of req decided Skipped with reason before any claim.
func (m *Module) skippedMessage(s *ent.Scoped, req Request, dest string, g gateResult, reason skipreason.Reason) *ent.OutboundMessageScopedCreate {
	return m.newMessage(s, req, dest, g).SetStatus(outboundmessage.StatusSkipped).SetSkippedReason(reason)
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
	create.SetWindowZones(windowZones(req.window))
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
	u.SetWindowZones(windowZones(req.window))
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

// windowZones is how the Sending window judged a send, as its message keeps it (ADR 0134): by no
// window, in no zone, in one, or in several.
func windowZones(j sendingwindow.Judgement) outboundmessage.WindowZones {
	switch {
	case !j.Windowed:
		return outboundmessage.WindowZonesUnjudged
	case j.Zones == 0:
		return outboundmessage.WindowZonesNoZone
	case j.Zones == 1:
		return outboundmessage.WindowZonesOneZone
	}
	return outboundmessage.WindowZonesSeveralZones
}
