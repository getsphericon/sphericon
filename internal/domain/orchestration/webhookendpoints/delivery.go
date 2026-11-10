package webhookendpoints

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// An endpoint disables on 410, or when a failure run reaches both bounds (ADR 0039).
const (
	DisableFailures = 20
	DisableWindow   = 2 * time.Hour
)

// RecordDelivery applies an outcome to the current failure run, in commit order. e is the
// snapshot loaded before the HTTP request: only its id and reset generation are authoritative
// here. Results from before a manual enable, or for a disabled/deleted endpoint, are stale.
// Health writes, automatic disabling, its audit and notification event commit together.
func (m *Module) RecordDelivery(ctx context.Context, s *ent.Scoped, e *ent.WebhookEndpoint, sendErr error, now time.Time) error {
	change := HealthChange{Trigger: Succeed, At: now}
	if sendErr != nil {
		if !errors.As(sendErr, &change.Failure) {
			return nil // not a delivery outcome (for example, an unusable secret)
		}
		change.Trigger = Fail
		if webhook.RateLimited(sendErr) != nil {
			change.Trigger = RateLimit
		}
	}
	return retryHealth(ctx, func() error {
		return m.bus.WithinScopedTx(ctx, events.Unaudited(s), func(ts *ent.Scoped, pub events.Publisher) error {
			before, err := ts.WebhookEndpoint().Get(ctx, e.ID)
			if ent.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if generation(before) != generation(e) || before.DisabledAt != nil {
				return nil
			}
			// The first conditional update holds the row through automatic disabling, its audited
			// write and the publication of its notification event.
			machine := HealthMachine(before, change, lifecycle.Storage[HealthState]{Write: func(ctx context.Context, _, to HealthState) (bool, error) {
				u := ts.WebhookEndpoint().UpdateOneID(before.ID).Where(atHealth(before)...)
				if change.Trigger == Succeed {
					clearFailures(u)
				} else {
					u.AddConsecutiveFailures(1).SetLastErrorKind(change.Failure.Kind).SetLastFailureAt(change.At)
					if before.FailingSince == nil {
						u.SetFailingSince(change.At)
					}
					if change.Failure.Kind == webhook.KindHTTPStatus {
						u.SetLastErrorStatus(change.Failure.Status)
					} else {
						u.ClearLastErrorStatus()
					}
				}
				err := u.Exec(ctx)
				if ent.IsNotFound(err) {
					return false, errHealthConflict
				}
				if err != nil {
					return false, err
				}
				if to != Disabled {
					return true, nil
				}
				reason, _ := disableReason(before, change.Failure, change.At)
				sys := m.bus.Act(ts, events.Actor{Kind: events.ActorSystem})
				// The first conditional update holds this row until the transaction ends. Audit
				// only the disable, not the preceding data-plane failure counters.
				err = sys.WebhookEndpoint().UpdateOneID(before.ID).
					Where(webhookendpoint.DisabledAtIsNil()).
					SetDisabledAt(change.At).SetDisabledReason(reason).Exec(ctx)
				if ent.IsNotFound(err) {
					return false, errHealthConflict
				}
				if err != nil {
					return false, err
				}
				err = pub.Publish(ctx, &events.WebhookEndpointDisabled{
					WorkspaceID: before.WorkspaceID, EndpointID: before.ID,
					Origin: originOf(before.URL), Reason: string(reason),
				})
				return err == nil, err
			}})
			return machine.FireCtx(ctx, change.Trigger)
		})
	})
}

// originOf keeps scheme and host of an endpoint URL for a mail: path and query may carry a secret.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}
