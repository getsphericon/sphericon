package jobs

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
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

func NewDeliverWebhookWorker(client *ent.Client, cipher *secrets.Cipher, doer webhook.Doer, now func() time.Time, bus *events.Bus, sender messaging.EmailSender) *DeliverWebhookWorker {
	return &DeliverWebhookWorker{ent: client, cipher: cipher, client: doer, now: now, mod: webhooks.New(bus, cipher, nil, nil, accounts.OwnerNotifier(client, sender))}
}

func NewVerifySendingDomainWorker(mod *domainverify.Module) *VerifySendingDomainWorker {
	return &VerifySendingDomainWorker{mod: mod}
}

func NewRecheckSendingDomainsWorker(mod *domainverify.Module) *RecheckSendingDomainsWorker {
	return &RecheckSendingDomainsWorker{mod: mod}
}

func NewVerifyJoinDomainWorker(mod *joindomains.Module) *VerifyJoinDomainWorker {
	return &VerifyJoinDomainWorker{mod: mod}
}

func NewRecheckJoinDomainsWorker(mod *joindomains.Module) *RecheckJoinDomainsWorker {
	return &RecheckJoinDomainsWorker{mod: mod}
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

func NewPruneUnverifiedUsersWorker(acc *accounts.Accounts, retention time.Duration) *PruneUnverifiedUsersWorker {
	return &PruneUnverifiedUsersWorker{accounts: acc, retention: retention}
}

func NewPruneEventsWorker(client *ent.Client, retention time.Duration) *PruneEventsWorker {
	return &PruneEventsWorker{ent: client, retention: retention}
}

func NewEvaluateTriggerWorker(client *ent.Client) *EvaluateTriggerWorker {
	return &EvaluateTriggerWorker{ent: client}
}

func NewRunStepWorker(client *ent.Client, fence automations.Fencer, mod *outbound.Module) *RunStepWorker {
	return &RunStepWorker{ent: client, mod: mod, fence: fence}
}

func NewRunHTTPStepWorker(client *ent.Client, fence automations.Fencer, mod *automations.Module, doer webhook.Doer, now func() time.Time) *RunHTTPStepWorker {
	return &RunHTTPStepWorker{ent: client, mod: mod, client: doer, now: now, fence: fence}
}

func NewSendBroadcastWorker(client *ent.Client, mod *outbound.Module) *SendBroadcastWorker {
	return &SendBroadcastWorker{ent: client, del: broadcasts.NewDelivery(mod)}
}

func NewSendRecipientWorker(client *ent.Client, mod *outbound.Module) *SendRecipientWorker {
	return &SendRecipientWorker{ent: client, del: broadcasts.NewDelivery(mod)}
}

// River exposes the wrapped client so tests can build a worker context.
func (c *Client) River() *river.Client[*sql.Tx] { return c.river }

// NewErrorHandler builds river's error sink over logger.
func NewErrorHandler(logger *slog.Logger) river.ErrorHandler { return &errorHandler{logger: logger} }

func NewPurgeAuthAttemptsWorker(client *ent.Client) *PurgeAuthAttemptsWorker {
	return &PurgeAuthAttemptsWorker{ent: client}
}

func NewRecheckTrackingDomainsWorker(mod *domainverify.Module) *RecheckTrackingDomainsWorker {
	return &RecheckTrackingDomainsWorker{mod: mod}
}

func NewVerifyTrackingDomainWorker(mod *domainverify.Module) *VerifyTrackingDomainWorker {
	return &VerifyTrackingDomainWorker{mod: mod}
}

func NewEraseWorkspacesWorker(deletion *workspacedeletion.Module, now func() time.Time) *EraseWorkspacesWorker {
	return &EraseWorkspacesWorker{deletion: deletion, now: now}
}

// NewPrunePasskeyChallengesWorker builds the worker on the clock the test moves.
func NewPrunePasskeyChallengesWorker(client *ent.Client, now func() time.Time) *PrunePasskeyChallengesWorker {
	return &PrunePasskeyChallengesWorker{ent: client, now: now}
}
