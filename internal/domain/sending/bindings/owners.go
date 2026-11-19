package bindings

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// Owners is the side of the Bindings that serves every Channel alike: it hands a Visitor's
// Bindings, whatever their Channel, to the Contact a proven Identify binds the Visitor to
// (ADR 0171, ADR 0179). It records the System events of those changes in the transaction of the
// scope it is given, so that scope must be bound to a transaction of its bus (events.Bus.PublishIn).
type Owners struct {
	bus *events.Bus
	now func() time.Time
}

// NewOwners builds the owners' side of the Bindings over the bus their events are published on; now
// is the clock a hand-over is dated by.
func NewOwners(bus *events.Bus, now func() time.Time) *Owners {
	return &Owners{bus: bus, now: now}
}

// HandOver gives the Contact every Binding of the Visitor vis on every Channel, which a proven
// Identify has just bound to it, and records each Binding's bound event on its Channel
// (BoundAction). Run it inside a transaction of the bus that s is bound to. A Visitor with no
// Binding hands over nothing, so a repeated Identify records nothing.
func (o *Owners) HandOver(ctx context.Context, s *ent.Scoped, vis *ent.Visitor, to *ent.Contact) error {
	rows, err := s.Binding().Query().Where(binding.VisitorID(vis.ID)).Order(binding.ByID()).All(ctx)
	if err != nil {
		return fmt.Errorf("list visitor bindings: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	ids := lo.Map(rows, func(b *ent.Binding, _ int) int64 { return b.ID })
	if err := s.Binding().Update().Where(binding.IDIn(ids...)).SetContactID(to.ID).ClearVisitorID().SetBoundAt(o.now()).Exec(ctx); err != nil {
		return fmt.Errorf("hand over bindings: %w", err)
	}
	for _, b := range rows {
		action, err := BoundAction(b.Channel)
		if err != nil {
			return fmt.Errorf("hand over binding %d: %w", b.ID, err)
		}
		dedup := fmt.Sprintf("%s:binding:%d:contact:%d/bound", b.Channel, b.ID, to.ID)
		if err := publishChange(ctx, o.bus, s, action, to, b.IntegrationID, dedup); err != nil {
			return err
		}
	}
	return nil
}

// BoundAction is the System event a Contact gaining a Binding on ch records: `<channel>.bound`
// (ADR 0179), which the contract lists among the System events of every Channel of Bindings. An
// error for a Channel whose bound event the contract does not have.
func BoundAction(ch binding.Channel) (string, error) {
	action := string(ch) + ".bound"
	if !slices.Contains(systemevent.Actions, action) {
		return "", fmt.Errorf("no bound event of channel %q", ch)
	}
	return action, nil
}
