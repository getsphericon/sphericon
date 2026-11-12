package webhookendpoints

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
)

// updateHealth owns an explicit enable/disable and any URL/filter edit made with it. Enable
// increments the durable generation even when already healthy: it is an explicit reset.
func (m *Module) updateHealth(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput, enabled bool) (Endpoint, error) {
	change := HealthChange{Trigger: Disable, At: m.now()}
	if enabled {
		change.Trigger = Enable
	}
	var saved *ent.WebhookEndpoint
	err := retryHealth(ctx, func() error {
		return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
			before, err := ts.WebhookEndpoint().Get(ctx, id)
			if err != nil {
				return err
			}
			// Refused here, not retried: a stale revision stays stale (ADR 0119).
			if before.Revision != in.Revision {
				return ErrRevisionChanged
			}
			machine := HealthMachine(before, change, lifecycle.Storage[HealthState]{Write: func(ctx context.Context, _, _ HealthState) (bool, error) {
				u := ts.WebhookEndpoint().UpdateOneID(id).Where(append(atHealth(before), webhookendpoint.Revision(before.Revision))...)
				patch.ApplySet(in.URL, u.SetURL)
				patch.ApplySet(in.EventTypes, u.SetEventTypes)
				if enabled {
					clearFailures(u)
					u.ClearDisabledAt().ClearDisabledReason().AddHealthGeneration(1)
				} else if before.DisabledAt == nil {
					u.Where(webhookendpoint.DisabledAtIsNil()).
						SetDisabledAt(change.At).SetDisabledReason(webhookendpoint.DisabledReasonOwner)
				}
				saved, err = u.Save(ctx)
				if ent.IsNotFound(err) {
					return false, errHealthConflict
				}
				return err == nil, err
			}})
			return machine.FireCtx(ctx, change.Trigger)
		})
	})
	if err != nil {
		return Endpoint{}, missing(err)
	}
	return m.open(saved)
}
