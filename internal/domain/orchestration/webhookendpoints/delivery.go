package webhookendpoints

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// An endpoint disables on 410, or when a failure run reaches both bounds (ADR 0039).
const (
	DisableFailures = 20
	DisableWindow   = 2 * time.Hour
)

// DeliverInput is one attempt at a queued delivery: what to send and the queue's attempt data.
type DeliverInput struct {
	EventName, DeliveryID string
	Body                  []byte
	Attempt               httpcall.Attempt
}

// Deliver makes one attempt at a queued delivery to e, the endpoint loaded by the job, through
// the module's HTTP client: it sends the signed body, records the outcome on the endpoint's health
// (RecordDelivery, which may disable it and tell its owners) and decides what the job does next by
// the Subscription policy. A disabled endpoint receives nothing. A receiver that never lets up
// cancels the delivery; the last attempt's failure is left to end the job as exhausted.
func (m *Module) Deliver(ctx context.Context, s *ent.Scoped, e *ent.WebhookEndpoint, in DeliverInput) (httpcall.Execution, error) {
	to, ok, err := m.DeliveryTo(e)
	if err != nil || !ok {
		return httpcall.Execution{}, err
	}
	sendErr := webhook.Send(ctx, m.client, to.URL, to.Secret, in.EventName, in.DeliveryID, in.Body, m.now())
	if err := m.RecordDelivery(ctx, s, e, sendErr, m.now()); err != nil {
		return httpcall.Execution{}, err
	}
	if sendErr == nil {
		return httpcall.Execution{}, nil
	}
	decision := httpcall.Subscription.Decide(sendErr, in.Attempt, m.now())
	switch {
	case decision.Outcome == httpcall.Wait:
		return httpcall.Execution{Wait: decision.Wait}, nil
	case decision.Outcome == httpcall.GiveUp && decision.Reason != httpcall.AttemptsExhausted:
		return httpcall.Execution{Cancel: sendErr}, nil
	}
	return httpcall.Execution{Retry: sendErr}, nil
}

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
			machine := HealthMachine(before, change, m.deliveryStorage(ts, pub, before, change))
			return machine.FireCtx(ctx, change.Trigger)
		})
	})
}

// deliveryStorage is the health storage of a delivery outcome, joining RecordDelivery's
// transaction: the first conditional update holds the row through automatic disabling, its audited
// write and the publication of its notification event.
func (m *Module) deliveryStorage(ts *ent.Scoped, pub events.Publisher, before *ent.WebhookEndpoint, change HealthChange) lifecycle.Storage[HealthState] {
	return lifecycle.Storage[HealthState]{Write: func(ctx context.Context, _, to HealthState) (bool, error) {
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
		// The first conditional update holds this row until the transaction ends. Audit only the
		// disable, not the preceding data-plane failure counters.
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
	}}
}

// originOf keeps scheme and host of an endpoint URL for a mail: path and query may carry a secret.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}
