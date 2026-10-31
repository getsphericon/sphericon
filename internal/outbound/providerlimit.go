package outbound

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/providerlimit"
)

// reserve takes one token from the Provider limit of the route's Integration (ADR 0023) and from
// the Send cap of the Workspace ws (ADR 0067), together or not at all, at the module's clock. It
// returns the shortfall when a limit is spent (how long until capacity returns and whether the Send
// cap is why), and the zero Shortfall when the message may go (also when neither applies, which
// costs no write). Transactional is never delayed by either, but it is spent from every bucket
// unconditionally, so they may go negative and later marketing sends wait longer. A route with no
// Integration row (a test double) has no Provider limit; its Workspace's Send cap still applies.
func (m *Module) reserve(ctx context.Context, s *ent.Scoped, req Request, r Route, ws *ent.Workspace) (providerlimit.Shortfall, error) {
	res := providerlimit.Reservation{SendCap: m.sendCaps.Of(ws)}
	if r.Integration != nil {
		res.IntegrationID, res.Limits = r.Integration.ID, r.Limits
	}
	if req.Kind == outboundmessage.KindTransactional {
		if err := providerlimit.Spend(ctx, s, m.bus.WithinAuditTx, res, m.now()); err != nil {
			return providerlimit.Shortfall{}, fmt.Errorf("outbound: spend send capacity: %w", err)
		}
		return providerlimit.Shortfall{}, nil
	}
	short, err := providerlimit.Reserve(ctx, s, m.bus.WithinAuditTx, res, m.now())
	if err != nil {
		return providerlimit.Shortfall{}, fmt.Errorf("outbound: reserve send capacity: %w", err)
	}
	return short, nil
}

// SendCapStatus is the Send cap that applies to the Workspace ws, whose scope s is, with its sends
// over the last 24 hours at the module's clock: what the send path enforces, as its members and
// agents read it. Nil when none applies (the Workspace has none, or the Instance has no Send caps,
// whatever is stored), which costs no query.
func (m *Module) SendCapStatus(ctx context.Context, s *ent.Scoped, ws *ent.Workspace) (*providerlimit.SendCapStatus, error) {
	perDay := m.sendCaps.Of(ws)
	if perDay == nil {
		return nil, nil
	}
	sent, err := providerlimit.SentLast24h(ctx, s, m.now())
	if err != nil {
		return nil, fmt.Errorf("outbound: count the sends of the last 24 hours: %w", err)
	}
	return &providerlimit.SendCapStatus{PerDay: *perDay, Sent24h: sent}, nil
}

// SendCapDrain is how long the Workspace's Send cap (ADR 0067) takes, from now, to let n more
// messages out: zero when its bucket holds them all. ok is false when no Send cap applies (none is
// set, or the Instance has none), so nothing but the Provider limit paces the Workspace's sends.
func (m *Module) SendCapDrain(ctx context.Context, s *ent.Scoped, n int, now time.Time) (wait time.Duration, ok bool, err error) {
	ws, err := m.workspace(ctx, s)
	if err != nil {
		return 0, false, err
	}
	sendCap := m.sendCaps.Of(ws)
	if sendCap == nil {
		return 0, false, nil
	}
	wait, err = providerlimit.SendCapDrain(ctx, s, *sendCap, n, now)
	if err != nil {
		return 0, false, fmt.Errorf("outbound: read the send cap bucket: %w", err)
	}
	return wait, true, nil
}
