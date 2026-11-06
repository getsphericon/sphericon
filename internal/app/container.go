package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/samber/do/v2"
	"go.opentelemetry.io/otel/metric"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accountlink"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	apiexternal "github.com/getsphericon/sphericon/internal/api/external"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/capturedmessages"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/infra/messaging/systemmail"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/instancestates"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/invitations"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/mailjobs"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/machine"
	"github.com/getsphericon/sphericon/internal/mcpserver"
	"github.com/getsphericon/sphericon/internal/memberships"
	"github.com/getsphericon/sphericon/internal/oauthclients"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/passkeys"
	"github.com/getsphericon/sphericon/internal/preset"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/setupchecklist"
	"github.com/getsphericon/sphericon/internal/signin"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/signupsurvey"
	"github.com/getsphericon/sphericon/internal/staffmembers"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/trackingdomains"
	"github.com/getsphericon/sphericon/internal/upgrade"
	"github.com/getsphericon/sphericon/internal/usecase/analyticsreport"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
	"github.com/getsphericon/sphericon/internal/usecase/customfieldretyping"
	"github.com/getsphericon/sphericon/internal/usecase/domainverification"
	"github.com/getsphericon/sphericon/internal/usecase/eventlog"
	"github.com/getsphericon/sphericon/internal/usecase/ingesthook"
	"github.com/getsphericon/sphericon/internal/usecase/integrationdeletion"
	"github.com/getsphericon/sphericon/internal/usecase/needsattention"
	"github.com/getsphericon/sphericon/internal/usecase/oauthauthorization"
	"github.com/getsphericon/sphericon/internal/usecase/releasecheck"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorenrolment"
	"github.com/getsphericon/sphericon/internal/usecase/secretreencryption"
	"github.com/getsphericon/sphericon/internal/usecase/sendingdomaindeletion"
	"github.com/getsphericon/sphericon/internal/usecase/stepexecution"
	"github.com/getsphericon/sphericon/internal/usecase/tagdeletion"
	"github.com/getsphericon/sphericon/internal/usecase/telemetryreport"
	"github.com/getsphericon/sphericon/internal/usecase/transactionalsend"
	"github.com/getsphericon/sphericon/internal/users"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
	"github.com/getsphericon/sphericon/internal/workspaces"

	// The pgx driver behind database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Container is the dependency container of an Instance over cfg: every singleton registered,
// none built yet (providers are lazy, so a process builds only what it invokes). It is the one
// composition root: the server, the commands and the test harness all build from it.
//
// The seams below are the services a test harness replaces with do.Override or
// do.OverrideValue before it invokes anything (an override does not reach a dependent that is
// already built): Clock, Version, Database, SystemSender, Enqueuer, SigninOptions,
// IntegrationOptions, OAuthFetcher, *JobsUI, and the plain types *licensekey.License,
// *credentials.Hasher, *messaging.Catalog, outbound.Senders, domainname.TXTLookup, machine.Source,
// *preset.Registry, *emaildoc.Renderer, *db.SchemaWatch and apisite.Migrations. Every other
// difference from production is configuration.
//
// A provider takes what it needs with do.MustInvoke while it builds and keeps no injector: a
// lookup made later adds an edge the container did not see at build time, and a cycle of such
// edges both fails the lookup and undoes the shutdown order. Two services that need each other
// are a design to change, not a lookup to defer.
func Container(cfg *config.Config) *do.RootScope {
	injector := do.New(
		storagePackage,
		messagingPackage,
		staffPackage,
		editionPackage,
		jobsPackage,
		deliveryPackage,
		identityPackage,
		audiencePackage,
		contentPackage,
		surfacesPackage,
	)
	do.ProvideValue(injector, cfg)
	do.ProvideValue(injector, Clock(time.Now))
	do.ProvideValue(injector, Version(buildinfo.Current().Version))
	do.ProvideValue(injector, SigninOptions{})
	do.ProvideValue(injector, IntegrationOptions{})
	return injector
}

// Clock is the one clock of the Instance: every module that reads the time reads this one.
type Clock func() time.Time

// Version is the running version the Instance module judges releases against.
type Version string

// Database is the main database/sql pool, which the ent client and the event bus
// share (the job queue has a pool of its own).
type Database struct {
	*sql.DB
}

// Shutdown closes the pool, after everything built on it.
func (d Database) Shutdown() error {
	return d.Close()
}

// SystemSender is sphericon's own (platform) transactional email sender, built from config
// via the messaging catalog. Distinct from a Workspace's customer sender.
type SystemSender struct {
	messaging.EmailSender
}

// Enqueuer is the enqueue side of the job queue the domain modules and the /site handlers
// queue work through: an insert-only jobs.Queue in production, which needs none of what the
// workers drive, and a test adapter that works each job at once through its real worker in
// tests.
type Enqueuer interface {
	mailjobs.Enqueuer
	integrations.QuotaEnqueuer
}

// SigninOptions configure the Sign-in providers (ADR 0048); none in production.
type SigninOptions []signin.Option

// IntegrationOptions configure the Integrations module; none in production.
type IntegrationOptions []integrations.Option

// OAuthFetcher downloads OAuth Client ID Metadata Documents (ADR 0016).
type OAuthFetcher struct {
	webhook.Doer
}

// Handler is the public listener's handler, every surface mounted on it.
type Handler struct {
	http.Handler
}

// InternalHandler is the internal listener's handler: the TLS ask always, /service with the
// `staff` license (ADR 0026, ADR 0038).
type InternalHandler struct {
	http.Handler
}

// riverPool is the database/sql pool river runs on (separate from the one the ent client and
// pubsub use, so job workers cannot starve the API).
// listener is the one connection river LISTENs on for job notifications (db.OpenRiverListener).
type riverPool struct {
	*sql.DB

	listener *pgxpool.Pool
	metrics  metric.Registration
}

func (p *riverPool) Shutdown() {
	cleanup.Report("unregister the pool metrics", p.metrics.Unregister())
	p.listener.Close()
	cleanup.Close(p, "the job queue pool")
}

// jobsClient is the river client that works the jobs; work is queued through the Enqueuer.
type jobsClient struct {
	*jobs.Client
}

// Shutdown stops the client: running jobs get jobs.SoftStopTimeout to finish and are cancelled
// after it; the context bounds the wait for the cancelled ones to return.
func (j *jobsClient) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), jobs.SoftStopTimeout+5*time.Second)
	defer cancel()
	return j.Stop(ctx)
}

// gauge is an operational gauge read on scrape (queue depth and lag); Shutdown takes its
// callback off the global meter before the pool it reads closes.
type gauge struct {
	unregister func() error
}

func (g *gauge) Shutdown() error { return g.unregister() }

type queueDepthGauge struct{ gauge }

// externalAPI is the external (/api) ogen server; the MCP surface dispatches through it.
type externalAPI struct{ http.Handler }

// mcpHandler is the /mcp surface (ADR 0016), projected from the external contract.
type mcpHandler struct{ http.Handler }

// JobsUI is the job queue's UI (River UI) the Staff console links to; Shutdown stops its
// background queries.
type JobsUI struct {
	http.Handler

	stop context.CancelFunc
}

func (u *JobsUI) Shutdown() {
	if u.stop != nil {
		u.stop()
	}
}

// staffHandler is the /staff surface (ADR 0026, ADR 0063); nil inside while the Staff sign-in
// is off, which server.New mounts as a 404.
type staffHandler struct{ http.Handler }

// storagePackage provides the Presets, the database and the ent client.
var storagePackage = do.Package(
	// The Presets of the Instance: the built-in ones and the Self-hoster's directory.
	do.Lazy(func(i do.Injector) (*preset.Registry, error) {
		cfg := do.MustInvoke[*config.Config](i)
		if cfg.PresetsDir == "" {
			return preset.Load(nil)
		}
		if _, err := os.Stat(cfg.PresetsDir); err != nil {
			return nil, fmt.Errorf("SPHERICON_PRESETS_DIR: %w", err)
		}
		return preset.Load(os.DirFS(cfg.PresetsDir))
	}),
	do.Lazy(func(i do.Injector) (Database, error) {
		cfg := do.MustInvoke[*config.Config](i)
		database, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			return Database{}, err
		}
		db.ConfigurePool(database, cfg.DBPool)
		return Database{DB: database}, nil
	}),
	// The long-lived client carries the Edition's ent hooks from the start, like the event bus's
	// transaction client, so no path writes on a client without them. Closing it would close the
	// pool under Database, which shuts down after it, so it needs no Shutdown of its own.
	do.Lazy(func(i do.Injector) (*ent.Client, error) {
		client := entclient.New(do.MustInvoke[Database](i).DB)
		client.Use(ee.Hooks(do.MustInvoke[*licensekey.License](i), do.MustInvoke[Clock](i))...)
		return client, nil
	}),
	// The development schema guard's view of the migrations the database has not run (ADR 0033):
	// one per process, so a migration run from the error page clears what SchemaGuard cached.
	do.Lazy(func(i do.Injector) (*db.SchemaWatch, error) {
		return db.NewSchemaWatch(do.MustInvoke[Database](i).DB), nil
	}),
)

// messagingPackage provides the email providers, the credential cipher and the senders.
var messagingPackage = do.Package(
	// The provider catalog (registry of email providers).
	do.Lazy(func(i do.Injector) (*messaging.Catalog, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return registry.New(cfg.SMTPAllowPrivateHosts, cfg.TelegramBotAPIURL, cfg.TwilioAPIURL), nil
	}),
	// The Tink cipher (credential encryption); fails fast at boot on a bad key.
	do.Lazy(func(i do.Injector) (*secrets.Cipher, error) {
		return secrets.NewCipher(do.MustInvoke[*config.Config](i).EncryptionKey)
	}),
	// The Test inbox keeps its messages here; the resolver binds it to a Workspace and an
	// Integration, and the site API reads it.
	do.Lazy(func(i do.Injector) (*capturedmessages.Module, error) {
		return capturedmessages.New(do.MustInvoke[*config.Config](i).TestInboxMaxMessages), nil
	}),
	// The workspace→sender resolver.
	do.Lazy(func(i do.Injector) (*messaging.Resolver, error) {
		captured := do.MustInvoke[*capturedmessages.Module](i)
		return messaging.NewResolver(do.MustInvoke[*secrets.Cipher](i), do.MustInvoke[*messaging.Catalog](i), messaging.WithCapturers(captured.Capturer)), nil
	}),
	// What Outbound send sends a Workspace's mail through.
	do.Lazy(func(i do.Injector) (outbound.Senders, error) {
		return do.MustInvoke[*messaging.Resolver](i), nil
	}),
	do.Lazy(func(i do.Injector) (SystemSender, error) {
		// The platform's own SMTP relay is self-hoster-configured, not tenant input, so it may
		// be a private host even where Workspace Integrations may not.
		sender, err := systemmail.Sender(do.MustInvoke[*config.Config](i), registry.Default())
		if err != nil {
			return SystemSender{}, err
		}
		return SystemSender{EmailSender: sender}, nil
	}),
	// How a Workspace's owners are told something: by mail through the platform's own sender,
	// never the Workspace's Integration, which a hold may freeze.
	do.Lazy(func(i do.Injector) (memberships.Notify, error) {
		return memberships.OwnerNotifier(do.MustInvoke[*ent.Client](i), do.MustInvoke[SystemSender](i).EmailSender), nil
	}),
	// The DKIM DNS resolver of Sending domain and Join domain verification (ADR 0010): published
	// DNS records on every Instance; a development stack points the resolver at its own DNS
	// server (SPHERICON_DNS_ADDR) that publishes the Brightpath demo records. The e2e suite's
	// domains self-verify (WithE2EDKIMLookup).
	do.Lazy(func(i do.Injector) (domainname.TXTLookup, error) {
		return dnsResolver(do.MustInvoke[*config.Config](i).DNSAddr).LookupTXT, nil
	}),
)

// staffPackage provides the Staff member module, the Sign-up items and the Consent records.
var staffPackage = do.Package(
	// The Staff member (ADR 0026, ADR 0063): its store, sign-in and sessions, on every
	// Instance; the sign-in is off without SPHERICON_STAFF_JWT_SECRET.
	do.Lazy(func(i do.Injector) (*staffmembers.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return staffmembers.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*secrets.Cipher](i), cfg.JWTSecrets(), staffmembers.Config{
			Secret: cfg.StaffJWTSecret, SessionTTL: cfg.StaffSessionTTL, SecureCookies: cfg.SecureCookies(), Clock: do.MustInvoke[Clock](i),
			Attempts: do.MustInvoke[*throttle.Attempts](i), Passwords: do.MustInvoke[*credentials.Hasher](i),
		})
	}),
	// The Sign-up items and their Legal documents (ADR 0062), on every Instance.
	do.Lazy(func(i do.Injector) (*signupitems.Module, error) {
		return signupitems.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i), do.MustInvoke[Clock](i)), nil
	}),
	// The Users' Consent records on those items (ADR 0062).
	do.Lazy(func(i do.Injector) (*consentrecords.Module, error) {
		return consentrecords.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i), do.MustInvoke[*signupitems.Module](i)), nil
	}),
)

// requireStaffLicenseForSaaS refuses a saas Instance without the `staff` license (ADR 0060): the
// Mode unlocks nothing, so without the key the Instance would run with no Staff surface at all.
func requireStaffLicenseForSaaS(mode config.Mode, lic *licensekey.License) error {
	if mode.SaaS() && !lic.Has(licensekey.FeatureStaff) {
		return errors.New("SPHERICON_MODE=saas needs a license key with the `staff` feature in SPHERICON_LICENSE_KEY")
	}
	return nil
}

// editionPackage provides the Enterprise Edition and the domain-event system.
var editionPackage = do.Package(
	// The runtime license key (ADR 0014). An empty or expired key is the plain core
	// (ADR 0035); a malformed or forged one fails boot.
	do.Lazy(func(i do.Injector) (*licensekey.License, error) {
		cfg := do.MustInvoke[*config.Config](i)
		lic, err := licensekey.Load(cfg.LicenseKey, licensekey.VerifyingKey(cfg.TrustsDevLicense()), do.MustInvoke[Clock](i)(), slog.Default())
		if err != nil {
			return nil, err
		}
		if err := requireStaffLicenseForSaaS(cfg.Mode, lic); err != nil {
			return nil, err
		}
		return lic, nil
	}),
	// The Enterprise Edition (ADR 0014): what the license key switches on.
	do.Lazy(func(i do.Injector) (*ee.Edition, error) {
		cfg := do.MustInvoke[*config.Config](i)
		members := do.MustInvoke[*staffmembers.Module](i)
		return ee.New(ee.Deps{
			Client:        do.MustInvoke[*ent.Client](i),
			Bus:           do.MustInvoke[*events.Bus](i),
			License:       do.MustInvoke[*licensekey.License](i),
			StaffSessions: members.Sessions(),
			Notify:        do.MustInvoke[memberships.Notify](i),
			Suspender:     do.MustInvoke[*suspension.Suspender](i),
			Reputation:    do.MustInvoke[*reputation.Module](i),
		}, staff.Config{
			Abuse:         abuse.LimitsFromPercent(cfg.AbuseVolumeFloor, cfg.AbuseWarnComplaintPercent, cfg.AbuseWarnBouncePercent, cfg.AbuseFreezeComplaintPercent, cfg.AbuseFreezeBouncePercent),
			SupportEmail:  cfg.SupportEmail,
			HoldMaxExpiry: cfg.StaffHoldMaxExpiry,
			Clock:         do.MustInvoke[Clock](i),
			SendCaps:      do.MustInvoke[providerlimit.SendCaps](i),
		})
	}),
	// A Workspace's freezes on the send chokepoint (ADR 0009, 0015), gated by the license: one
	// module for the send path and every read of a hold shown to a Workspace.
	do.Lazy(func(i do.Injector) (*holds.Holds, error) {
		return do.MustInvoke[*ee.Edition](i).Holds(), nil
	}),
	// The one sequence that suspends a Workspace and tells its owners (ADR 0007): the command
	// line, the Staff console and the abuse detector all suspend through it.
	do.Lazy(func(i do.Injector) (*suspension.Suspender, error) {
		return suspension.NewSuspender(do.MustInvoke[*events.Bus](i), do.MustInvoke[*ent.Client](i), do.MustInvoke[memberships.Notify](i),
			do.MustInvoke[*config.Config](i).SupportEmail), nil
	}),
	do.Lazy(func(i do.Injector) (*orders.Module, error) {
		return orders.New(do.MustInvoke[*workspaces.Module](i)), nil
	}),
	// The domain-event bus over the main pool: a publish stores the Event and queues its jobs.
	do.Lazy(func(i do.Injector) (*events.Bus, error) {
		database := do.MustInvoke[Database](i)
		bus := events.New(database.DB, entclient.NewTx)
		// The Edition's ent hooks on every transaction the bus opens; the Edition itself
		// is built after the bus, which its Audit log records through.
		bus.Use(ee.Hooks(do.MustInvoke[*licensekey.License](i), do.MustInvoke[Clock](i))...)
		inserter, err := jobs.NewInserter(database.DB)
		if err != nil {
			return nil, err
		}
		bus.UseJobs(inserter)
		// The consumers of the domain events: a publish inserts a job for each that wants it.
		bus.UseConsumers(append(jobs.EventConsumers(), ee.EventConsumers()...)...)
		// A provider's bounce or complaint counts on the send-outcome counter once it is stored.
		bus.OnStored(ingesthook.RecordOutcome)
		return bus, nil
	}),
)

// trackingProbeTimeout bounds one HTTPS probe of a Tracking domain.
const trackingProbeTimeout = 10 * time.Second

// jobsPackage provides the job queue: its connection pool, its client and its gauges.
var jobsPackage = do.Package(
	do.Lazy(func(i do.Injector) (*riverPool, error) {
		cfg := do.MustInvoke[*config.Config](i)
		database := do.MustInvoke[Database](i)
		pool, err := db.OpenRiverDB(cfg.DatabaseURL, cfg.DBPool)
		if err != nil {
			return nil, err
		}
		listener, err := db.OpenRiverListener(context.Background(), cfg.DatabaseURL)
		if err != nil {
			cleanup.Close(pool, "the job queue pool")
			return nil, err
		}
		reg, err := db.RegisterPoolMetrics(database.DB, pool)
		if err != nil {
			listener.Close()
			cleanup.Close(pool, "the job queue pool")
			return nil, err
		}
		return &riverPool{DB: pool, listener: listener, metrics: reg}, nil
	}),
	// The queue depth gauge reads on scrape through the job queue's pool.
	do.Lazy(func(i do.Injector) (*queueDepthGauge, error) {
		unregister, err := jobs.RegisterQueueMetrics(do.MustInvoke[*riverPool](i).DB)
		if err != nil {
			return nil, err
		}
		return &queueDepthGauge{gauge{unregister}}, nil
	}),
	// The Workspaces' Tracking domains (ADR 0071): their CNAMEs are checked against the configured
	// edge host, and the feature is off while SPHERICON_TRACKING_CNAME_TARGET is unset (ADR 0038).
	do.Lazy(func(i do.Injector) (*trackingdomains.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		client := do.MustInvoke[*ent.Client](i)
		var checker *trackingdomains.Checker
		if cfg.TrackingCNAMETarget != "" {
			checker = &trackingdomains.Checker{
				Lookup: dnsResolver(cfg.DNSAddr).LookupCNAME,
				Target: cfg.TrackingCNAMETarget,
				// The SSRF-guarded client: a customer's DNS names the host the probe dials.
				HTTP:   webhook.NewClient(trackingProbeTimeout),
				Secret: cfg.JWTSecret,
			}
		}
		return trackingdomains.New(client, do.MustInvoke[*events.Bus](i), do.MustInvoke[memberships.Notify](i), trackingdomains.Config{
			Checker:  checker,
			OwnHosts: []string{urlHost(cfg.AppURL), urlHost(cfg.TrackerURL)},
			Now:      do.MustInvoke[Clock](i),
		}), nil
	}),
	// The Instance's report of itself (ADR 0032), previewed in the Staff console and sent by a
	// periodic job.
	do.Lazy(func(i do.Injector) (*telemetryreport.UseCase, error) {
		cfg := do.MustInvoke[*config.Config](i)
		lapsedAt, _ := do.MustInvoke[*licensekey.License](i).LapsedAt()
		host := telemetryreport.HostOf(cfg, string(do.MustInvoke[Version](i)), lapsedAt)
		return telemetryreport.New(do.MustInvoke[*instancestates.Module](i), host, cfg, do.MustInvoke[Clock](i))
	}),
	// The Release check (ADR 0032): a daily job, the line at start and the banner of an owner.
	do.Lazy(func(i do.Injector) (*releasecheck.UseCase, error) {
		source, err := upgrade.NewGitHubAt(do.MustInvoke[*config.Config](i).GitHubAPIURL)
		if err != nil {
			return nil, err
		}
		return releasecheck.New(do.MustInvoke[*instancestates.Module](i), source), nil
	}),
	do.Lazy(func(i do.Injector) (*jobsClient, error) {
		cfg := do.MustInvoke[*config.Config](i)
		edition := do.MustInvoke[*ee.Edition](i)
		pool := do.MustInvoke[*riverPool](i)
		jc, err := jobs.NewClient(riverdatabasesql.NewWithPgxListener(pool.DB, pool.listener), jobs.Deps{
			Ent:                do.MustInvoke[*ent.Client](i),
			Bus:                do.MustInvoke[*events.Bus](i),
			Outbound:           do.MustInvoke[*outbound.Module](i),
			Cipher:             do.MustInvoke[*secrets.Cipher](i),
			SystemSender:       do.MustInvoke[SystemSender](i).EmailSender,
			Catalog:            do.MustInvoke[*messaging.Catalog](i),
			Rules:              do.MustInvoke[*rules.Module](i),
			Users:              do.MustInvoke[*users.Module](i),
			Orders:             do.MustInvoke[*orders.Module](i),
			Deletion:           do.MustInvoke[*workspacedeletion.Module](i),
			DomainVerification: do.MustInvoke[*domainverification.UseCase](i),
			StepExecution:      do.MustInvoke[*stepexecution.UseCase](i),
			JoinDomains:        do.MustInvoke[*joindomains.Module](i),
			TrackingDomains:    do.MustInvoke[*trackingdomains.Module](i),
			Machine:            do.MustInvoke[*machine.Detector](i),
			Clock:              do.MustInvoke[Clock](i),
			AppURL:             cfg.AppURL,
			OutboundAllow:      cfg.OutboundAllow,
			Retention:          jobs.Retention{Events: cfg.EventsRetention, UnverifiedUsers: cfg.UnverifiedUserRetention, CapturedMessages: cfg.TestInboxRetention},
			Webhooks:           edition.Webhooks,
		}, edition.Jobs(), edition.ExportJobs(cfg.EventsRetention), do.MustInvoke[*releasecheck.UseCase](i).Jobs(), do.MustInvoke[*telemetryreport.UseCase](i).Jobs())
		if err != nil {
			return nil, err
		}
		return &jobsClient{Client: jc}, nil
	}),
	// The enqueue side implements every enqueue seam: the platform mails and the Integration quota
	// refresh. It inserts over the main pool, like the bus, and works nothing, so what queues work
	// never depends on the job client or on what its workers drive.
	do.Lazy(func(i do.Injector) (Enqueuer, error) {
		return jobs.NewQueue(do.MustInvoke[Database](i).DB)
	}),
)

// deliveryPackage provides the link tokens, machine detection, rate limits and Outbound send.
var deliveryPackage = do.Package(
	// The signed-token authority for tracking, unsubscribe and confirmation links:
	// one instance, so every surface mints and verifies with the same secret.
	do.Lazy(func(i do.Injector) (*tracking.Tracker, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return tracking.New(cfg.JWTSecret, cfg.AppURL, cfg.JWTVerifySecrets()...), nil
	}),
	// Apple's published Private Relay list, which the refresh job reads.
	do.Lazy(func(do.Injector) (machine.Source, error) {
		return machine.NewHTTPSource(), nil
	}),
	// Machine engagement (ADR 0053): the detector tracking judges hits by and the refresh job
	// keeps current.
	do.Lazy(func(i do.Injector) (*machine.Detector, error) {
		return machine.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[machine.Source](i)), nil
	}),
	// Whether the Instance has Send caps (ADR 0067): the saas Mode has them. It is decided here,
	// once, and every module that enforces, shows or sets a Send cap takes this one value.
	do.Lazy(func(i do.Injector) (providerlimit.SendCaps, error) {
		return providerlimit.SendCaps(do.MustInvoke[*config.Config](i).Mode.SaaS()), nil
	}),
	// The request budgets of the process (ADR 0025): one limiter for both listeners, so a
	// client's budget is the same whichever one it reaches.
	do.Lazy(func(i do.Injector) (*ratelimit.Limiter, error) {
		return ratelimit.New(do.MustInvoke[*config.Config](i).RateLimits), nil
	}),
	// The Outbound send module (ADR 0015): the one place an email leaves sphericon on a
	// Workspace's behalf, for every send surface.
	do.Lazy(func(i do.Injector) (*outbound.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		opts := []outbound.Option{
			outbound.WithStartLinks(do.MustInvoke[*bindings.Module](i)),
			outbound.WithTestSendBudget(do.MustInvoke[*throttle.Attempts](i)),
			outbound.WithSendCaps(do.MustInvoke[providerlimit.SendCaps](i)),
			outbound.WithTelegramBots(do.MustInvoke[*messaging.Resolver](i)),
			outbound.WithSMSSenders(do.MustInvoke[*messaging.Resolver](i), do.MustInvoke[*integrations.Module](i)),
		}
		if cfg.SystemMailConfigured() {
			opts = append(opts, outbound.WithSystemSender(do.MustInvoke[SystemSender](i).EmailSender))
		}
		if cfg.TrackingCNAMETarget != "" {
			opts = append(opts, outbound.WithTrackingDomains(do.MustInvoke[*trackingdomains.Module](i)))
		}
		return outbound.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*holds.Holds](i), do.MustInvoke[outbound.Senders](i), do.MustInvoke[*tracking.Tracker](i), opts...), nil
	}),
)

// identityPackage provides the Users, Invitations, Workspace, Membership and Instance state modules,
// the Login throttle and the second factor.
var identityPackage = do.Package(
	// One password hasher for every password a User or a Staff member sets.
	do.Lazy(func(do.Injector) (*credentials.Hasher, error) {
		return credentials.NewHasher(), nil
	}),
	// The Workspace with its settings (ADR 0102), over the raw client: the tenant row has no scoped
	// wrapper.
	do.Lazy(func(i do.Injector) (*workspaces.Module, error) {
		return workspaces.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i)), nil
	}),
	// The Memberships (ADR 0102): the lookup of a Workspace's scope and the changes of a Role.
	do.Lazy(func(i do.Injector) (*memberships.Module, error) {
		return memberships.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i)), nil
	}),
	// The Users (ADR 0102), over the raw client, with the creation of a Workspace until it is a
	// use case. On an Instance that has Send caps (ADR 0067) a new Workspace starts with the Instance's
	// default; elsewhere it has none.
	do.Lazy(func(i do.Injector) (*users.Module, error) {
		return users.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i), do.MustInvoke[*credentials.Hasher](i), do.MustInvoke[*consentrecords.Module](i),
			users.WithSendCaps(do.MustInvoke[providerlimit.SendCaps](i), instancestates.DefaultSendCapIn)), nil
	}),
	// The Invitations (ADR 0102): their lookup by token takes the raw client.
	do.Lazy(func(i do.Injector) (*invitations.Module, error) {
		return invitations.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i)), nil
	}),
	// The Workspace deletion cycle (ADR 0043), over the raw client.
	do.Lazy(func(i do.Injector) (*workspacedeletion.Module, error) {
		return workspacedeletion.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i)), nil
	}),
	// The Instance state (ADR 0032), set by a Staff member in the console (ADR 0063), over the raw
	// client.
	do.Lazy(func(i do.Injector) (*instancestates.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		lapsedAt, _ := do.MustInvoke[*licensekey.License](i).LapsedAt()
		env := instancestates.EnvironmentOf(cfg, string(do.MustInvoke[Version](i)), lapsedAt)
		return instancestates.New(do.MustInvoke[*ent.Client](i), env, do.MustInvoke[Clock](i),
			instancestates.WithSendCaps(do.MustInvoke[providerlimit.SendCaps](i))), nil
	}),
	// The Login throttle and the mail budgets of an address (ADR 0025), over the raw client.
	do.Lazy(func(i do.Injector) (*throttle.Attempts, error) {
		return throttle.NewAttempts(do.MustInvoke[*ent.Client](i),
			throttle.WithClock(do.MustInvoke[Clock](i)),
			throttle.WithRateLimits(do.MustInvoke[*config.Config](i).RateLimits)), nil
	}),
	do.Lazy(func(i do.Injector) (*secondfactorenrolment.UseCase, error) {
		return secondfactorenrolment.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i), do.MustInvoke[*secrets.Cipher](i), do.MustInvoke[Clock](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*passkeys.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return passkeys.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i), cfg.AppURL, cfg.JWTSecret, do.MustInvoke[Clock](i))
	}),
)

// audiencePackage provides the domain modules of the audience and its integrations. Each is
// built once and shared by /site, /api and /mcp, so the surfaces cannot diverge on how a module
// is constructed.
var audiencePackage = do.Package(
	do.Lazy(func(i do.Injector) (*segments.Module, error) {
		return segments.New(do.MustInvoke[*events.Bus](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*contacts.Module, error) {
		return contacts.New(do.MustInvoke[*events.Bus](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*contacterasure.UseCase, error) {
		return contacterasure.New(do.MustInvoke[*events.Bus](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*integrations.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return integrations.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*secrets.Cipher](i), do.MustInvoke[*messaging.Catalog](i), do.MustInvoke[Enqueuer](i),
			do.MustInvoke[*preset.Registry](i), cfg.AppURL, do.MustInvoke[IntegrationOptions](i)...), nil
	}),
	do.Lazy(func(i do.Injector) (*integrationdeletion.UseCase, error) {
		return integrationdeletion.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*integrations.Module](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*bindings.Module, error) {
		return bindings.New(do.MustInvoke[*integrations.Module](i), do.MustInvoke[Clock](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*sendingdomains.Module, error) {
		return sendingdomains.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*secrets.Cipher](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*sendingdomaindeletion.UseCase, error) {
		return sendingdomaindeletion.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*sendingdomains.Module](i), do.MustInvoke[*messaging.Resolver](i)), nil
	}),
	// The Sending domain checks: the Verify button a person waits for and the re-check jobs (ADR 0010).
	do.Lazy(func(i do.Injector) (*domainverification.UseCase, error) {
		return domainverification.New(do.MustInvoke[*sendingdomains.Module](i), do.MustInvoke[domainname.TXTLookup](i), do.MustInvoke[memberships.Notify](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*joindomains.Module, error) {
		return joindomains.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[*events.Bus](i), do.MustInvoke[domainname.TXTLookup](i), do.MustInvoke[Clock](i),
			do.MustInvoke[memberships.Notify](i)), nil
	}),
)

// contentPackage provides the domain modules of content, automation and sending.
var contentPackage = do.Package(
	// One document renderer for every save that holds a document; the server warms it at start.
	do.Lazy(func(do.Injector) (*emaildoc.Renderer, error) {
		return emaildoc.New(), nil
	}),
	do.Lazy(func(i do.Injector) (*templates.Module, error) {
		return templates.New(do.MustInvoke[*emaildoc.Renderer](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*brandkit.Module, error) {
		return brandkit.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*templates.Module](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*tags.Module, error) {
		return tags.New(do.MustInvoke[*events.Bus](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*tagdeletion.UseCase, error) {
		return tagdeletion.New(do.MustInvoke[*events.Bus](i)), nil
	}),
	do.Lazy(func(_ do.Injector) (*customfieldretyping.UseCase, error) {
		return customfieldretyping.New(), nil
	}),
	// The transactional send surface of /api: a Template sent through Outbound send.
	do.Lazy(func(i do.Injector) (*transactionalsend.UseCase, error) {
		return transactionalsend.New(do.MustInvoke[*templates.Module](i), do.MustInvoke[*outbound.Module](i)), nil
	}),
	// Webhooks: the audit.entry rule asks the Edition whether the license is active.
	do.Lazy(func(i do.Injector) (*webhooks.Module, error) {
		cfg := do.MustInvoke[*config.Config](i)
		client := webhook.NewClientAllowing(webhooks.TestSendTimeout, cfg.OutboundAllow)
		return webhooks.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*secrets.Cipher](i), do.MustInvoke[*ee.Edition](i).Audit, client, nil), nil
	}),
	// Publish asks the Send route of Outbound send whether the email steps can send.
	do.Lazy(func(i do.Injector) (*automations.Module, error) {
		return automations.New(do.MustInvoke[*secrets.Cipher](i), do.MustInvoke[*events.Bus](i), do.MustInvoke[*emaildoc.Renderer](i), do.MustInvoke[*outbound.Module](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*rules.Module, error) {
		return rules.New(do.MustInvoke[*secrets.Cipher](i), do.MustInvoke[*events.Bus](i)), nil
	}),
	// `sphericon encryption-key reencrypt`: each owner sweeps its stored secrets on the raw client.
	do.Lazy(func(i do.Injector) (*secretreencryption.UseCase, error) {
		client := do.MustInvoke[*ent.Client](i)
		return secretreencryption.New(secretreencryption.Owners{
			Integrations:     integrations.NewSecrets(client),
			WebhookEndpoints: webhooks.NewSecrets(client),
			SendingDomains:   sendingdomains.NewSecrets(client),
			Users:            users.NewSecrets(client),
			StaffMembers:     staffmembers.NewSecrets(client),
			Automations:      automations.NewSecrets(client),
			Rules:            rules.NewSecrets(client),
		}, do.MustInvoke[*secrets.Cipher](i)), nil
	}),
	// The steps of the Enrollments (the run_step and http_step jobs): a send is fenced against an
	// exit of its Enrollment on the raw client.
	do.Lazy(func(i do.Injector) (*stepexecution.UseCase, error) {
		return stepexecution.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*outbound.Module](i), automations.FenceOn(do.MustInvoke[*ent.Client](i))), nil
	}),
	do.Lazy(func(i do.Injector) (*eventlog.UseCase, error) {
		policy := eventlog.LivePolicy{Window: do.MustInvoke[*config.Config](i).EventLiveWindow, Now: do.MustInvoke[Clock](i)}
		return eventlog.New(do.MustInvoke[*events.Bus](i), policy), nil
	}),
	do.Lazy(func(i do.Injector) (*setupchecklist.Module, error) {
		return setupchecklist.New(do.MustInvoke[*workspaces.Module](i)).WithClock(do.MustInvoke[Clock](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*needsattention.UseCase, error) {
		watch := do.MustInvoke[*ee.Edition](i).AbuseWatch()
		return needsattention.New(watch, do.MustInvoke[providerlimit.SendCaps](i), do.MustInvoke[*outbound.Module](i)).WithClock(do.MustInvoke[Clock](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*analyticsreport.UseCase, error) {
		return analyticsreport.New().WithClock(do.MustInvoke[Clock](i)).WithRetention(do.MustInvoke[*config.Config](i).EventsRetention), nil
	}),
	do.Lazy(func(do.Injector) (*reputation.Module, error) {
		return reputation.New(), nil
	}),
	// The broadcasts module claims a Broadcast and queues its send job on the bus's transaction;
	// Schedule and the progress estimate ask the Send route of Outbound send.
	do.Lazy(func(i do.Injector) (*broadcasts.Module, error) {
		return broadcasts.New(do.MustInvoke[*events.Bus](i), do.MustInvoke[*emaildoc.Renderer](i), do.MustInvoke[*outbound.Module](i)), nil
	}),
)

// surfacesPackage provides the API surfaces, their Deps and the handlers they are mounted on.
var surfacesPackage = do.Package(
	do.Lazy(func(i do.Injector) (*authtoken.Signer, error) {
		return authtoken.New(do.MustInvoke[*config.Config](i).JWTSecret).WithClock(do.MustInvoke[Clock](i)), nil
	}),
	// The site session cookie's issuer (ADR 0020): the login operation is the one route that
	// mints a session.
	do.Lazy(func(i do.Injector) (*apiauth.Sessions, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return apiauth.NewSessions(cfg.JWTSecret, cfg.SessionTTL, cfg.SecureCookies(), do.MustInvoke[Clock](i), cfg.JWTVerifySecrets()...), nil
	}),
	do.Lazy(func(i do.Injector) (OAuthFetcher, error) {
		return OAuthFetcher{oauthclients.NewFetcher(do.MustInvoke[*config.Config](i).OutboundAllow)}, nil
	}),
	// OAuth 2.1 for MCP connectors (ADR 0016).
	do.Lazy(func(i do.Injector) (*oauthclients.Module, error) {
		return oauthclients.New(do.MustInvoke[*ent.Client](i), do.MustInvoke[OAuthFetcher](i).Doer), nil
	}),
	// The Sign-in providers the Instance enabled (ADR 0048).
	do.Lazy(func(i do.Injector) (*signin.Providers, error) {
		return signin.FromConfig(do.MustInvoke[*config.Config](i), do.MustInvoke[Clock](i), do.MustInvoke[SigninOptions](i)...), nil
	}),
	// The error page's button that runs the migrations: a development build only (ADR 0033), nil
	// elsewhere.
	do.Lazy(func(i do.Injector) (apisite.Migrations, error) {
		cfg := do.MustInvoke[*config.Config](i)
		if !cfg.MigratesFromThePage() {
			return nil, nil
		}
		return db.NewPageMigrations(cfg.DatabaseURL, cfg.MigrateTimeout, do.MustInvoke[*db.SchemaWatch](i)), nil
	}),
	do.Lazy(externalDeps),
	do.Lazy(siteDeps),
	// The Bearer-token handler of /api and of /mcp, which rides the same token auth: one, so
	// both meet the same acceptance gate and the same clock.
	do.Lazy(func(i do.Injector) (*apiauth.ExternalSecurityHandler, error) {
		return server.ExternalSecurity(do.MustInvoke[*ent.Client](i), do.MustInvoke[apiexternal.Deps](i)), nil
	}),
	do.Lazy(func(i do.Injector) (*externalAPI, error) {
		h, err := server.NewExternalAPI(do.MustInvoke[apiexternal.Deps](i), do.MustInvoke[*apiauth.ExternalSecurityHandler](i))
		if err != nil {
			return nil, err
		}
		return &externalAPI{h}, nil
	}),
	do.Lazy(func(i do.Injector) (*mcpHandler, error) {
		cfg := do.MustInvoke[*config.Config](i)
		h, err := mcpserver.New(sphericon.ExternalOpenAPI, do.MustInvoke[*externalAPI](i).Handler, do.MustInvoke[*apiauth.ExternalSecurityHandler](i),
			mcpserver.WithResourceMetadataURL(oauthauthorization.ResourceMetadataURL(cfg.MCPOrigin())))
		if err != nil {
			return nil, err
		}
		return &mcpHandler{h}, nil
	}),
	do.Lazy(func(i do.Injector) (*staffHandler, error) {
		edition := do.MustInvoke[*ee.Edition](i)
		sections := edition.StaffSections()
		members := do.MustInvoke[*staffmembers.Module](i)
		var jobsUI http.Handler
		// The job queue's UI is built only where a Staff member can sign in to reach it.
		if members.Sessions() != nil {
			ui, err := do.Invoke[*JobsUI](i)
			if err != nil {
				return nil, err
			}
			jobsUI = ui
		}
		h, err := server.NewStaffAPI(members, do.MustInvoke[*signupitems.Module](i), do.MustInvoke[*instancestates.Module](i),
			do.MustInvoke[*telemetryreport.UseCase](i), sections, jobsUI)
		if err != nil {
			return nil, err
		}
		return &staffHandler{h}, nil
	}),
	// The job queue's UI (River UI) for the Staff console, running its background queries until
	// shutdown.
	do.Lazy(func(i do.Injector) (*JobsUI, error) {
		h, err := server.NewJobsUI(do.MustInvoke[*riverPool](i).DB)
		if err != nil {
			return nil, err
		}
		ctx, stop := context.WithCancel(context.Background())
		if err := h.Start(ctx); err != nil {
			stop()
			return nil, err
		}
		return &JobsUI{Handler: h, stop: stop}, nil
	}),
	// The handler of the internal listener (ADR 0026): the service endpoints of the
	// Staff surface (with the `staff` license) and the edge's TLS ask (always).
	do.Lazy(func(i do.Injector) (InternalHandler, error) {
		surface := do.MustInvoke[*ee.Edition](i).Service()
		h, err := server.NewInternal(do.MustInvoke[*config.Config](i), do.MustInvoke[*ent.Client](i), do.MustInvoke[*ratelimit.Limiter](i), surface)
		if err != nil {
			return InternalHandler{}, err
		}
		return InternalHandler{h}, nil
	}),
	// The handler every public surface is mounted on.
	do.Lazy(func(i do.Injector) (Handler, error) {
		h, err := server.New(do.MustInvoke[*config.Config](i), do.MustInvoke[Database](i).DB, do.MustInvoke[*db.SchemaWatch](i), do.MustInvoke[*ent.Client](i),
			do.MustInvoke[apisite.Deps](i), do.MustInvoke[*machine.Detector](i), do.MustInvoke[*ratelimit.Limiter](i),
			do.MustInvoke[*externalAPI](i).Handler, do.MustInvoke[*mcpHandler](i).Handler, do.MustInvoke[*staffHandler](i).Handler)
		if err != nil {
			return Handler{}, err
		}
		return Handler{h}, nil
	}),
)

// externalDeps is what the /api handlers are built from.
func externalDeps(i do.Injector) (apiexternal.Deps, error) {
	edition := do.MustInvoke[*ee.Edition](i)
	return apiexternal.Deps{
		Clock:                 do.MustInvoke[Clock](i),
		Users:                 do.MustInvoke[*users.Module](i),
		Workspaces:            do.MustInvoke[*workspaces.Module](i),
		Bus:                   do.MustInvoke[*events.Bus](i),
		Webhooks:              do.MustInvoke[*webhooks.Module](i),
		Outbound:              do.MustInvoke[*outbound.Module](i),
		Segments:              do.MustInvoke[*segments.Module](i),
		EventLog:              do.MustInvoke[*eventlog.UseCase](i),
		Contacts:              do.MustInvoke[*contacts.Module](i),
		Erasure:               do.MustInvoke[*contacterasure.UseCase](i),
		Tags:                  do.MustInvoke[*tags.Module](i),
		TagDeletion:           do.MustInvoke[*tagdeletion.UseCase](i),
		Bindings:              do.MustInvoke[*bindings.Module](i),
		Templates:             do.MustInvoke[*templates.Module](i),
		Transactional:         do.MustInvoke[*transactionalsend.UseCase](i),
		BrandKit:              do.MustInvoke[*brandkit.Module](i),
		Automations:           do.MustInvoke[*automations.Module](i),
		Rules:                 do.MustInvoke[*rules.Module](i),
		Broadcasts:            do.MustInvoke[*broadcasts.Module](i),
		Reputation:            do.MustInvoke[*reputation.Module](i),
		Integrations:          do.MustInvoke[*integrations.Module](i),
		CapturedMessages:      do.MustInvoke[*capturedmessages.Module](i),
		SendingDomains:        do.MustInvoke[*sendingdomains.Module](i),
		IntegrationDeletion:   do.MustInvoke[*integrationdeletion.UseCase](i),
		SendingDomainDeletion: do.MustInvoke[*sendingdomaindeletion.UseCase](i),
		DomainVerification:    do.MustInvoke[*domainverification.UseCase](i),
		TrackingDomains:       do.MustInvoke[*trackingdomains.Module](i),
		BootstrapToken:        do.MustInvoke[*config.Config](i).BootstrapToken,
		Audit:                 edition.Audit,
		Export:                edition.Export,

		CustomFieldRetyping: do.MustInvoke[*customfieldretyping.UseCase](i),
	}, nil
}

// siteDeps is what the /site handlers are built from.
func siteDeps(i do.Injector) (apisite.Deps, error) {
	cfg := do.MustInvoke[*config.Config](i)
	edition := do.MustInvoke[*ee.Edition](i)
	bus := do.MustInvoke[*events.Bus](i)
	enqueue := do.MustInvoke[Enqueuer](i)
	accountLink, _, err := accountlink.FromConfig(cfg)
	if err != nil {
		return apisite.Deps{}, err
	}
	return apisite.Deps{
		// The Sign-up survey is on only with an explicit switch and the `staff` license; the
		// warning for a switch without the license is logged here, once, at start.
		SignupSurvey:             signupsurvey.New(bus, signupsurvey.Enabled(cfg.SignupSurvey, edition.StaffLicensed(), slog.Default())),
		SignupItems:              do.MustInvoke[*signupitems.Module](i),
		ConsentRecords:           do.MustInvoke[*consentrecords.Module](i),
		Release:                  do.MustInvoke[*releasecheck.UseCase](i),
		Deletion:                 do.MustInvoke[*workspacedeletion.Module](i),
		AccountLink:              accountLink,
		Holds:                    do.MustInvoke[*holds.Holds](i),
		Attention:                do.MustInvoke[*needsattention.UseCase](i),
		SetupChecklist:           do.MustInvoke[*setupchecklist.Module](i),
		SupportEmail:             cfg.SupportEmail,
		Origins:                  apisite.Origins{MCP: cfg.MCPOrigin(), Tracker: cfg.TrackerOrigin()},
		Users:                    do.MustInvoke[*users.Module](i),
		Invitations:              do.MustInvoke[*invitations.Module](i),
		Workspaces:               do.MustInvoke[*workspaces.Module](i),
		Memberships:              do.MustInvoke[*memberships.Module](i),
		Passwords:                do.MustInvoke[*credentials.Hasher](i),
		Attempts:                 do.MustInvoke[*throttle.Attempts](i),
		OAuth:                    do.MustInvoke[*oauthclients.Module](i),
		Bus:                      bus,
		Webhooks:                 do.MustInvoke[*webhooks.Module](i),
		Outbound:                 do.MustInvoke[*outbound.Module](i),
		Segments:                 do.MustInvoke[*segments.Module](i),
		EventLog:                 do.MustInvoke[*eventlog.UseCase](i),
		Contacts:                 do.MustInvoke[*contacts.Module](i),
		Erasure:                  do.MustInvoke[*contacterasure.UseCase](i),
		Tags:                     do.MustInvoke[*tags.Module](i),
		TagDeletion:              do.MustInvoke[*tagdeletion.UseCase](i),
		CustomFieldRetyping:      do.MustInvoke[*customfieldretyping.UseCase](i),
		Bindings:                 do.MustInvoke[*bindings.Module](i),
		Templates:                do.MustInvoke[*templates.Module](i),
		BrandKit:                 do.MustInvoke[*brandkit.Module](i),
		Automations:              do.MustInvoke[*automations.Module](i),
		Rules:                    do.MustInvoke[*rules.Module](i),
		Broadcasts:               do.MustInvoke[*broadcasts.Module](i),
		Mail:                     enqueue,
		SendingDomains:           do.MustInvoke[*sendingdomains.Module](i),
		SendingDomainDeletion:    do.MustInvoke[*sendingdomaindeletion.UseCase](i),
		DomainVerification:       do.MustInvoke[*domainverification.UseCase](i),
		TrackingDomains:          do.MustInvoke[*trackingdomains.Module](i),
		JoinDomains:              do.MustInvoke[*joindomains.Module](i),
		Integrations:             do.MustInvoke[*integrations.Module](i),
		IntegrationDeletion:      do.MustInvoke[*integrationdeletion.UseCase](i),
		CapturedMessages:         do.MustInvoke[*capturedmessages.Module](i),
		Tokens:                   do.MustInvoke[*authtoken.Signer](i),
		Sessions:                 do.MustInvoke[*apiauth.Sessions](i),
		Tracker:                  do.MustInvoke[*tracking.Tracker](i),
		AppURL:                   cfg.AppURL,
		Audit:                    edition.Audit,
		SecondFactor:             do.MustInvoke[*secondfactorenrolment.UseCase](i),
		Passkeys:                 do.MustInvoke[*passkeys.Module](i),
		Analytics:                do.MustInvoke[*analyticsreport.UseCase](i),
		Clock:                    do.MustInvoke[Clock](i),
		RequireEmailVerification: cfg.RequireEmailVerification,
		Signin:                   do.MustInvoke[*signin.Providers](i),
		Migrations:               do.MustInvoke[apisite.Migrations](i),
	}, nil
}

// dnsResolver is the resolver of Sending domain, Join domain and Tracking domain verification: the
// host's own when addr is empty, otherwise the DNS server at addr (host:port), which a development
// stack runs for its demo records.
func dnsResolver(addr string) *net.Resolver {
	if addr == "" {
		return net.DefaultResolver
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}
}

// urlHost is the lower-case hostname of an origin such as https://app.example.com:8443, "" for
// an empty or malformed one.
func urlHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
