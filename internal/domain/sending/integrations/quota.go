package integrations

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// The provider's answer to a send quota lookup is stored on the Integration's row (ADR 0067):
// internal/domain/sending/providerlimit.RefreshQuota asks the provider and calls these.

// RecordQuotaUnavailable stores that the lookup for the Integration id failed: the moment it was
// checked, and the moment the lookup first failed, which a second failure keeps. The last known
// provider values and standing stay.
func RecordQuotaUnavailable(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	if err := s.Integration().UpdateOneID(id).SetProviderQuotaCheckedAt(at).Exec(ctx); err != nil {
		return err
	}
	return markQuotaFailed(ctx, s, id, at)
}

// RecordQuota stores the quota and standing the provider reported for the Integration id, with
// the moment it was checked, and ends a recorded failure.
func RecordQuota(ctx context.Context, s *ent.Scoped, id int64, quota messaging.Quota, at time.Time) error {
	upd := s.Integration().UpdateOneID(id).SetProviderQuotaCheckedAt(at)
	upd.SetNillableProviderMaxPerSecond(quota.PerSecond).
		SetNillableProviderMaxPerDay(quota.PerDay)
	if quota.PerSecond == nil {
		upd.ClearProviderMaxPerSecond()
	}
	if quota.PerDay == nil {
		upd.ClearProviderMaxPerDay()
	}
	if quota.Enforcement == providers.EnforcementNone {
		upd.ClearProviderEnforcement()
	} else {
		upd.SetProviderEnforcement(quota.Enforcement)
	}
	upd.SetProviderSandbox(quota.Sandbox)
	if err := upd.Exec(ctx); err != nil {
		return err
	}
	return clearQuotaFailed(ctx, s, id)
}

// markQuotaFailed records the moment the lookup first failed: it writes only while no failure is
// recorded, so a second failed lookup keeps the first moment.
func markQuotaFailed(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	_, err := s.Integration().Update().
		Where(integration.ID(id), integration.ProviderQuotaFailedAtIsNil()).
		SetProviderQuotaFailedAt(at).
		Save(ctx)
	return err
}

// clearQuotaFailed ends a failure after a successful lookup; it writes only while one is recorded.
func clearQuotaFailed(ctx context.Context, s *ent.Scoped, id int64) error {
	_, err := s.Integration().Update().
		Where(integration.ID(id), integration.ProviderQuotaFailedAtNotNil()).
		ClearProviderQuotaFailedAt().
		Save(ctx)
	return err
}

// RecordRefused stores, at, that the provider refused the Integration's credentials
// (ProviderRefusedAt), which holds the Integration's Send route: Outbound send records it.
func RecordRefused(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	return s.Integration().UpdateOneID(id).SetProviderRefusedAt(at).Exec(ctx)
}
