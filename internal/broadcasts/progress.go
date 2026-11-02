package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/providerlimit"
)

// Progress is how far a sending Broadcast has got (ADR 0023). A Broadcast stays
// "sending" while it is paced, held or deferred, so the progress is derived from its
// recipients, not from a status.
type Progress struct {
	// Processed counts recipients already decided: sent, skipped or failed.
	Processed int
	// Remaining counts recipients still pending.
	Remaining int
	// EstimatedCompletion is when the last remaining recipient is expected to go out;
	// nil when it cannot be known (a hold, or nothing that limits the sending).
	EstimatedCompletion *time.Time
	// LimitedBy is what the estimate waits for: the Send cap of the Workspace (when it lets the
	// remaining recipients out later than the Provider limit would) or the Provider limit of the
	// Integration; "" when nothing limits the sending (or it is held).
	LimitedBy providerlimit.Cause
}

// Detail is one Broadcast's figures and, while it is sending, its Progress, both from a
// single count of its recipients. A list of Broadcasts reads StatsOf instead: no Progress,
// two grouped queries for the page (ADR 0023).
type Detail struct {
	Stats    Stats
	Progress *Progress
}

// Detail reads a Broadcast's figures and, when it is sending, its Progress.
func (m *Module) Detail(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, now time.Time) (Detail, error) {
	figures, err := StatsOf(ctx, s, b.ID)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Stats: figures[b.ID]}
	if b.Status != broadcast.StatusSending {
		return d, nil
	}
	d.Progress, err = m.progressOf(ctx, s, b, d.Stats, now)
	return d, err
}

// progressOf computes the progress of a sending Broadcast from its figures. The estimate is the
// later of two: the Provider limit's, which is the last scheduled recipient job while that is still
// ahead and otherwise (jobs were deferred, or the limit was lowered mid-send) the remaining
// recipients at the current rate of its Send route; and the Send cap's, the remaining recipients
// beyond the Workspace bucket's current tokens at the Send cap per 24 hours (the planner paces by
// the Provider limit alone, so the Send cap shows only here). A hold has no estimate, recorded or
// the route's own: the source is blocked, not busy, and nothing says when it lifts.
func (m *Module) progressOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, st Stats, now time.Time) (*Progress, error) {
	pending := st.Pending
	p := &Progress{Remaining: pending, Processed: st.Processed()}
	if pending == 0 || b.HoldReason != nil {
		return p, nil
	}
	channel := outbound.BroadcastContent(b).Channel()
	route, err := m.out.Route(ctx, s, channel, b.IntegrationID)
	if err != nil {
		return nil, err
	}
	if route.Held() {
		return p, nil
	}
	switch {
	case b.LastScheduledAt != nil && b.LastScheduledAt.After(now):
		p.EstimatedCompletion, p.LimitedBy = b.LastScheduledAt, providerlimit.CauseProviderLimit
	case route.Interval() > 0:
		eta := now.Add(time.Duration(pending) * route.Interval())
		p.EstimatedCompletion, p.LimitedBy = &eta, providerlimit.CauseProviderLimit
	}
	wait, capped, err := m.out.SendCapDrain(ctx, s, pending, now)
	if err != nil {
		return nil, err
	}
	if eta := now.Add(wait); capped && wait > 0 && (p.EstimatedCompletion == nil || eta.After(*p.EstimatedCompletion)) {
		p.EstimatedCompletion, p.LimitedBy = &eta, providerlimit.CauseSendCap
	}
	return p, nil
}
