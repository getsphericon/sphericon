// Package testhelper is the test harness: it builds the application over a per-process database
// with the committed fixtures loaded, and hands tests typed actors for each API surface.
package testhelper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/samber/do/v2"

	"net/http/httptest"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	eejobs "github.com/getsphericon/sphericon/ee/jobs"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/identity/admission"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/domain/sending/privaterelayranges"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/usecase/noticedelivery"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorreset"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
	"github.com/getsphericon/sphericon/internal/usecase/userdeletion"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"

	// The pgx driver behind database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mokevnin/dbcleaner"

	// The Postgres cleaner of dbcleaner.
	_ "github.com/mokevnin/dbcleaner/postgres"
	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/require"
)

// Every test process (one per package) leases a database of its own, cloned from a
// schema-only template (see initBaseline), so packages run in parallel without sharing
// rows or locks. Each test then asks the cleaner for a database: a real transaction that is
// rolled back when the test ends (dbcleaner's Transaction strategy, go-txdb underneath), so
// tests are fully isolated and the DB stays at the fixture state. No hand-rolled
// transaction plumbing.
var (
	once    sync.Once
	baseCfg *config.Config
	cleaner *dbcleaner.Cleaner
	errLoad error
)

func initBaseline() {
	once.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		projectRoot := filepath.Join(filepath.Dir(file), "..", "..")

		cfg, err := config.Load(config.EnvTest)
		if err != nil {
			errLoad = err
			return
		}
		// Read the shared URL before SPHERICON_DATABASE_URL is pointed at this process's own database.
		if _, err := sharedDatabaseURL(); err != nil {
			errLoad = err
			return
		}
		version, err := schemaHash()
		if err != nil {
			errLoad = err
			return
		}
		// Every process leases a database of its own, cloned from a schema-only template
		// (dbcleaner.WithTemplate), so packages run in parallel without sharing rows or
		// locks. The fixtures go in at the start of every process, not into the template:
		// their relative dates must be relative to now.
		cleaner = dbcleaner.New(cfg.DatabaseURL,
			dbcleaner.WithTemplate(version, func(ctx context.Context, sqlDB *sql.DB, _ string) error {
				return migrateSchema(ctx, sqlDB)
			}),
			dbcleaner.WithSetup(func(ctx context.Context, sqlDB *sql.DB) error {
				return loadFixtures(ctx, sqlDB, cfg, projectRoot)
			}),
		)
		url, err := cleaner.URL(context.Background())
		if err != nil {
			errLoad = err
			return
		}
		// A test that loads the config itself (to open a pool of its own) must reach this
		// process's database, not the shared one.
		cfg.DatabaseURL = url
		if err := os.Setenv("SPHERICON_DATABASE_URL", url); err != nil {
			errLoad = err
			return
		}
		// A developer's own .env may enable Sign-in providers; no test starts from them, each one
		// enables what it needs (WithSignin, WithSigninGitHub).
		cfg.Signin = nil
		// The dev stack serves the tracker on its own host; a test starts on the app origin.
		cfg.TrackerURL = ""
		baseCfg = cfg
	})
}

type TestEnv struct {
	// Instance is the Instance state module the server shares: run its Release check with
	// CheckRelease; the preview of its Telemetry report is TelemetryReport.
	Instance *instancestates.Module
	// DB is the plain client the modules under test get, transaction-bound and rolled back when
	// the test finishes. Like production's (internal/app/entclient) it refuses a change the Workspace
	// feed, the User feed, the Tracking host log or, with the `staff` license, the Usage peak would
	// record outside a bus transaction, so a module that forgets the transaction fails its tests.
	DB *ent.Client
	// FeedDB is a client on the same transaction that records those changes as a bus transaction
	// does: for a test that moves a fixture row the hooks watch (a Workspace marked deleted, a
	// User's address changed, a Contact's email cleared) as its setup, never for the operation
	// under test.
	FeedDB *ent.Client
	SQLDB  *sql.DB     // the same txdb connection the ent client and bus ride
	Bus    *events.Bus // domain-event bus over SQLDB (nested savepoint per WithinTx)
	Server http.Handler
	// Internal is the handler of the internal listener (ADR 0026), from the production
	// container: the TLS ask, and the service endpoints with the `staff` license.
	Internal http.Handler
	// Tracker is the instance the server and the sender share: mint tracking,
	// unsubscribe and confirmation tokens with it, never with a second one.
	Tracker *messagetoken.Tracker
	// Cipher is the instance the server seals with, for modules a test builds directly.
	Cipher *secrets.Cipher
	// Integrations is the module the server serves Integrations and their hooks with.
	Integrations *integrations.Module
	// RemovalCheck is the Removal check the server's removals run (ADR 0049, ADR 0099), over the
	// server's resolver: for a test of the check, or one that builds a removal use case with New.
	RemovalCheck *removalcheck.Module
	// ConsentRecords is the module the server records the Users' decisions on the Sign-up items
	// with (ADR 0062): read a User's current answers with Current.
	ConsentRecords *consentrecords.Module

	// AppURL is SPHERICON_APP_URL of the env: the origin and relying party id of a Passkey
	// ceremony (ADR 0051).
	AppURL string

	// WorkspaceView reads the Workspace summary the surfaces map, and makes the changes of a
	// Workspace that answer with it, creating and restoring one among them (ADR 0111).
	WorkspaceView *workspaceview.UseCase

	// Holds is the module the sender's freeze check reads, for a test that builds its own
	// Outbound module.
	Holds *holds.Holds

	// Orders is the orders consumer's module, which the orders jobs drive, for a test that hands
	// it an order Event itself.
	Orders *orders.Module

	edition *ee.Edition // the EE parts, built like the composition root builds them
	// license is the Instance's License, which the Edition's consumers check (EditionJobs).
	license *licensekey.License

	performers func(testing.TB) performers // the workers of PerformEnqueuedJobs
	tb         testing.TB                  // the test, for the Notices' jobs SystemMail works before a read

	sender    *outboundmessages.Module // the Outbound module the queued Broadcast sends run through
	jwtSecret string                   // for tokens a test needs in a state the Tracker never mints

	now        func() time.Time // the clock the server checks site session expiry against
	sessionTTL time.Duration    // SPHERICON_SESSION_TTL, the lifetime of the site tokens tests mint

	// Captured sends of the jobs tests work at once, for assertions.
	SystemMail   *CapturingSender // the Notices (welcome, …), sent through the System sender
	CustomerMail *CapturingSender // workspace/campaign mail (broadcasts)

	// SES scripts the send quota every "ses" Integration reports.
	SES *FakeSES

	// Telegram is the fake Bot API every Telegram Integration's bot is reached at
	// (SPHERICON_TELEGRAM_BOT_API_URL points at it): add bots, script failures, read the calls.
	Telegram *telegramtest.Server

	// Twilio is the fake REST API every SMS Integration's Twilio account is reached at
	// (SPHERICON_TWILIO_API_URL points at it): add accounts, keys and senders, read the calls.
	Twilio *twiliotest.Server

	// Machine is the detector the tracking endpoints judge hits by, fed by PrivateRelay, the
	// scripted source of the published Private Relay list: Refresh replaces the stored ranges.
	Machine      *privaterelayranges.Detector
	PrivateRelay *FakeRelayRanges

	// Confirmations records the subscription URLs a Preset's confirm action fetched.
	Confirmations *Confirmations

	// SecondFactor is the module the server enrolls, verifies and reads Second factors with,
	// on the env's clock (ADR 0020).
	SecondFactor *secondfactors.Module
	// Admission admits a request on a User's own credential, a token of theirs included (ADR 0061).
	Admission *admission.Module
	// SecondFactorReset is the use case the server resets a member's Second factor and
	// Passkeys with (ADR 0102).
	SecondFactorReset *secondfactorreset.UseCase
	// UserDeletion is the use case the server deletes a User with at their own ask (ADR 0050).
	UserDeletion *userdeletion.UseCase

	// SignUp is the use case the server signs Users up with and makes them Verified with their
	// first Workspace, an Owner made from a shell among them (ADR 0102).
	SignUp *signup.UseCase
	// StepExecution is the Automations module's Step execution the Instance runs an Enrollment's
	// steps with, on the env's clock (ADR 0112). A test that drives an Automation over senders of its
	// own builds one with automations.NewStepExecution instead. A test hands it an Event (OnEvent)
	// or a Rule's enrollment (EnrollOnRule) too.
	StepExecution *automations.StepExecution

	// StaffMembers is the core Staff member module the server signs staff in with (ADR 0026,
	// ADR 0063), on every env.
	StaffMembers *staffmembers.Module

	// StaffEnterprise is the module of the Staff console's Enterprise sections and of the
	// service endpoints; it refuses every call on an unlicensed env.
	StaffEnterprise *workspaceoversight.Module
	// ServiceCredentials is the module of the service endpoints' Service credentials (ADR
	// 0026); it refuses every call on an unlicensed env.
	ServiceCredentials *servicecredentials.Module
}

// TrackingCNAMETarget is the edge host the test instance names for Tracking domain CNAMEs
// (SPHERICON_TRACKING_CNAME_TARGET); WithConfig clears it to test the feature off.
const TrackingCNAMETarget = "edge.sphericon.test"

// Option tunes the server a test builds with Setup.
type Option func(*setup)

// setup is what the options edit: a private copy of the test config, the clock the
// account attempt module reads, and whether the instance has no EE license.
type setup struct {
	cfg        *config.Config
	now        func() time.Time
	unlicensed bool
	// noStaffSecret builds the Instance without a Staff member secret (WithoutStaffSecret).
	noStaffSecret bool
	// volumeFloor is the Staff console's rate floor; zero is the default.
	volumeFloor int
	// version is the running version the Instance module judges releases against; the
	// test build is "dev", which is no release.
	version string
	// licenseKey is the key WithLicenseKey loads against licensePub; a nil licensePub keeps the default license.
	licenseKey string
	licensePub ed25519.PublicKey
	// certs is where an `sns` Preset gets its signing certificate; nil is the test certificate.
	certs preset.CertFetcher
	// oauthFetcher downloads OAuth Client ID Metadata Documents; nil is the production fetcher.
	oauthFetcher webhook.Doer
	// noSystemSender builds the Instance without outbound mail.
	noSystemSender bool
	// noSystemFrom gives the System sender no default From (it is not a DefaultFromer).
	noSystemFrom bool
	// txt is the TXT resolver Sending domain and Join domain verification use; nil is the
	// always-NXDOMAIN stub (WithTXTLookup).
	txt domainname.TXTLookup
	// mail is the DNS the Contact intake reads an address's domain from; nil is
	// addresscheck.AllAccept (WithMailResolver).
	mail addresscheck.Resolver
	// signinOpts point the Sign-in registry at a fake provider (WithSignin).
	signinOpts []signinprovider.Option
}

// WithTXTLookup replaces the TXT resolver of Sending domain and Join domain verification, so a
// test publishes and withdraws records in a zone of its own (dnstest.NewZone). The default
// answers NXDOMAIN to everything.
func WithTXTLookup(lookup domainname.TXTLookup) Option { return func(s *setup) { s.txt = lookup } }

// mailResolver is the DNS of the Contact intake's address check: the test's, or every domain
// accepting mail.
func (s *setup) mailResolver() addresscheck.Resolver {
	if s.mail != nil {
		return s.mail
	}
	return addresscheck.AllAccept{}
}

// WithMailResolver replaces the DNS the Contact intake reads an address's domain from (ADR 0117),
// so a test serves the domains of its addresses (dnstest.Resolver). The default gives every domain
// a mail exchanger, as the fixtures' addresses are at no real domain.
func WithMailResolver(r addresscheck.Resolver) Option { return func(s *setup) { s.mail = r } }

// WithOAuthFetcher replaces the client that downloads OAuth Client ID Metadata Documents, so a
// test serves a document from an httptest TLS server (the production client refuses loopback).
func WithOAuthFetcher(f webhook.Doer) Option { return func(s *setup) { s.oauthFetcher = f } }

// WithoutSystemSender runs the Instance with no outbound mail, so it has no System sender
// for a Test send to fall back to (ADR 0041).
func WithoutSystemSender() Option { return func(s *setup) { s.noSystemSender = true } }

// WithSystemSenderWithoutFrom gives the Instance a System sender that names no From of its own,
// which a Test send must refuse rather than send with an empty From.
func WithSystemSenderWithoutFrom() Option { return func(s *setup) { s.noSystemFrom = true } }

// WithRunningVersion sets the version the Instance's Release check compares against, so a
// test runs as a release build ("0.1.0") instead of the "dev" test build.
func WithRunningVersion(v string) Option { return func(s *setup) { s.version = v } }

// WithSNSCertificates replaces where an `sns` Preset gets its signing certificate (the
// test certificate by default), so a test makes the fetch fail.
func WithSNSCertificates(f preset.CertFetcher) Option { return func(s *setup) { s.certs = f } }

// WithoutLicense builds the instance with no EE license key, like a plain core
// self-host. Setup's default is an instance licensed for every EE feature.
func WithoutLicense() Option { return func(s *setup) { s.unlicensed = true } }

// WithoutStaffSecret builds the instance without SPHERICON_STAFF_JWT_SECRET, so the Staff
// sign-in is off. It needs WithoutLicense: the `staff` feature refuses to start without it.
func WithoutStaffSecret() Option { return func(s *setup) { s.noStaffSecret = true } }

// WithStaffVolumeFloor sets the fewest sends in the Staff console's window for a
// rate to be defined, so a test crosses the floor with a handful of fixture Events.
func WithStaffVolumeFloor(n int) Option { return func(s *setup) { s.volumeFloor = n } }

// WithMode runs the test Instance in a Mode, with every setting the Mode decides.
func WithMode(m config.Mode) Option { return func(s *setup) { s.cfg.ApplyMode(m) } }

// WithConfig edits a private copy of the test config before the server is built.
func WithConfig(edit func(*config.Config)) Option { return func(s *setup) { edit(s.cfg) } }

// WithClock replaces the clock of the Instance (app.Clock: session expiry, the login delay,
// holds, second factors, the Outbound send's Provider limit and claim lease, the open and click
// stamps, an Enrollment's waits, the Automation report's period, …), so a test freezes or advances
// time instead of sleeping.
func WithClock(now func() time.Time) Option { return func(s *setup) { s.now = now } }

// WithRateLimits turns the rate limits on with the given (small) budgets. Without
// it every limit is 0 (disabled), so tests never throttle each other; limiters
// are per Setup, so budgets never leak between tests either.
func WithRateLimits(limits config.RateLimits) Option {
	return func(s *setup) { s.cfg.RateLimits = limits }
}

// newStaffSecret is the secret Staff member sessions are signed with in a test env: random
// and its own, never the site's.
func newStaffSecret() string { return rand.Text() + rand.Text() }

// testLicense mints, once per process, an EE license key signed by a throwaway key
// pair and verified through the same Parse the production composition root uses.
var testLicense = sync.OnceValues(func() (*licensekey.License, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	key, err := licensekey.Issue(priv, nil, licensekey.FeatureAudit, licensekey.FeatureRetention, licensekey.FeatureStaff, licensekey.FeatureExport)
	if err != nil {
		return nil, err
	}
	return licensekey.Parse(key, pub, time.Now())
})

// WithLicenseKey builds the instance as the composition root does for SPHERICON_LICENSE_KEY: the
// key is loaded through licensekey.Load against pub, so a lapsed key boots as the core and a
// forged one fails the test's setup (ADR 0035).
func WithLicenseKey(pub ed25519.PublicKey, key string) Option {
	return func(s *setup) { s.licenseKey, s.licensePub = key, pub }
}

// license is the License of the test Instance: every feature by default, none under
// WithoutLicense, and the key of WithLicenseKey loaded as the composition root loads one.
func (s *setup) license(t *testing.T) *licensekey.License {
	t.Helper()
	if s.licensePub != nil {
		lic, err := licensekey.Load(s.licenseKey, s.licensePub, s.now(), slog.Default())
		require.NoError(t, err, "load license key")
		s.cfg.LicenseKey = s.licenseKey
		return lic
	}
	if s.unlicensed {
		lic, err := licensekey.Parse("", licensekey.ProductionKey, time.Now())
		require.NoError(t, err, "parse empty license")
		return lic
	}
	lic, err := testLicense()
	require.NoError(t, err, "mint test license")
	return lic
}

// testConfig is a private copy of the baseline config for one test. The test License carries every
// feature, so the harness runs in the saas Mode and the existing tests keep the gates the SaaS has
// on (verification, the Sign-up survey); a test of selfhosted says so with WithMode (ADR 0060).
func testConfig() config.Config {
	cfg := *baseCfg
	cfg.ApplyMode(config.ModeSaaS)
	cfg.RateLimits = config.RateLimits{}
	cfg.TrackingCNAMETarget = TrackingCNAMETarget
	return cfg
}

// Setup builds the test Instance from the production container (app.Container) over the test's
// own transaction. What differs from production is configuration, set here and by the Options,
// and the seams the container names, replaced here: the clock, the database, the License, the
// password hasher, the mail senders and the provider catalog, DNS, the job queue (run inline),
// the Presets, the document renderer, the Private Relay list, the OAuth fetcher and the
// Sign-in providers.
func Setup(t *testing.T, opts ...Option) *TestEnv {
	t.Helper()
	initBaseline()
	require.NoError(t, errLoad, "init test baseline")

	cfg := testConfig()
	st := &setup{cfg: &cfg, now: time.Now, version: buildinfo.Current().Version}
	for _, opt := range opts {
		opt(st)
	}
	cfg.StaffJWTSecret = newStaffSecret()
	if st.noStaffSecret {
		cfg.StaffJWTSecret = ""
	}
	if st.volumeFloor > 0 {
		cfg.AbuseVolumeFloor = st.volumeFloor
	}
	if st.noSystemSender {
		cfg.SMTPHost, cfg.SystemEmailProvider = "", providers.SMTP
	}
	lic := st.license(t)

	// All queries ride the SAME transaction (the cleaner limits the pool to one
	// connection); it is rolled back when the test ends. The bus opens a nested (savepoint)
	// transaction on it, so producer writes, Event rows and the jobs queued in a bus
	// transaction roll back with the test; no worker runs them.
	txDB := cleaner.DB(t)

	// Captured sends of the jobs tests work at once, for assertions.
	systemMail := &CapturingSender{}
	customerMail := &CapturingSender{}
	var systemSender messaging.EmailSender = systemMailFrom{systemMail}
	if st.noSystemFrom {
		systemSender = struct{ messaging.EmailSender }{systemMail}
	}
	// stubTXT keeps sending-domain verification off real DNS: every lookup is
	// NXDOMAIN, so an inline verify deterministically resolves to "not published".
	var stubTXT domainname.TXTLookup = func(context.Context, string) ([]string, error) {
		return nil, &net.DNSError{IsNotFound: true}
	}
	if st.txt != nil {
		stubTXT = st.txt
	}
	// An `sns` Preset checks signatures against the test certificate, so a test posts a
	// notification signed offline; confirmations are recorded, never fetched. A test with
	// certificates of its own loads the Presets over them.
	presets, err := testPresets()
	if st.certs != nil {
		presets, err = preset.Load(nil, preset.WithSNSCertificates(st.certs))
	}
	require.NoError(t, err, "load presets")
	oauthFetcher := st.oauthFetcher
	if oauthFetcher == nil {
		oauthFetcher = oauthclients.NewFetcher(cfg.OutboundAllow)
	}
	fakeSES := &FakeSES{}
	botAPI := telegramtest.New(t)
	cfg.TelegramBotAPIURL = botAPI.URL()
	twilioAPI := twiliotest.New(t)
	cfg.TwilioAPIURL = twilioAPI.URL()
	relay := &FakeRelayRanges{}
	confirmations := &Confirmations{}

	injector := app.Container(&cfg)
	do.OverrideValue(injector, app.Clock(st.now))
	do.OverrideValue(injector, app.Version(st.version))
	do.OverrideValue(injector, app.Database{DB: txDB})
	do.OverrideValue(injector, lic)
	do.OverrideValue(injector, Passwords())
	do.OverrideValue(injector, app.SystemSender{EmailSender: systemSender})
	// SES's quota lookup is answered by a fake instead of the AWS API.
	do.OverrideValue(injector, catalogWith(fakeSES, cfg.TelegramBotAPIURL, cfg.TwilioAPIURL))
	do.OverrideValue(injector, stubTXT)
	do.OverrideValue(injector, st.mailResolver())
	do.OverrideValue[privaterelayranges.Source](injector, relay)
	do.OverrideValue(injector, presets)
	do.OverrideValue(injector, Renderer())
	do.OverrideValue(injector, migrated)
	// No error-page button that runs the migrations: it would migrate the process's database from
	// under the test's transaction.
	do.OverrideValue[apisite.Migrations](injector, nil)
	do.OverrideValue(injector, app.OAuthFetcher{Doer: oauthFetcher})
	do.OverrideValue(injector, app.SigninOptions(st.signinOpts))
	do.OverrideValue(injector, app.IntegrationOptions{integrations.WithSubscriptionConfirmer(confirmations.confirm)})
	// A Workspace's mail lands in CustomerMail unless its Integration is a Test inbox.
	do.Override(injector, func(i do.Injector) (outboundmessages.Senders, error) {
		return inboxResolver{fixed: fixedResolver{sender: customerMail}, real: do.MustInvoke[*messaging.Resolver](i)}, nil
	})

	// The job queue's UI is River UI's own; a test reaches a stand-in through the Staff session
	// guard (JobsUIStub).
	do.OverrideValue(injector, &app.JobsUI{Handler: JobsUIStub})

	handler, err := do.Invoke[app.Handler](injector)
	require.NoError(t, err, "build server")
	internal, err := do.Invoke[app.InternalHandler](injector)
	require.NoError(t, err, "build internal handler")
	edition := do.MustInvoke[*ee.Edition](injector)
	// Every live event the test publishes is handed to the harness's recorder too, so
	// QueuedEnvelopes sees it whether or not a consumer of the application wants it.
	do.MustInvoke[*events.Bus](injector).UseConsumers(recorder)

	// The client of a test's own writes of rows the hooks watch, on the same transaction, built
	// through the application's own builder, so it carries the hooks every other client does.
	feedClient := do.MustInvoke[entclient.Clients](injector).NewOnTx(txDB)

	// The workers PerformEnqueuedJobs works the queued jobs with, against the capturing senders,
	// so a test asserts the real effect.
	workers := func(tb testing.TB) performers {
		tb.Helper()
		client := do.MustInvoke[*ent.Client](injector)
		p := performers{}
		add(tb, p, txDB, client, jobs.NewSendNoticeWorker(systemSender))
		add(tb, p, txDB, client, jobs.NewRefreshIntegrationQuotaWorker(client, do.MustInvoke[*integrations.Module](injector)))
		add(tb, p, txDB, client, jobs.NewVerifySendingDomainWorker(client, do.MustInvoke[*sendingdomains.Module](injector)))
		// The consumers of the domain events, core's and the Edition's (ADR 0101).
		add(tb, p, txDB, client, jobs.NewSuppressEventWorker(client))
		add(tb, p, txDB, client, jobs.NewOrderEventWorker(client, do.MustInvoke[*orders.Module](injector)))
		add(tb, p, txDB, client, jobs.NewBroadcastUnsubscribeEventWorker(client))
		add(tb, p, txDB, client, jobs.NewMessageObservationWorker(client))
		add(tb, p, txDB, client, jobs.NewNoticeEventWorker(do.MustInvoke[*noticedelivery.UseCase](injector), systemSender))
		add[recordedArgs](tb, p, txDB, client, recordedWorker{})
		add(tb, p, txDB, client, jobs.NewWebhookEventWorker(client, edition.Webhooks(jobs.NewWebhookDispatcher())))
		add(tb, p, txDB, client, eejobs.NewEntryEventWorker(client, lic))
		add(tb, p, txDB, client, eejobs.NewErasureEventWorker(client, lic, do.MustInvoke[*secrets.Cipher](injector)))
		return p
	}

	env := &TestEnv{
		Instance:          do.MustInvoke[*instancestates.Module](injector),
		AppURL:            cfg.AppURL,
		DB:                do.MustInvoke[*ent.Client](injector),
		FeedDB:            feedClient,
		SQLDB:             txDB,
		Bus:               do.MustInvoke[*events.Bus](injector),
		Server:            handler.Handler,
		Internal:          internal.Handler,
		Tracker:           do.MustInvoke[*messagetoken.Tracker](injector),
		Cipher:            do.MustInvoke[*secrets.Cipher](injector),
		Integrations:      do.MustInvoke[*integrations.Module](injector),
		RemovalCheck:      do.MustInvoke[*removalcheck.Module](injector),
		ConsentRecords:    do.MustInvoke[*consentrecords.Module](injector),
		Holds:             do.MustInvoke[*holds.Holds](injector),
		WorkspaceView:     do.MustInvoke[*workspaceview.UseCase](injector),
		Orders:            do.MustInvoke[*orders.Module](injector),
		edition:           edition,
		license:           lic,
		performers:        workers,
		sender:            do.MustInvoke[*outboundmessages.Module](injector),
		jwtSecret:         cfg.JWTSecret,
		now:               st.now,
		sessionTTL:        cfg.SessionTTL,
		SystemMail:        systemMail,
		CustomerMail:      customerMail,
		SES:               fakeSES,
		Telegram:          botAPI,
		Twilio:            twilioAPI,
		Machine:           do.MustInvoke[*privaterelayranges.Detector](injector),
		PrivateRelay:      relay,
		Confirmations:     confirmations,
		SecondFactor:      do.MustInvoke[*secondfactors.Module](injector),
		Admission:         do.MustInvoke[*admission.Module](injector),
		SecondFactorReset: do.MustInvoke[*secondfactorreset.UseCase](injector),
		UserDeletion:      do.MustInvoke[*userdeletion.UseCase](injector),
		SignUp:            do.MustInvoke[*signup.UseCase](injector),
		StepExecution:     do.MustInvoke[*automations.StepExecution](injector),
		StaffMembers:      do.MustInvoke[*staffmembers.Module](injector),
		StaffEnterprise:   edition.Staff,

		ServiceCredentials: edition.ServiceCredentials,
	}
	// The Notices a request's events call for are sent by the time a test reads them.
	env.tb = t
	systemMail.pending = env.performMail
	return env
}

// CoreHolds is the holds module of an instance that ignores Staff holds (no `staff` license),
// for a test under testing/synctest: fake time starts in 2000, before the real-clock expiry
// of the fixture holds, which would read active there.
func (env *TestEnv) CoreHolds() *holds.Holds {
	return holds.New(func() bool { return false }, time.Now)
}

// CapturingSender is a messaging.EmailSender that records sends for assertions.
// Set Err to make Send fail with it — used to drive the send-path error branches
// (e.g. the verified-domain gate) that the real providers are stubbed out of.
type CapturingSender struct {
	mu   sync.Mutex
	sent []messaging.EmailMessage
	Err  error

	// pending works the Notices' jobs waiting in the queue before a read, so a test reads the mail
	// its request queued as sent (SystemMail); nil for a sender no job sends through.
	pending func()
}

// SetErr makes subsequent Send calls fail with err (nil clears it).
func (s *CapturingSender) SetErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Err = err
}

// DefaultFrom is the integration-configured sender the fixtures' default
// integration uses; its domain (codebasics.dev) is a verified Sending domain, so
// sends without an explicit From pass Outbound send's domain gate (ADR 0010).
func (s *CapturingSender) DefaultFrom() (address, name string) {
	return "hello@codebasics.dev", "CodeBasics"
}

func (s *CapturingSender) Send(_ context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return messaging.Receipt{}, s.Err
	}
	s.sent = append(s.sent, msg)
	return messaging.Receipt{MessageID: fmt.Sprintf("<test-%d@sphericon.test>", len(s.sent))}, nil
}

// Messages returns a copy of the captured sends, once the Notices' jobs waiting in the queue have run.
func (s *CapturingSender) Messages() []messaging.EmailMessage {
	if s.pending != nil {
		s.pending()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]messaging.EmailMessage(nil), s.sent...)
}

// SystemFrom is the From address of the test Instance's System sender.
const SystemFrom = "noreply@sphericon.test"

// systemMailFrom is the System sender over the capturing system mail: it sends from SystemFrom,
// where the capturing sender itself defaults to a Workspace's verified domain.
type systemMailFrom struct{ *CapturingSender }

func (systemMailFrom) DefaultFrom() (address, name string) { return SystemFrom, "sphericon" }

// fixedResolver returns the same capturing sender for any workspace, so inline
// broadcast sends work without a configured integration.
type fixedResolver struct{ sender messaging.EmailSender }

func (r fixedResolver) EmailSender(context.Context, *ent.Scoped, *ent.Integration) (messaging.EmailSender, error) {
	return r.sender, nil
}

// inboxResolver is the capturing sender for every Workspace, except one whose default email
// Integration is a Test inbox: that one is resolved as in production (the real Resolver over the
// harness catalog, its Captured messages written to the database), so a Test inbox scenario sends
// through the real provider while every other test keeps the capturing sender.
type inboxResolver struct {
	fixed fixedResolver
	real  *messaging.Resolver
}

func (r inboxResolver) EmailSender(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error) {
	if integ != nil && integ.Provider == providers.Inbox {
		return r.real.EmailSender(ctx, s, integ)
	}
	return r.fixed.EmailSender(ctx, s, integ)
}

// Transport returns an ogen http.Client that dispatches in-memory to the
// server (no socket). Pass it via the generated client's WithClient option so
// the typed client builds URLs and encodes/decodes DTOs for you. extraHeaders
// are applied to every request (e.g. x-collect-key).
func (env *TestEnv) Transport(extraHeaders map[string]string) ht.Client {
	return injectClient{handler: env.Server, headers: extraHeaders}
}

type injectClient struct {
	handler http.Handler
	headers map[string]string
}

func (c injectClient) Do(r *http.Request) (*http.Response, error) {
	for k, v := range c.headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// migrateSchema applies the embedded migrations to an empty database: the same files, the same
// way (goose), that production gets, so the app schema and river's tables
// come from them. The worker databases are cloned from a template that has just
// this (see initBaseline).
func migrateSchema(ctx context.Context, sqlDB *sql.DB) error {
	_, err := db.Migrate(ctx, sqlDB)
	return err
}

// loadFixtures replaces the contents of every fixture table with the committed rows. It
// runs at the start of every test process, on the process's two databases (the one Setup's
// transactions ride and the baseline RealDB clones), never once into a template: fixtures use
// templating for relative dates, which must be relative to now, not to when a template was
// built.
func loadFixtures(ctx context.Context, sqlDB *sql.DB, cfg *config.Config, projectRoot string) error {
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	return fixtures.Load(ctx, sqlDB, cipher, filepath.Join(projectRoot, "fixtures"))
}

// migrated is the schema watch of the test server: every test database is cloned from the
// migrated template, so none is ever behind. The guard's own answers are tested on it directly
// (internal/server), and asking the database for every test cost a query each.
var migrated = db.WatchSchema(func(context.Context) (int, error) { return 0, nil }, time.Now)

// testPresets are the built-in Presets over the test SNS certificate, loaded once per process:
// a Registry is read-only once loaded, and loading compiles every Preset's parameter schema.
var testPresets = sync.OnceValues(func() (*preset.Registry, error) {
	return preset.Load(nil, preset.WithSNSCertificates(preset.TestSNSCertificates))
})
