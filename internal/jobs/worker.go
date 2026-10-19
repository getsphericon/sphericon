// Package jobs is the river-backed (Postgres) async job queue. It owns the
// worker registry, the typed enqueue API the HTTP handlers call, and river's own
// schema migration (kept out of Atlas — see Migrate).
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/riverqueue/rivercontrib/otelriver"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// webhookDeliveryTimeout bounds a single outbound webhook request.
const webhookDeliveryTimeout = 15 * time.Second

// QueueWebhooks isolates outbound webhook delivery from the default queue.
// Webhook deliveries are external HTTP calls (up to webhookDeliveryTimeout each),
// so a burst of them must not starve broadcast sends and automation steps.
const QueueWebhooks = "webhooks"

// QueueBroadcasts isolates broadcast sends. A single blast fans out into one job
// per recipient; keeping them off the default queue stops a large broadcast from
// starving automation steps and welcome emails (the same isolation QueueWebhooks
// gives outbound webhooks).
const QueueBroadcasts = "broadcasts"

// Client wraps the river client and exposes the typed enqueue methods the API
// handlers use, so callers don't depend on river directly.
type Client struct {
	river *river.Client[pgx.Tx]
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
func NewClient(pool *pgxpool.Pool, entClient *ent.Client, mod *outbound.Module, cipher *secrets.Cipher, bus *events.Bus, systemSender messaging.EmailSender, lookup sending.TXTLookup, tracking *trackingdomain.Verifier, catalog *messaging.Catalog, appURL string, outboundAllow webhook.Allowlist, retention Retention, ext ...Extension) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &SendBroadcastWorker{ent: entClient, mod: mod})
	river.AddWorker(workers, &SendRecipientWorker{ent: entClient, mod: mod})
	river.AddWorker(workers, &EvaluateTriggerWorker{ent: entClient})
	river.AddWorker(workers, &RunStepWorker{ent: entClient, mod: mod})
	river.AddWorker(workers, &RunHTTPStepWorker{
		ent:    entClient,
		mod:    automations.New(cipher),
		client: webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow),
		now:    time.Now,
	})
	river.AddWorker(workers, &DeliverWebhookWorker{
		ent:    entClient,
		cipher: cipher,
		client: webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow),
		now:    time.Now,
		bus:    bus,
		sender: systemSender,
	})
	river.AddWorker(workers, &SendWelcomeWorker{sender: systemSender})
	river.AddWorker(workers, &SendAuthMailWorker{sender: systemSender, appURL: appURL})
	river.AddWorker(workers, &SendMemberInviteWorker{sender: systemSender})
	river.AddWorker(workers, &NotifySecondFactorRequiredWorker{ent: entClient, sender: systemSender, appURL: appURL})
	river.AddWorker(workers, &RemindSecondFactorWorker{ent: entClient, sender: systemSender, appURL: appURL})
	// Sending-domain DKIM verification (ADR 0010). LookupTXT re-checks published
	// DNS; verified is a live property re-validated by the periodic job below.
	river.AddWorker(workers, &VerifySendingDomainWorker{ent: entClient, lookup: lookup, sender: systemSender, tracking: tracking})
	river.AddWorker(workers, &RecheckSendingDomainsWorker{ent: entClient})
	river.AddWorker(workers, &VerifyTrackingDomainWorker{ent: entClient, sender: systemSender, tracking: tracking})
	river.AddWorker(workers, &RecheckTrackingDomainsWorker{ent: entClient})
	// SES send quota discovery (ADR 0023): refreshed hourly because quotas grow as an
	// account matures, and on Integration save by the API handlers.
	river.AddWorker(workers, &RefreshSendQuotasWorker{ent: entClient})
	river.AddWorker(workers, &RefreshIntegrationQuotaWorker{ent: entClient, cipher: cipher, catalog: catalog})
	river.AddWorker(workers, &PurgeAuthAttemptsWorker{ent: entClient})
	river.AddWorker(workers, &PruneOutboxWorker{ent: entClient, floor: retention.OutboxFloor})
	river.AddWorker(workers, &PruneEventsWorker{ent: entClient, retention: retention.Events})
	river.AddWorker(workers, &PruneUnverifiedUsersWorker{ent: entClient, retention: retention.UnverifiedUsers})

	var extraPeriodic []*river.PeriodicJob
	for _, e := range ext {
		if e.Workers != nil {
			e.Workers(workers)
		}
		extraPeriodic = append(extraPeriodic, e.PeriodicJobs...)
	}

	logger := slog.Default()
	rc, err := river.NewClient(riverpgxv5.New(pool), newRiverConfig(workers, logger, extraPeriodic...))
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
			river.QueueDefault: {MaxWorkers: 5},
			QueueBroadcasts:    {MaxWorkers: 10},
			QueueWebhooks:      {MaxWorkers: 5},
			QueueHTTPSteps:     {MaxWorkers: 5},
		},
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
			// Delete expired analytical Events daily at 03:00 UTC (ADR 0019).
			river.NewPeriodicJob(
				dailyAtUTC{hour: eventsRetentionHourUTC},
				func() (river.JobArgs, *river.InsertOpts) {
					return PruneEventsArgs{}, nil
				},
				nil,
			),
			// Delete stale Unverified Users daily at 04:00 UTC (ADR 0030).
			river.NewPeriodicJob(
				dailyAtUTC{hour: unverifiedUsersHourUTC},
				func() (river.JobArgs, *river.InsertOpts) {
					return PruneUnverifiedUsersArgs{}, nil
				},
				nil,
			),
		}, extra...),
	}
}

// Start begins processing jobs (run in a goroutine; returns once started).
func (c *Client) Start(ctx context.Context) error { return c.river.Start(ctx) }

// Stopped is closed once the workers have stopped (after Stop, or when the context
// given to Start is cancelled).
func (c *Client) Stopped() <-chan struct{} { return c.river.Stopped() }

// Stop drains and stops the workers.
func (c *Client) Stop(ctx context.Context) error { return c.river.Stop(ctx) }

// EnqueueBroadcast schedules a broadcast send. A nil scheduledAt sends ASAP;
// otherwise the job runs at scheduledAt.
func (c *Client) EnqueueBroadcast(ctx context.Context, broadcastID int64, scheduledAt *time.Time) error {
	opts := &river.InsertOpts{Queue: QueueBroadcasts}
	if scheduledAt != nil {
		opts.ScheduledAt = *scheduledAt
	}
	_, err := c.river.Insert(ctx, SendBroadcastArgs{BroadcastID: broadcastID, ScheduledAt: scheduledAt}, opts)
	return err
}

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
func (c *Client) OnEvent(ctx context.Context, workspaceID, contactID int64, action string) error {
	_, err := c.river.Insert(ctx, EvaluateTriggerArgs{
		WorkspaceID: workspaceID,
		ContactID:   contactID,
		Action:      action,
	}, nil)
	return err
}
