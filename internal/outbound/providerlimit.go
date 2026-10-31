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
func (m *Module) reserve(ctx context.Context, s *ent.Scoped, req Request, r Route) (time.Duration, error) {
	if r.Integration == nil {
		return 0, nil
	}
	if req.Kind == outboundmessage.KindTransactional {
		if err := providerlimit.Spend(ctx, s, r.Integration.ID, r.Limits, m.now()); err != nil {
			return 0, fmt.Errorf("outbound: spend send capacity: %w", err)
		}
		return 0, nil
	}
	wait, err := providerlimit.Reserve(ctx, s, r.Integration.ID, r.Limits, m.now())
	if err != nil {
		return 0, fmt.Errorf("outbound: reserve send capacity: %w", err)
	}
	return wait, nil
}
