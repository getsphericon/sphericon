package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/operator"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/analytics"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	apiexternal "github.com/getsphericon/sphericon/internal/api/external"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/mcpserver"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/do/v2"
	"go.opentelemetry.io/otel/metric"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type App struct {
	Config *config.Config
	Server *http.Server
	// Metrics is the opt-in Prometheus listener (ADR 0018); nil when METRICS_ADDR
	// is empty and for the operator app.
	Metrics *telemetry.MetricsServer

	injector       *do.RootScope
	listener       net.Listener // caller-provided; nil means listen on Config.Port
	events         *eventsRuntime
	jobs           *jobsClient
	shutdownOnce   sync.Once
	closeOnce      sync.Once
	closeErr       error
	cancel         context.CancelFunc
	eventsDone     chan struct{}
	jobsStarted    bool // river's Start was attempted: its Stopped channel closes on success and on failure
	done           chan error
	shutdownReport *do.ShutdownReport
}

type sqlDB struct {
	*sql.DB
}

func (d *sqlDB) Shutdown() error {
	return d.Close()
}

type entClient struct {
	*ent.Client
}

func (c *entClient) Shutdown() error {
	return c.Close()
}

// eventsRuntime is the consume side of the domain-event system: the watermill
// router hosting the persist/automations/webhooks subscribers. (The produce side
// is eventsBus.) Start runs the router; Shutdown closes it.
type eventsRuntime struct {
	router *message.Router
}

func (e *eventsRuntime) Shutdown() error {
	// A router that never ran has nothing to drain, and Close would wait out its
	// timeout for handlers that were never started.
	if !e.router.IsRunning() && !e.router.IsClosed() {
		return nil
	}
	return e.router.Close()
}

// eventsBus is the producer side of the domain-event system (transactional
// outbox over the database/sql pool). No Shutdown: it borrows the sqlDB pool.
type eventsBus struct {
	*events.Bus
}

// systemSender is sphericon's own (platform) transactional email sender, built from
// config via the messaging catalog. Distinct from a workspace's customer sender.
type systemSender struct {
	messaging.EmailSender
}

// dkimLookup is the DI-provided DKIM DNS resolver for sending-domain
// verification (ADR 0010). The composition root is the single place that decides
// the implementation — real DNS in prod, a seeded-domain-trusting stub in dev —
// so nothing downstream branches on the environment.
type dkimLookup struct {
	sending.TXTLookup
}

// pgxPool is the pgx connection pool river runs on (separate from the
// database/sql pool the ent client and pubsub use).
type pgxPool struct {
	*pgxpool.Pool
	metrics metric.Registration
}

func (p *pgxPool) Shutdown() {
	_ = p.metrics.Unregister()
	p.Close()
}

// jobsClient is the river-backed async job queue (workers + enqueue API).
// outboundModule is the Outbound send module singleton (ADR 0015): the one place an
// email leaves sphericon on a Workspace's behalf, for every send surface.
type outboundModule struct{ *outbound.Module }

// externalAPI is the external (/api) ogen server; the MCP surface dispatches through it.
type externalAPI struct{ http.Handler }

// mcpHandler is the /mcp surface (ADR 0016), projected from the external contract.
type mcpHandler struct{ http.Handler }

type jobsClient struct {
	*jobs.Client
}

func (j *jobsClient) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return j.Stop(ctx)
}

// Option customises how New builds the App.
type Option func(*options)

type options struct {
	listener      net.Listener
	e2eDKIMLookup bool
}

// ErrE2EOnly: an end-to-end-only option was passed to an app that is not the e2e profile.
var ErrE2EOnly = errors.New("app: option is only valid in the " + config.EnvE2E + " environment")

// WithE2EDKIMLookup swaps the DKIM DNS lookup for the development one, which echoes a
// Sending domain's own stored key, so the end-to-end suite verifies domains without
// real DNS while the development flag stays off. New refuses it outside the e2e
// profile, so it cannot be wired into a production configuration.
func WithE2EDKIMLookup() Option {
	return func(o *options) { o.e2eDKIMLookup = true }
}

// WithListener makes the App serve on a listener the caller already opened (a free
// port, say). The configured public URL (APP_URL, the base of unsubscribe, confirm
// and tracking links) and PORT are derived from the listener's address, so the order
// is: open the listener, then New, then Start. Close closes the listener.
func WithListener(ln net.Listener) Option {
	return func(o *options) { o.listener = ln }
}

func New(env string, opts ...Option) (*App, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.e2eDKIMLookup && env != config.EnvE2E {
		return nil, ErrE2EOnly
	}
	injector := do.New()
	register(injector, env, o)

	cfg, err := do.Invoke[*config.Config](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}

	// Set the process-wide locale for system emails and API validation messages
	// (internal/i18n). Instance-global, read-only after this point.
	i18n.Configure(cfg.Locale)

	handler, err := do.Invoke[http.Handler](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}

	evRuntime, err := do.Invoke[*eventsRuntime](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}

	jobsCli, err := do.Invoke[*jobsClient](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}

	// Operational gauges (outbox lag, queue depth). They read on scrape through the
	// shared pools and live for the process, so they are never unregistered.
	database, err := do.Invoke[*sqlDB](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}
	pool, err := do.Invoke[*pgxPool](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}
	if _, err := events.RegisterLagGauge(database.DB); err != nil {
		_ = injector.Shutdown()
		return nil, err
	}
	if _, err := jobs.RegisterQueueMetrics(pool.Pool); err != nil {
		_ = injector.Shutdown()
		return nil, err
	}

	var metrics *telemetry.MetricsServer
	if cfg.MetricsAddr != "" {
		metrics = telemetry.NewMetricsServer(cfg.MetricsAddr)
	}

	addr := ":" + cfg.Port
	if o.listener != nil {
		addr = o.listener.Addr().String()
	}

	return &App{
		Config:   cfg,
		Metrics:  metrics,
		listener: o.listener,
		Server: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		injector: injector,
		done:     make(chan error, 1),
		events:   evRuntime,
		jobs:     jobsCli,
	}, nil
}

// ErrNotServable: Start was called on an app that has no servers (the operator app).
var ErrNotServable = errors.New("app: this app has no servers to start")

// ErrAlreadyStarted: Start was called a second time; an App starts once.
var ErrAlreadyStarted = errors.New("app: already started")

// Start brings the application up and returns without blocking: it binds the metrics
// listener, starts the domain-event router and the job queue, then serves the public
// HTTP server. It returns the first startup error after unwinding whatever it had
// already started (the container included), so a failed Start leaves nothing running.
//
// Start owns its cancellation: it derives its own context from ctx, and Close cancels
// it. Done reports how the public server ends; Close stops everything. An App starts
// once: a second Start returns ErrAlreadyStarted, whether or not the first succeeded.
func (a *App) Start(ctx context.Context) error {
	if a.Server == nil {
		return ErrNotServable
	}
	if a.cancel != nil {
		return ErrAlreadyStarted
	}
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	if err := a.start(runCtx); err != nil {
		unwind, stop := context.WithTimeout(context.WithoutCancel(ctx), unwindTimeout)
		defer stop()
		return errors.Join(err, a.Close(unwind))
	}
	return nil
}

// unwindTimeout bounds the teardown of a Start that failed partway.
const unwindTimeout = 15 * time.Second

func (a *App) start(ctx context.Context) error {
	// The metrics listener is bound first: a bind failure (port in use) must fail the
	// boot, not leave an unmonitored instance.
	if err := a.bindMetrics(ctx); err != nil {
		return err
	}
	go func() {
		if err := a.Metrics.Serve(); err != nil {
			slog.Error("metrics server stopped", "err", err)
		}
	}()

	eventsErr := make(chan error, 1)
	a.eventsDone = make(chan struct{})
	go func() {
		defer close(a.eventsDone)
		if err := a.runEvents(ctx); err != nil {
			eventsErr <- err
		}
	}()
	select {
	case <-a.events.router.Running():
	case err := <-eventsErr:
		return fmt.Errorf("start event router: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}

	// Marked before the attempt: a Start that fails partway still closes river's Stopped
	// channel, and Close waits for it so no worker outlives the unwind.
	a.jobsStarted = true
	if err := a.runJobs(ctx); err != nil {
		return fmt.Errorf("start job queue: %w", err)
	}

	go func() { a.done <- a.serve() }()
	return nil
}

// Done delivers, once, how the public server ended. A failure to serve arrives as an
// error; after a graceful Close the server ends cleanly and a nil error arrives, so a
// receiver must read nil as "stopped", not "still running". Nothing arrives if Start
// failed before serving. A binary waits on it next to its signals.
func (a *App) Done() <-chan error { return a.done }

// Close cancels the event router and the job workers, waits for them to stop, shuts
// the HTTP servers down and then the container. Safe to call more than once and after
// a failed Start; every call returns the first call's result.
func (a *App) Close(ctx context.Context) error {
	a.closeOnce.Do(func() { a.closeErr = a.close(ctx) })
	return a.closeErr
}

func (a *App) close(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	var errs []error
	if a.eventsDone != nil {
		select {
		case <-a.eventsDone:
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("event router did not stop: %w", ctx.Err()))
		}
	}
	if a.jobsStarted {
		select {
		case <-a.jobs.Stopped():
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("job workers did not stop: %w", ctx.Err()))
		}
	}
	errs = append(errs, a.stop(ctx))
	if report := a.Shutdown(ctx); !report.Succeed {
		errs = append(errs, fmt.Errorf("shutdown incomplete: %w", *report))
	}
	return errors.Join(errs...)
}

// bindMetrics binds the metrics listener when one is configured.
func (a *App) bindMetrics(ctx context.Context) error { return a.Metrics.Listen(ctx) }

// serve serves the public server and blocks until it stops.
func (a *App) serve() error {
	var err error
	if a.listener != nil {
		err = a.Server.Serve(a.listener)
	} else {
		err = a.Server.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// stop gracefully stops the HTTP servers (public and metrics), under one context.
// A caller-provided listener is closed too, so a Start that failed before serving
// does not leave it bound (closing one the server already closed is harmless).
func (a *App) stop(ctx context.Context) error {
	errs := []error{a.Server.Shutdown(ctx), a.Metrics.Shutdown(ctx)}
	if a.listener != nil {
		if err := a.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NewOperator builds the minimal app the operator commands need (config, database,
// ent client, system sender) without the HTTP server, the event router or the job
// workers. Providers are lazy, so only what a command invokes is ever constructed;
// that keeps `sphericon workspace …` quick to start and to shut down.
func NewOperator(env string) (*App, error) {
	injector := do.New()
	register(injector, env, options{})

	cfg, err := do.Invoke[*config.Config](injector)
	if err != nil {
		_ = injector.Shutdown()
		return nil, err
	}
	i18n.Configure(cfg.Locale)
	return &App{Config: cfg, injector: injector}, nil
}

// runEvents runs the domain-event router (persist/automations/webhooks
// subscribers). Blocks until ctx is cancelled; stop happens via Shutdown.
func (a *App) runEvents(ctx context.Context) error {
	return a.events.router.Run(ctx)
}

// runJobs starts the river worker pool. It returns once started; workers run
// until the context is cancelled (stop happens via Shutdown).
func (a *App) runJobs(ctx context.Context) error {
	return a.jobs.Start(ctx)
}

// SuspendWorkspace freezes a Workspace's outbound sending (ADR 0007) and tells its
// owner(s) through the system sender. It reports whether anything changed; an
// already-suspended Workspace is left as it was and the owner is not told twice. If
// the notice cannot be sent the suspension still stands and the error says so.
func (a *App) SuspendWorkspace(ctx context.Context, slug string, by suspension.Actor, reason string) (bool, error) {
	client, err := do.Invoke[*entClient](a.injector)
	if err != nil {
		return false, err
	}
	id, err := accounts.WorkspaceIDBySlug(ctx, client.Client, slug)
	if err != nil {
		return false, err
	}
	bus, err := do.Invoke[*eventsBus](a.injector)
	if err != nil {
		return false, err
	}
	changed, err := suspension.SuspendWorkspace(ctx, bus.Bus, id, by, reason)
	if err != nil || !changed {
		return changed, err
	}
	sys, err := do.Invoke[*systemSender](a.injector)
	if err != nil {
		return true, fmt.Errorf("workspace suspended, but the owner notice could not be sent: %w", err)
	}
	if err := jobs.NotifyWorkspaceSuspended(ctx, client.Client, sys.EmailSender, id); err != nil {
		return true, fmt.Errorf("workspace suspended, but the owner notice could not be sent: %w", err)
	}
	return true, nil
}

// UnsuspendWorkspace lifts a Workspace's suspension; held sends resume on their own.
func (a *App) UnsuspendWorkspace(ctx context.Context, slug string, by suspension.Actor) (bool, error) {
	client, err := do.Invoke[*entClient](a.injector)
	if err != nil {
		return false, err
	}
	id, err := accounts.WorkspaceIDBySlug(ctx, client.Client, slug)
	if err != nil {
		return false, err
	}
	bus, err := do.Invoke[*eventsBus](a.injector)
	if err != nil {
		return false, err
	}
	return suspension.UnsuspendWorkspace(ctx, bus.Bus, id, by)
}

// ResetSecondFactor clears the Second factor and Recovery codes of the User with
// this email and ends every session of theirs (ADR 0020), recorded as
// `user.second_factor_reset` by the Operator "cli" in each of their Workspaces. It
// reports whether anything changed: a User without a Second factor is left as is.
func (a *App) ResetSecondFactor(ctx context.Context, email string) (bool, error) {
	client, err := do.Invoke[*entClient](a.injector)
	if err != nil {
		return false, err
	}
	id, err := client.User.Query().Where(user.Email(email)).OnlyID(ctx)
	if err != nil {
		return false, fmt.Errorf("user %q: %w", email, err)
	}
	sf, err := do.Invoke[*secondfactor.Module](a.injector)
	if err != nil {
		return false, err
	}
	err = sf.ResetByOperator(ctx, id, "cli")
	if errors.Is(err, secondfactor.ErrNotActive) {
		return false, nil
	}
	return err == nil, err
}

// CreateOperator makes a platform Operator (ADR 0026) and returns its one-time password:
// the only way an Operator comes to exist (there is no signup). It refuses a duplicate
// email and an instance without the `operator` license.
func (a *App) CreateOperator(ctx context.Context, email string) (string, error) {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return "", err
	}
	return edition.Operators.Create(ctx, email)
}

// ResetOperatorTOTP clears the TOTP of the Operator with this email and ends its
// sessions (ADR 0026): the only way a lost TOTP is reset. The Operator re-enrols at next
// login. operator.ErrNotFound for an unknown email, ErrNotLicensed without the license.
func (a *App) ResetOperatorTOTP(ctx context.Context, email string) error {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return err
	}
	return edition.Operators.ResetTOTP(ctx, email)
}

// Accounts is the product's Accounts module from the DI container, for harnesses that
// arrange users and Workspaces through the same instance the HTTP surface runs on
// (the end-to-end suite), so they open no second connection pool.
func (a *App) Accounts() (*accounts.Accounts, error) {
	return do.Invoke[*accounts.Accounts](a.injector)
}

func (a *App) Shutdown(ctx context.Context) *do.ShutdownReport {
	a.shutdownOnce.Do(func() {
		a.shutdownReport = a.injector.ShutdownWithContext(ctx)
	})
	return a.shutdownReport
}

func register(injector do.Injector, env string, o options) {
	do.Provide(injector, func(do.Injector) (*config.Config, error) {
		cfg, err := config.Load(env)
		if err != nil {
			return nil, err
		}
		if o.listener != nil {
			cfg.UseListener(o.listener.Addr())
		}
		return cfg, nil
	})

	do.Provide(injector, func(i do.Injector) (*sqlDB, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}

		database, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}

		db.ConfigurePool(database, cfg.DBPool)

		return &sqlDB{DB: database}, nil
	})

	do.Provide(injector, func(i do.Injector) (*entClient, error) {
		database, err := do.Invoke[*sqlDB](i)
		if err != nil {
			return nil, err
		}

		return &entClient{Client: db.NewEntClient(database.DB)}, nil
	})

	// Shared, stateless singletons — provided once and injected, rather than
	// rebuilt per consumer. The provider catalog (registry of email providers),
	// the Tink cipher (credential encryption; fails fast at boot on a bad key),
	// and the workspace→sender resolver built from them.
	do.Provide(injector, func(do.Injector) (*messaging.Catalog, error) {
		return registry.Default(), nil
	})

	do.Provide(injector, func(i do.Injector) (*secrets.Cipher, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		return secrets.NewCipher(cfg.EncryptionKey)
	})

	do.Provide(injector, func(i do.Injector) (*messaging.Resolver, error) {
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		catalog, err := do.Invoke[*messaging.Catalog](i)
		if err != nil {
			return nil, err
		}
		return messaging.NewResolver(cipher, catalog), nil
	})

	do.Provide(injector, func(i do.Injector) (*systemSender, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		catalog, err := do.Invoke[*messaging.Catalog](i)
		if err != nil {
			return nil, err
		}
		sender, err := buildSystemSender(cfg, catalog)
		if err != nil {
			return nil, err
		}
		return &systemSender{EmailSender: sender}, nil
	})

	do.Provide(injector, func(i do.Injector) (*dkimLookup, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		// Dev trusts seeded domains so the local send gate isn't blocked by real
		// DNS; prod verifies against published DKIM TXT records (ADR 0010).
		if cfg.IsDev || o.e2eDKIMLookup {
			client, err := do.Invoke[*entClient](i)
			if err != nil {
				return nil, err
			}
			return &dkimLookup{TXTLookup: jobs.DevTXTLookup(client.Client)}, nil
		}
		return &dkimLookup{TXTLookup: net.DefaultResolver.LookupTXT}, nil
	})

	// The Enterprise Edition (ADR 0014): what the runtime license key switches on. An
	// empty key is the plain core; a malformed, forged or expired one fails boot.
	do.Provide(injector, func(i do.Injector) (*ee.Edition, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		lic, err := licensekey.Parse(cfg.LicenseKey, licensekey.ProductionKey, time.Now())
		if err != nil {
			return nil, err
		}
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		attempts, err := do.Invoke[*accounts.Attempts](i)
		if err != nil {
			return nil, err
		}
		return ee.New(client.Client, lic, cipher, cfg.JWTSecret, operator.Config{
			Secret: cfg.OperatorJWTSecret, SessionTTL: cfg.OperatorSessionTTL, SecureCookies: cfg.SecureCookies(), Attempts: attempts,
		})
	})

	do.Provide(injector, func(i do.Injector) (*eventsRuntime, error) {
		database, err := do.Invoke[*sqlDB](i)
		if err != nil {
			return nil, err
		}
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		edition, err := do.Invoke[*ee.Edition](i)
		if err != nil {
			return nil, err
		}
		jc, err := do.Invoke[*jobsClient](i)
		if err != nil {
			return nil, err
		}

		// Domain events: create the outbox topic schema up front (the tx publisher
		// can't self-initialize), build the router, and register the consumers. The
		// automations subscriber enrolls and the webhooks subscriber dispatches via
		// the river client.
		if err := events.InitSchema(context.Background(), database.DB); err != nil {
			return nil, err
		}
		router, err := events.NewRouter()
		if err != nil {
			return nil, err
		}
		if err := events.RegisterSubscribers(router, database.DB, client.Client, jc.Client, edition.Webhooks(jc.Client), edition.Consumers...); err != nil {
			return nil, err
		}
		return &eventsRuntime{router: router}, nil
	})

	do.Provide(injector, func(i do.Injector) (*eventsBus, error) {
		database, err := do.Invoke[*sqlDB](i)
		if err != nil {
			return nil, err
		}
		return &eventsBus{Bus: events.New(database.DB)}, nil
	})

	do.Provide(injector, func(i do.Injector) (*pgxPool, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		database, err := do.Invoke[*sqlDB](i)
		if err != nil {
			return nil, err
		}
		pool, err := db.NewPGXPool(context.Background(), cfg.DatabaseURL, cfg.DBPool)
		if err != nil {
			return nil, err
		}
		reg, err := db.RegisterPoolMetrics(database.DB, pool)
		if err != nil {
			pool.Close()
			return nil, err
		}
		return &pgxPool{Pool: pool, metrics: reg}, nil
	})

	do.Provide(injector, func(i do.Injector) (*jobsClient, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		pool, err := do.Invoke[*pgxPool](i)
		if err != nil {
			return nil, err
		}
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		sys, err := do.Invoke[*systemSender](i)
		if err != nil {
			return nil, err
		}
		sender, err := do.Invoke[*outboundModule](i)
		if err != nil {
			return nil, err
		}

		lookup, err := do.Invoke[*dkimLookup](i)
		if err != nil {
			return nil, err
		}
		edition, err := do.Invoke[*ee.Edition](i)
		if err != nil {
			return nil, err
		}
		catalog, err := do.Invoke[*messaging.Catalog](i)
		if err != nil {
			return nil, err
		}
		database, err := do.Invoke[*sqlDB](i)
		if err != nil {
			return nil, err
		}
		jc, err := jobs.NewClient(pool.Pool, client.Client, database.DB, sender.Module, cipher, sys.EmailSender, lookup.TXTLookup, catalog, cfg.AppURL,
			jobs.Retention{OutboxFloor: cfg.OutboxFloor, Events: cfg.EventsRetention}, edition.Jobs())
		if err != nil {
			return nil, err
		}
		return &jobsClient{Client: jc}, nil
	})

	// The signed-token authority for tracking, unsubscribe and confirmation links:
	// one instance, so every surface mints and verifies with the same secret.
	do.Provide(injector, func(i do.Injector) (*tracking.Tracker, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		return tracking.New(cfg.JWTSecret, cfg.AppURL), nil
	})

	do.Provide(injector, func(i do.Injector) (*outboundModule, error) {
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		// The workspace's configured integration (the same resolver the jobs use).
		resolver, err := do.Invoke[*messaging.Resolver](i)
		if err != nil {
			return nil, err
		}
		tracker, err := do.Invoke[*tracking.Tracker](i)
		if err != nil {
			return nil, err
		}
		return &outboundModule{outbound.New(bus.Bus, resolver, tracker)}, nil
	})

	// Accounts is the raw-client home of Users, Workspaces and Memberships (ADR 0017).
	do.Provide(injector, func(i do.Injector) (*accounts.Accounts, error) {
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		return accounts.New(client.Client, bus.Bus), nil
	})

	// Attempts counts failed logins per account (ADR 0025) over the raw client.
	do.Provide(injector, func(i do.Injector) (*accounts.Attempts, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		return accounts.NewAttempts(client.Client, accounts.WithRateLimits(cfg.RateLimits)), nil
	})
	do.Provide(injector, func(i do.Injector) (*secondfactor.Module, error) {
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		return secondfactor.New(client.Client, bus.Bus, cipher, nil), nil
	})

	// Domain modules: each built once and shared by /site, /api and /mcp, so the
	// surfaces cannot diverge on how a module is constructed.
	do.Provide(injector, func(do.Injector) (*segments.Module, error) {
		return segments.New(), nil
	})

	do.Provide(injector, func(i do.Injector) (*contacts.Module, error) {
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		return contacts.New(bus.Bus), nil
	})

	do.Provide(injector, func(i do.Injector) (*erasure.Module, error) {
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		return erasure.New(bus.Bus), nil
	})

	do.Provide(injector, func(i do.Injector) (*integrations.Module, error) {
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		catalog, err := do.Invoke[*messaging.Catalog](i)
		if err != nil {
			return nil, err
		}
		jc, err := do.Invoke[*jobsClient](i)
		if err != nil {
			return nil, err
		}
		return integrations.New(bus.Bus, cipher, catalog, jc.Client), nil
	})

	do.Provide(injector, func(i do.Injector) (*sendingdomains.Module, error) {
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		jc, err := do.Invoke[*jobsClient](i)
		if err != nil {
			return nil, err
		}
		return sendingdomains.New(bus.Bus, cipher, jc.Client), nil
	})

	do.Provide(injector, func(do.Injector) (*templates.Module, error) {
		return templates.New(), nil
	})
	do.Provide(injector, func(do.Injector) (*tags.Module, error) {
		return tags.New(), nil
	})

	// Webhooks: the audit.entry rule asks the Edition whether the license is active.
	do.Provide(injector, func(i do.Injector) (*webhooks.Module, error) {
		cipher, err := do.Invoke[*secrets.Cipher](i)
		if err != nil {
			return nil, err
		}
		edition, err := do.Invoke[*ee.Edition](i)
		if err != nil {
			return nil, err
		}
		return webhooks.New(cipher, edition.Audit), nil
	})

	do.Provide(injector, func(do.Injector) (*automations.Module, error) {
		return automations.New(), nil
	})

	do.Provide(injector, func(i do.Injector) (*eventlog.Module, error) {
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		return eventlog.New(bus.Bus), nil
	})

	do.Provide(injector, func(do.Injector) (*analytics.Module, error) {
		return analytics.New(), nil
	})

	do.Provide(injector, func(do.Injector) (*reputation.Module, error) {
		return reputation.New(), nil
	})

	// The river jobs client is the broadcasts module's enqueue seam.
	do.Provide(injector, func(i do.Injector) (*broadcasts.Module, error) {
		jc, err := do.Invoke[*jobsClient](i)
		if err != nil {
			return nil, err
		}
		return broadcasts.New(jc.Client), nil
	})

	do.Provide(injector, func(i do.Injector) (*authtoken.Signer, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		return authtoken.New(cfg.JWTSecret), nil
	})

	do.Provide(injector, func(i do.Injector) (*externalAPI, error) {
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		deps, err := externalDeps(i)
		if err != nil {
			return nil, err
		}
		h, err := server.NewExternalAPI(client.Client, deps)
		if err != nil {
			return nil, err
		}
		return &externalAPI{h}, nil
	})

	do.Provide(injector, func(i do.Injector) (*mcpHandler, error) {
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		external, err := do.Invoke[*externalAPI](i)
		if err != nil {
			return nil, err
		}
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		h, err := mcpserver.New(sphericon.ExternalOpenAPI, external.Handler, apiauth.NewExternalSecurityHandler(client.Client, bus.Bus), mcpserver.WithResourceMetadataURL(oauthserver.ResourceMetadataURL(cfg.MCPOrigin())))
		if err != nil {
			return nil, err
		}
		return &mcpHandler{h}, nil
	})

	do.Provide(injector, func(i do.Injector) (http.Handler, error) {
		cfg, err := do.Invoke[*config.Config](i)
		if err != nil {
			return nil, err
		}
		database, err := do.Invoke[*sqlDB](i)
		if err != nil {
			return nil, err
		}
		client, err := do.Invoke[*entClient](i)
		if err != nil {
			return nil, err
		}
		site, err := siteDeps(i)
		if err != nil {
			return nil, err
		}
		external, err := do.Invoke[*externalAPI](i)
		if err != nil {
			return nil, err
		}
		mcp, err := do.Invoke[*mcpHandler](i)
		if err != nil {
			return nil, err
		}

		edition, err := do.Invoke[*ee.Edition](i)
		if err != nil {
			return nil, err
		}
		bus, err := do.Invoke[*eventsBus](i)
		if err != nil {
			return nil, err
		}
		sys, err := do.Invoke[*systemSender](i)
		if err != nil {
			return nil, err
		}
		operatorAPI, err := server.NewOperatorAPI(edition.Operator(bus.Bus, sys.EmailSender))
		if err != nil {
			return nil, err
		}

		return server.New(cfg, database.DB, client.Client, site, external.Handler, mcp.Handler, operatorAPI)
	})
}

// externalDeps collects the shared singletons the /api handlers are built from.
func externalDeps(i do.Injector) (apiexternal.Deps, error) {
	cfg, err := do.Invoke[*config.Config](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	acc, err := do.Invoke[*accounts.Accounts](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	bus, err := do.Invoke[*eventsBus](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	wh, err := do.Invoke[*webhooks.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	sender, err := do.Invoke[*outboundModule](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	seg, err := do.Invoke[*segments.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	evlog, err := do.Invoke[*eventlog.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	con, err := do.Invoke[*contacts.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	er, err := do.Invoke[*erasure.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	tpl, err := do.Invoke[*templates.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	tg, err := do.Invoke[*tags.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	auto, err := do.Invoke[*automations.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	bc, err := do.Invoke[*broadcasts.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	rep, err := do.Invoke[*reputation.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	integ, err := do.Invoke[*integrations.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	sd, err := do.Invoke[*sendingdomains.Module](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	edition, err := do.Invoke[*ee.Edition](i)
	if err != nil {
		return apiexternal.Deps{}, err
	}
	return apiexternal.Deps{
		Accounts: acc, Bus: bus.Bus, Webhooks: wh, Outbound: sender.Module,
		Segments: seg, EventLog: evlog, Contacts: con, Erasure: er, Tags: tg, Templates: tpl, Automations: auto,
		Broadcasts: bc, Reputation: rep, Integrations: integ, SendingDomains: sd, BootstrapToken: cfg.BootstrapToken, Audit: edition.Audit,
	}, nil
}

// siteDeps collects the shared singletons the /site handlers are built from.
func siteDeps(i do.Injector) (apisite.Deps, error) {
	cfg, err := do.Invoke[*config.Config](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	client, err := do.Invoke[*entClient](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	acc, err := do.Invoke[*accounts.Accounts](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	bus, err := do.Invoke[*eventsBus](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	wh, err := do.Invoke[*webhooks.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	sender, err := do.Invoke[*outboundModule](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	seg, err := do.Invoke[*segments.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	evlog, err := do.Invoke[*eventlog.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	con, err := do.Invoke[*contacts.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	er, err := do.Invoke[*erasure.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	tpl, err := do.Invoke[*templates.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	tg, err := do.Invoke[*tags.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	auto, err := do.Invoke[*automations.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	bc, err := do.Invoke[*broadcasts.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	tokens, err := do.Invoke[*authtoken.Signer](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	tracker, err := do.Invoke[*tracking.Tracker](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	// The river jobs client implements the remaining enqueue seams: welcome, the
	// self-service account mail (reset/verify/change), and sending-domain DKIM
	// verification.
	jc, err := do.Invoke[*jobsClient](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	edition, err := do.Invoke[*ee.Edition](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	attempts, err := do.Invoke[*accounts.Attempts](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	integ, err := do.Invoke[*integrations.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	sd, err := do.Invoke[*sendingdomains.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	sf, err := do.Invoke[*secondfactor.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	an, err := do.Invoke[*analytics.Module](i)
	if err != nil {
		return apisite.Deps{}, err
	}
	return apisite.Deps{
		Accounts: acc, Attempts: attempts, OAuth: oauthserver.NewService(client.Client), Bus: bus.Bus, Webhooks: wh, Outbound: sender.Module,
		Segments: seg, EventLog: evlog, Contacts: con, Erasure: er, Tags: tg, Templates: tpl, Automations: auto,
		Broadcasts: bc, Welcome: jc.Client, SysMail: jc.Client, SendingDomains: sd, Integrations: integ,
		Tokens: tokens, Tracker: tracker, AppURL: cfg.AppURL, Audit: edition.Audit, SecondFactor: sf, Analytics: an,
	}, nil
}

// buildSystemSender constructs sphericon's platform email sender from config via the
// messaging catalog: smtp (dev → mailpit) or ses (prod). Same abstraction as
// customer sends, just sphericon's own provider.
func buildSystemSender(cfg *config.Config, catalog *messaging.Catalog) (messaging.EmailSender, error) {
	from := messaging.FirstNonEmpty(cfg.SystemEmailFrom, cfg.SMTPFrom)
	switch cfg.SystemEmailProvider {
	case "ses":
		blob, err := json.Marshal(map[string]any{
			"region":          cfg.SESRegion,
			"accessKeyId":     cfg.SESAccessKeyID,
			"secretAccessKey": cfg.SESSecretAccessKey,
			"from":            from,
			"fromName":        "sphericon",
		})
		if err != nil {
			return nil, err
		}
		// nil signer: platform mail is not signed with a workspace sending domain.
		return catalog.BuildEmail(messaging.ProviderSES, blob, nil)
	default: // smtp (dev → mailpit)
		blob, err := json.Marshal(map[string]any{
			"host":     cfg.SMTPHost,
			"port":     cfg.SMTPPort,
			"username": cfg.SMTPUser,
			"password": cfg.SMTPPass,
			"from":     from,
			"fromName": "sphericon",
		})
		if err != nil {
			return nil, err
		}
		return catalog.BuildEmail(messaging.ProviderSMTP, blob, nil)
	}
}
