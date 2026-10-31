package broadcasts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/linkclicks"
)

// LinkClicksOf counts a Broadcast's `email.clicked` Events by link, most clicked first. Every
// click is an Event (ADR 0053), so these are repeat counts, and they cover the Event
// retention window. Machine clicks are left out. The unique click figure (Stats.Clicked) comes
// from the rollup and does not expire.
func (m *Module) LinkClicksOf(ctx context.Context, s *ent.Scoped, id int64) ([]linkclicks.Clicks, error) {
	if _, err := m.Get(ctx, s, id); err != nil {
		return nil, err
	}
	return linkclicks.ByLink(ctx, s, linkclicks.Between{}, event.BroadcastID(id))
}
