package providerlimit

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/sendlimiter"
)

// The Send cap (ADR 0067) is a third token bucket beside an Integration's two: rolling 24 hours,
// one per Workspace (a send_limiters row), spent by every Outbound send of the Workspace across
// all its Integrations. A reservation takes one token from the Workspace bucket and from the
// Integration's buckets, or from none, in one transaction that locks the Workspace bucket first,
// so concurrent workers always lock in the same order. A Workspace without a Send cap costs no
// write.

// Cause is what a denied reservation waits for.
type Cause string

// The causes of a denied reservation.
const (
	// CauseProviderLimit: the Integration's Provider limit is spent.
	CauseProviderLimit Cause = "provider_limit"
	// CauseSendCap: the Workspace's Send cap is spent.
	CauseSendCap Cause = "send_cap"
)

// errSpent ends a reservation's transaction without a write: the Workspace token it took is
// handed back because the Integration's buckets were short.
var errSpent = errors.New("providerlimit: spent")

// ReserveAll takes one token from the Workspace's Send cap bucket (when sendCap is set) and from
// the Integration's Provider limit buckets at now, all or none. It returns a zero wait when the
// tokens were taken, or the time until the short bucket holds a token again and which one it was.
func ReserveAll(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, sendCap *int, now time.Time) (time.Duration, Cause, error) {
	if sendCap == nil {
		wait, err := Reserve(ctx, s, integrationID, l, now)
		return wait, causeOf(wait, CauseProviderLimit), err
	}
	var (
		wait  time.Duration
		cause Cause
	)
	err := s.Atomic(ctx, func(ts *ent.Scoped) error {
		w, err := reserveSendCap(ctx, ts, *sendCap, now)
		if err != nil || w > 0 {
			wait, cause = w, CauseSendCap
			return err
		}
		if w, err = Reserve(ctx, ts, integrationID, l, now); err != nil {
			return err
		}
		if w > 0 {
			wait, cause = w, CauseProviderLimit
			return errSpent
		}
		return nil
	})
	if errors.Is(err, errSpent) {
		err = nil
	}
	return wait, cause, err
}

// SpendAll takes one token from the Workspace's Send cap bucket (when sendCap is set) and from
// the Integration's Provider limit buckets without waiting, so the buckets may go negative
// (Transactional: never delayed, yet it makes later marketing sends wait longer).
func SpendAll(ctx context.Context, s *ent.Scoped, integrationID int64, l Limits, sendCap *int, now time.Time) error {
	if sendCap == nil {
		return Spend(ctx, s, integrationID, l, now)
	}
	return s.Atomic(ctx, func(ts *ent.Scoped) error {
		if err := spendSendCap(ctx, ts, *sendCap, now); err != nil {
			return err
		}
		return Spend(ctx, ts, integrationID, l, now)
	})
}

// SendCapStatus is a Workspace's Send cap as shown to it: the most messages per rolling 24 hours
// and the messages it sent over the last 24 hours. Each surface maps it to its own DTO.
type SendCapStatus struct {
	PerDay  int
	Sent24h int
}

// SendCapStatusOf is the Workspace's Send cap with its last-24-hour sends (SentLast24h).
func SendCapStatusOf(ctx context.Context, s *ent.Scoped, sendCap int, now time.Time) (SendCapStatus, error) {
	sent, err := SentLast24h(ctx, s, now)
	return SendCapStatus{PerDay: sendCap, Sent24h: sent}, err
}

// SentLast24h counts the Workspace's Outbound messages the provider accepted over the 24 hours
// before now, across its Integrations and Transactional included: what its Send cap spends.
func SentLast24h(ctx context.Context, s *ent.Scoped, now time.Time) (int, error) {
	return s.OutboundMessage().Query().
		Where(outboundmessage.StatusEQ(outboundmessage.StatusSent), outboundmessage.SentAtGT(now.Add(-window))).
		Count(ctx)
}

func causeOf(wait time.Duration, c Cause) Cause {
	if wait > 0 {
		return c
	}
	return ""
}

// dayBucket is the Send cap's bucket: a day's worth of messages, refilled evenly over 24 hours.
func dayBucket(sendCap int) bucket {
	return bucket{column: sendlimiter.FieldDayFill, capacity: &sendCap, fillPerSec: 1 / daySeconds}
}

func reserveSendCap(ctx context.Context, s *ent.Scoped, sendCap int, now time.Time) (time.Duration, error) {
	n, err := takeSendCap(ctx, s, sendCap, now, true)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		if err := ensureSendCap(ctx, s, now); err != nil {
			return 0, err
		}
		if n, err = takeSendCap(ctx, s, sendCap, now, true); err != nil {
			return 0, err
		}
	}
	if n == 1 {
		return 0, nil
	}
	return sendCapWait(ctx, s, sendCap, now)
}

func spendSendCap(ctx context.Context, s *ent.Scoped, sendCap int, now time.Time) error {
	n, err := takeSendCap(ctx, s, sendCap, now, false)
	if err != nil || n > 0 {
		return err
	}
	if err := ensureSendCap(ctx, s, now); err != nil {
		return err
	}
	_, err = takeSendCap(ctx, s, sendCap, now, false)
	return err
}

// takeSendCap refills the Workspace bucket, (when guarded) checks it holds a token, and spends
// one, in a single UPDATE. It returns the rows changed.
func takeSendCap(ctx context.Context, s *ent.Scoped, sendCap int, now time.Time, guarded bool) (int, error) {
	b := dayBucket(sendCap)
	enough := predicate.SendLimiter(func(sel *entsql.Selector) {
		if !guarded {
			return
		}
		// raw sql: compares a refilled value computed from the stored fill and elapsed time; ent
		// predicates compare columns only.
		sel.Where(entsql.P(func(bld *entsql.Builder) {
			b.writeRefilled(bld, now) // refilled_at, like provider_limiters
			bld.WriteString(" >= ")
			bld.Arg(b.threshold() - epsilon)
			bld.WriteString("::float8")
		}))
	})
	return s.SendLimiter().Update().
		Where(enough).
		Modify(func(u *ent.ScopedAssign) {
			// raw sql: refill and spend one token in the same UPDATE, computed from the stored row;
			// ent's setters take a value, not an expression over columns.
			u.Set(b.column, entsql.ExprFunc(func(bld *entsql.Builder) {
				bld.WriteString("(")
				b.writeRefilled(bld, now) // refilled_at, like provider_limiters
				bld.WriteString(") - ")
				bld.Arg(b.threshold())
				bld.WriteString("::float8")
			}))
			// raw sql: GREATEST(refilled_at, now) so the clock never moves back; ent has no GREATEST.
			u.Set(sendlimiter.FieldRefilledAt, entsql.ExprFunc(func(bld *entsql.Builder) {
				bld.WriteString("GREATEST(" + sendlimiter.FieldRefilledAt + ", ")
				bld.Arg(now)
				bld.WriteString("::timestamptz)")
			}))
		}).
		Save(ctx)
}

// ensureSendCap creates the Workspace's bucket full, unless a concurrent reservation already did.
func ensureSendCap(ctx context.Context, s *ent.Scoped, now time.Time) error {
	err := s.SendLimiter().Create().
		SetDayFill(1).
		SetRefilledAt(now).
		OnConflictColumns(sendlimiter.FieldWorkspaceID).
		DoNothing().
		Exec(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // DO NOTHING on conflict reports no row: someone else created it
	}
	return err
}

// sendCapWait reads the Workspace bucket and computes how long until it holds one token.
func sendCapWait(ctx context.Context, s *ent.Scoped, sendCap int, now time.Time) (time.Duration, error) {
	row, err := s.SendLimiter().Query().Only(ctx)
	if err != nil {
		return 0, err
	}
	b := dayBucket(sendCap)
	fill := math.Min(1, row.DayFill+math.Max(0, now.Sub(row.RefilledAt).Seconds())*b.fillPerSec)
	wait := math.Max(0, (b.threshold()-epsilon-fill)/b.fillPerSec)
	// Round up to a millisecond so a retry at exactly the reported wait succeeds.
	return max(time.Millisecond, time.Duration(math.Ceil(wait*1000))*time.Millisecond), nil
}
