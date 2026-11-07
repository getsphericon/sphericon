package providerlimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// quotaTimeout bounds one provider quota lookup, so a hung endpoint cannot stall a
// save or a job.
const quotaTimeout = 15 * time.Second

// RefreshQuota asks the Integration's provider for its send quota and the account's
// standing and stores the answer: EffectiveOf folds the quota into the ceiling by the
// minimum rule, and a stopped account holds the Integration's sends (ADR 0067). It runs
// when the Integration is saved and hourly from the periodic job, because SES quotas
// grow as an account matures and an account is stopped and resumed without notice.
//
// A failed lookup (no ses:GetAccount permission, an SES-compatible service such as
// Postbox, a timeout) never blocks sending and is not returned: it is recorded as
// the moment it first failed (provider_quota_failed_at), which the UI shows as a warning, and the last known
// provider values and standing stay, so a transient blip neither lifts a ceiling nor
// sends into a stopped account. A later success clears the warning. Providers that
// cannot report a quota (SMTP) are left untouched.
// An error is returned only when the stored config cannot be read or the row cannot
// be written.
func RefreshQuota(ctx context.Context, s *ent.Scoped, cipher *secrets.Cipher, catalog *messaging.Catalog, integ *ent.Integration, now time.Time) error {
	config, err := cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt integration %d config: %w", integ.ID, err)
	}
	built, err := catalog.BuildEmail(integ.Provider, config, messaging.Deps{})
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

	if lookupErr != nil {
		slog.WarnContext(ctx, "provider send quota unavailable",
			"integration_id", integ.ID, "workspace_id", integ.WorkspaceID, "err", lookupErr)
		return integrations.RecordQuotaUnavailable(ctx, s, integ.ID, now)
	}
	return integrations.RecordQuota(ctx, s, integ.ID, quota, now)
}
