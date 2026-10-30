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

	"net/http/httptest"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accountlink"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/analytics"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	apiexternal "github.com/getsphericon/sphericon/internal/api/external"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/capturedmessages"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/machine"
	"github.com/getsphericon/sphericon/internal/mcpserver"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/preset"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/signin"
	"github.com/getsphericon/sphericon/internal/signupsurvey"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/transactional"
	"github.com/getsphericon/sphericon/internal/upgrade"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"

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

		cfg, err := config.Load("test")
		if err != nil {
			errLoad = err
			return
		}
		// Read the shared URL before DATABASE_URL is pointed at this process's own database.
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
		if err := os.Setenv("DATABASE_URL", url); err != nil {
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
	// Instance is the module the server shares: run its Release check with CheckRelease.
	Instance *instance.Module
	DB       *ent.Client // transaction-bound; rolled back when the test finishes
	SQLDB    *sql.DB     // the same txdb connection the ent client and bus ride
	Bus      *events.Bus // domain-event bus over SQLDB (nested savepoint per WithinTx)
	Server   http.Handler
	// Internal is the handler of the internal listener (ADR 0026), built like the
	// composition root builds it: the service endpoints and nothing else. Nil on an
	// unlicensed instance, where no listener would be bound.
	Internal http.Handler
	// Tracker is the instance the server and the sender share: mint tracking,
	// unsubscribe and confirmation tokens with it, never with a second one.
	Tracker *tracking.Tracker
	// Cipher is the instance the server seals with, for modules a test builds directly.
	Cipher *secrets.Cipher
	// Integrations is the module the server serves Integrations and their hooks with.
	Integrations *integrations.Module

	// AppURL is SPHERICON_APP_URL of the env: the origin and relying party id of a Passkey
	// ceremony (ADR 0051).
	AppURL string

	// Holds is the module the sender's freeze check reads, for a test that builds its own
	// Outbound module.
	Holds *holds.Holds

	orders  *orders.Module // the orders subscriber's module, driven by DeliverOrders
	edition *ee.Edition    // the EE parts, built like the composition root builds them

	sender    *outbound.Module // the Outbound module the queued Broadcast sends run through
	jwtSecret string           // for tokens a test needs in a state the Tracker never mints

	now        func() time.Time // the clock the server checks site session expiry against
	sessionTTL time.Duration    // SPHERICON_SESSION_TTL, the lifetime of the site tokens tests mint

	// Captured sends from the inline jobs adapter, for assertions.
	SystemMail   *CapturingSender // platform mail (welcome, …)
	CustomerMail *CapturingSender // workspace/campaign mail (broadcasts)

	// SES scripts the send quota every "ses" Integration reports.
	SES *FakeSES

	// Machine is the detector the tracking endpoints judge hits by, fed by PrivateRelay, the
	// scripted source of the published Private Relay list: Refresh replaces the stored ranges.
	Machine      *machine.Detector
	PrivateRelay *FakeRelayRanges

	// Confirmations records the subscription URLs a Preset's confirm action fetched.
	Confirmations *Confirmations

	// SecondFactor is the module the server verifies Second factors with, on the
	// env's clock (ADR 0020).
	SecondFactor *secondfactor.Module

	// Staff members is the Staff member module the server logs staff in with (ADR 0026); it
	// refuses every call on an unlicensed env.
	StaffMembers *staff.Module
}

// TrackingCNAMETarget is the edge host the test instance names for Tracking domain CNAMEs
// (SPHERICON_TRACKING_CNAME_TARGET); WithConfig clears it to test the feature off.
const TrackingCNAMETarget = "edge.sphericon.test"

// Option tunes the server a test builds with Setup.
type Option func(*setup)

// setup is what the options edit: a private copy of the test config, the clock the
// account attempt module reads, and whether the instance has no EE license.
type setup struct {
	cfg *config.Config
	// signupRule replaces the sign-up rule of the Mode when set (WithSignupRule).
	signupRule *accounts.SignupRule
	now        func() time.Time
	unlicensed bool
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
	txt sending.TXTLookup
	// signinOpts point the Sign-in registry at a fake provider (WithSignin).
	signinOpts []signin.Option
}

// WithTXTLookup replaces the TXT resolver of Sending domain and Join domain verification, so a
// test publishes and withdraws records in a zone of its own (dnstest.NewZone). The default
// answers NXDOMAIN to everything.
func WithTXTLookup(lookup sending.TXTLookup) Option { return func(s *setup) { s.txt = lookup } }

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

// WithStaffVolumeFloor sets the fewest sends in the Staff console's window for a
// rate to be defined, so a test crosses the floor with a handful of fixture Events.
func WithStaffVolumeFloor(n int) Option { return func(s *setup) { s.volumeFloor = n } }

// WithMode runs the test Instance in a Mode, with every setting the Mode decides.
func WithMode(m config.Mode) Option { return func(s *setup) { s.cfg.ApplyMode(m) } }

// WithSignupRule opens or closes Sign-up whatever the Mode says. The fixtures already hold
// Users, so a selfhosted test Instance is closed; a test of what a new person gets on such an
// Instance (the Sign-up survey) opens it with accounts.SignupAlways.
func WithSignupRule(rule accounts.SignupRule) Option { return func(s *setup) { s.signupRule = &rule } }

// signupRuleOf is the sign-up rule of the test Instance: WithSignupRule's, else its Mode's.
func (s *setup) signupRuleOf(cfg *config.Config) accounts.SignupRule {
	if s.signupRule != nil {
		return *s.signupRule
	}
	return accounts.SignupRuleOf(cfg.Mode.SaaS())
}

// WithConfig edits a private copy of the test config before the server is built.
func WithConfig(edit func(*config.Config)) Option { return func(s *setup) { edit(s.cfg) } }

// WithClock replaces the clock of the account attempt module (the login delay), so
// a test freezes or advances time instead of sleeping.
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

func Setup(t *testing.T, opts ...Option) *TestEnv {
	t.Helper()
	initBaseline()
	require.NoError(t, errLoad, "init test baseline")

	cfg := testConfig()
	st := &setup{cfg: &cfg, now: time.Now, version: buildinfo.Current().Version}
	for _, opt := range opts {
		opt(st)
	}

	// All queries ride the SAME transaction (the cleaner limits the pool to one
	// connection); it is rolled back when the test ends.
	txDB := cleaner.DB(t)

	client := db.NewEntClientOnTx(txDB)
	// Bus over the same txdb connection: WithinTx opens a nested (savepoint)
	// transaction, so producer writes + outbox inserts roll back with the test.
	bus := events.New(txDB)
	// Jobs queued in a bus transaction land in river_job on the same txdb connection, and
	// roll back with the test; no worker runs them.
	inserter, err := jobs.NewInserter(txDB)
	require.NoError(t, err, "build job inserter")
	bus.UseJobs(inserter)

	// Inline jobs adapter (Rails :inline equivalent): enqueued jobs run
	// synchronously against capturing senders, so tests assert the real effect.
	systemMail := &CapturingSender{}
	customerMail := &CapturingSender{}
	capturedModule := capturedmessages.New(cfg.TestInboxMaxMessages)
	// stubTXT keeps sending-domain verification off real DNS: every lookup is
	// NXDOMAIN, so an inline verify deterministically resolves to "not published".
	var stubTXT sending.TXTLookup = func(context.Context, string) ([]string, error) {
		return nil, &net.DNSError{IsNotFound: true}
	}
	if st.txt != nil {
		stubTXT = st.txt
	}
	tracker := tracking.New(cfg.JWTSecret, cfg.AppURL, cfg.JWTVerifySecrets()...)
	// Cipher (over the fixture-sealing key) and provider catalog for the site
	// handlers and the inline jobs — mirrors the app's DI singletons, except that
	// SES's quota lookup is answered by a fake instead of the AWS API.
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err, "build cipher")
	lic, err := licensekey.Parse("", licensekey.ProductionKey, time.Now())
	require.NoError(t, err, "parse empty license")
	if !st.unlicensed {
		lic, err = testLicense()
		require.NoError(t, err, "mint test license")
	}
	if st.licensePub != nil {
		lic, err = licensekey.Load(st.licenseKey, st.licensePub, st.now(), slog.Default())
		require.NoError(t, err, "load license key")
		cfg.LicenseKey = st.licenseKey
	}
	attempts := accounts.NewAttempts(client,
		accounts.WithClock(st.now),
		accounts.WithRateLimits(cfg.RateLimits))
	limits := abuse.DefaultLimits()
	if st.volumeFloor > 0 {
		limits.VolumeFloor = st.volumeFloor
	}
	edition, err := ee.New(client, bus, lic, cipher, cfg.JWTSecrets(), staff.Config{
		Secret: newStaffSecret(), SessionTTL: cfg.StaffSessionTTL, HoldMaxExpiry: cfg.StaffHoldMaxExpiry, SecureCookies: cfg.SecureCookies(), Clock: st.now,
		Attempts: attempts, Abuse: limits, SupportEmail: cfg.SupportEmail, Passwords: Passwords(),
	})
	require.NoError(t, err, "build edition")
	// The Edition's ent hooks, on the client and on every transaction the bus opens.
	client.Use(edition.Hooks()...)
	bus.Use(edition.Hooks()...)
	outboundOpts := []outbound.Option{outbound.WithTrackingDomains(), outbound.WithTestSendBudget(attempts)}
	switch {
	case st.noSystemSender:
	case st.noSystemFrom:
		outboundOpts = append(outboundOpts, outbound.WithSystemSender(struct{ messaging.EmailSender }{systemMail}))
	default:
		outboundOpts = append(outboundOpts, outbound.WithSystemSender(systemMailFrom{systemMail}))
	}
	fakeSES := &FakeSES{}
	catalog := catalogWith(fakeSES)
	resolver := inboxResolver{
		fixed: fixedResolver{sender: customerMail},
		real:  messaging.NewResolver(cipher, catalog, messaging.WithCapturers(capturedModule.Capturer)),
	}
	sender := outbound.New(bus, edition.Holds(), resolver, tracker, outboundOpts...)
	inline := jobs.NewInline(client, sender, systemMail, domainverify.New(client, stubTXT, nil, accounts.OwnerNotifier(client, systemMail)), cipher, catalog, cfg.AppURL)
	// The transactional send surface resolves a workspace sender directly (not via
	// river), so it gets the same capturing resolver — its sends land in CustomerMail.
	// inline implements every enqueue seam (broadcast, welcome, account mail,
	// sending-domain verify).
	// Domain modules, built once here and shared by /site and /api exactly like the
	// app's DI singletons.
	eventLog := eventlog.New(bus, eventlog.LivePolicy{Window: cfg.EventLiveWindow, Now: st.now})
	segmentsModule := segments.New(bus)
	contactsModule := contacts.New(bus)
	erasureModule := erasure.New(bus)
	templatesModule := templates.New(Renderer())
	brandKitModule := brandkit.New(bus, templatesModule)
	tagsModule := tags.New()
	webhooksModule := webhooks.New(bus, cipher, edition.Audit, webhook.NewClientAllowing(webhooks.TestSendTimeout, cfg.OutboundAllow), nil)
	// An `sns` Preset checks signatures against the test certificate, so a test posts a
	// notification signed offline; confirmations are recorded, never fetched. A test with
	// certificates of its own loads the Presets over them.
	presets, err := testPresets()
	if st.certs != nil {
		presets, err = preset.Load(nil, preset.WithSNSCertificates(st.certs))
	}
	if err != nil {
		t.Fatalf("load presets: %v", err)
	}
	confirmations := &Confirmations{}
	integrationsModule := integrations.New(bus, cipher, catalog, inline, presets, cfg.AppURL, integrations.WithSubscriptionConfirmer(confirmations.confirm))
	sendingDomainsModule := sendingdomains.New(bus, cipher, inline, cfg.TrackingCNAMETarget)
	joinDomainsModule := joindomains.New(client, bus, stubTXT, st.now, accounts.OwnerNotifier(client, systemMail))
	automationsModule := automations.New(cipher, bus, Renderer())
	rulesModule := rules.New(cipher, bus)
	broadcastsModule := broadcasts.New(bus, Renderer())
	acc := accounts.New(client, bus, Passwords(), accounts.WithSignupRule(st.signupRuleOf(&cfg)))
	external, err := server.NewExternalAPI(client, apiexternal.Deps{
		Clock: st.now, Accounts: acc, Bus: bus, Webhooks: webhooksModule, Outbound: sender,
		Segments: segmentsModule, EventLog: eventLog, Contacts: contactsModule, Erasure: erasureModule, Tags: tagsModule, Templates: templatesModule, Transactional: transactional.New(templatesModule, sender),
		Automations: automationsModule, Rules: rulesModule, Broadcasts: broadcastsModule, Reputation: reputation.New(), Integrations: integrationsModule, CapturedMessages: capturedModule, SendingDomains: sendingDomainsModule,
		BootstrapToken: baseCfg.BootstrapToken, Audit: edition.Audit, Export: edition.Export,
	})
	require.NoError(t, err, "build external API")
	mcpHandler, err := mcpserver.New(sphericon.ExternalOpenAPI, external, apiauth.NewExternalSecurityHandler(client, bus, st.now), mcpserver.WithResourceMetadataURL(oauthserver.ResourceMetadataURL(cfg.MCPOrigin())))
	require.NoError(t, err, "build MCP handler")
	oauthFetcher := st.oauthFetcher
	if oauthFetcher == nil {
		oauthFetcher = oauthserver.NewFetcher(cfg.OutboundAllow)
	}
	secondFactor := secondfactor.New(client, bus, cipher, st.now)
	passkeys, err := secondfactor.NewPasskeys(client, bus, cfg.AppURL, cfg.JWTSecret, st.now)
	require.NoError(t, err, "build passkeys")
	staffHandler, err := server.NewStaffAPI(edition.Staff(bus, systemMail))
	require.NoError(t, err, "build staff API")
	accountLink, _, err := accountlink.FromConfig(&cfg)
	require.NoError(t, err, "build account link")
	source, err := upgrade.NewGitHubAt(cfg.GitHubAPIURL)
	require.NoError(t, err, "build release source")
	lapsedAt, _ := edition.LicenseLapsedAt()
	inst := instance.New(client, instance.EnvironmentOf(&cfg), instance.HostOf(&cfg, st.version, lapsedAt), st.now, instance.WithReleaseSource(source))
	relay := &FakeRelayRanges{}
	detector := machine.New(client, relay)
	handler, err := server.New(&cfg, txDB, migrated, client, apisite.Deps{
		SignupSurvey: signupsurvey.New(bus, signupsurvey.Enabled(cfg.SignupSurvey, edition.StaffLicensed(), slog.Default())),
		Deletion:     workspacedeletion.New(client, bus),
		AccountLink:  accountLink, Holds: edition.Holds(), AbuseWatch: edition.AbuseWatch(), SupportEmail: cfg.SupportEmail, Origins: apisite.Origins{MCP: cfg.MCPOrigin(), Tracker: cfg.TrackerOrigin()},
		Instance: inst,
		Accounts: acc, Passwords: Passwords(), Attempts: attempts, OAuth: oauthserver.NewService(client, oauthFetcher), Bus: bus, Webhooks: webhooksModule, Outbound: sender,
		Segments: segmentsModule, EventLog: eventLog, Contacts: contactsModule, Erasure: erasureModule, Tags: tagsModule, Templates: templatesModule, BrandKit: brandKitModule,
		Automations: automationsModule, Rules: rulesModule, Broadcasts: broadcastsModule,
		Welcome: inline, SysMail: inline, SendingDomains: sendingDomainsModule, JoinDomains: joinDomainsModule, Integrations: integrationsModule, CapturedMessages: capturedModule,
		Tokens: authtoken.New(baseCfg.JWTSecret).WithClock(st.now), Tracker: tracker, AppURL: baseCfg.AppURL, Audit: edition.Audit, Analytics: analytics.New().WithClock(st.now).WithRetention(cfg.EventsRetention),
		SecondFactor:             secondFactor,
		Passkeys:                 passkeys,
		Clock:                    st.now,
		RequireEmailVerification: cfg.RequireEmailVerification,
		Signin:                   signin.FromConfig(&cfg, st.now, st.signinOpts...),
	}, detector, external, mcpHandler, staffHandler)
	require.NoError(t, err, "build server")
	internalHandler, err := server.NewInternal(&cfg, client, edition.Service(bus))
	require.NoError(t, err, "build internal handler")

	return &TestEnv{
		sender:   sender,
		Instance: inst, AppURL: cfg.AppURL, DB: client, SQLDB: txDB, Bus: bus, Server: handler, Internal: internalHandler, Tracker: tracker, Cipher: cipher, Integrations: integrationsModule, jwtSecret: baseCfg.JWTSecret, edition: edition, orders: orders.New(), Holds: edition.Holds(), now: st.now, sessionTTL: cfg.SessionTTL,
		SystemMail: systemMail, CustomerMail: customerMail, SES: fakeSES, Machine: detector, PrivateRelay: relay, Confirmations: confirmations, SecondFactor: secondFactor, StaffMembers: edition.StaffMembers,
	}
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

// Messages returns a copy of the captured sends.
func (s *CapturingSender) Messages() []messaging.EmailMessage {
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
	if integ != nil && messaging.Provider(integ.Provider) == messaging.ProviderInbox {
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

// migrateBaseline brings an empty database to the fixture state every test starts from:
// the schema, then the committed fixtures. A pgtestdb template gets it once per change of
// either (RealDB).
func migrateBaseline(ctx context.Context, sqlDB *sql.DB, cfg *config.Config, projectRoot string) error {
	if err := migrateSchema(ctx, sqlDB); err != nil {
		return err
	}
	return loadFixtures(ctx, sqlDB, cfg, projectRoot)
}

// migrateSchema applies the embedded migrations to an empty database: the same files, the same
// way (goose), that production gets, so the app schema, river's tables and the domain-event
// outbox all come from them. The worker databases are cloned from a template that has just
// this (see initBaseline).
func migrateSchema(ctx context.Context, sqlDB *sql.DB) error {
	_, err := db.Migrate(ctx, sqlDB)
	return err
}

// loadFixtures replaces the contents of every fixture table with the committed rows. It
// runs at the start of every test process, never once into a template: fixtures use
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
