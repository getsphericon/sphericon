package integrations

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// quotaTimeout bounds one provider quota lookup, so a hung endpoint cannot stall a
// save or a job.
const quotaTimeout = 15 * time.Second

// DueForQuotaRefresh returns up to limit Integrations whose provider quota is due a read, each
// with only its id and its Workspace (the lookup job names both), least-recently-checked first
// with NULLS FIRST, so a never-checked Integration is reached before already-checked ones. Only
// SES reports a quota; SMTP has none. A deleted Integration is not kept working (ADR 0099): nobody
// sends with its credential. The read crosses Workspaces, so it takes the job's raw client.
func DueForQuotaRefresh(ctx context.Context, client *ent.Client, limit int) ([]*ent.Integration, error) {
	return client.Integration.Query().
		Where(integration.ProviderEQ(providers.SES), integration.DiscardedAtIsNil()).
		Order(integration.ByProviderQuotaCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		Select(integration.FieldID, integration.FieldWorkspaceID).
		All(ctx)
}

// RefreshQuota asks the Integration's provider for its send quota and the account's
// standing and stores the answer on the Integration's row (ADR 0067): providerlimit.EffectiveOf
// folds the quota into the ceiling by the minimum rule, and a stopped account holds the
// Integration's sends. It runs after the Integration is saved and hourly from the periodic job,
// because SES quotas grow as an account matures and an account is stopped and resumed without
// notice.
//
// A failed lookup (no ses:GetAccount permission, an SES-compatible service such as
// Postbox, a timeout) never blocks sending and is not returned: it is recorded as
// the moment it first failed (provider_quota_failed_at), which the UI shows as a warning, and the last known
// provider values and standing stay, so a transient blip neither lifts a ceiling nor
// sends into a stopped account. A later success clears the warning. Providers that
// cannot report a quota (SMTP) are left untouched.
// An error is returned only when the stored config cannot be read or the row cannot
// be written.
func (m *Module) RefreshQuota(ctx context.Context, s *ent.Scoped, integ *ent.Integration) error {
	reader, ok, err := m.quotaReader(integ)
	if err != nil || !ok {
		return err
	}

	lookupCtx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	quota, lookupErr := reader.SendQuota(lookupCtx)

	now := m.now()
	if lookupErr != nil {
		slog.WarnContext(ctx, "provider send quota unavailable",
			"integration_id", integ.ID, "workspace_id", integ.WorkspaceID, "err", lookupErr)
		return recordQuotaUnavailable(ctx, s, integ.ID, now)
	}
	return recordQuota(ctx, s, integ.ID, quota, now)
}

// quotaReader is the sender of the Integration's provider when it can say what it allows (a
// messaging.QuotaReader); ok is false for a provider that cannot. An error is a stored config that
// cannot be read or built from.
func (m *Module) quotaReader(integ *ent.Integration) (reader messaging.QuotaReader, ok bool, err error) {
	if integ.Channel != channels.Email {
		return nil, false, nil
	}
	config, err := m.cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return nil, false, fmt.Errorf("decrypt integration %d config: %w", integ.ID, err)
	}
	built, err := m.catalog.BuildEmail(integ.Provider, config, messaging.Deps{})
	if err != nil {
		return nil, false, err
	}
	reader, ok = messaging.AsQuotaReader(built)
	return reader, ok, nil
}

// recordQuotaUnavailable stores that the lookup for the Integration id failed: the moment it was
// checked, and the moment the lookup first failed, which a second failure keeps. The last known
// provider values and standing stay.
func recordQuotaUnavailable(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	if err := s.Integration().UpdateOneID(id).SetProviderQuotaCheckedAt(at).Exec(ctx); err != nil {
		return err
	}
	return markQuotaFailed(ctx, s, id, at)
}

// recordQuota stores the quota and standing the provider reported for the Integration id, with
// the moment it was checked, and ends a recorded failure.
func recordQuota(ctx context.Context, s *ent.Scoped, id int64, quota messaging.Quota, at time.Time) error {
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
