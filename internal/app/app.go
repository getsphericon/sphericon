// Package app composes the application: it registers every singleton in the dependency container
// and starts and stops the servers and the job workers.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/samber/do/v2"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/infra/devdns"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/sidelistener"
	"github.com/getsphericon/sphericon/internal/infra/telemetry"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorreset"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

type App struct {
	Config *config.Config
	Server *http.Server
	// Metrics is the opt-in Prometheus listener (ADR 0018); nil when SPHERICON_METRICS_ADDR
	// is empty and for the command app.
	Metrics *telemetry.MetricsServer
	// Internal is the opt-in internal listener: the edge's TLS ask (ADR 0038, always) and the
	// service endpoints (ADR 0026, with the `staff` license); nil when SPHERICON_INTERNAL_ADDR
	// is empty and for the command app.
	Internal *sidelistener.Server

	injector       *do.RootScope
	listener       net.Listener // caller-provided; nil means listen on Config.Port
	jobs           *jobsClient
	shutdownOnce   sync.Once
	closeOnce      sync.Once
	closeErr       error
	cancel         context.CancelFunc
	jobsStarted    bool // river's Start was attempted: its Stopped channel closes on success and on failure
	done           chan error
	shutdownReport *do.ShutdownReport
}

// Option customises how New builds the App.
type Option func(*options)

type options struct {
	listener net.Listener
	e2eDNS   bool
}

// ErrE2EOnly means an end-to-end-only option was passed to an app that is not the e2e profile.
var ErrE2EOnly = errors.New("app: option is only valid in the " + config.EnvE2E + " environment")

// WithE2EDNS takes the end-to-end suite off real DNS, whose domains are not real: the DKIM DNS
// lookup is the development one, which echoes a Sending domain's own stored key, so the suite
// verifies domains while the development flag stays off, and every address's domain accepts mail
// in the Contact intake (ADR 0117). New refuses it outside the e2e profile, so it cannot be wired
// into a production configuration.
func WithE2EDNS() Option {
	return func(o *options) { o.e2eDNS = true }
}

// WithListener makes the App serve on a listener the caller already opened (a free
// port, say). The configured public URL (SPHERICON_APP_URL, the base of unsubscribe, confirm
// and tracking links) and PORT are derived from the listener's address, so the order
// is: open the listener, then New, then Start. Close closes the listener.
func WithListener(ln net.Listener) Option {
	return func(o *options) { o.listener = ln }
}

// New loads the configuration of env and builds the App on it. An end-to-end-only option is
// refused before anything is loaded.
func New(env string, opts ...Option) (*App, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.e2eDNS && env != config.EnvE2E {
		return nil, ErrE2EOnly
	}
	cfg, err := config.Load(env)
	if err != nil {
		return nil, err
	}
	return newApp(cfg, o)
}

// newApp builds the App over a configuration already loaded (Serve loads it once, for the
// logger and the telemetry too): the container, then every singleton a running server needs,
// so a broken one fails here rather than on the first request.
func newApp(cfg *config.Config, o options) (*App, error) {
	if o.listener != nil {
		own := *cfg
		own.UseListener(o.listener.Addr())
		cfg = &own
	}
	injector := Container(cfg)
	if o.e2eDNS {
		// The e2e suite's domains self-verify: devdns answers each record from the rows of the
		// module that proves it.
		do.Override(injector, func(i do.Injector) (domainname.TXTLookup, error) {
			client := do.MustInvoke[*ent.Client](i)
			return devdns.Lookup(sendingdomains.DevRecords(client), joindomains.DevRecords(client)), nil
		})
		do.OverrideValue[addresscheck.Resolver](injector, addresscheck.AllAccept{})
	}

	// Set the process-wide locale for system emails and API validation messages
	// (internal/kernel/i18n). Instance-global, read-only after this point.
	i18n.Configure(cfg.Locale)

	handler, err := do.Invoke[Handler](injector)
	if err != nil {
		return nil, abort(injector, err)
	}
	// The Presets load at start: a broken custom file stops it here, not on a callback.
	if _, err := do.Invoke[*preset.Registry](injector); err != nil {
		return nil, abort(injector, err)
	}
	jobsCli, err := do.Invoke[*jobsClient](injector)
	if err != nil {
		return nil, abort(injector, err)
	}
	// The operational gauge (queue depth and lag), read on scrape; the container takes it off
	// the meter when it shuts down.
	if _, err := do.Invoke[*queueDepthGauge](injector); err != nil {
		return nil, abort(injector, err)
	}

	var metrics *telemetry.MetricsServer
	if cfg.MetricsAddr != "" {
		metrics = telemetry.NewMetricsServer(cfg.MetricsAddr)
	}

	// The internal listener exists only when an address is configured: it always
	// serves the edge's TLS ask (ADR 0038), the service endpoints only with the
	// `staff` license (ADR 0026).
	var internal *sidelistener.Server
	if cfg.InternalAddr != "" {
		ih, err := do.Invoke[InternalHandler](injector)
		if err != nil {
			return nil, abort(injector, err)
		}
		internal = sidelistener.New("internal", cfg.InternalAddr, ih.Handler, sidelistener.WithTimeouts(30*time.Second, 2*time.Minute))
	}

	addr := ":" + cfg.Port
	if o.listener != nil {
		addr = o.listener.Addr().String()
	}

	return &App{
		Config:   cfg,
		Metrics:  metrics,
		Internal: internal,
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
		jobs:     jobsCli,
	}, nil
}

// ErrNotServable means Start was called on an app that has no servers (the command app).
var ErrNotServable = errors.New("app: this app has no servers to start")

// ErrAlreadyStarted means Start was called a second time; an App starts once.
var ErrAlreadyStarted = errors.New("app: already started")

// NewCommandApp builds the minimal app the commands need (config, database,
// ent client, system sender) without the HTTP server or the job
// workers. Providers are lazy, so only what a command invokes is ever constructed;
// that keeps `sphericon workspace …` quick to start and to shut down.
func NewCommandApp(env string) (*App, error) {
	cfg, err := config.Load(env)
	if err != nil {
		return nil, err
	}
	i18n.Configure(cfg.Locale)
	return &App{Config: cfg, injector: Container(cfg)}, nil
}

// abort shuts a container down after a start that failed, and returns the error that failed it.
func abort(injector do.Injector, err error) error {
	cleanup.Shutdown(injector.Shutdown())
	return err
}

// Start brings the application up and returns without blocking: it binds the metrics
// listener, starts the job queue, then serves the public
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
	// The first start creates the Instance state row and its random Instance id (ADR 0032).
	inst, err := do.Invoke[*instancestates.Module](a.injector)
	if err != nil {
		return err
	}
	row, err := inst.Ensure(ctx)
	if err != nil {
		return err
	}
	// The APIs show every primary key sealed under the Instance's id key (ADR 0142).
	if err := publicid.Use(row.IDKey); err != nil {
		return err
	}
	if err := inst.AnnounceRelease(ctx); err != nil {
		return err
	}
	if err := telemetry.ObserveReleaseAvailable(inst.LatestRelease); err != nil {
		return err
	}
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return err
	}
	if err := telemetry.ObserveLicenseExpired(func() bool { _, lapsed := edition.LicenseLapsedAt(); return lapsed }); err != nil {
		return err
	}
	// The metrics listener is bound first: a bind failure (port in use) must fail the
	// boot, not leave an unmonitored instance.
	if err := a.bindMetrics(ctx); err != nil {
		return err
	}
	go func() {
		if err := a.Metrics.Serve(); err != nil {
			slog.ErrorContext(ctx, "metrics server stopped", "err", err)
		}
	}()
	// The internal listener is bound before anything else is served for the same
	// reason: a self-hoster who asked for it must not silently run without it.
	if err := a.Internal.Listen(ctx); err != nil {
		return err
	}
	go func() {
		if err := a.Internal.Serve(); err != nil {
			slog.ErrorContext(ctx, "internal server stopped", "err", err)
		}
	}()

	// Marked before the attempt: a Start that fails partway still closes river's Stopped
	// channel, and Close waits for it so no worker outlives the unwind.
	a.jobsStarted = true
	if err := a.runJobs(ctx); err != nil {
		return fmt.Errorf("start job queue: %w", err)
	}

	go func() { a.done <- a.serve() }()
	return nil
}

// WarmDocuments starts the document renderer's runtime in the background
// (emaildoc.Renderer.Warm). Its start cannot be interrupted, so it is the server binary's call,
// not part of Start: a process that only opens and closes an App never starts one.
func (a *App) WarmDocuments(ctx context.Context) error {
	renderer, err := do.Invoke[*emaildoc.Renderer](a.injector)
	if err != nil {
		return err
	}
	renderer.Warm(ctx)
	return nil
}

// Done delivers, once, how the public server ended. A failure to serve arrives as an
// error; after a graceful Close the server ends cleanly and a nil error arrives, so a
// receiver must read nil as "stopped", not "still running". Nothing arrives if Start
// failed before serving. A binary waits on it next to its signals.
func (a *App) Done() <-chan error { return a.done }

// Close cancels the job workers, waits for them to stop, shuts
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
	errs := []error{a.Server.Shutdown(ctx), a.Metrics.Shutdown(ctx), a.Internal.Shutdown(ctx)}
	if a.listener != nil {
		if err := a.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// runJobs starts the river worker pool. It returns once started; workers run
// until the context is cancelled (stop happens via Shutdown).
func (a *App) runJobs(ctx context.Context) error {
	return a.jobs.Start(ctx)
}

// The modules and use cases the staff member commands call (cmd/server, a driver, ADR 0111), each
// built by the container only when a command asks for it, so a command constructs only what it
// uses.

// Suspender is the Workspace suspension from the DI container.
func (a *App) Suspender() (*suspension.Suspender, error) {
	return do.Invoke[*suspension.Suspender](a.injector)
}

// SecondFactorReset is the Second factor reset use case from the DI container.
func (a *App) SecondFactorReset() (*secondfactorreset.UseCase, error) {
	return do.Invoke[*secondfactorreset.UseCase](a.injector)
}

// SignUp is the Sign-up use case from the DI container, which creates an Owner from a shell.
func (a *App) SignUp() (*signup.UseCase, error) {
	return do.Invoke[*signup.UseCase](a.injector)
}

// StaffMembers is the platform Staff members module from the DI container.
func (a *App) StaffMembers() (*staffmembers.Module, error) {
	return do.Invoke[*staffmembers.Module](a.injector)
}

// ServiceCredentials is the Enterprise Edition's Service credentials module, from the Edition the
// DI container builds.
func (a *App) ServiceCredentials() (*servicecredentials.Module, error) {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return nil, err
	}
	return edition.ServiceCredentials, nil
}

// SecretReencryption is the re-encryption of every stored secret from the DI container.
func (a *App) SecretReencryption() (SecretReencryption, error) {
	return do.Invoke[SecretReencryption](a.injector)
}

// Users is the product's Users module from the DI container, for harnesses that
// arrange users and Workspaces through the same instance the HTTP surface runs on
// (the end-to-end suite), so they open no second connection pool.
func (a *App) Users() (*users.Module, error) {
	return do.Invoke[*users.Module](a.injector)
}

// WorkspaceView is the product's Workspace view use case from the DI container, for the
// same harnesses: they create a Workspace for a User they made through it.
func (a *App) WorkspaceView() (*workspaceview.UseCase, error) {
	return do.Invoke[*workspaceview.UseCase](a.injector)
}

// Memberships is the product's Memberships module from the DI container, for the same harnesses:
// they scope to a Workspace and change its Roles through it.
func (a *App) Memberships() (*memberships.Module, error) {
	return do.Invoke[*memberships.Module](a.injector)
}

// Bus is the event bus from the DI container, for harnesses that must change data
// inside a transaction (events.Bus.WithinScopedTx) like the product's own write paths.
func (a *App) Bus() (*events.Bus, error) {
	return do.Invoke[*events.Bus](a.injector)
}

// Workspaces is the Workspaces module from the DI container, which resolves the Workspace id an
// operator names (`sphericon events`).
func (a *App) Workspaces() (*workspaces.Module, error) {
	return do.Invoke[*workspaces.Module](a.injector)
}

func (a *App) Shutdown(ctx context.Context) *do.ShutdownReport {
	a.shutdownOnce.Do(func() {
		a.shutdownReport = a.injector.ShutdownWithContext(ctx)
	})
	return a.shutdownReport
}
