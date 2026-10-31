// Package app composes the application: it registers every singleton in the dependency container
// and starts and stops the servers, the event router and the job workers.
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

	"github.com/samber/do/v2"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/preset"
	"github.com/getsphericon/sphericon/internal/rekey"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/sidelistener"
	"github.com/getsphericon/sphericon/internal/staffmembers"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/telemetry"
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

// Option customises how New builds the App.
type Option func(*options)

type options struct {
	listener      net.Listener
	e2eDKIMLookup bool
}

// ErrE2EOnly means an end-to-end-only option was passed to an app that is not the e2e profile.
var ErrE2EOnly = errors.New("app: option is only valid in the " + config.EnvE2E + " environment")

// WithE2EDKIMLookup swaps the DKIM DNS lookup for the development one, which echoes a
// Sending domain's own stored key, so the end-to-end suite verifies domains without
// real DNS while the development flag stays off. New refuses it outside the e2e
// profile, so it cannot be wired into a production configuration.
func WithE2EDKIMLookup() Option {
	return func(o *options) { o.e2eDKIMLookup = true }
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
	if o.e2eDKIMLookup && env != config.EnvE2E {
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
	if o.e2eDKIMLookup {
		// The e2e suite's domains self-verify through DevTXTLookup, which echoes a Sending
		// domain's own stored key.
		do.Override(injector, func(i do.Injector) (sending.TXTLookup, error) {
			return domainverify.DevTXTLookup(do.MustInvoke[*ent.Client](i)), nil
		})
	}

	// Set the process-wide locale for system emails and API validation messages
	// (internal/i18n). Instance-global, read-only after this point.
	i18n.Configure(cfg.Locale)

	handler, err := do.Invoke[Handler](injector)
	if err != nil {
		return nil, abort(injector, err)
	}
	// The Presets load at start: a broken custom file stops it here, not on a callback.
	if _, err := do.Invoke[*preset.Registry](injector); err != nil {
		return nil, abort(injector, err)
	}
	evRuntime, err := do.Invoke[*eventsRuntime](injector)
	if err != nil {
		return nil, abort(injector, err)
	}
	jobsCli, err := do.Invoke[*jobsClient](injector)
	if err != nil {
		return nil, abort(injector, err)
	}
	// Operational gauges (outbox lag, queue depth), read on scrape; the container takes them
	// off the meter when it shuts down.
	if _, err := do.Invoke[*outboxLagGauge](injector); err != nil {
		return nil, abort(injector, err)
	}
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
		events:   evRuntime,
		jobs:     jobsCli,
	}, nil
}

// ErrNotServable means Start was called on an app that has no servers (the command app).
var ErrNotServable = errors.New("app: this app has no servers to start")

// ErrAlreadyStarted means Start was called a second time; an App starts once.
var ErrAlreadyStarted = errors.New("app: already started")

// NewCommandApp builds the minimal app the commands need (config, database,
// ent client, system sender) without the HTTP server, the event router or the job
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
	// The first start creates the Instance state row and its random Instance id (ADR 0032).
	inst, err := do.Invoke[*instance.Module](a.injector)
	if err != nil {
		return err
	}
	if _, err := inst.Ensure(ctx); err != nil {
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
			slog.Error("metrics server stopped", "err", err)
		}
	}()
	// The internal listener is bound before anything else is served for the same
	// reason: a self-hoster who asked for it must not silently run without it.
	if err := a.Internal.Listen(ctx); err != nil {
		return err
	}
	go func() {
		if err := a.Internal.Serve(); err != nil {
			slog.Error("internal server stopped", "err", err)
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
	errs := []error{a.Server.Shutdown(ctx), a.Metrics.Shutdown(ctx), a.Internal.Shutdown(ctx)}
	if a.listener != nil {
		if err := a.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
func (a *App) SuspendWorkspace(ctx context.Context, publicID string, by suspension.Actor, reason string) (bool, error) {
	client, err := do.Invoke[*ent.Client](a.injector)
	if err != nil {
		return false, err
	}
	id, err := accounts.WorkspaceIDByPublicID(ctx, client, publicID)
	if err != nil {
		return false, err
	}
	bus, err := do.Invoke[*events.Bus](a.injector)
	if err != nil {
		return false, err
	}
	changed, err := suspension.SuspendWorkspace(ctx, bus, id, by, reason)
	if err != nil || !changed {
		return changed, err
	}
	sys, err := do.Invoke[SystemSender](a.injector)
	if err != nil {
		return true, fmt.Errorf("workspace suspended, but the owner notice could not be sent: %w", err)
	}
	cfg, err := do.Invoke[*config.Config](a.injector)
	if err != nil {
		return true, fmt.Errorf("workspace suspended, but the owner notice could not be sent: %w", err)
	}
	if err := suspension.NotifyOwners(ctx, client, accounts.OwnerNotifier(client, sys.EmailSender), cfg.SupportEmail, id); err != nil {
		return true, fmt.Errorf("workspace suspended, but the owner notice could not be sent: %w", err)
	}
	return true, nil
}

// UnsuspendWorkspace lifts a Workspace's suspension; held sends resume on their own.
func (a *App) UnsuspendWorkspace(ctx context.Context, publicID string, by suspension.Actor) (bool, error) {
	client, err := do.Invoke[*ent.Client](a.injector)
	if err != nil {
		return false, err
	}
	id, err := accounts.WorkspaceIDByPublicID(ctx, client, publicID)
	if err != nil {
		return false, err
	}
	bus, err := do.Invoke[*events.Bus](a.injector)
	if err != nil {
		return false, err
	}
	return suspension.UnsuspendWorkspace(ctx, bus, id, by)
}

// ResetSecondFactor clears the Second factor and Recovery codes of the User with
// this email and ends every session of theirs (ADR 0020), recorded as
// `user.second_factor_reset` by the Staff member "cli" in each of their Workspaces. It
// reports whether anything changed: a User without a Second factor is left as is.
func (a *App) ResetSecondFactor(ctx context.Context, email string) (bool, error) {
	client, err := do.Invoke[*ent.Client](a.injector)
	if err != nil {
		return false, err
	}
	id, err := client.User.Query().Where(user.Email(accounts.NormalizeEmail(email))).OnlyID(ctx)
	if err != nil {
		return false, fmt.Errorf("user %q: %w", email, err)
	}
	sf, err := do.Invoke[*secondfactor.Module](a.injector)
	if err != nil {
		return false, err
	}
	err = sf.ResetByStaff(ctx, id, "cli")
	if errors.Is(err, secondfactor.ErrNothingToReset) {
		return false, nil
	}
	return err == nil, err
}

// CreateOwner makes a Verified User with its first Workspace and a one-time password
// (ADR 0042): an Owner made from a shell, with no Sign-up.
func (a *App) CreateOwner(ctx context.Context, email, name string) (string, error) {
	acc, err := do.Invoke[*accounts.Accounts](a.injector)
	if err != nil {
		return "", err
	}
	return acc.CreateOwner(ctx, email, name)
}

// CreateStaffMember makes a platform Staff member (ADR 0026, ADR 0063) and returns its one-time
// password: the only way a Staff member comes to exist (there is no signup). It refuses a
// duplicate email and an instance whose Staff sign-in is off (no Staff member secret).
func (a *App) CreateStaffMember(ctx context.Context, email string) (string, error) {
	members, err := do.Invoke[*staffmembers.Module](a.injector)
	if err != nil {
		return "", err
	}
	return members.Create(ctx, email)
}

// ResetStaffTOTP clears the TOTP of the Staff member with this email and ends its
// sessions (ADR 0026): the only way a lost TOTP is reset. The Staff member re-enrols at next
// login. staffmembers.ErrNotFound for an unknown email.
func (a *App) ResetStaffTOTP(ctx context.Context, email string) error {
	members, err := do.Invoke[*staffmembers.Module](a.injector)
	if err != nil {
		return err
	}
	return members.ResetTOTP(ctx, email)
}

// IssueServiceCredential creates a Service credential (ADR 0026) and returns it with its
// value, shown once. Only the Staff member's command line makes one: no endpoint can.
func (a *App) IssueServiceCredential(ctx context.Context, in staff.IssueInput) (*staff.Issued, error) {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return nil, err
	}
	return edition.Staff.IssueServiceCredential(ctx, in)
}

// RotateServiceCredential issues a new credential with the name and scopes of an active
// one, leaving the old one active until it is revoked.
func (a *App) RotateServiceCredential(ctx context.Context, id int64, expiresAt *time.Time) (*staff.Issued, error) {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return nil, err
	}
	return edition.Staff.RotateServiceCredential(ctx, id, expiresAt)
}

// RevokeServiceCredential ends a Service credential.
func (a *App) RevokeServiceCredential(ctx context.Context, id int64) error {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return err
	}
	return edition.Staff.RevokeServiceCredential(ctx, id)
}

// ServiceCredentials lists the Service credentials with their state, narrowed to one
// service's by name when set.
func (a *App) ServiceCredentials(ctx context.Context, name string) ([]*ent.ServiceCredential, error) {
	edition, err := do.Invoke[*ee.Edition](a.injector)
	if err != nil {
		return nil, err
	}
	return edition.Staff.ServiceCredentials(ctx, name)
}

// Accounts is the product's Accounts module from the DI container, for harnesses that
// arrange users and Workspaces through the same instance the HTTP surface runs on
// (the end-to-end suite), so they open no second connection pool.
func (a *App) Accounts() (*accounts.Accounts, error) {
	return do.Invoke[*accounts.Accounts](a.injector)
}

// Bus is the event bus from the DI container, for harnesses that must change data
// inside a transaction (events.Bus.WithinScopedTx) like the product's own write paths.
func (a *App) Bus() (*events.Bus, error) {
	return do.Invoke[*events.Bus](a.injector)
}

func (a *App) Shutdown(ctx context.Context) *do.ShutdownReport {
	a.shutdownOnce.Do(func() {
		a.shutdownReport = a.injector.ShutdownWithContext(ctx)
	})
	return a.shutdownReport
}

// Reencrypt seals every stored secret under the primary key of the encryption keyset
// (`sphericon encryption-key reencrypt`) and returns what it did per column.
func (a *App) Reencrypt(ctx context.Context) ([]rekey.Count, error) {
	client, err := do.Invoke[*ent.Client](a.injector)
	if err != nil {
		return nil, err
	}
	cipher, err := do.Invoke[*secrets.Cipher](a.injector)
	if err != nil {
		return nil, err
	}
	return rekey.New(client, cipher).Run(ctx)
}
