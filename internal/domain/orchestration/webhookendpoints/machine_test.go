package webhookendpoints_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The machine's decisions are pure; database tests exercise its conditional writer separately.
func applyHealth(t *rapid.T, before *ent.WebhookEndpoint, change webhookendpoints.HealthChange) webhookendpoints.HealthState {
	t.Helper()
	var stored webhookendpoints.HealthState
	machine := webhookendpoints.HealthMachine(before, change, lifecycle.Storage[webhookendpoints.HealthState]{Write: func(_ context.Context, _, to webhookendpoints.HealthState) (bool, error) {
		stored = to
		return true, nil
	}})
	err := machine.FireCtx(t.Context(), change.Trigger)
	require.NoError(t, err)
	return stored
}

func failingEndpoint(count int, began time.Time) *ent.WebhookEndpoint {
	return &ent.WebhookEndpoint{ConsecutiveFailures: &count, FailingSince: &began}
}

func TestPersistentFailureNeedsBothTheCountAndWindow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
		count := rapid.IntRange(webhookendpoints.DisableFailures-1, 1000).Draw(t, "failures before next outcome")
		age := rapid.Int64Range(int64(webhookendpoints.DisableWindow), int64(24*time.Hour)).Draw(t, "series age")
		before := failingEndpoint(count, now.Add(-time.Duration(age)))

		stored := applyHealth(t, before, webhookendpoints.HealthChange{Trigger: webhookendpoints.Fail,
			Failure: &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: http.StatusServiceUnavailable}, At: now})

		assert.Equal(t, webhookendpoints.Disabled, stored)
	})
}

func TestAYoungFailureSeriesNeverDisablesForItsCountAlone(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
		count := rapid.IntRange(1, 1000).Draw(t, "failures")
		age := rapid.Int64Range(0, int64(webhookendpoints.DisableWindow)-1).Draw(t, "series age")
		before := failingEndpoint(count, now.Add(-time.Duration(age)))

		stored := applyHealth(t, before, webhookendpoints.HealthChange{Trigger: webhookendpoints.Fail,
			Failure: &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: http.StatusServiceUnavailable}, At: now})

		assert.Equal(t, webhookendpoints.Failing, stored)
	})
}

func TestAnOldFailureSeriesNeverDisablesBelowTheCount(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
		count := rapid.IntRange(1, webhookendpoints.DisableFailures-2).Draw(t, "failures before next outcome")
		age := rapid.Int64Range(0, int64(365*24*time.Hour)).Draw(t, "series age")
		before := failingEndpoint(count, now.Add(-time.Duration(age)))

		stored := applyHealth(t, before, webhookendpoints.HealthChange{Trigger: webhookendpoints.Fail,
			Failure: &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: http.StatusServiceUnavailable}, At: now})

		assert.Equal(t, webhookendpoints.Failing, stored)
	})
}

func TestGoneDisablesRegardlessOfTheFailureSeries(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
		count := rapid.IntRange(0, 1000).Draw(t, "failures")
		age := rapid.Int64Range(0, int64(24*time.Hour)).Draw(t, "series age")
		before := failingEndpoint(count, now.Add(-time.Duration(age)))

		stored := applyHealth(t, before, webhookendpoints.HealthChange{Trigger: webhookendpoints.Fail,
			Failure: &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: http.StatusGone}, At: now})

		assert.Equal(t, webhookendpoints.Disabled, stored)
	})
}

func TestOwnerEnableAlwaysReturnsToHealthy(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
		count := rapid.IntRange(1, 1000).Draw(t, "failures")
		before := rapid.SampledFrom([]*ent.WebhookEndpoint{
			{}, failingEndpoint(count, now), {DisabledAt: &now},
		}).Draw(t, "owner's starting state")

		stored := applyHealth(t, before, webhookendpoints.HealthChange{Trigger: webhookendpoints.Enable, At: now})

		assert.Equal(t, webhookendpoints.Healthy, stored)
	})
}

func TestInternalHealthChangesStillRequireTheConditionalWrite(t *testing.T) {
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		before *ent.WebhookEndpoint
		change webhookendpoints.HealthChange
	}{
		{"healthy success", &ent.WebhookEndpoint{}, webhookendpoints.HealthChange{Trigger: webhookendpoints.Succeed}},
		{"owner reset", &ent.WebhookEndpoint{}, webhookendpoints.HealthChange{Trigger: webhookendpoints.Enable}},
		{"another failure", failingEndpoint(1, now), webhookendpoints.HealthChange{Trigger: webhookendpoints.Fail,
			Failure: &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: http.StatusServiceUnavailable}, At: now}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := webhookendpoints.HealthMachine(tc.before, tc.change, lifecycle.Storage[webhookendpoints.HealthState]{Write: func(context.Context, webhookendpoints.HealthState, webhookendpoints.HealthState) (bool, error) {
				return false, nil
			}})

			err := machine.FireCtx(t.Context(), tc.change.Trigger)

			require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		})
	}
}
