package jobs

import (
	"context"
	stdsql "database/sql"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// quotaRefreshBatchSize bounds how many Integrations one hourly tick refreshes, like
// recheckBatchSize does for Sending domains; least-recently-checked go first.
const quotaRefreshBatchSize = 100

// --- hourly refresh of every SES Integration's provider quota (ADR 0023) ---

type RefreshSendQuotasArgs struct{}

func (RefreshSendQuotasArgs) Kind() string { return "send_quota_refresh" }

type RefreshSendQuotasWorker struct {
	river.WorkerDefaults[RefreshSendQuotasArgs]

	ent *ent.Client
}

// NewRefreshSendQuotasWorker builds the worker that queues a quota read of every Integration due one.
func NewRefreshSendQuotasWorker(client *ent.Client) *RefreshSendQuotasWorker {
	return &RefreshSendQuotasWorker{ent: client}
}

// Work fans the hourly tick out into one RefreshIntegrationQuota job per Integration,
// so each lookup has its own isolation and retry (the Sending-domain recheck pattern).
func (w *RefreshSendQuotasWorker) Work(ctx context.Context, _ *river.Job[RefreshSendQuotasArgs]) error {
	due, err := IntegrationsDueForQuotaRefresh(ctx, w.ent, quotaRefreshBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, row := range due {
		args := integrations.RefreshQuotaArgs{WorkspaceID: row.WorkspaceID, IntegrationID: row.ID}
		if _, err := rc.Insert(ctx, args, nil); err != nil {
			return err
		}
	}
	return nil
}

// IntegrationsDueForQuotaRefresh returns up to limit SES Integrations, each with only its id and
// its Workspace (the lookup job names both), least-recently-checked first with NULLS FIRST, so a never-checked Integration is
// reached before already-checked ones. Only SES reports a quota; SMTP has none. A deleted
// Integration is not kept working (ADR 0099): nobody sends with its credential.
func IntegrationsDueForQuotaRefresh(ctx context.Context, client *ent.Client, limit int) ([]*ent.Integration, error) {
	return client.Integration.Query().
		Where(integration.ProviderEQ(providers.SES), integration.DiscardedAtIsNil()).
		Order(integration.ByProviderQuotaCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		Select(integration.FieldID, integration.FieldWorkspaceID).
		All(ctx)
}

type RefreshIntegrationQuotaWorker struct {
	river.WorkerDefaults[integrations.RefreshQuotaArgs]

	ent     *ent.Client
	cipher  *secrets.Cipher
	catalog *messaging.Catalog
	now     func() time.Time
}

// NewRefreshIntegrationQuotaWorker builds the worker that reads one Integration's provider send quota,
// stamped by the Instance clock now.
func NewRefreshIntegrationQuotaWorker(client *ent.Client, cipher *secrets.Cipher, catalog *messaging.Catalog, now func() time.Time) *RefreshIntegrationQuotaWorker {
	return &RefreshIntegrationQuotaWorker{ent: client, cipher: cipher, catalog: catalog, now: now}
}

func (w *RefreshIntegrationQuotaWorker) Work(ctx context.Context, job *river.Job[integrations.RefreshQuotaArgs]) error {
	return RefreshIntegrationQuotaByID(ctx, w.ent, w.cipher, w.catalog, job.Args.IntegrationID, w.now())
}

// RefreshIntegrationQuotaByID reads one Integration's provider quota and stores it
// (see providerlimit.RefreshQuota). The Integration may have been deleted since the
// tick queued it, which is nothing to do: the job checks again when it runs (ADR 0099).
// Pure of the queue so tests call it directly.
func RefreshIntegrationQuotaByID(ctx context.Context, client *ent.Client, cipher *secrets.Cipher, catalog *messaging.Catalog, id int64, now time.Time) error {
	row, err := client.Integration.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.DiscardedAt != nil {
		return nil
	}
	return providerlimit.RefreshQuota(ctx, client.Scoped(row.WorkspaceID), cipher, catalog, row, now)
}
