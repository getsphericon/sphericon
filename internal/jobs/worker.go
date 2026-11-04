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
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/machine"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/trackingdomains"
	"github.com/getsphericon/sphericon/internal/webhook"
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
	// domains runs the checks a person waits for (the Verify button of a Sending domain).
	domains *domainverify.Module
	// appURL builds the links of account emails (rehearsed resets render one).
	appURL string
}

// Extension plugs extra workers and periodic jobs into the client without core
// importing them: the composition root hands in the Enterprise Edition's (ADR 0014).
type Extension struct {
	Workers      func(*river.Workers)
	PeriodicJobs []*river.PeriodicJob
	// Webhooks, when set, wraps the dispatcher the webhooks consumer fans an event out with (the
	// Edition's license gate on Audit entries, ADR 0022).
	Webhooks func(next events.WebhookDispatcher) events.WebhookDispatcher
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
	// CapturedMessages is the age past which a Test inbox's Captured message is deleted
	// (0 disables; ADR 0058).
	CapturedMessages time.Duration
}

// NewClient builds the river client with all workers registered. Workers carry
// their own dependencies (ent client, sender resolver, secrets cipher, the
// platform system sender). appURL is the public origin used to build the links
// in account emails (reset/verify/change). outboundAllow is the Instance's internal allowlist for
// webhook deliveries (ADR 0039). The instance-wide outbox and Event prunes
// run on entClient, the raw client; retention carries the prune settings. ext plugs in the
// Enterprise Edition's workers and periodic jobs. driver is river's database/sql driver over the
// job queue's pool; built with riverdatabasesql.NewWithPgxListener it carries the LISTEN
// connection that tells the client of a new job, a cancellation or a paused queue at once, and
// built with riverdatabasesql.New (tests on one transaction) the client only polls.
func NewClient(driver *riverdatabasesql.Driver, entClient *ent.Client, mod *outbound.Module, cipher *secrets.Cipher, bus *events.Bus, systemSender messaging.EmailSender, lookup sending.TXTLookup, tracking *trackingdomains.Module, catalog *messaging.Catalog, appURL string, outboundAllow webhook.Allowlist, ranges *machine.Detector, retention Retention, ext ...Extension) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewSendBroadcastWorker(entClient, mod))
	river.AddWorker(workers, NewSendRecipientWorker(entClient, mod))
	stepper := stepping.New(bus, mod, automations.FenceOn(entClient))
	river.AddWorker(workers, NewEvaluateTriggerWorker(entClient, bus))
	river.AddWorker(workers, NewRunStepWorker(entClient, stepper))
	river.AddWorker(workers, NewLateNoticeWorker(entClient, bus, time.Now))
	ruleMod := rules.New(cipher, bus)
	river.AddWorker(workers, &EvaluateRulesWorker{ent: entClient, mod: ruleMod})
	river.AddWorker(workers, &RunRuleActionsWorker{ent: entClient, mod: ruleMod, runners: RuleActionRunners(mod, bus)})
	river.AddWorker(workers, NewRunRuleHTTPWorker(entClient, ruleMod, webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow), time.Now))
	river.AddWorker(workers, NewRunHTTPStepWorker(entClient, cipher, stepper, webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow), time.Now))
	river.AddWorker(workers, NewDeliverWebhookWorker(entClient, cipher, webhook.NewClientAllowing(webhookDeliveryTimeout, outboundAllow), time.Now, bus, systemSender))
	river.AddWorker(workers, NewWelcomeWorker(systemSender))
	river.AddWorker(workers, NewAuthMailWorker(systemSender, appURL))
	river.AddWorker(workers, NewMemberInviteWorker(systemSender))
	river.AddWorker(workers, NewNotifySecondFactorRequiredWorker(entClient, systemSender, appURL))
	river.AddWorker(workers, &RemindSecondFactorWorker{ent: entClient, sender: systemSender, appURL: appURL})
	notifyOwners := accounts.OwnerNotifier(entClient, systemSender)
	// Sending-domain DKIM verification (ADR 0010). LookupTXT re-checks published
	// DNS; verified is a live property re-validated by the periodic job below.
	domains := domainverify.New(entClient, lookup, notifyOwners)
	river.AddWorker(workers, NewVerifySendingDomainWorker(domains))
	river.AddWorker(workers, NewRecheckSendingDomainsWorker(domains))
	// Join domains stay verified only while their TXT record is published.
	joinDomains := joindomains.New(entClient, bus, lookup, time.Now, notifyOwners)
	river.AddWorker(workers, NewVerifyJoinDomainWorker(joinDomains))
	river.AddWorker(workers, NewRecheckJoinDomainsWorker(joinDomains))
	// Tracking domains (ADR 0071): the verified hosts are re-checked so one that stops answering is
	// demoted and a retired one whose CNAME left goes.
	river.AddWorker(workers, NewVerifyTrackingDomainWorker(tracking))
	river.AddWorker(workers, NewRecheckTrackingDomainsWorker(tracking))
	// SES send quota discovery (ADR 0023): refreshed hourly because quotas grow as an
	// account matures, and on Integration save by the API handlers.
	river.AddWorker(workers, NewRefreshSendQuotasWorker(entClient))
	river.AddWorker(workers, NewRefreshIntegrationQuotaWorker(entClient, cipher, catalog))
	river.AddWorker(workers, NewPurgeAuthAttemptsWorker(entClient))
	river.AddWorker(workers, NewPruneOutboxWorker(entClient, retention.OutboxFloor))
	river.AddWorker(workers, NewRefreshPrivateRelayWorker(ranges))
	river.AddWorker(workers, NewPruneEventsWorker(entClient, retention.Events))
	river.AddWorker(workers, NewPruneCapturedMessagesWorker(entClient, retention.CapturedMessages))
	acc := accounts.New(entClient, bus, credentials.NewHasher(), consentrecords.New(entClient, bus, signupitems.New(entClient, bus, time.Now)))
	river.AddWorker(workers, NewPruneUnverifiedUsersWorker(acc, retention.UnverifiedUsers))
	river.AddWorker(workers, NewPrunePasskeyChallengesWorker(entClient, time.Now))
	river.AddWorker(workers, NewPrunePendingSignupsWorker(acc, time.Now))
	river.AddWorker(workers, NewPruneStartTokensWorker(entClient, time.Now))
	river.AddWorker(workers, NewEraseWorkspacesWorker(workspacedeletion.New(entClient, bus), time.Now))
	// The consumers of the domain events (EventConsumers). Those that queue the work an event
	// calls for queue it through the client itself, which exists once the workers are registered.
	c := &Client{ent: entClient, domains: domains, appURL: appURL}
	river.AddWorker(workers, NewSuppressEventWorker(entClient))
	river.AddWorker(workers, NewOrderEventWorker(entClient, orders.New(acc)))
	river.AddWorker(workers, NewAutomationEventWorker(clientInserter{c}))
	river.AddWorker(workers, NewRuleEventWorker(clientInserter{c}))
	dispatcher := NewWebhookDispatcher(clientInserter{c})

	var extraPeriodic []*river.PeriodicJob
	for _, e := range ext {
		if e.Workers != nil {
			e.Workers(workers)
		}
		if e.Webhooks != nil {
			dispatcher = e.Webhooks(dispatcher)
		}
		extraPeriodic = append(extraPeriodic, e.PeriodicJobs...)
	}
	river.AddWorker(workers, NewWebhookEventWorker(entClient, dispatcher))

	logger := slog.Default()
	cfg := newRiverConfig(workers, logger, extraPeriodic...)
	if driver.SupportsListener() {
		// A notification brings every new job at once; the poll only catches one that was missed
		// (the listener reconnecting), so it can be slow.
		cfg.FetchPollInterval = listenerFallbackPoll
	}
	rc, err := river.NewClient(driver, cfg)
	if err != nil {
		return nil, err
	}
	c.river = rc
	return c, nil
}

// clientInserter queues through the Client's river client, which NewClient sets after it has
// registered the workers that take it.
type clientInserter struct{ c *Client }

func (q clientInserter) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return q.c.river.Insert(ctx, args, opts)
}

// SoftStopTimeout is how long a stopping client lets its running jobs finish before it
// cancels them. Every job is idempotent or fenced, so a cancelled one is simply retried.
const SoftStopTimeout = 10 * time.Second

// listenerFallbackPoll is how often a client that LISTENs still polls for jobs, in case it missed
// a notification. A poll-only client (no listener) keeps river's default of a second.
const listenerFallbackPoll = 5 * time.Second

// River job retention, explicit rather than river's defaults: bounds the
// river_job table (Event retention is separate, ADR 0019). A job that ran out of attempts, an
// event consumer's included, stays discarded for DiscardedJobRetention: time to notice it, fix
// the cause and retry it from River UI (ADR 0101).
const (
	completedJobRetention = 24 * time.Hour
	cancelledJobRetention = 24 * time.Hour
	// DiscardedJobRetention is how long a discarded job is kept.
	DiscardedJobRetention = 30 * 24 * time.Hour
)

// BaseConfig is the river configuration every client of the application shares, the worker
// client and the insert-only one alike, and the one a test works a job with, so that the job
// meets the plugins and middleware it meets in production.
//
// otelriver is a plugin, so it is both middleware (a span and metrics per job inserted and
// worked) and a hook (the metrics of each fetch), through the global providers telemetry.Setup
// sets (a no-op when telemetry is disabled, as in tests). With trace propagation a job carries
// the trace context it was inserted in, and the span of its work links to it. Plugins run
// outside the middleware, so the span covers what the middleware does.
//
// events.JobCorrelation carries the chain of the domain event being handled through the jobs
// queued while handling it, to the events they publish; logging.JobLogs keeps the lines each job
// logged on the job, shown on its page in the job queue UI.
func BaseConfig() *river.Config {
	return &river.Config{
		Plugins:    []rivertype.Plugin{otelriver.NewMiddleware(&otelriver.MiddlewareConfig{EnableTracePropagation: true})},
		Middleware: []rivertype.Middleware{events.JobCorrelation(), logging.JobLogs()},
	}
}

// Queues are the queues the worker client works, each with its number of workers.
func Queues() map[string]river.QueueConfig {
	return map[string]river.QueueConfig{
		river.QueueDefault:      {MaxWorkers: 5},
		broadcasts.Queue:        {MaxWorkers: 10},
		QueueWebhooks:           {MaxWorkers: 5},
		stepping.QueueHTTPSteps: {MaxWorkers: 5},
		// The domain events' consumers (ADR 0101): short jobs, most of which only queue more.
		events.QueueEvents: {MaxWorkers: 10},
	}
}

// newRiverConfig builds the river config; extra are the Edition's periodic jobs, added to
// core's.
func newRiverConfig(workers *river.Workers, logger *slog.Logger, extra ...*river.PeriodicJob) *river.Config {
	cfg := BaseConfig()
	cfg.CompletedJobRetentionPeriod = completedJobRetention
	cfg.CancelledJobRetentionPeriod = cancelledJobRetention
	cfg.DiscardedJobRetentionPeriod = DiscardedJobRetention
	cfg.Queues = Queues()
	// Stop lets running jobs finish for SoftStopTimeout, then cancels their contexts.
	cfg.SoftStopTimeout = SoftStopTimeout
	cfg.Workers = workers
	cfg.Logger = logger
	cfg.ErrorHandler = NewErrorHandler(logger)
	cfg.PeriodicJobs = periodicJobs(extra)
	return cfg
}

// periodicJobs are core's periodic jobs, then extra, the Edition's.
func periodicJobs(extra []*river.PeriodicJob) []*river.PeriodicJob {
	return append([]*river.PeriodicJob{
		// Re-validate every Sending domain's DKIM DNS periodically so a record
		// that disappears flips the domain back to unverified (ADR 0010).
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
		// Tracking domains are re-checked on their own cadence (ADR 0038, ADR 0071).
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
		// Delete Captured messages past the Test inbox's retention window daily at 06:00 UTC (ADR 0058).
		river.NewPeriodicJob(
			dailyAtUTC{hour: capturedMessagesHourUTC},
			func() (river.JobArgs, *river.InsertOpts) {
				return PruneCapturedMessagesArgs{}, nil
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
		// Delete the lapsed and used Start tokens daily at 04:00 UTC (ADR 0076).
		river.NewPeriodicJob(
			dailyAtUTC{hour: startTokensHourUTC},
			func() (river.JobArgs, *river.InsertOpts) {
				return PruneStartTokensArgs{}, nil
			},
			nil,
		),
		// Delete the expired pending sign-ups of the Sign-in providers hourly (ADR 0062).
		river.NewPeriodicJob(
			river.PeriodicInterval(pendingSignupsInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return PrunePendingSignupsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}, extra...)
}

// NewInserter is the insert-only river client over db, for the transactional enqueues of
// events.Bus.WithinScopedEnqueueTx: a job is inserted on the caller's *sql.Tx, so it exists
// iff that transaction commits. It runs no workers; the Client's own river client works the jobs.
// It has the worker client's plugins and middleware (BaseConfig), so a job it inserts carries
// the trace context and the chain it was inserted in.
func NewInserter(db *sql.DB) (*river.Client[*sql.Tx], error) {
	return river.NewClient(riverdatabasesql.New(db), BaseConfig())
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

// VerifySendingDomain runs both checks of one Sending domain now (the "Verify" button) and
// returns once they are done: the result is on the row.
func (c *Client) VerifySendingDomain(ctx context.Context, sendingDomainID int64) error {
	return c.domains.Verify(ctx, sendingDomainID)
}
