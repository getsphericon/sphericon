package sendlimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// quotaTimeout bounds one provider quota lookup, so a hung endpoint cannot stall a
// save or a job.
const quotaTimeout = 15 * time.Second

// RefreshQuota asks the Integration's provider for its send quota and stores the
// answer, which EffectiveOf then folds into the ceiling by the minimum rule. It runs
// when the Integration is saved and hourly from the periodic job, because SES quotas
// grow as an account matures.
//
// A failed lookup (no ses:GetSendQuota permission, an SES-compatible service such as
// Postbox, a timeout) never blocks sending and is not returned: it is recorded as
// the moment it first failed (provider_quota_failed_at), which the UI shows as a warning, and the last known
// provider values stay so a transient blip does not lift a ceiling. A later success
// clears the warning. Providers that cannot report a quota (SMTP) are left untouched.
// An error is returned only when the stored config cannot be read or the row cannot
// be written.
func RefreshQuota(ctx context.Context, s *ent.Scoped, cipher *secrets.Cipher, catalog *messaging.Catalog, integ *ent.Integration) error {
	config, err := cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt integration %d config: %w", integ.ID, err)
	}
	built, err := catalog.BuildEmail(messaging.Provider(integ.Provider), config, messaging.Deps{})
	if err != nil {
		return err
	}
	reader, ok := messaging.AsQuotaReader(built)
	if !ok {
		return nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	quota, lookupErr := reader.SendQuota(lookupCtx)

	upd := s.Integration().UpdateOneID(integ.ID).SetProviderQuotaCheckedAt(time.Now())
	if lookupErr != nil {
		slog.WarnContext(ctx, "provider send quota unavailable",
			"integration_id", integ.ID, "workspace_id", integ.WorkspaceID, "err", lookupErr)
		if err := upd.Exec(ctx); err != nil {
			return err
		}
		return markQuotaFailed(ctx, s, integ.ID)
	}
	upd.SetNillableProviderMaxPerSecond(quota.PerSecond).
		SetNillableProviderMaxPerDay(quota.PerDay)
	if quota.PerSecond == nil {
		upd.ClearProviderMaxPerSecond()
	}
	if quota.PerDay == nil {
		upd.ClearProviderMaxPerDay()
	}
	if err := upd.Exec(ctx); err != nil {
		return err
	}
	return clearQuotaFailed(ctx, s, integ.ID)
}

// markQuotaFailed records the moment the lookup first failed: it writes only while no failure is
// recorded, so a second failed lookup keeps the first moment.
func markQuotaFailed(ctx context.Context, s *ent.Scoped, id int64) error {
	_, err := s.Integration().Update().
		Where(integration.ID(id), integration.ProviderQuotaFailedAtIsNil()).
		SetProviderQuotaFailedAt(time.Now()).
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
