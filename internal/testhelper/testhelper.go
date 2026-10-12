package testhelper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"net/http/httptest"

	"github.com/DATA-DOG/go-txdb"
	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/operator"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accountlink"
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
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/mcpserver"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/go-testfixtures/testfixtures/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/require"
)

// Baseline is applied ONCE per test process: migrate schema + load fixtures
// (committed). Each test then opens a go-txdb connection — a real transaction
// that is rolled back on Close — so tests are fully isolated and the DB stays
// at the fixture state. No hand-rolled transaction plumbing.
var (
	once    sync.Once
	baseCfg *config.Config
	loadErr error
)

func initBaseline() {
	once.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		projectRoot := filepath.Join(filepath.Dir(file), "..", "..")

		cfg, err := config.Load("test")
		if err != nil {
			loadErr = err
			return
		}

		sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			loadErr = err
			return
		}
		defer func() { _ = sqlDB.Close() }()

		if err := db.NewEntClient(sqlDB).Schema.Create(context.Background()); err != nil {
			loadErr = err
			return
		}

		// The domain-event outbox topic table is a watermill-sql table, not part
		// of the ent schema, so create it here too (once per process, on the real
		// DB) — otherwise the transactional publisher hits "relation does not exist".
		if err := events.InitSchema(context.Background(), sqlDB); err != nil {
			loadErr = err
			return
		}

		// river's own tables, so Erasure can clear the jobs that name a Contact (the
		// queue itself stays inline in tests; see JobsOf and EnqueueJob).
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			loadErr = err
			return
		}
		defer pool.Close()
		if err := jobs.Migrate(context.Background(), pool); err != nil {
			loadErr = err
			return
		}

		cipher, err := secrets.NewCipher(cfg.EncryptionKey)
		if err != nil {
			loadErr = err
			return
		}

		loader, err := testfixtures.New(
			testfixtures.Database(sqlDB),
			testfixtures.Dialect("postgres"),
			// Template options MUST precede Directory: Directory eagerly reads and
			// renders the files when its option is applied. Fixtures use templating
			// for relative dates and load-time encryption (internal/fixtures).
			testfixtures.Template(),
			testfixtures.TemplateFuncs(fixtures.TemplateFuncs(cipher)),
			testfixtures.Directory(filepath.Join(projectRoot, "fixtures")),
			// ResetSequencesTo keeps the explicit fixture ids clear of app-inserted rows.
			testfixtures.ResetSequencesTo(100000),
		)
		if err != nil {
			loadErr = err
			return
		}
		if err := loader.Load(); err != nil {
			loadErr = err
			return
		}

		// Register a transactional driver backed by the real pgx connection.
		txdb.Register("txdb", "pgx", cfg.DatabaseURL)
		baseCfg = cfg
	})
}

type TestEnv struct {
	DB     *ent.Client // transaction-bound; rolled back when the test finishes
	SQLDB  *sql.DB     // the same txdb connection the ent client and bus ride
	Bus    *events.Bus // domain-event bus over SQLDB (nested savepoint per WithinTx)
	Server http.Handler
	// Internal is the handler of the internal listener (ADR 0026), built like the
	// composition root builds it: the service endpoints and nothing else. Nil on an
	// unlicensed instance, where no listener would be bound.
	Internal http.Handler
	// Tracker is the instance the server and the sender share: mint tracking,
	// unsubscribe and confirmation tokens with it, never with a second one.
	Tracker *tracking.Tracker
	// Cipher is the instance the server seals with, for modules a test builds directly.
	Cipher *secrets.Cipher

	edition *ee.Edition // the EE parts, built like the composition root builds them

	jwtSecret string // for tokens a test needs in a state the Tracker never mints

	now        func() time.Time // the clock the server checks site session expiry against
	sessionTTL time.Duration    // SESSION_TTL, the lifetime of the site tokens tests mint

	// Captured sends from the inline jobs adapter, for assertions.
	SystemMail   *CapturingSender // platform mail (welcome, …)
	CustomerMail *CapturingSender // workspace/campaign mail (broadcasts)

	// SES scripts the send quota every "ses" Integration reports.
	SES *FakeSES

	// SecondFactor is the module the server verifies Second factors with, on the
	// env's clock (ADR 0020).
	SecondFactor *secondfactor.Module

	// Operators is the Operator module the server logs staff in with (ADR 0026); it
	// refuses every call on an unlicensed env.
	Operators *operator.Module
}

// Option tunes the server a test builds with Setup.
type Option func(*setup)

// setup is what the options edit: a private copy of the test config, the clock the
// account attempt module reads, and whether the instance has no EE license.
type setup struct {
	cfg        *config.Config
	now        func() time.Time
	unlicensed bool
	// volumeFloor is the Operator console's rate floor; zero is the default.
	volumeFloor int
}

// WithoutLicense builds the instance with no EE license key, like a plain core
// self-host. Setup's default is an instance licensed for every EE feature.
func WithoutLicense() Option { return func(s *setup) { s.unlicensed = true } }

// WithOperatorVolumeFloor sets the fewest sends in the Operator console's window for a
// rate to be defined, so a test crosses the floor with a handful of fixture Events.
func WithOperatorVolumeFloor(n int) Option { return func(s *setup) { s.volumeFloor = n } }

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

// newOperatorSecret is the secret Operator sessions are signed with in a test env: random
// and its own, never the site's.
func newOperatorSecret() string { return rand.Text() + rand.Text() }

// testLicense mints, once per process, an EE license key signed by a throwaway key
// pair and verified through the same Parse the production composition root uses.
var testLicense = sync.OnceValues(func() (*licensekey.License, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	key, err := licensekey.Issue(priv, nil, licensekey.FeatureAudit, licensekey.FeatureRetention, licensekey.FeatureOperator)
	if err != nil {
		return nil, err
	}
	return licensekey.Parse(key, pub, time.Now())
})

func Setup(t *testing.T, opts ...Option) *TestEnv {
	t.Helper()
	initBaseline()
	require.NoError(t, loadErr, "init test baseline")

	cfg := *baseCfg
	cfg.RateLimits = config.RateLimits{}
	st := &setup{cfg: &cfg, now: time.Now}
	for _, opt := range opts {
		opt(st)
	}

	// dsn arg is just a pool identifier; each Open is its own transaction.
	txDB, err := sql.Open("txdb", t.Name())
	require.NoError(t, err, "open txdb")
	// All queries must ride the SAME transaction; >1 conn would each get a
	// separate tx and not see this test's writes (e.g. seeded tokens).
	txDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = txDB.Close() }) // rollback

	client := db.NewEntClient(txDB)
	// Bus over the same txdb connection: WithinTx opens a nested (savepoint)
	// transaction, so producer writes + outbox inserts roll back with the test.
	bus := events.New(txDB)

	// Inline jobs adapter (Rails :inline equivalent): enqueued jobs run
	// synchronously against capturing senders, so tests assert the real effect.
	systemMail := &CapturingSender{}
	customerMail := &CapturingSender{}
	resolver := fixedResolver{sender: customerMail}
	// stubTXT keeps sending-domain verification off real DNS: every lookup is
	// NXDOMAIN, so an inline verify deterministically resolves to "not published".
	stubTXT := func(context.Context, string) ([]string, error) {
		return nil, &net.DNSError{IsNotFound: true}
	}
	tracker := tracking.New(cfg.JWTSecret, cfg.AppURL)
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
	attempts := accounts.NewAttempts(client,
		accounts.WithClock(st.now),
		accounts.WithRateLimits(cfg.RateLimits))
	edition, err := ee.New(client, lic, cipher, cfg.JWTSecret, operator.Config{
		Secret: newOperatorSecret(), SessionTTL: cfg.OperatorSessionTTL, HoldMaxExpiry: cfg.OperatorHoldMaxExpiry, SecureCookies: cfg.SecureCookies(), Clock: st.now,
		Attempts: attempts, VolumeFloor: st.volumeFloor,
	})
	require.NoError(t, err, "build edition")
	// The Edition's ent hooks, on the client and on every transaction the bus opens.
	client.Use(edition.Hooks()...)
	bus.Use(edition.Hooks()...)
	// The Operator hold freezer rides the send chokepoint like the app's.
	sender := outbound.New(bus, resolver, tracker, outbound.WithFreezers(edition.Freezer()))
	fakeSES := &FakeSES{}
	catalog := catalogWith(fakeSES)
	inline := jobs.NewInline(client, sender, systemMail, stubTXT, cipher, catalog, cfg.AppURL)
	// The transactional send surface resolves a workspace sender directly (not via
	// river), so it gets the same capturing resolver — its sends land in CustomerMail.
	// inline implements every enqueue seam (broadcast, welcome, account mail,
	// sending-domain verify).
	// Domain modules, built once here and shared by /site and /api exactly like the
	// app's DI singletons.
	eventLog := eventlog.New(bus)
	segmentsModule := segments.New()
	contactsModule := contacts.New(bus)
	erasureModule := erasure.New(bus)
	templatesModule := templates.New()
	tagsModule := tags.New()
	webhooksModule := webhooks.New(cipher, edition.Audit)
	integrationsModule := integrations.New(bus, cipher, catalog, inline)
	sendingDomainsModule := sendingdomains.New(bus, cipher, inline)
	automationsModule := automations.New()
	broadcastsModule := broadcasts.New(inline)
	acc := accounts.New(client, bus)
	external, err := server.NewExternalAPI(client, apiexternal.Deps{
		Accounts: acc, Bus: bus, Webhooks: webhooksModule, Outbound: sender,
		Segments: segmentsModule, EventLog: eventLog, Contacts: contactsModule, Erasure: erasureModule, Tags: tagsModule, Templates: templatesModule,
		Automations: automationsModule, Broadcasts: broadcastsModule, Reputation: reputation.New(), Integrations: integrationsModule, SendingDomains: sendingDomainsModule,
		BootstrapToken: baseCfg.BootstrapToken, Audit: edition.Audit,
	})
	require.NoError(t, err, "build external API")
	mcpHandler, err := mcpserver.New(sphericon.ExternalOpenAPI, external, apiauth.NewExternalSecurityHandler(client, bus), mcpserver.WithResourceMetadataURL(oauthserver.ResourceMetadataURL(cfg.MCPOrigin())))
	require.NoError(t, err, "build MCP handler")
	secondFactor := secondfactor.New(client, bus, cipher, st.now)
	operatorHandler, err := server.NewOperatorAPI(edition.Operator(bus, systemMail))
	require.NoError(t, err, "build operator API")
	accountLink, err := accountlink.FromConfig(&cfg)
	require.NoError(t, err, "build account link")
	handler, err := server.New(&cfg, txDB, client, apisite.Deps{
		AccountLink: accountLink,
		Accounts:    acc, Attempts: attempts, OAuth: oauthserver.NewService(client), Bus: bus, Webhooks: webhooksModule, Outbound: sender,
		Segments: segmentsModule, EventLog: eventLog, Contacts: contactsModule, Erasure: erasureModule, Tags: tagsModule, Templates: templatesModule,
		Automations: automationsModule, Broadcasts: broadcastsModule,
		Welcome: inline, SysMail: inline, SendingDomains: sendingDomainsModule, Integrations: integrationsModule,
		Tokens: authtoken.New(baseCfg.JWTSecret), Tracker: tracker, AppURL: baseCfg.AppURL, Audit: edition.Audit, Analytics: analytics.New(),
		SecondFactor: secondFactor,
		Clock:        st.now,
	}, external, mcpHandler, operatorHandler)
	require.NoError(t, err, "build server")
	internalHandler, err := server.NewInternal(&cfg, edition.Service(bus))
	require.NoError(t, err, "build internal handler")

	return &TestEnv{
		DB: client, SQLDB: txDB, Bus: bus, Server: handler, Internal: internalHandler, Tracker: tracker, Cipher: cipher, jwtSecret: baseCfg.JWTSecret, edition: edition, now: st.now, sessionTTL: cfg.SessionTTL,
		SystemMail: systemMail, CustomerMail: customerMail, SES: fakeSES, SecondFactor: secondFactor, Operators: edition.Operators,
	}
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
func (s *CapturingSender) DefaultFrom() (string, string) { return "hello@codebasics.dev", "CodeBasics" }

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

// fixedResolver returns the same capturing sender for any workspace, so inline
// broadcast sends work without a configured integration.
type fixedResolver struct{ sender messaging.EmailSender }

func (r fixedResolver) EmailSender(context.Context, *ent.Scoped) (messaging.EmailSender, error) {
	return r.sender, nil
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
