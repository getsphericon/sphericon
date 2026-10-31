package outbound

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/providerlimit"
)

// reserve takes one token from the Provider limit of the route's Integration (ADR 0023), at the
// module's clock. It returns the wait until capacity returns when the limit is spent, and zero when
// the message may go (also when the Integration is not limited, which costs no write).
// Transactional is never delayed by the limit, but it is spent from both buckets
// unconditionally, so they may go negative and later marketing sends wait longer.
// A route with no Integration row (a test double) has no limit to spend.
//
// With Send caps on (WithSendCap), the Workspace's Send cap (ADR 0067) is spent with the Provider
// limit, all or none, and the returned cause says which one is short.
func (m *Module) reserve(ctx context.Context, s *ent.Scoped, req Request, r Route, ws *ent.Workspace) (time.Duration, providerlimit.Cause, error) {
	if r.Integration == nil {
		return 0, "", nil
	}
	var sendCap *int
	if m.sendCap {
		sendCap = ws.SendCap
	}
	if req.Kind == outboundmessage.KindTransactional {
		if err := providerlimit.SpendAll(ctx, s, r.Integration.ID, r.Limits, sendCap, m.now()); err != nil {
			return 0, "", fmt.Errorf("outbound: spend send capacity: %w", err)
		}
		return 0, "", nil
	}
	wait, cause, err := providerlimit.ReserveAll(ctx, s, r.Integration.ID, r.Limits, sendCap, m.now())
	if err != nil {
		return 0, "", fmt.Errorf("outbound: reserve send capacity: %w", err)
	}
	return wait, cause, nil
}

// SendCap is the Workspace's Send cap as it applies to its sends, with its last-24-hour sends; nil
// when none applies: the Workspace has no Send cap, or the Instance has no Send caps (WithSendCap),
// where a stored one has no effect and so is not shown either.
func (m *Module) SendCap(ctx context.Context, s *ent.Scoped, ws *ent.Workspace) (*providerlimit.SendCapStatus, error) {
	if !m.sendCap || ws.SendCap == nil {
		return nil, nil
	}
	st, err := providerlimit.SendCapStatusOf(ctx, s, *ws.SendCap, m.now())
	if err != nil {
		return nil, err
	}
	return &st, nil
}
