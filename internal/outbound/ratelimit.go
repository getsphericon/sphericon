package outbound

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/sendlimit"
)

// reserve takes one token from the Integration's Send rate limit (ADR 0023).
// It returns the wait until capacity returns when the limit is spent, and zero when
// the message may go (also when the Integration is not limited, which costs no write).
// Transactional is never delayed by the limit, but it is spent from both buckets
// unconditionally, so they may go negative and later marketing sends wait longer.
// integ is the Integration the gate resolved; nil (a test double has no row) means
// there is no limit to spend.
func (m *Module) reserve(ctx context.Context, s *ent.Scoped, req Request, integ *ent.Integration) (time.Duration, error) {
	if integ == nil {
		return 0, nil
	}
	limits := sendlimit.EffectiveOf(integ).Limits()
	if req.Kind == outboundmessage.KindTransactional {
		if err := sendlimit.Spend(ctx, s, integ.ID, limits, time.Now()); err != nil {
			return 0, fmt.Errorf("outbound: spend send capacity: %w", err)
		}
		return 0, nil
	}
	wait, err := sendlimit.Reserve(ctx, s, integ.ID, limits, time.Now())
	if err != nil {
		return 0, fmt.Errorf("outbound: reserve send capacity: %w", err)
	}
	return wait, nil
}
