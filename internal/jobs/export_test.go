package jobs

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// Worker constructors for the external jobs_test package: Work is exercised
// directly (no river runtime), with the dependencies each worker would carry.

func NewWelcomeWorker(sender messaging.EmailSender) *SendWelcomeWorker {
	return &SendWelcomeWorker{sender: sender}
}

func NewAuthMailWorker(sender messaging.EmailSender, appURL string) *SendAuthMailWorker {
	return &SendAuthMailWorker{sender: sender, appURL: appURL}
}

func NewMemberInviteWorker(sender messaging.EmailSender) *SendMemberInviteWorker {
	return &SendMemberInviteWorker{sender: sender}
}

func NewDeliverWebhookWorker(client *ent.Client, cipher *secrets.Cipher, doer webhook.Doer) *DeliverWebhookWorker {
	return &DeliverWebhookWorker{ent: client, cipher: cipher, client: doer}
}

func NewVerifySendingDomainWorker(client *ent.Client, lookup sending.TXTLookup, sender messaging.EmailSender) *VerifySendingDomainWorker {
	return &VerifySendingDomainWorker{ent: client, lookup: lookup, sender: sender}
}

func NewRecheckSendingDomainsWorker(client *ent.Client) *RecheckSendingDomainsWorker {
	return &RecheckSendingDomainsWorker{ent: client}
}

func NewRefreshSendQuotasWorker(client *ent.Client) *RefreshSendQuotasWorker {
	return &RefreshSendQuotasWorker{ent: client}
}

func NewRefreshIntegrationQuotaWorker(client *ent.Client, cipher *secrets.Cipher, catalog *messaging.Catalog) *RefreshIntegrationQuotaWorker {
	return &RefreshIntegrationQuotaWorker{ent: client, cipher: cipher, catalog: catalog}
}

func NewPruneOutboxWorker(client *ent.Client, floor time.Duration) *PruneOutboxWorker {
	return &PruneOutboxWorker{ent: client, floor: floor}
}

func NewPruneUnverifiedUsersWorker(client *ent.Client, retention time.Duration) *PruneUnverifiedUsersWorker {
	return &PruneUnverifiedUsersWorker{ent: client, retention: retention}
}

func NewPruneEventsWorker(client *ent.Client, retention time.Duration) *PruneEventsWorker {
	return &PruneEventsWorker{ent: client, retention: retention}
}

func NewEvaluateTriggerWorker(client *ent.Client) *EvaluateTriggerWorker {
	return &EvaluateTriggerWorker{ent: client}
}

func NewRunStepWorker(client *ent.Client, mod *outbound.Module) *RunStepWorker {
	return &RunStepWorker{ent: client, mod: mod}
}

func NewSendBroadcastWorker(client *ent.Client, mod *outbound.Module) *SendBroadcastWorker {
	return &SendBroadcastWorker{ent: client, mod: mod}
}

func NewSendRecipientWorker(client *ent.Client, mod *outbound.Module) *SendRecipientWorker {
	return &SendRecipientWorker{ent: client, mod: mod}
}

// River exposes the wrapped client so tests can build a worker context.
func (c *Client) River() *river.Client[pgx.Tx] { return c.river }

// NewErrorHandler builds river's error sink over logger.
func NewErrorHandler(logger *slog.Logger) river.ErrorHandler { return &errorHandler{logger: logger} }

func NewPurgeAuthAttemptsWorker(client *ent.Client) *PurgeAuthAttemptsWorker {
	return &PurgeAuthAttemptsWorker{ent: client}
}
