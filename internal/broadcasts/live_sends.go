package broadcasts

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/inuse"
)

// LiveSends returns the sends of the Broadcasts in Live use (ADR 0099), in id order: one per
// scheduled or sending Broadcast, through the Integration its Send route takes and from its own
// From. A draft sends nothing until it is scheduled (ADR 0065); a sent or failed Broadcast is
// history.
func LiveSends(ctx context.Context, s *ent.Scoped) ([]inuse.Send, error) {
	live, err := s.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	through := messaging.NewThroughs(s)
	sends := make([]inuse.Send, 0, len(live))
	for _, b := range live {
		integ, err := through.Of(ctx, b.Channel, b.IntegrationID)
		if err != nil {
			return nil, err
		}
		sends = append(sends, inuse.Send{
			User:        inuse.User{Kind: inuse.Broadcast, ID: b.ID, Name: b.Name},
			Channel:     b.Channel,
			Integration: integ,
			From:        lo.FromPtr(b.FromEmail),
		})
	}
	return sends, nil
}
