package broadcasts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
)

// LinkClicksOf counts a Broadcast's click Events (`email.clicked`, `telegram.clicked`, `sms.clicked`) by link, most clicked first. Every
// click is an Event (ADR 0053), so these are repeat counts, and they cover the Event
// retention window. Machine clicks are left out. The unique click figure (Stats.Clicked) comes
// from the rollup and does not expire.
func (m *Module) LinkClicksOf(ctx context.Context, s *ent.Scoped, id int64) ([]linkclick.Clicks, error) {
	if _, err := m.row(ctx, s, id); err != nil {
		return nil, err
	}
	return linkclick.ByLink(ctx, s, linkclick.Between{}, event.BroadcastID(id))
}
