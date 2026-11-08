package webhookendpoints

import (
	"context"
	"net/http"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// HealthState is derived from the endpoint's existing disable moment and failure run.
// No second status column duplicates those facts.
type HealthState string

const (
	Healthy  HealthState = "healthy"
	Failing  HealthState = "failing"
	Disabled HealthState = "disabled"
)

// HealthTrigger is a delivery outcome or the owner's explicit control of the endpoint.
type HealthTrigger string

const (
	Succeed   HealthTrigger = "succeed"
	Fail      HealthTrigger = "fail"
	RateLimit HealthTrigger = "rate_limit"
	Enable    HealthTrigger = "enable"
	Disable   HealthTrigger = "disable"
)

// HealthChange describes one operation. Fail carries its delivery error; At dates its run.
type HealthChange struct {
	Trigger HealthTrigger
	Failure *webhook.DeliveryError
	At      time.Time
}

func healthState(e *ent.WebhookEndpoint) HealthState {
	if e.DisabledAt != nil {
		return Disabled
	}
	if e.ConsecutiveFailures != nil && *e.ConsecutiveFailures > 0 {
		return Failing
	}
	return Healthy
}

// HealthMachine configures one change against a freshly loaded endpoint. Fire change.Trigger
// once, within the transaction that owns write. Internal transitions explicitly invoke write:
// stateless does not invoke its external storage mutator for them.
func HealthMachine(before *ent.WebhookEndpoint, change HealthChange, write func(context.Context, HealthState) (bool, error)) *stateless.StateMachine {
	current := healthState(before)
	persist := func(ctx context.Context, to HealthState) (bool, error) {
		swapped, err := write(ctx, to)
		if err == nil && swapped {
			current = to
		}
		return swapped, err
	}
	m := lifecycle.New(lifecycle.Storage[HealthState]{
		Read: func(context.Context) (HealthState, error) { return current, nil },
		Write: func(ctx context.Context, _, to HealthState) (bool, error) {
			return persist(ctx, to)
		},
	})
	internal := func(ctx context.Context, _ ...any) error {
		swapped, err := persist(ctx, current)
		if err != nil {
			return err
		}
		if !swapped {
			return lifecycle.ErrInvalidTransition
		}
		return nil
	}
	when := func(allowed bool) stateless.GuardFunc {
		return func(context.Context, ...any) bool { return allowed }
	}
	healthy, failing, disabled := m.Configure(Healthy), m.Configure(Failing), m.Configure(Disabled)
	switch change.Trigger {
	case Succeed:
		healthy.InternalTransition(Succeed, internal)
		failing.Permit(Succeed, Healthy)
		disabled.Ignore(Succeed)
	case Fail:
		_, retires := disableReason(before, change.Failure, change.At)
		valid := when(change.Failure != nil)
		healthy.Permit(Fail, Disabled, valid, when(retires))
		healthy.Permit(Fail, Failing, valid, when(!retires))
		failing.Permit(Fail, Disabled, valid, when(retires))
		failing.InternalTransition(Fail, internal, valid, when(!retires))
		disabled.Ignore(Fail)
	case RateLimit:
		healthy.Ignore(RateLimit)
		failing.Ignore(RateLimit)
		disabled.Ignore(RateLimit)
	case Enable:
		healthy.InternalTransition(Enable, internal)
		failing.Permit(Enable, Healthy)
		disabled.Permit(Enable, Healthy)
	case Disable:
		healthy.Permit(Disable, Disabled)
		failing.Permit(Disable, Disabled)
		// A repeated owner disable may edit the URL/filter, but preserves its moment and cause.
		disabled.InternalTransition(Disable, internal)
	}
	return m
}

// disableReason decides from the current run plus this next failure, never from the snapshot
// that preceded the HTTP request. A 410 retires immediately; otherwise both bounds must hold.
func disableReason(e *ent.WebhookEndpoint, failure *webhook.DeliveryError, now time.Time) (webhookendpoint.DisabledReason, bool) {
	if failure == nil {
		return "", false
	}
	if failure.Kind == webhook.KindHTTPStatus && failure.Status == http.StatusGone {
		return webhookendpoint.DisabledReasonGone, true
	}
	failures := 1
	if e.ConsecutiveFailures != nil {
		failures += *e.ConsecutiveFailures
	}
	if failures >= DisableFailures && e.FailingSince != nil && now.Sub(*e.FailingSince) >= DisableWindow {
		return webhookendpoint.DisabledReasonPersistentFailure, true
	}
	return "", false
}
