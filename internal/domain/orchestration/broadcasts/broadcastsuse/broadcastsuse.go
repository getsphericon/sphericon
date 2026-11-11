// Package broadcastsuse reads the use the Broadcasts make of a row: the Definition use of an armed
// Broadcast's copied Condition (ADR 0065) and the Live use of the Broadcasts that will still send
// (ADR 0099). It is a reader of the Removal check (internal/domain/orchestration/removalcheck), the
// one package easement lets import it, so every removal that checks a row reads every holder. The
// broadcasts module does not import it.
package broadcastsuse

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
)

// UsingTag returns the armed Broadcasts (scheduled, or sending and so still to be planned) whose
// copied Condition names the Tag by its name, in id order (ADR 0065). The copy keeps the name, not
// an id, so deleting the Tag would change who they reach. A draft follows its Segment, which
// answers for itself; a finished Broadcast resolves nothing any more.
func UsingTag(ctx context.Context, l *ent.WorkspaceLocked, name string) ([]*ent.Broadcast, error) {
	armed, err := armedWithCondition(ctx, l)
	if err != nil {
		return nil, err
	}
	return lo.Filter(armed, func(b *ent.Broadcast, _ int) bool {
		g, err := condition.Parse(*b.Condition)
		return err == nil && g.NamesTag(name)
	}), nil
}

// UnfitForCustomFieldType returns the armed Broadcasts (scheduled, or sending and so still to be
// planned), in id order, whose copied Condition tests the Custom field key with a leaf a field of
// type t does not take (ADR 0065): retyping the field would fail their planning. A draft follows
// its Segment, which answers for itself.
func UnfitForCustomFieldType(ctx context.Context, l *ent.WorkspaceLocked, key string, t customfield.Type) ([]*ent.Broadcast, error) {
	armed, err := armedWithCondition(ctx, l)
	if err != nil {
		return nil, err
	}
	return lo.Filter(armed, func(b *ent.Broadcast, _ int) bool {
		g, err := condition.Parse(*b.Condition)
		return err == nil && !g.FitsCustomFieldType(key, t)
	}), nil
}

// armedWithCondition returns the armed Broadcasts that hold a copied Condition, in id order.
func armedWithCondition(ctx context.Context, l *ent.WorkspaceLocked) ([]*ent.Broadcast, error) {
	return l.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending), broadcast.ConditionNotNil()).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
}

// LiveSends returns the sends of the Broadcasts in Live use (ADR 0099), in id order: one per
// scheduled or sending Broadcast, through the Integration its Send route takes and from its own
// From. A draft sends nothing until it is scheduled (ADR 0065); a sent or failed Broadcast is
// history.
func LiveSends(ctx context.Context, l *ent.WorkspaceLocked) ([]liveuse.Send, error) {
	live, err := l.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	through := messaging.NewThroughs(l.Scope())
	sends := make([]liveuse.Send, 0, len(live))
	for _, b := range live {
		integ, err := through.Of(ctx, b.Channel, b.IntegrationID)
		if err != nil {
			return nil, err
		}
		sends = append(sends, liveuse.Send{
			User:        liveuse.User{Kind: liveuse.Broadcast, ID: b.ID, Name: b.Name},
			Channel:     b.Channel,
			Integration: integ,
			From:        lo.FromPtr(b.FromEmail),
		})
	}
	return sends, nil
}
