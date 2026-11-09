// Package jobs is the river-backed (Postgres) async job queue. It owns the
// worker registry, the typed enqueue API the HTTP handlers call, and river's own
// schema migration (kept out of the ent diff — see Migrate).
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/riverqueue/rivercontrib/otelriver"
	"github.com/robfig/cron/v3"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/privaterelayranges"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/usecase/callbackinterpretation"
	"github.com/getsphericon/sphericon/internal/usecase/noticedelivery"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
)

// QueueWebhooks isolates outbound webhook delivery from the default queue.
// Webhook deliveries are external HTTP calls (up to webhook.DeliveryTimeout each),
// so a burst of them must not starve broadcast sends and automation steps.
const QueueWebhooks = "webhooks"

// Client is the river client that works the jobs: every worker registered, the periodic jobs
// scheduled. The modules queue work through a Queue, which needs none of what the workers drive.
type Client struct {
	river *river.Client[*sql.Tx]
}

// Retention groups the data-retention settings of the prune jobs.
type Retention struct {
	// Events is the age past which analytical Events are deleted (0 disables).
	Events time.Duration
	// UnverifiedUsers is the age past which an Unverified User with no Membership is
	// deleted (0 disables; ADR 0030).
	UnverifiedUsers time.Duration
	// CapturedMessages is the age past which a Test inbox's Captured message is deleted
	// (0 disables; ADR 0058).
	CapturedMessages time.Duration
}

// Deps are the services the workers drive, each the one the composition root built for the rest of
// the Instance: a worker builds no module of its own. A test leaves nil what the workers it runs do
// not reach.
type Deps struct {
	// Ent is the raw client of the job entry points (ADR 0017), which scope to a loaded row's
	// Workspace, and of the instance-wide prunes.
	Ent      *ent.Client
	Bus      *events.Bus
	Outbound *outboundmessages.Module
	Cipher   *secrets.Cipher
	// SystemSender is the platform's own sender, which delivers the Notices (nil on an Instance
	// with no outbound mail).
	SystemSender messaging.EmailSender
	// Notices issues the Notices of the domain events (ADR 0105).
	Notices *noticedelivery.UseCase
	// Callbacks reads again a Delivery notice that came before its message (the late notice job).
	Callbacks *callbackinterpretation.UseCase
	Catalog   *messaging.Catalog
	Rules     *rules.Module
	Users     *users.Module
	// SecondFactor reminds the Users whose Two-factor grace ends soon (ADR 0020).
	SecondFactor *secondfactors.Module
	Orders       *orders.Module
	Deletion     *workspacedeletion.UseCase
	// SendingDomains, JoinDomains and TrackingDomains re-check the published DNS of the domains
	// (ADR 0010, ADR 0071); verified is a live property.
	SendingDomains *sendingdomains.Module
	// StepExecution executes the steps of the Enrollments (the run_step and http_step jobs).
	StepExecution *automations.StepExecution
	JoinDomains   *joindomains.Module
	// Integrations reads an Integration's provider send quota (ADR 0023).
	Integrations    *integrations.Module
	TrackingDomains *trackingdomains.Module
	Machine         *privaterelayranges.Detector
	// Clock is the Instance's clock (app.Clock), which every worker reads; NewClient refuses none.
	Clock func() time.Time
	// OutboundAllow is the Instance's internal allowlist for webhook deliveries (ADR 0039).
	OutboundAllow webhook.Allowlist
	Retention     Retention
	// Webhooks, when set, wraps the dispatcher the webhooks consumer fans an event out with (the
	// Edition's license gate on Audit entries, ADR 0022).
	Webhooks func(next events.WebhookDispatcher) events.WebhookDispatcher
	// Instance is the Instance state, whose Release check and Telemetry report are the Instance's
	// own ticks (instance.go); nil schedules neither.
	Instance *instancestates.Module
}

// NewClient builds the river client with every worker registered over deps. ext plugs in the
// workers and periodic jobs of the Enterprise Edition and of the Instance. driver is river's database/sql driver over the
// job queue's pool; built with riverdatabasesql.NewWithPgxListener it carries the LISTEN
// connection that tells the client of a new job, a cancellation or a paused queue at once, and
// built with riverdatabasesql.New (tests on one transaction) the client only polls.
func NewClient(driver *riverdatabasesql.Driver, deps Deps, ext ...jobextension.Extension) (*Client, error) {
	entClient, mod, bus, cipher, systemSender := deps.Ent, deps.Outbound, deps.Bus, deps.Cipher, deps.SystemSender
	now := deps.Clock
	if now == nil {
		return nil, errors.New("jobs: Deps.Clock is required: the workers read the Instance clock")
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, NewSendBroadcastWorker(entClient, mod))
	river.AddWorker(workers, NewSendRecipientWorker(entClient, mod))
	stepper := deps.StepExecution
	river.AddWorker(workers, NewEvaluateTriggerWorker(entClient, stepper))
	river.AddWorker(workers, NewRunStepWorker(entClient, stepper))
	river.AddWorker(workers, NewLateNoticeWorker(entClient, deps.Callbacks))
	river.AddWorker(workers, &EvaluateRulesWorker{ent: entClient, mod: deps.Rules})
	river.AddWorker(workers, &RunRuleActionsWorker{ent: entClient, mod: deps.Rules, runners: RuleActionRunners(mod, bus, stepper)})
	river.AddWorker(workers, NewRunRuleHTTPWorker(entClient, deps.Rules))
	river.AddWorker(workers, NewRunHTTPStepWorker(entClient, stepper))
	river.AddWorker(workers, NewDeliverWebhookWorker(entClient, cipher, webhook.NewClientAllowing(webhook.DeliveryTimeout, deps.OutboundAllow), now, bus))
	river.AddWorker(workers, NewNoticeEventWorker(deps.Notices, systemSender))
	river.AddWorker(workers, NewSendNoticeWorker(systemSender))
	river.AddWorker(workers, NewRemindSecondFactorWorker(deps.SecondFactor, now))
	river.AddWorker(workers, NewVerifySendingDomainWorker(entClient, deps.SendingDomains))
	river.AddWorker(workers, NewRecheckSendingDomainsWorker(entClient))
	river.AddWorker(workers, NewVerifyJoinDomainWorker(deps.JoinDomains))
	river.AddWorker(workers, NewRecheckJoinDomainsWorker(deps.JoinDomains))
	river.AddWorker(workers, NewVerifyTrackingDomainWorker(deps.TrackingDomains))
	river.AddWorker(workers, NewRecheckTrackingDomainsWorker(deps.TrackingDomains))
	// SES send quota discovery (ADR 0023): refreshed hourly because quotas grow as an
	// account matures, and on Integration save by the API handlers.
	river.AddWorker(workers, NewRefreshSendQuotasWorker(entClient))
	river.AddWorker(workers, NewRefreshIntegrationQuotaWorker(entClient, deps.Integrations))
	river.AddWorker(workers, NewPurgeAuthAttemptsWorker(entClient, now))
	river.AddWorker(workers, NewRefreshPrivateRelayWorker(deps.Machine))
	river.AddWorker(workers, NewPruneEventsWorker(entClient, deps.Retention.Events, now))
	river.AddWorker(workers, NewPruneCapturedMessagesWorker(entClient, deps.Retention.CapturedMessages, now))
	river.AddWorker(workers, NewPruneUnverifiedUsersWorker(deps.Users, deps.Retention.UnverifiedUsers))
	river.AddWorker(workers, NewPrunePasskeyChallengesWorker(entClient, now))
	river.AddWorker(workers, NewPrunePendingSignupsWorker(deps.Users, now))
	river.AddWorker(workers, NewPruneStartTokensWorker(entClient, now))
	river.AddWorker(workers, NewEraseWorkspacesWorker(deps.Deletion))
	// The consumers of the domain events (EventConsumers). Those that queue the work an event
	// calls for queue it through the client itself, which exists once the workers are registered.
	river.AddWorker(workers, NewSuppressEventWorker(entClient))
	river.AddWorker(workers, NewBroadcastUnsubscribeEventWorker(entClient))
	river.AddWorker(workers, NewMessageObservationWorker(entClient))
	river.AddWorker(workers, NewOrderEventWorker(entClient, deps.Orders))
	dispatcher := NewWebhookDispatcher()
	if deps.Webhooks != nil {
		dispatcher = deps.Webhooks(dispatcher)
	}

	extraPeriodic := instanceJobs(workers, deps)
	for _, e := range ext {
		if e.Workers != nil {
			e.Workers(workers)
		}
		extraPeriodic = append(extraPeriodic, e.PeriodicJobs...)
	}
	river.AddWorker(workers, NewWebhookEventWorker(entClient, dispatcher))

	logger := slog.Default()
	cfg := newRiverConfig(entClient, workers, logger, extraPeriodic...)
	if driver.SupportsListener() {
		// A notification brings every new job at once; the poll only catches one that was missed
		// (the listener reconnecting), so it can be slow.
		cfg.FetchPollInterval = listenerFallbackPoll
	}
	rc, err := river.NewClient(driver, cfg)
	if err != nil {
		return nil, err
	}
	return &Client{river: rc}, nil
}

// SoftStopTimeout is how long a stopping client lets its running jobs finish before it
// cancels them. Every job is idempotent or fenced, so a cancelled one is simply retried.
const SoftStopTimeout = 10 * time.Second

// listenerFallbackPoll is how often a client that LISTENs still polls for jobs, in case it missed
// a notification. A poll-only client (no listener) keeps river's default of a second.
const listenerFallbackPoll = 5 * time.Second

// River job retention, explicit rather than river's defaults: bounds the
// river_job table (Event retention is separate, ADR 0019). A job that ran out of attempts, an
// event consumer's included, stays discarded for discardedJobRetention: time to notice it, fix
// the cause and retry it from River UI (ADR 0101).
const (
	completedJobRetention = 24 * time.Hour
	cancelledJobRetention = 24 * time.Hour
	discardedJobRetention = 30 * 24 * time.Hour
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
// logged on the job, shown on its page in the job queue UI; workspaceNamed refuses a job of a
// Workspace queued without its Workspace.
func BaseConfig() *river.Config {
	return &river.Config{
		Plugins:    []rivertype.Plugin{otelriver.NewMiddleware(&otelriver.MiddlewareConfig{EnableTracePropagation: true})},
		Middleware: []rivertype.Middleware{events.JobCorrelation(), logging.JobLogs(), &workspaceNamed{}},
	}
}

// WorkerConfig is BaseConfig for a client that works jobs, the worker client and the one a test
// works a job with: it also cancels a job of an erased Workspace before its worker runs
// (erasedWorkspace), reading the Workspace over client.
func WorkerConfig(client *ent.Client) *river.Config {
	cfg := BaseConfig()
	cfg.Middleware = append(cfg.Middleware, &erasedWorkspace{client: client})
	return cfg
}

// Queues are the queues the worker client works, each with its number of workers.
func Queues() map[string]river.QueueConfig {
	return map[string]river.QueueConfig{
		river.QueueDefault:         {MaxWorkers: 5},
		broadcasts.Queue:           {MaxWorkers: 10},
		QueueWebhooks:              {MaxWorkers: 5},
		automations.QueueHTTPSteps: {MaxWorkers: 5},
		// The domain events' consumers (ADR 0101): short jobs, most of which only queue more.
		events.QueueEvents: {MaxWorkers: 10},
	}
}

// newRiverConfig builds the river config; extra are the Edition's periodic jobs, added to
// core's.
func newRiverConfig(client *ent.Client, workers *river.Workers, logger *slog.Logger, extra ...*river.PeriodicJob) *river.Config {
	cfg := WorkerConfig(client)
	cfg.CompletedJobRetentionPeriod = completedJobRetention
	cfg.CancelledJobRetentionPeriod = cancelledJobRetention
	cfg.DiscardedJobRetentionPeriod = discardedJobRetention
	cfg.Queues = Queues()
	// Stop lets running jobs finish for SoftStopTimeout, then cancels their contexts.
	cfg.SoftStopTimeout = SoftStopTimeout
	cfg.Workers = workers
	cfg.Logger = logger
	cfg.ErrorHandler = NewErrorHandler(logger)
	cfg.PeriodicJobs = periodicJobs(extra)
	return cfg
}

// mustParseCron is the river schedule of a standard cron expression, as river's docs schedule a
// periodic job at a time of day (robfig/cron); CRON_TZ=UTC fixes the zone. The expressions are
// constants, so one that does not parse is a programming error.
func mustParseCron(expr string) cron.Schedule {
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		panic(fmt.Sprintf("jobs: the schedule %q: %v", expr, err))
	}
	return schedule
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
			eventsRetentionSchedule,
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
			unverifiedUsersSchedule,
			func() (river.JobArgs, *river.InsertOpts) {
				return PruneUnverifiedUsersArgs{}, nil
			},
			nil,
		),
		// Delete Captured messages past the Test inbox's retention window daily at 06:00 UTC (ADR 0058).
		river.NewPeriodicJob(
			capturedMessagesSchedule,
			func() (river.JobArgs, *river.InsertOpts) {
				return PruneCapturedMessagesArgs{}, nil
			},
			nil,
		),
		// Delete spent Passkey challenges daily at 05:00 UTC (ADR 0051).
		river.NewPeriodicJob(
			passkeyChallengesSchedule,
			func() (river.JobArgs, *river.InsertOpts) {
				return PrunePasskeyChallengesArgs{}, nil
			},
			nil,
		),
		// Delete the lapsed and used Start tokens daily at 04:00 UTC (ADR 0076).
		river.NewPeriodicJob(
			startTokensSchedule,
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
