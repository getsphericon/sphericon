// Package providerlimit enforces the send limits a message spends from (no Redis): an
// Integration's Provider limit (ADR 0023), two token buckets, per-second and rolling 24 hours, in
// one provider_limiters row per Integration; and its Workspace's Send cap (ADR 0067), one rolling
// 24-hour bucket in one send_cap_limiters row per Workspace. A reservation takes one token from
// every bucket that applies or from none: the Integration's two in a single UPDATE through the
// scoped client, and, when a Send cap applies as well, the Workspace's first and the
// Integration's second in one transaction, so concurrent workers and instances cannot overspend
// or deadlock. A denial reports how long until capacity returns and which limit is short.
//
// Buckets are stored as a fill fraction of capacity (1 is full). Capacity and refill
// derive from the limits passed in on every call, so a changed limit or Send cap
// takes effect at once, and a bucket with no limit is simply kept full.
package providerlimit

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/ent/sendcaplimiter"
)

// daySeconds is the length of the rolling window in seconds.
const daySeconds = 86400.0

// epsilon absorbs floating point drift in the "enough for one token" comparison.
const epsilon = 1e-9

// Limits are an Integration's configured ceilings; a nil field is not limited.
// The effective ceiling is the minimum of the available values (manual only for
// now), so this is where a provider-reported or warmup value would be folded in.
type Limits struct {
	PerSecond *int
	PerDay    *int
}

// Any reports whether at least one ceiling is set.
func (l Limits) Any() bool { return l.PerSecond != nil || l.PerDay != nil }

// Interval is the spacing that keeps a stream of messages within the limits: one over
// the effective rate, which is the slower of the per-second ceiling and the 24-hour
// ceiling spread evenly over the day. It is zero when no ceiling is set.
func (l Limits) Interval() time.Duration {
	var step time.Duration
	if l.PerSecond != nil && *l.PerSecond > 0 {
		step = time.Second / time.Duration(*l.PerSecond)
	}
	if l.PerDay != nil && *l.PerDay > 0 {
		step = max(step, time.Duration(daySeconds)*time.Second/time.Duration(*l.PerDay))
	}
	return step
}

// Reservation is what one message spends from: the Provider limit of the Integration it goes
// through and the Send cap of its Workspace (the scope's).
type Reservation struct {
	// IntegrationID is the Integration's row; zero for a route with none (a test double),
	// which has no Provider limit.
	IntegrationID int64
	// Limits is the Integration's effective Provider limit.
	Limits Limits
	// SendCap is the Workspace's Send cap in messages per rolling 24 hours; nil when none
	// applies, which costs no write.
	SendCap *int
}

// integration reports whether the Integration's buckets apply.
func (r Reservation) integration() bool { return r.IntegrationID != 0 && r.Limits.Any() }

// Cause is the limit that slows sending: what a Deferral waits for, and what paces the rest of a
// sending Broadcast. Each surface maps it to its own enum once, in its converter.
type Cause string

const (
	// CauseProviderLimit: the Integration's Provider limit (ADR 0023).
	CauseProviderLimit Cause = "provider_limit"
	// CauseSendCap: the Workspace's Send cap (ADR 0067).
	CauseSendCap Cause = "send_cap"
	// CauseProvider: the provider's own, which answered "too fast" or "quota exceeded"; only a
	// Deferral names it.
	CauseProvider Cause = "provider"
)

// Shortfall is a denied reservation: the wait until every short bucket holds a token again, and
// the limit whose wait is the longest. The zero Shortfall is a granted one.
type Shortfall struct {
	Wait  time.Duration
	Cause Cause
}

// Tx runs fn in one transaction over the scope s (events.Bus.WithinAuditTx), so the Workspace
// and Integration buckets are spent together or not at all.
type Tx func(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped) error) error

// errShort rolls back a reservation that found a bucket short.
var errShort = errors.New("providerlimit: a bucket is short")

// Reserve takes one token from every bucket of r at now, or none. It returns the zero Shortfall
// when the tokens were taken; otherwise how long until every bucket holds one again (nothing is
// taken then). A reservation with no limit and no Send cap does nothing.
func Reserve(ctx context.Context, s *ent.Scoped, tx Tx, r Reservation, now time.Time) (Shortfall, error) {
	return spend(ctx, s, tx, r, now, true)
}

// Spend takes one token from every bucket of r at now without waiting and without the capacity
// check, so a bucket may go negative (Transactional, ADR 0023 and ADR 0067: never delayed, yet it
// makes later marketing sends wait longer). A reservation with no limit and no Send cap does
// nothing.
func Spend(ctx context.Context, s *ent.Scoped, tx Tx, r Reservation, now time.Time) error {
	_, err := spend(ctx, s, tx, r, now, false)
	return err
}

// spend takes from the buckets of r: the Workspace's first, then the Integration's, inside one
// transaction when both apply (one fixed order, so concurrent workers cannot deadlock), as a
// single statement when only one does. A guarded take that finds a bucket short rolls back and
// reports the shortfall, read after the rollback.
func spend(ctx context.Context, s *ent.Scoped, tx Tx, r Reservation, now time.Time, guarded bool) (Shortfall, error) {
	steps := make([]func(*ent.Scoped) (bool, error), 0, 2)
	if r.SendCap != nil {
		steps = append(steps, func(ts *ent.Scoped) (bool, error) { return takeWorkspace(ctx, ts, *r.SendCap, now, guarded) })
	}
	if r.integration() {
		steps = append(steps, func(ts *ent.Scoped) (bool, error) {
			return takeIntegration(ctx, ts, r.IntegrationID, r.Limits, now, guarded)
		})
	}
	run := func(ts *ent.Scoped) error {
		for _, step := range steps {
			ok, err := step(ts)
			if err != nil {
				return err
			}
			if !ok {
				return errShort
			}
		}
		return nil
	}
	var err error
	switch len(steps) {
	case 0:
		return Shortfall{}, nil
	case 1:
		err = run(s)
	default:
		err = tx(ctx, s, run)
	}
	if errors.Is(err, errShort) {
		return shortfall(ctx, s, r, now)
	}
	return Shortfall{}, err
}

// bucket is one token bucket: its fill and refill-time columns, capacity in tokens and
// the fill fraction regained per second.
type bucket struct {
	column     string
	refilledAt string
	capacity   *int
	fillPerSec float64
}

func integrationBuckets(l Limits) []bucket {
	return []bucket{
		// Capacity is one second's worth of messages, refilled over one second.
		{column: providerlimiter.FieldSecondFill, refilledAt: providerlimiter.FieldRefilledAt, capacity: l.PerSecond, fillPerSec: 1},
		// Capacity is a day's worth of messages, refilled evenly over 24 hours.
		{column: providerlimiter.FieldDayFill, refilledAt: providerlimiter.FieldRefilledAt, capacity: l.PerDay, fillPerSec: 1 / daySeconds},
	}
}

// workspaceBucket is the Send cap's: a day's worth of messages, refilled evenly over 24 hours.
func workspaceBucket(sendCap int) bucket {
	return bucket{column: sendcaplimiter.FieldDayFill, refilledAt: sendcaplimiter.FieldRefilledAt, capacity: &sendCap, fillPerSec: 1 / daySeconds}
}

// writeRefilled writes the SQL for a bucket's fill as of now, capped at full.
func (b bucket) writeRefilled(bld *entsql.Builder, now time.Time) {
	bld.WriteString("LEAST(1.0, " + b.column + " + GREATEST(0, EXTRACT(EPOCH FROM (")
	bld.Arg(now)
	bld.WriteString("::timestamptz - " + b.refilledAt + "))) * ")
	bld.Arg(b.fillPerSec)
	bld.WriteString("::float8)")
}

// threshold is the fill a bucket needs to cover one token.
func (b bucket) threshold() float64 { return 1 / float64(*b.capacity) }

// enough narrows an UPDATE to a row whose limited buckets each hold a token as of now.
func enough(sel *entsql.Selector, bs []bucket, now time.Time) {
	for _, b := range bs {
		if b.capacity == nil {
			continue
		}
		// raw sql: compares a refilled value computed from the stored fill and elapsed time; ent
		// predicates compare columns only.
		sel.Where(entsql.P(func(bld *entsql.Builder) {
			b.writeRefilled(bld, now)
			bld.WriteString(" >= ")
			bld.Arg(b.threshold() - epsilon)
			bld.WriteString("::float8")
		}))
	}
}

// assignSpent sets every bucket to its refilled fill less one token, and the refill time to now.
func assignSpent(u *ent.ScopedAssign, bs []bucket, refilledAt string, now time.Time) {
	for _, b := range bs {
		if b.capacity == nil {
			u.Set(b.column, 1.0) // not limited: keep it full for when a limit appears
			continue
		}
		// raw sql: refill and spend one token in the same UPDATE, computed from the stored row;
		// ent's setters take a value, not an expression over columns.
		u.Set(b.column, entsql.ExprFunc(func(bld *entsql.Builder) {
			bld.WriteString("(")
			b.writeRefilled(bld, now)
			bld.WriteString(") - ")
			bld.Arg(b.threshold())
			bld.WriteString("::float8")
		}))
	}
	// raw sql: GREATEST(refilled_at, now) so the clock never moves back; ent has no GREATEST.
	u.Set(refilledAt, entsql.ExprFunc(func(bld *entsql.Builder) {
		bld.WriteString("GREATEST(" + refilledAt + ", ")
		bld.Arg(now)
		bld.WriteString("::timestamptz)")
	}))
}

// takeIntegration is the Integration's one atomic statement: refill both buckets, (when guarded)
// check each holds a token, and spend one, all in a single UPDATE, creating the row full first
// when there is none. It reports whether the token was taken.
func takeIntegration(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, now time.Time, guarded bool) (bool, error) {
	bs := integrationBuckets(l)
	update := func() (int, error) {
		return s.ProviderLimiter().Update().
			Where(providerlimiter.IntegrationID(integrationID), predicate.ProviderLimiter(func(sel *entsql.Selector) {
				if guarded {
					enough(sel, bs, now)
				}
			})).
			Modify(func(u *ent.ScopedAssign) { assignSpent(u, bs, providerlimiter.FieldRefilledAt, now) }).
			Save(ctx)
	}
	ensure := func() error {
		return ignoreConflict(s.ProviderLimiter().Create().
			SetIntegrationID(integrationID).
			SetSecondFill(1).
			SetDayFill(1).
			SetRefilledAt(now).
			OnConflictColumns(providerlimiter.FieldIntegrationID).
			DoNothing().
			Exec(ctx))
	}
	return takeOrCreate(update, ensure)
}

// takeWorkspace is takeIntegration for the Workspace's Send cap bucket.
func takeWorkspace(ctx context.Context, s *ent.Scoped, sendCap int, now time.Time, guarded bool) (bool, error) {
	bs := []bucket{workspaceBucket(sendCap)}
	update := func() (int, error) {
		return s.SendCapLimiter().Update().
			Where(predicate.SendCapLimiter(func(sel *entsql.Selector) {
				if guarded {
					enough(sel, bs, now)
				}
			})).
			Modify(func(u *ent.ScopedAssign) { assignSpent(u, bs, sendcaplimiter.FieldRefilledAt, now) }).
			Save(ctx)
	}
	ensure := func() error {
		return ignoreConflict(s.SendCapLimiter().Create().
			SetDayFill(1).
			SetRefilledAt(now).
			OnConflictColumns(sendcaplimiter.FieldWorkspaceID).
			DoNothing().
			Exec(ctx))
	}
	return takeOrCreate(update, ensure)
}

// takeOrCreate runs update; when it touched no row (either there is none yet or a bucket is
// short) it makes sure the row exists with full buckets and runs update once more.
func takeOrCreate(update func() (int, error), ensure func() error) (bool, error) {
	n, err := update()
	if err != nil || n > 0 {
		return n > 0, err
	}
	if err := ensure(); err != nil {
		return false, err
	}
	n, err = update()
	return n > 0, err
}

// ignoreConflict treats an insert that did nothing on conflict as done: someone else created the
// row. DO NOTHING on conflict reports no row.
func ignoreConflict(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// shortfall reads the buckets of r and computes how long until each holds one token, reporting
// the longest wait and its limit. It is informational (the reservation itself
// already failed atomically and was rolled back), so a concurrent reservation between the two can
// only make the answer slightly stale; a row that does not exist (it was created in the rolled
// back transaction) is a full bucket.
func shortfall(ctx context.Context, s *ent.Scoped, r Reservation, now time.Time) (Shortfall, error) {
	var workspaceWait, integrationWait float64
	if r.SendCap != nil {
		row, err := s.SendCapLimiter().Query().Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return Shortfall{}, err
		}
		if row != nil {
			workspaceWait = workspaceBucket(*r.SendCap).secondsUntil(1, row.DayFill, row.RefilledAt, now)
		}
	}
	if r.integration() {
		row, err := s.ProviderLimiter().Query().Where(providerlimiter.IntegrationID(r.IntegrationID)).Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return Shortfall{}, err
		}
		if row != nil {
			for _, b := range integrationBuckets(r.Limits) {
				fill := row.SecondFill
				if b.column == providerlimiter.FieldDayFill {
					fill = row.DayFill
				}
				integrationWait = math.Max(integrationWait, b.secondsUntil(1, fill, row.RefilledAt, now))
			}
		}
	}
	cause := CauseProviderLimit
	if workspaceWait > 0 && workspaceWait >= integrationWait {
		cause = CauseSendCap
	}
	wait := math.Max(workspaceWait, integrationWait)
	// Round up to a millisecond so a retry at exactly the reported wait succeeds.
	return Shortfall{
		Wait:  max(time.Millisecond, time.Duration(math.Ceil(wait*1000))*time.Millisecond),
		Cause: cause,
	}, nil
}

// fillAt is the bucket's fill at now, from fill as of refilledAt: refilled for the time since,
// capped at full (the Go side of writeRefilled).
func (b bucket) fillAt(fill float64, refilledAt, now time.Time) float64 {
	elapsed := math.Max(0, now.Sub(refilledAt).Seconds())
	return math.Min(1, fill+elapsed*b.fillPerSec)
}

// secondsUntil is the seconds from now until the bucket, at fill as of refilledAt, has held n
// tokens: zero for a bucket with no limit or one that holds them already. Past the bucket's
// capacity the tokens beyond it come at the refill rate.
func (b bucket) secondsUntil(n int, fill float64, refilledAt, now time.Time) float64 {
	if b.capacity == nil {
		return 0
	}
	return math.Max(0, (float64(n)*b.threshold()-epsilon-b.fillAt(fill, refilledAt, now))/b.fillPerSec)
}
