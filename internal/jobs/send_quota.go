package jobs

import (
	"context"
	stdsql "database/sql"

	"entgo.io/ent/dialect/sql"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sendlimit"
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
	ids, err := IntegrationsDueForQuotaRefresh(ctx, w.ent, quotaRefreshBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, RefreshIntegrationQuotaArgs{IntegrationID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// IntegrationsDueForQuotaRefresh returns up to limit SES Integration ids,
// least-recently-checked first with NULLS FIRST, so a never-checked Integration is
// reached before already-checked ones. Only SES reports a quota; SMTP has none.
func IntegrationsDueForQuotaRefresh(ctx context.Context, client *ent.Client, limit int) ([]int64, error) {
	return client.Integration.Query().
		Where(integration.ProviderEQ(integration.ProviderSes)).
		Order(integration.ByProviderQuotaCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		IDs(ctx)
}

type RefreshIntegrationQuotaArgs struct {
	IntegrationID int64 `json:"integration_id"`
}

func (RefreshIntegrationQuotaArgs) Kind() string { return "integration_quota_refresh" }

type RefreshIntegrationQuotaWorker struct {
	river.WorkerDefaults[RefreshIntegrationQuotaArgs]

	ent     *ent.Client
	cipher  *secrets.Cipher
	catalog *messaging.Catalog
}

// NewRefreshIntegrationQuotaWorker builds the worker that reads one Integration's provider send quota.
func NewRefreshIntegrationQuotaWorker(client *ent.Client, cipher *secrets.Cipher, catalog *messaging.Catalog) *RefreshIntegrationQuotaWorker {
	return &RefreshIntegrationQuotaWorker{ent: client, cipher: cipher, catalog: catalog}
}

func (w *RefreshIntegrationQuotaWorker) Work(ctx context.Context, job *river.Job[RefreshIntegrationQuotaArgs]) error {
	return RefreshIntegrationQuotaByID(ctx, w.ent, w.cipher, w.catalog, job.Args.IntegrationID)
}

// RefreshIntegrationQuotaByID reads one Integration's provider quota and stores it
// (see sendlimit.RefreshQuota). The Integration may have been deleted since the
// tick queued it, which is nothing to do. Pure of the queue so tests call it directly.
func RefreshIntegrationQuotaByID(ctx context.Context, client *ent.Client, cipher *secrets.Cipher, catalog *messaging.Catalog, id int64) error {
	row, err := client.Integration.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return sendlimit.RefreshQuota(ctx, client.Scoped(row.WorkspaceID), cipher, catalog, row)
}
