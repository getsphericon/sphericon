package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
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
	// nil when it cannot be known (a hold, or no limit to pace by).
	EstimatedCompletion *time.Time
	// LimitedBy is what holds the estimate back: the Send cap or the Provider limit, whichever
	// finishes later; "" when nothing limits the send.
	LimitedBy Limit
}

// Limit is what slows a sending Broadcast.
type Limit string

// The limits a Broadcast's progress names.
const (
	// LimitSendCap: the Workspace's Send cap (ADR 0067), set by the platform.
	LimitSendCap Limit = "send_cap"
	// LimitProviderLimit: the Provider limit of its Integration (ADR 0023).
	LimitProviderLimit Limit = "provider_limit"
)

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

// progressOf computes the progress of a sending Broadcast from its figures. A hold has no
// estimate: the source is blocked, not busy, and nothing says when it lifts. Otherwise the
// estimate is the later of the Provider limit's (providerFinish) and the Send cap's: recipients
// beyond the Workspace bucket's tokens go out at the Send cap per 24 hours. LimitedBy names
// whichever is later.
func (m *Module) progressOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, st Stats, now time.Time) (*Progress, error) {
	pending := st.Pending
	p := &Progress{Remaining: pending, Processed: st.Processed()}
	if pending == 0 || b.HoldReason != nil {
		return p, nil
	}
	provider, held, err := m.providerFinish(ctx, s, b, pending, now)
	if err != nil || held {
		return p, err
	}
	capped, err := m.out.SendCapFinish(ctx, s, pending, now)
	if err != nil {
		return nil, err
	}
	switch {
	case capped != nil && (provider == nil || capped.After(*provider)):
		p.EstimatedCompletion, p.LimitedBy = capped, LimitSendCap
	case provider != nil:
		p.EstimatedCompletion, p.LimitedBy = provider, LimitProviderLimit
	}
	return p, nil
}

// providerFinish is when the Provider limit lets the last pending recipient go: the last scheduled
// recipient job while that is still ahead; once it has passed (jobs were deferred, or the limit
// was lowered mid-send), the remaining recipients at the current rate of the Send route. Nil when
// the Provider limit does not pace the route; held when the route cannot send.
func (m *Module) providerFinish(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, pending int, now time.Time) (*time.Time, bool, error) {
	if b.LastScheduledAt != nil && b.LastScheduledAt.After(now) {
		return b.LastScheduledAt, false, nil
	}
	route, err := m.out.Route(ctx, s, b.IntegrationID)
	if err != nil || route.Held() {
		return nil, route.Held(), err
	}
	if step := route.Interval(); step > 0 {
		eta := now.Add(time.Duration(pending) * step)
		return &eta, false, nil
	}
	return nil, false, nil
}
