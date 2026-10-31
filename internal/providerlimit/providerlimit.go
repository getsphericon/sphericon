// Package providerlimit enforces an Integration's Provider limit (ADR 0023): two token
// buckets, per-second and rolling 24 hours, whose state is one send_limiters row per
// Integration in Postgres (no Redis). A reservation takes one token from both buckets
// or from none, in a single UPDATE through the scoped client, so concurrent workers
// and instances cannot overspend; a denial reports how long until capacity returns.
//
// Buckets are stored as a fill fraction of capacity (1 is full). Capacity and refill
// derive from the limits passed in on every call, so a user changing a limit
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
	"github.com/getsphericon/sphericon/ent/sendlimiter"
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

// bucket is one of the two token buckets: its fill column, capacity in tokens and
// the fill fraction regained per second.
type bucket struct {
	column     string
	capacity   *int
	fillPerSec float64
}

func buckets(l Limits) []bucket {
	return []bucket{
		// Capacity is one second's worth of messages, refilled over one second.
		{column: sendlimiter.FieldSecondFill, capacity: l.PerSecond, fillPerSec: 1},
		// Capacity is a day's worth of messages, refilled evenly over 24 hours.
		{column: sendlimiter.FieldDayFill, capacity: l.PerDay, fillPerSec: 1 / daySeconds},
	}
}

// writeRefilled writes the SQL for a bucket's fill as of now, capped at full.
func (b bucket) writeRefilled(bld *entsql.Builder, now time.Time) {
	bld.WriteString("LEAST(1.0, " + b.column + " + GREATEST(0, EXTRACT(EPOCH FROM (")
	bld.Arg(now)
	bld.WriteString("::timestamptz - " + sendlimiter.FieldRefilledAt + "))) * ")
	bld.Arg(b.fillPerSec)
	bld.WriteString("::float8)")
}

// threshold is the fill a bucket needs to cover one token.
func (b bucket) threshold() float64 { return 1 / float64(*b.capacity) }

// Reserve takes one token from both buckets at now. It returns a zero wait when the
// token was taken, or, when a bucket is short, the time until both buckets hold a
// token again (nothing is taken then). A call with no limit set does nothing.
func Reserve(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, now time.Time) (time.Duration, error) {
	if !l.Any() {
		return 0, nil
	}
	n, err := take(ctx, s, integrationID, l, now, true)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		// Either the row does not exist yet or a bucket is short: make sure the row
		// exists (full buckets), then try once more.
		if err := ensure(ctx, s, integrationID, now); err != nil {
			return 0, err
		}
		if n, err = take(ctx, s, integrationID, l, now, true); err != nil {
			return 0, err
		}
	}
	if n == 1 {
		return 0, nil
	}
	return waitFor(ctx, s, integrationID, l, now)
}

// Spend takes one token from both buckets at now without waiting and without the
// capacity check, so a bucket may go negative (Transactional, ADR 0023: never delayed,
// yet it makes later marketing sends wait longer). A call with no limit set does nothing.
func Spend(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, now time.Time) error {
	if !l.Any() {
		return nil
	}
	n, err := take(ctx, s, integrationID, l, now, false)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if err := ensure(ctx, s, integrationID, now); err != nil {
		return err
	}
	_, err = take(ctx, s, integrationID, l, now, false)
	return err
}

// take is the one atomic statement: refill both buckets, (when guarded) check each
// holds a token, and spend one, all in a single UPDATE. It returns the rows changed.
func take(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, now time.Time, guarded bool) (int, error) {
	bs := buckets(l)
	enough := predicate.SendLimiter(func(sel *entsql.Selector) {
		if !guarded {
			return
		}
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
	})
	return s.SendLimiter().Update().
		Where(sendlimiter.IntegrationID(integrationID), enough).
		Modify(func(u *ent.ScopedAssign) {
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
			u.Set(sendlimiter.FieldRefilledAt, entsql.ExprFunc(func(bld *entsql.Builder) {
				bld.WriteString("GREATEST(" + sendlimiter.FieldRefilledAt + ", ")
				bld.Arg(now)
				bld.WriteString("::timestamptz)")
			}))
		}).
		Save(ctx)
}

// ensure creates the Integration's limiter row with both buckets full, unless a
// concurrent reservation already did.
func ensure(ctx context.Context, s *ent.Scoped, integrationID int64, now time.Time) error {
	err := s.SendLimiter().Create().
		SetIntegrationID(integrationID).
		SetSecondFill(1).
		SetDayFill(1).
		SetRefilledAt(now).
		OnConflictColumns(sendlimiter.FieldIntegrationID).
		DoNothing().
		Exec(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // DO NOTHING on conflict reports no row: someone else created it
	}
	return err
}

// waitFor reads the row and computes how long until every limited bucket holds one
// token. It is informational (the reservation itself already failed atomically), so
// a concurrent reservation between the two can only make the answer slightly stale.
func waitFor(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, now time.Time) (time.Duration, error) {
	row, err := s.SendLimiter().Query().Where(sendlimiter.IntegrationID(integrationID)).Only(ctx)
	if err != nil {
		return 0, err
	}
	elapsed := math.Max(0, now.Sub(row.RefilledAt).Seconds())
	var wait float64
	for _, b := range buckets(l) {
		if b.capacity == nil {
			continue
		}
		fill := row.SecondFill
		if b.column == sendlimiter.FieldDayFill {
			fill = row.DayFill
		}
		fill = math.Min(1, fill+elapsed*b.fillPerSec)
		if deficit := b.threshold() - epsilon - fill; deficit > 0 {
			wait = math.Max(wait, deficit/b.fillPerSec)
		}
	}
	// Round up to a millisecond so a retry at exactly the reported wait succeeds.
	return max(time.Millisecond, time.Duration(math.Ceil(wait*1000))*time.Millisecond), nil
}
