// Package jobs is the river-backed (Postgres) async job queue. It owns the
// worker registry, the typed enqueue API the HTTP handlers call, and river's own
// schema migration (kept out of the ent diff — see Migrate).
package jobs

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/riverqueue/rivercontrib/otelriver"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/machine"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

// webhookDeliveryTimeout bounds a single outbound webhook request.
const webhookDeliveryTimeout = 15 * time.Second

// QueueWebhooks isolates outbound webhook delivery from the default queue.
// Webhook deliveries are external HTTP calls (up to webhookDeliveryTimeout each),
// so a burst of them must not starve broadcast sends and automation steps.
const QueueWebhooks = "webhooks"

// Client wraps the river client and exposes the typed enqueue methods the API
// handlers use, so callers don't depend on river directly.
type Client struct {
	river *river.Client[*sql.Tx]
	ent   *ent.Client
	// appURL builds the links of account emails (rehearsed resets render one).
	appURL string
}

// Extension plugs extra workers and periodic jobs into the client without core
// importing them: the composition root hands in the Enterprise Edition's (ADR 0014).
type Extension struct {
	Workers      func(*river.Workers)
	PeriodicJobs []*river.PeriodicJob
}

// Retention groups the data-retention settings of the prune jobs.
type Retention struct {
	// OutboxFloor is the minimum age of a pruned outbox row.
	OutboxFloor time.Duration
	// Events is the age past which analytical Events are deleted (0 disables).
	Events time.Duration
	// UnverifiedUsers is the age past which an Unverified User with no Membership is
	// deleted (0 disables; ADR 0030).
	UnverifiedUsers time.Duration
}

// NewClient builds the river client with all workers registered. Workers carry
// their own dependencies (ent client, sender resolver, secrets cipher, the
// platform system sender). appURL is the public origin used to build the links
// in account emails (reset/verify/change). outboundAllow is the Instance's internal allowlist for
// webhook deliveries (ADR 0039). The instance-wide outbox and Event prunes
// run on entClient, the raw client; retention carries the prune settings. ext plugs in the
// Enterprise Edition's workers and periodic jobs.
func NewClient(riverDB *sql.DB, entClient *ent.Client, mod *outbound.Module, cipher *secrets.Cipher, bus *events.Bus, systemSender messaging.EmailSender, lookup sending.TXTLookup, tracking *trackingdomain.Verifier, catalog *messaging.Catalog, appURL string, outboundAllow webhook.Allowlist, ranges *machine.Detector, retention Retention, ext ...Extension) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &SendBroadcastWorker{ent: entClient, del: broadcasts.NewDelivery(mod)})
	river.AddWorker(workers, &SendRecipientWorker{ent: entClient, del: broadcasts.NewDelivery(mod)})
	stepper := stepping.New(bus, mod, automations.FenceOn(entClient))
	river.AddWorker(workers, &EvaluateTriggerWorker{ent: entClient, bus: bus})
	river.AddWorker(workers, NewRunStepWorker(entClient, stepper))
	ruleMod := rules.New(cipher, bus)
	river.AddWorker(workers, &EvaluateRulesWorker{ent: entClient, mod: ruleMod})
	river.AddWorker(workers, &RunRuleActionsWorker{ent: entClient, mod: ruleMod, runners: RuleActionRunners(mod, bus)})
	river.AddWorker(workers, &RunRuleHTTPWorker{ent: entClient, mod: ruleMod, client: webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow), now: time.Now})
	river.AddWorker(workers, &RunHTTPStepWorker{
		ent:    entClient,
		mod:    automations.New(cipher, bus),
		step:   stepper,
		client: webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow),
		now:    time.Now,
		fence:  automations.FenceOn(entClient),
	})
	notifyOwners := accounts.OwnerNotifier(entClient, systemSender)
	river.AddWorker(workers, &DeliverWebhookWorker{
		ent:    entClient,
		cipher: cipher,
		client: webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow),
		now:    time.Now,
		mod:    webhooks.New(bus, cipher, nil, nil, notifyOwners),
	})
	river.AddWorker(workers, &SendWelcomeWorker{sender: systemSender})
	river.AddWorker(workers, &SendAuthMailWorker{sender: systemSender, appURL: appURL})
	river.AddWorker(workers, &SendMemberInviteWorker{sender: systemSender})
	river.AddWorker(workers, &NotifySecondFactorRequiredWorker{ent: entClient, sender: systemSender, appURL: appURL})
	river.AddWorker(workers, &RemindSecondFactorWorker{ent: entClient, sender: systemSender, appURL: appURL})
	// Sending-domain DKIM verification (ADR 0010). LookupTXT re-checks published
	// DNS; verified is a live property re-validated by the periodic job below.
	domains := domainverify.New(entClient, lookup, tracking, notifyOwners)
	river.AddWorker(workers, &VerifySendingDomainWorker{mod: domains})
	river.AddWorker(workers, &RecheckSendingDomainsWorker{mod: domains})
	// Join domains stay verified only while their TXT record is published.
	joinDomains := joindomains.New(entClient, bus, lookup, time.Now, notifyOwners)
	river.AddWorker(workers, &VerifyJoinDomainWorker{mod: joinDomains})
	river.AddWorker(workers, &RecheckJoinDomainsWorker{mod: joinDomains})
	river.AddWorker(workers, &VerifyTrackingDomainWorker{mod: domains})
	river.AddWorker(workers, &RecheckTrackingDomainsWorker{mod: domains})
	// SES send quota discovery (ADR 0023): refreshed hourly because quotas grow as an
	// account matures, and on Integration save by the API handlers.
	river.AddWorker(workers, &RefreshSendQuotasWorker{ent: entClient})
	river.AddWorker(workers, &RefreshIntegrationQuotaWorker{ent: entClient, cipher: cipher, catalog: catalog})
	river.AddWorker(workers, &PurgeAuthAttemptsWorker{ent: entClient})
	river.AddWorker(workers, &PruneOutboxWorker{ent: entClient, floor: retention.OutboxFloor})
	river.AddWorker(workers, &RefreshPrivateRelayWorker{ranges: ranges})
	river.AddWorker(workers, &PruneEventsWorker{ent: entClient, retention: retention.Events})
	river.AddWorker(workers, &PruneUnverifiedUsersWorker{accounts: accounts.New(entClient, bus), retention: retention.UnverifiedUsers})
	river.AddWorker(workers, &PrunePasskeyChallengesWorker{ent: entClient, now: time.Now})
	river.AddWorker(workers, &EraseWorkspacesWorker{deletion: workspacedeletion.New(entClient, bus), now: time.Now})

	var extraPeriodic []*river.PeriodicJob
	for _, e := range ext {
		if e.Workers != nil {
			e.Workers(workers)
		}
		extraPeriodic = append(extraPeriodic, e.PeriodicJobs...)
	}

	logger := slog.Default()
	rc, err := river.NewClient(riverdatabasesql.New(riverDB), newRiverConfig(workers, logger, extraPeriodic...))
	if err != nil {
		return nil, err
	}
	return &Client{river: rc, ent: entClient, appURL: appURL}, nil
}

// River job retention, explicit rather than river's defaults: bounds the
// river_job table (Event retention is separate, ADR 0019).
const (
	completedJobRetention = 24 * time.Hour
	cancelledJobRetention = 24 * time.Hour
	discardedJobRetention = 14 * 24 * time.Hour
)

// newRiverConfig builds the river config; extra are the Edition's periodic jobs, added to
// core's.
func newRiverConfig(workers *river.Workers, logger *slog.Logger, extra ...*river.PeriodicJob) *river.Config {
	return &river.Config{
		CompletedJobRetentionPeriod: completedJobRetention,
		CancelledJobRetentionPeriod: cancelledJobRetention,
		DiscardedJobRetentionPeriod: discardedJobRetention,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault:      {MaxWorkers: 5},
			broadcasts.Queue:        {MaxWorkers: 10},
			QueueWebhooks:           {MaxWorkers: 5},
			stepping.QueueHTTPSteps: {MaxWorkers: 5},
		},
		// The database/sql driver has no LISTEN connection: the client polls for new jobs.
		PollOnly:     true,
		Workers:      workers,
		Logger:       logger,
		ErrorHandler: &errorHandler{logger: logger},
		// OTel spans + metrics per job insert/work, via the global providers set
		// by telemetry.Setup (a no-op when telemetry is disabled, e.g. tests).
		Middleware: []rivertype.Middleware{otelriver.NewMiddleware(nil)},
		// Re-validate every Sending domain's DKIM DNS periodically so a record
		// that disappears flips the domain back to unverified (ADR 0010).
		PeriodicJobs: append([]*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return RecheckSendingDomainsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return RecheckJoinDomainsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Tracking domains are re-checked on their own cadence (ADR 0038).
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return RecheckTrackingDomainsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			river.NewPeriodicJob(
				river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) {
					return RefreshSendQuotasArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Drop failed-attempt rows past their window (ADR 0025).
			river.NewPeriodicJob(
				river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) {
					return PurgeAuthAttemptsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Remind Users a day before their Two-factor grace ends (ADR 0020).
			river.NewPeriodicJob(
				river.PeriodicInterval(secondFactorReminderInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return RemindSecondFactorArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Prune the domain-events outbox below the slowest consumer (ADR 0019).
			river.NewPeriodicJob(
				river.PeriodicInterval(outboxPruneInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return PruneOutboxArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Refresh the iCloud Private Relay egress ranges machine detection reads (ADR 0053).
			river.NewPeriodicJob(
				river.PeriodicInterval(privateRelayRefreshInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return RefreshPrivateRelayArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Delete expired analytical Events daily at 03:00 UTC (ADR 0019).
			river.NewPeriodicJob(
				dailyAtUTC{hour: eventsRetentionHourUTC},
				func() (river.JobArgs, *river.InsertOpts) {
					return PruneEventsArgs{}, nil
				},
				nil,
			),
			// Erase Workspaces whose 30-day deletion window has passed (ADR 0043).
			river.NewPeriodicJob(
				river.PeriodicInterval(workspaceErasureInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return EraseWorkspacesArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			// Delete stale Unverified Users daily at 04:00 UTC (ADR 0030).
			river.NewPeriodicJob(
				dailyAtUTC{hour: unverifiedUsersHourUTC},
				func() (river.JobArgs, *river.InsertOpts) {
					return PruneUnverifiedUsersArgs{}, nil
				},
				nil,
			),
			// Delete spent Passkey challenges daily at 05:00 UTC (ADR 0051).
			river.NewPeriodicJob(
				dailyAtUTC{hour: passkeyChallengesHourUTC},
				func() (river.JobArgs, *river.InsertOpts) {
					return PrunePasskeyChallengesArgs{}, nil
				},
				nil,
			),
		}, extra...),
	}
}

// NewInserter is the insert-only river client over db, for the transactional enqueues of
// events.Bus.WithinScopedEnqueueTx: a job is inserted on the caller's *sql.Tx, so it exists
// iff that transaction commits. It runs no workers; the Client's own river client works the jobs.
func NewInserter(db *sql.DB) (*river.Client[*sql.Tx], error) {
	return river.NewClient(riverdatabasesql.New(db), &river.Config{Middleware: []rivertype.Middleware{otelriver.NewMiddleware(nil)}})
}

// Start begins processing jobs (run in a goroutine; returns once started).
func (c *Client) Start(ctx context.Context) error { return c.river.Start(ctx) }

// Stopped is closed once the workers have stopped (after Stop, or when the context
// given to Start is cancelled).
func (c *Client) Stopped() <-chan struct{} { return c.river.Stopped() }

// Stop drains and stops the workers.
func (c *Client) Stop(ctx context.Context) error { return c.river.Stop(ctx) }

// EnqueueIntegrationQuotaRefresh schedules a read of one Integration's provider send
// quota (on save; the hourly tick covers the rest). Fire-and-forget: the result, or
// the warning that it could not be read, lands on the row.
func (c *Client) EnqueueIntegrationQuotaRefresh(ctx context.Context, integrationID int64) error {
	_, err := c.river.Insert(ctx, RefreshIntegrationQuotaArgs{IntegrationID: integrationID}, nil)
	return err
}

// EnqueueSendingDomainVerify schedules an immediate DKIM re-check of one Sending
// domain (the "Verify" button). Fire-and-forget: the result lands on the row.
func (c *Client) EnqueueSendingDomainVerify(ctx context.Context, sendingDomainID int64) error {
	_, err := c.river.Insert(ctx, VerifySendingDomainArgs{SendingDomainID: sendingDomainID}, nil)
	return err
}

// OnEvent enrolls a contact into automations triggered by the given Event action.
// It is the trigger seam injected into the event-recording paths; a no-op
// implementation keeps tests inert (no river runtime, writes roll back with the
// tx). Fire-and-forget: enrollment is best-effort and must never block the write.
func (c *Client) OnEvent(ctx context.Context, workspaceID, contactID int64, action string, sourceAutomationID int64) error {
	_, err := c.river.Insert(ctx, EvaluateTriggerArgs{
		WorkspaceID:        workspaceID,
		ContactID:          contactID,
		Action:             action,
		SourceAutomationID: sourceAutomationID,
	}, nil)
	return err
}
