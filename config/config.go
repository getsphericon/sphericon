package config

import (
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/caarlos0/env/v11"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/joho/godotenv"
)

// BodyLimits are the request body caps in bytes. /collect is the public tracking
// ingestion (its key ships in customer pages, so anyone can post to it): Collect
// caps a batch (POST /collect/events), CollectEvent one event (an identify body, or
// each event inside a batch). Default applies to every other surface.
type BodyLimits struct {
	Default      int64
	Collect      int64
	CollectEvent int64
}

// RateLimits are the per-policy request budgets per minute (ADR 0025). Every limit
// has a default and 0 disables it. They are core, never gated by the EE licence.
type RateLimits struct {
	// Human caps the public human-facing endpoints (signup, invitation accept,
	// consent confirm) per client IP and endpoint.
	Human int
	// APIBurst caps /api and /mcp per Workspace per second (one shared budget).
	APIBurst int
	// APIPerMinute caps /api and /mcp per Workspace per minute, stacked on APIBurst.
	APIPerMinute int
	// FailedAuth caps failed credential checks (bearer token, collect key) per
	// client IP per minute; successful ones are not counted.
	FailedAuth int
	// Tracking caps how many opens and clicks one client IP may have recorded per
	// minute. It never refuses a recipient: over it the event is not recorded.
	Tracking int
	// LoginFailures is how many failed logins one account may have within 15 minutes
	// before login answers 429 with an exponentially growing delay (no lockout).
	LoginFailures int
	// LoginIP caps login requests per client IP per minute.
	LoginIP int
	// Collect caps /collect per Workspace per minute (a budget of its own, apart
	// from /api and /mcp).
	Collect int
	// CollectIP caps /collect per client IP per minute.
	CollectIP int
	// ServicePerMinute caps the internal listener's service endpoints per Service
	// credential per minute.
	ServicePerMinute int
	// ForgotAddress is how many mails one address may be sent per hour by each of
	// forgot-password, registration and an email-change request (ADR 0031). Over it
	// the request is still answered 202 and nothing is sent.
	ForgotAddress int
	// ForgotIP caps forgot-password requests per client IP per hour (429 over it).
	ForgotIP int
}

// DefaultRateLimits are the production budgets.
var DefaultRateLimits = RateLimits{Human: 60, APIBurst: 20, APIPerMinute: 600, FailedAuth: 30, Tracking: 600, LoginFailures: 5, LoginIP: 20, Collect: 6000, CollectIP: 300, ServicePerMinute: 600, ForgotAddress: 3, ForgotIP: 10}

// DBPool bounds the Postgres connections one replica may open: the
// database/sql pool (ent, pubsub) and the pgx pool river runs on. Defaults
// assume at most two replicas against max_connections=100:
// (15+25) x 2 x 1.15 = 92 <= 97.
type DBPool struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	// PGXMaxConns bounds river's own queries (fetch, completion, LISTEN, leader election); job
	// workers use the database/sql pool, so it need not match river's MaxWorkers sum.
	PGXMaxConns int32
}

// telemetryReceiverURL is the address of the Telemetry report receiver, fixed in the binary.
const telemetryReceiverURL = "https://telemetry.getsphericon.app"

// telemetrySchemaURL is the page that documents the Telemetry report's schema.
const telemetrySchemaURL = "https://docs.getsphericon.app/operations/telemetry-schema"

type Config struct {
	DatabaseURL    string
	Port           string
	CollectSiteKey string
	BootstrapToken string
	CORSOrigins    []string
	JWTSecret      string
	// SessionTTL is the lifetime of a site session: both the token's exp and the
	// cookie's Max-Age (SPHERICON_SESSION_TTL, default 24h, no refresh; ADR 0020).
	SessionTTL time.Duration
	AppURL     string
	// MCPURL is the public origin of a dedicated MCP host (SPHERICON_MCP_URL); empty means the
	// MCP resource lives on AppURL. Read it through MCPOrigin.
	MCPURL        string
	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPass      string
	SMTPFrom      string
	EncryptionKey string
	// LicenseKey is the offline EE license key (ADR 0014). Empty runs the plain core.
	LicenseKey string
	// AccountLinkURL is the address of the external account page the Owner's Account
	// link opens (SPHERICON_ACCOUNT_LINK_URL). The link is enabled only with AccountLinkSigningKey too.
	AccountLinkURL string
	// AccountLinkSigningKey is the base64 Ed25519 seed that signs the Account link
	// token (SPHERICON_ACCOUNT_LINK_SIGNING_KEY).
	AccountLinkSigningKey string
	// StaffJWTSecret signs the Staff member session and login challenge (SPHERICON_STAFF_JWT_SECRET,
	// ADR 0026). It is required, and must differ from SPHERICON_JWT_SECRET, only when the license
	// has the `staff` feature; ee.New validates it there.
	StaffJWTSecret string
	// StaffSessionTTL is the lifetime of a Staff member session (SPHERICON_STAFF_SESSION_TTL,
	// default 4h, no refresh, no remember-me): shorter than a customer's.
	StaffSessionTTL time.Duration
	// StaffHoldMaxExpiry is how far ahead a service may set a Staff hold's expiry
	// (SPHERICON_STAFF_HOLD_MAX_EXPIRY, default 720h): it is refused beyond it.
	StaffHoldMaxExpiry time.Duration
	// AbuseVolumeFloor is the fewest sends in the 24-hour window, per Sending domain, for
	// the abuse detector to judge a rate (SPHERICON_ABUSE_VOLUME_FLOOR, default 1000; ADR 0026).
	AbuseVolumeFloor int
	// The abuse detector's limits, in percent (ADR 0026): a warning at the first pair, a
	// freeze at the second. Defaults: warn 0.1% complaints / 5% bounces, freeze 0.3% / 10%.
	AbuseWarnComplaintPercent, AbuseWarnBouncePercent     float64
	AbuseFreezeComplaintPercent, AbuseFreezeBouncePercent float64
	// SupportEmail is the support contact shown with a suspension and in the owner notices
	// (SPHERICON_SUPPORT_EMAIL): where an owner appeals. Empty shows none.
	SupportEmail string
	// BillingURL is the billing site a Workspace member under a hold is pointed to
	// (SPHERICON_BILLING_URL). Empty (the self-host default) shows no link and no billing text.
	BillingURL  string
	AutoMigrate bool
	// MigrateTimeout bounds the lock wait and the run of a start with AutoMigrate on
	// (SPHERICON_DB_MIGRATE_TIMEOUT); 0 is no limit.
	MigrateTimeout time.Duration
	BodyLimits     BodyLimits
	RateLimits     RateLimits
	// OutboxFloor is the minimum age of a domain-event outbox row before the
	// prune job may delete it (SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS, default 7; ADR 0019).
	OutboxFloor time.Duration
	// EventsRetention is the age past which analytical Events are deleted
	// (SPHERICON_EVENTS_RETENTION_DAYS, default 400, 0 disables; ADR 0019).
	EventsRetention time.Duration
	// UnverifiedUserRetention is the age past which an Unverified User with no Membership
	// is deleted (SPHERICON_UNVERIFIED_USER_RETENTION_DAYS, default 7, 0 disables; ADR 0030).
	UnverifiedUserRetention time.Duration
	DBPool                  DBPool
	// IsDev is true for non-production envs (development/test). Used to relax
	// production-only behaviour locally — e.g. the sending-domain DKIM re-check
	// trusts seeded domains instead of hitting real DNS (ADR 0010).
	IsDev bool
	// SMTPAllowPrivateHosts lets a Workspace's SMTP Integration point at a loopback or
	// private address and any port (SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS). Off in production by
	// default, so a tenant cannot use the server to reach internal hosts; on in
	// development and tests, where the relay is a local Mailpit.
	SMTPAllowPrivateHosts bool
	// OutboundAllow names the internal host:port pairs an outbound call to a third-party URL
	// (a Webhook endpoint) may reach although the address is private (SPHERICON_OUTBOUND_ALLOW,
	// ADR 0039). Empty by default and never set in SaaS; a malformed value fails the load.
	OutboundAllow webhook.Allowlist

	// Locale is the instance-wide UI/email language (a SaaS deployment runs per
	// country). One of the supported locales; anything else is coerced to "en".
	// Drives both the SPA (injected into index.html) and the backend's own
	// system emails and validation messages (internal/i18n).
	Locale string

	// Telemetry is SPHERICON_TELEMETRY when it is set to true or false: the environment
	// then decides the Telemetry report (ADR 0032). Nil (unset, or any other value) leaves
	// the decision to the Instance's Owner.
	Telemetry *bool
	// DoNotTrack is DO_NOT_TRACK=1: it switches the Telemetry report off and never on.
	DoNotTrack bool
	// TelemetryReceiverURL is where the daily Telemetry report goes. It is a constant of the
	// binary with no environment variable (ADR 0032): a test points it at its own receiver.
	TelemetryReceiverURL string
	// TelemetrySchemaURL is the page that documents the Telemetry report's schema; the SPA
	// links to it. A constant of the binary with no environment variable.
	TelemetrySchemaURL string
	// ReleaseCheck is SPHERICON_RELEASE_CHECK (default on): the daily Release check for a
	// newer release (ADR 0032). It sends nothing about the Instance and is independent of
	// the Telemetry report and DO_NOT_TRACK.
	ReleaseCheck bool
	// GitHubAPIURL is the API root the Release check asks; empty is api.github.com. It has
	// no environment variable: tests point it at a server of their own.
	GitHubAPIURL string

	// RequireEmailVerification keeps the email verification gate on (ADR 0030). Only an
	// explicit SPHERICON_REQUIRE_EMAIL_VERIFICATION=false turns it off, for a self-hosted install
	// without outbound mail; the default and any other value keep it on. It is never
	// inferred from the SMTP settings.
	RequireEmailVerification bool

	// Logging: level is one of debug|info|warn|error; format is text|json.
	// Dev defaults to a human-readable coloured text handler, prod to JSON.
	LogLevel  string
	LogFormat string

	// OtelServiceName is the service.name reported on OTel traces/metrics. The
	// export destinations come from the declarative config file OTEL_CONFIG_FILE
	// names (read by otelconf in internal/telemetry).
	OtelServiceName string

	// MetricsAddr (host:port) is where the opt-in Prometheus listener binds. Empty
	// (the default) means no listener; the public port never serves /metrics
	// (ADR 0025).
	MetricsAddr string

	// InternalAddr (host:port) is where the opt-in internal listener binds: the only
	// place the service endpoints of the Staff surface are served (ADR 0026). Empty
	// (the default) means no listener and no route for them anywhere; it must differ
	// from the public PORT and from SPHERICON_METRICS_ADDR. Nothing is served either without the
	// `staff` license.
	InternalAddr string

	// PresetsDir is a directory of the Instance's custom Presets (ADR 0037): every
	// *.yaml and *.yml file at its root loads beside the built-in ones at start, and a
	// file that does not load, or reuses a built-in name, stops the start. Empty (the
	// default) means only the built-in Presets.
	PresetsDir string

	// System (platform) transactional email — sphericon's OWN sender, distinct from a
	// customer's per-workspace integration. Dev uses smtp → mailpit (the SMTP_*
	// values); prod uses ses (the SES_* values). Sent via the same messaging
	// Catalog, not a separate sender.
	SystemEmailProvider string // "smtp" | "ses"
	SystemEmailFrom     string
	SESRegion           string
	SESAccessKeyID      string
	SESSecretAccessKey  string
}

// environment is every variable Load reads, with the defaults that do not depend on
// the environment name. Those that do (rate limits in the e2e profile, the log
// format, SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS) are seeded into the variable map by seedDefaults.
type environment struct {
	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	Port        string `env:"PORT" envDefault:"3000"`
	AppURL      string `env:"SPHERICON_APP_URL" envDefault:"http://localhost:3000"`
	MCPURL      string `env:"SPHERICON_MCP_URL"`

	CollectSiteKey string        `env:"SPHERICON_COLLECT_SITE_KEY"`
	BootstrapToken string        `env:"SPHERICON_BOOTSTRAP_TOKEN"`
	CORSOrigins    string        `env:"SPHERICON_CORS_ORIGINS"`
	JWTSecret      string        `env:"SPHERICON_JWT_SECRET"`
	SessionTTL     time.Duration `env:"SPHERICON_SESSION_TTL" envDefault:"24h"`
	EncryptionKey  string        `env:"SPHERICON_ENCRYPTION_KEY"`
	LicenseKey     string        `env:"SPHERICON_LICENSE_KEY"`

	SMTPHost string `env:"SPHERICON_SMTP_HOST"`
	SMTPPort int    `env:"SPHERICON_SMTP_PORT" envDefault:"1025"`
	SMTPUser string `env:"SPHERICON_SMTP_USER"`
	SMTPPass string `env:"SPHERICON_SMTP_PASS"`
	SMTPFrom string `env:"SPHERICON_SMTP_FROM"`

	AccountLinkURL        string `env:"SPHERICON_ACCOUNT_LINK_URL"`
	AccountLinkSigningKey string `env:"SPHERICON_ACCOUNT_LINK_SIGNING_KEY"`

	StaffJWTSecret     string        `env:"SPHERICON_STAFF_JWT_SECRET"`
	StaffSessionTTL    time.Duration `env:"SPHERICON_STAFF_SESSION_TTL" envDefault:"4h"`
	StaffHoldMaxExpiry time.Duration `env:"SPHERICON_STAFF_HOLD_MAX_EXPIRY" envDefault:"720h"`

	AbuseVolumeFloor            int     `env:"SPHERICON_ABUSE_VOLUME_FLOOR" envDefault:"1000"`
	AbuseWarnComplaintPercent   float64 `env:"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT" envDefault:"0.1"`
	AbuseWarnBouncePercent      float64 `env:"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT" envDefault:"5"`
	AbuseFreezeComplaintPercent float64 `env:"SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT" envDefault:"0.3"`
	AbuseFreezeBouncePercent    float64 `env:"SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT" envDefault:"10"`

	SupportEmail string `env:"SPHERICON_SUPPORT_EMAIL"`
	BillingURL   string `env:"SPHERICON_BILLING_URL"`
	AutoMigrate  bool   `env:"SPHERICON_DB_AUTO_MIGRATE"`

	MigrateTimeout time.Duration `env:"SPHERICON_DB_MIGRATE_TIMEOUT" envDefault:"10m"`

	MaxBodyBytes         int64 `env:"SPHERICON_MAX_BODY_BYTES" envDefault:"1048576"`
	CollectMaxBodyBytes  int64 `env:"SPHERICON_COLLECT_MAX_BODY_BYTES" envDefault:"512000"`
	CollectMaxEventBytes int64 `env:"SPHERICON_COLLECT_MAX_EVENT_BYTES" envDefault:"32768"`

	RateLimitHuman         int `env:"SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE"`
	RateLimitAPIBurst      int `env:"SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND"`
	RateLimitAPIPerMinute  int `env:"SPHERICON_RATE_LIMIT_API_PER_MINUTE"`
	RateLimitFailedAuth    int `env:"SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE"`
	RateLimitTracking      int `env:"SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE"`
	RateLimitLoginFailures int `env:"SPHERICON_RATE_LIMIT_LOGIN_FAILURES"`
	RateLimitLoginIP       int `env:"SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE"`
	RateLimitCollect       int `env:"SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE"`
	RateLimitCollectIP     int `env:"SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE"`
	RateLimitService       int `env:"SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE"`
	RateLimitForgotAddress int `env:"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR"`
	RateLimitForgotIP      int `env:"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR"`

	OutboxFloorDays             int `env:"SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS" envDefault:"7"`
	EventsRetentionDays         int `env:"SPHERICON_EVENTS_RETENTION_DAYS" envDefault:"400"`
	UnverifiedUserRetentionDays int `env:"SPHERICON_UNVERIFIED_USER_RETENTION_DAYS" envDefault:"7"`

	DBMaxOpenConns    int           `env:"SPHERICON_DB_MAX_OPEN_CONNS" envDefault:"15"`
	DBMaxIdleConns    *int          `env:"SPHERICON_DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime time.Duration `env:"SPHERICON_DB_CONN_MAX_LIFETIME" envDefault:"30m"`
	PGXMaxConns       int32         `env:"SPHERICON_PGX_MAX_CONNS" envDefault:"25"`

	SMTPAllowPrivateHosts    bool   `env:"SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS"`
	OutboundAllow            string `env:"SPHERICON_OUTBOUND_ALLOW"`
	Locale                   string `env:"SPHERICON_APP_LOCALE" envDefault:"en"`
	Telemetry                string `env:"SPHERICON_TELEMETRY"`
	DoNotTrack               string `env:"DO_NOT_TRACK"`
	ReleaseCheck             bool   `env:"SPHERICON_RELEASE_CHECK" envDefault:"true"`
	RequireEmailVerification string `env:"SPHERICON_REQUIRE_EMAIL_VERIFICATION"`

	LogLevel        string `env:"SPHERICON_LOG_LEVEL" envDefault:"info"`
	LogFormat       string `env:"SPHERICON_LOG_FORMAT"`
	OtelServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"sphericon"`
	MetricsAddr     string `env:"SPHERICON_METRICS_ADDR"`
	InternalAddr    string `env:"SPHERICON_INTERNAL_ADDR"`
	PresetsDir      string `env:"SPHERICON_PRESETS_DIR"`

	SystemEmailProvider string `env:"SPHERICON_SYSTEM_EMAIL_PROVIDER" envDefault:"smtp"`
	SystemEmailFrom     string `env:"SPHERICON_SYSTEM_EMAIL_FROM" envDefault:"noreply@sphericon.localhost"`
	SESRegion           string `env:"SPHERICON_SES_REGION"`
	SESAccessKeyID      string `env:"SPHERICON_SES_ACCESS_KEY_ID"`
	SESSecretAccessKey  string `env:"SPHERICON_SES_SECRET_ACCESS_KEY"`
}

// Load reads the configuration from the process environment layered over the .env
// files (see envFiles): a variable the process sets wins over every file.
func Load(envName string) (*Config, error) {
	_, file, _, _ := runtime.Caller(0)
	rootDir := filepath.Join(filepath.Dir(file), "..")

	vars, err := readEnvFiles(envFiles(rootDir, envName))
	if err != nil {
		return nil, err
	}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && v != "" {
			vars[k] = v
		}
	}
	seedDefaults(vars, envName)

	var e environment
	if err := env.ParseWithOptions(&e, env.Options{Environment: vars}); err != nil {
		return nil, fmt.Errorf("read environment: %w", err)
	}

	outboundAllow, err := webhook.ParseAllowlist(e.OutboundAllow)
	if err != nil {
		return nil, fmt.Errorf("SPHERICON_OUTBOUND_ALLOW: %w", err)
	}

	// Idle defaults to the open cap so connections are reused, not churned.
	maxIdle := e.DBMaxOpenConns
	if e.DBMaxIdleConns != nil {
		maxIdle = *e.DBMaxIdleConns
	}

	cfg := &Config{
		DatabaseURL:    e.DatabaseURL,
		Port:           e.Port,
		CollectSiteKey: e.CollectSiteKey,
		BootstrapToken: e.BootstrapToken,
		CORSOrigins:    strings.FieldsFunc(e.CORSOrigins, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }),
		JWTSecret:      e.JWTSecret,
		SessionTTL:     e.SessionTTL,
		AppURL:         e.AppURL,
		MCPURL:         e.MCPURL,
		SMTPHost:       e.SMTPHost,
		SMTPPort:       e.SMTPPort,
		SMTPUser:       e.SMTPUser,
		SMTPPass:       e.SMTPPass,
		SMTPFrom:       e.SMTPFrom,
		EncryptionKey:  e.EncryptionKey,
		LicenseKey:     e.LicenseKey,

		AccountLinkURL:        e.AccountLinkURL,
		AccountLinkSigningKey: e.AccountLinkSigningKey,

		StaffJWTSecret:     e.StaffJWTSecret,
		StaffSessionTTL:    e.StaffSessionTTL,
		StaffHoldMaxExpiry: e.StaffHoldMaxExpiry,

		AbuseVolumeFloor:            e.AbuseVolumeFloor,
		AbuseWarnComplaintPercent:   e.AbuseWarnComplaintPercent,
		AbuseWarnBouncePercent:      e.AbuseWarnBouncePercent,
		AbuseFreezeComplaintPercent: e.AbuseFreezeComplaintPercent,
		AbuseFreezeBouncePercent:    e.AbuseFreezeBouncePercent,
		SupportEmail:                strings.TrimSpace(e.SupportEmail),
		BillingURL:                  strings.TrimSpace(e.BillingURL),
		AutoMigrate:                 e.AutoMigrate,
		MigrateTimeout:              e.MigrateTimeout,

		BodyLimits: BodyLimits{
			Default:      e.MaxBodyBytes,
			Collect:      e.CollectMaxBodyBytes,
			CollectEvent: e.CollectMaxEventBytes,
		},
		RateLimits: RateLimits{
			Human:            e.RateLimitHuman,
			APIBurst:         e.RateLimitAPIBurst,
			APIPerMinute:     e.RateLimitAPIPerMinute,
			FailedAuth:       e.RateLimitFailedAuth,
			Tracking:         e.RateLimitTracking,
			LoginFailures:    e.RateLimitLoginFailures,
			LoginIP:          e.RateLimitLoginIP,
			Collect:          e.RateLimitCollect,
			CollectIP:        e.RateLimitCollectIP,
			ServicePerMinute: e.RateLimitService,
			ForgotAddress:    e.RateLimitForgotAddress,
			ForgotIP:         e.RateLimitForgotIP,
		},
		DBPool: DBPool{
			MaxOpenConns:    e.DBMaxOpenConns,
			MaxIdleConns:    maxIdle,
			ConnMaxLifetime: e.DBConnMaxLifetime,
			PGXMaxConns:     e.PGXMaxConns,
		},
		OutboxFloor:             time.Duration(e.OutboxFloorDays) * 24 * time.Hour,
		EventsRetention:         time.Duration(e.EventsRetentionDays) * 24 * time.Hour,
		UnverifiedUserRetention: time.Duration(e.UnverifiedUserRetentionDays) * 24 * time.Hour,
		IsDev:                   isDevEnv(envName),
		SMTPAllowPrivateHosts:   e.SMTPAllowPrivateHosts,
		OutboundAllow:           outboundAllow,
		Locale:                  i18n.Normalize(e.Locale),
		Telemetry:               parseExplicitBool(e.Telemetry),
		DoNotTrack:              strings.TrimSpace(e.DoNotTrack) == "1",
		TelemetryReceiverURL:    telemetryReceiverURL,
		TelemetrySchemaURL:      telemetrySchemaURL,
		ReleaseCheck:            e.ReleaseCheck,

		RequireEmailVerification: strings.TrimSpace(e.RequireEmailVerification) != "false",
		LogLevel:                 e.LogLevel,
		LogFormat:                e.LogFormat,

		OtelServiceName: e.OtelServiceName,
		MetricsAddr:     e.MetricsAddr,
		InternalAddr:    e.InternalAddr,
		PresetsDir:      e.PresetsDir,

		SystemEmailProvider: e.SystemEmailProvider,
		SystemEmailFrom:     e.SystemEmailFrom,
		SESRegion:           e.SESRegion,
		SESAccessKeyID:      e.SESAccessKeyID,
		SESSecretAccessKey:  e.SESSecretAccessKey,
	}

	if isE2E(envName) {
		// The suite never scrapes metrics; a global SPHERICON_METRICS_ADDR (the dev stack's) would
		// make concurrent runs, or a running dev stack, collide on the port.
		cfg.MetricsAddr = ""
		cfg.InternalAddr = "" // likewise a global SPHERICON_INTERNAL_ADDR
	}

	if err := cfg.validate(envName); err != nil {
		return nil, err
	}
	return cfg, nil
}

// readEnvFiles merges the .env files that exist, later files overriding earlier ones.
// A missing or unreadable file is skipped, as no file is required.
func readEnvFiles(files []string) (map[string]string, error) {
	vars := map[string]string{}
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			continue
		}
		parsed, err := godotenv.Read(file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		maps.Copy(vars, parsed)
	}
	return vars, nil
}

// seedDefaults fills the variables whose default depends on the environment name,
// unless the process or a file set them (an empty value counts as unset): the rate
// limits (all disabled in the e2e profile, which polls freely), the log format
// (human-readable in development, JSON elsewhere) and SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS.
func seedDefaults(vars map[string]string, envName string) {
	limits := DefaultRateLimits
	if isE2E(envName) {
		limits = RateLimits{}
	}
	logFormat := "json"
	if isDevEnv(envName) {
		logFormat = "text"
	}
	for key, value := range map[string]string{
		"SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE":                     strconv.Itoa(limits.Human),
		"SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND":                 strconv.Itoa(limits.APIBurst),
		"SPHERICON_RATE_LIMIT_API_PER_MINUTE":                       strconv.Itoa(limits.APIPerMinute),
		"SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE":               strconv.Itoa(limits.FailedAuth),
		"SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE":                  strconv.Itoa(limits.Tracking),
		"SPHERICON_RATE_LIMIT_LOGIN_FAILURES":                       strconv.Itoa(limits.LoginFailures),
		"SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE":                  strconv.Itoa(limits.LoginIP),
		"SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE":                   strconv.Itoa(limits.Collect),
		"SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE":                strconv.Itoa(limits.CollectIP),
		"SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE":                   strconv.Itoa(limits.ServicePerMinute),
		"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR": strconv.Itoa(limits.ForgotAddress),
		"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR":          strconv.Itoa(limits.ForgotIP),
		"SPHERICON_LOG_FORMAT":                                      logFormat,
		// Allowed in development, tests and the e2e suite (a local Mailpit), refused in production.
		"SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS": strconv.FormatBool(isDevEnv(envName) || isE2E(envName)),
	} {
		if vars[key] == "" {
			vars[key] = value
		}
	}
}

// SecureCookies reports whether session cookies carry the Secure attribute: whenever the
// instance is served over HTTPS (SPHERICON_APP_URL reflects the public scheme, even though a proxy
// terminates TLS and the Go server listens on plain HTTP). Local http dev stays false,
// where a Secure cookie would never be sent back.
func (c *Config) SecureCookies() bool { return strings.HasPrefix(c.AppURL, "https://") }

// validate enforces invariants that depend on the deployment environment.
func (c *Config) validate(envName string) error {
	// Outside development/test, an empty SPHERICON_JWT_SECRET silently signs auth tokens
	// with an empty key — refuse to boot rather than ship that footgun.
	if !isDevEnv(envName) && !isE2E(envName) {
		if err := validateJWTSecret(c.JWTSecret); err != nil {
			return err
		}
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("SPHERICON_SESSION_TTL must be positive")
	}
	if c.StaffSessionTTL <= 0 {
		return fmt.Errorf("SPHERICON_STAFF_SESSION_TTL must be positive")
	}
	if c.StaffHoldMaxExpiry <= 0 {
		return fmt.Errorf("SPHERICON_STAFF_HOLD_MAX_EXPIRY must be positive")
	}
	if c.AbuseVolumeFloor <= 0 {
		return fmt.Errorf("SPHERICON_ABUSE_VOLUME_FLOOR must be positive")
	}
	if !(0 < c.AbuseWarnComplaintPercent && c.AbuseWarnComplaintPercent < c.AbuseFreezeComplaintPercent && c.AbuseFreezeComplaintPercent <= 100) {
		return fmt.Errorf("SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT must be above 0 and below SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT, which is at most 100")
	}
	if !(0 < c.AbuseWarnBouncePercent && c.AbuseWarnBouncePercent < c.AbuseFreezeBouncePercent && c.AbuseFreezeBouncePercent <= 100) {
		return fmt.Errorf("SPHERICON_ABUSE_WARN_BOUNCE_PERCENT must be above 0 and below SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT, which is at most 100")
	}
	if (c.AccountLinkURL == "") != (c.AccountLinkSigningKey == "") {
		return fmt.Errorf("SPHERICON_ACCOUNT_LINK_URL and SPHERICON_ACCOUNT_LINK_SIGNING_KEY must be set together")
	}
	if c.BodyLimits.Default <= 0 {
		return fmt.Errorf("SPHERICON_MAX_BODY_BYTES must be positive")
	}
	if c.BodyLimits.Collect <= 0 {
		return fmt.Errorf("SPHERICON_COLLECT_MAX_BODY_BYTES must be positive")
	}
	if c.BodyLimits.CollectEvent <= 0 {
		return fmt.Errorf("SPHERICON_COLLECT_MAX_EVENT_BYTES must be positive")
	}
	for name, limit := range map[string]int{
		"SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE":       c.RateLimits.Human,
		"SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND":   c.RateLimits.APIBurst,
		"SPHERICON_RATE_LIMIT_API_PER_MINUTE":         c.RateLimits.APIPerMinute,
		"SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE": c.RateLimits.FailedAuth,
	} {
		if limit < 0 {
			return fmt.Errorf("%s must not be negative (0 disables)", name)
		}
	}
	if c.RateLimits.Tracking < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.LoginFailures < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_LOGIN_FAILURES must not be negative (0 disables)")
	}
	if c.RateLimits.LoginIP < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.Collect < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.CollectIP < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.ServicePerMinute < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.ForgotAddress < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR must not be negative (0 disables)")
	}
	if c.RateLimits.ForgotIP < 0 {
		return fmt.Errorf("SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR must not be negative (0 disables)")
	}
	if c.OutboxFloor < 0 {
		return fmt.Errorf("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS must not be negative")
	}
	if c.EventsRetention < 0 {
		return fmt.Errorf("SPHERICON_EVENTS_RETENTION_DAYS must not be negative")
	}
	if c.UnverifiedUserRetention < 0 {
		return fmt.Errorf("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS must not be negative")
	}
	if c.DBPool.MaxOpenConns <= 0 {
		return fmt.Errorf("SPHERICON_DB_MAX_OPEN_CONNS must be positive")
	}
	if c.DBPool.MaxIdleConns < 0 || c.DBPool.MaxIdleConns > c.DBPool.MaxOpenConns {
		return fmt.Errorf("SPHERICON_DB_MAX_IDLE_CONNS must be between 0 and SPHERICON_DB_MAX_OPEN_CONNS")
	}
	if c.DBPool.ConnMaxLifetime <= 0 {
		return fmt.Errorf("SPHERICON_DB_CONN_MAX_LIFETIME must be positive")
	}
	if c.DBPool.PGXMaxConns <= 0 {
		return fmt.Errorf("SPHERICON_PGX_MAX_CONNS must be positive")
	}
	if err := c.validateMetricsAddr(); err != nil {
		return err
	}
	return c.validateInternalAddr()
}

// validateMetricsAddr rejects a malformed SPHERICON_METRICS_ADDR and one sharing the public
// PORT (the public server binds every interface, so any host collides).
func (c *Config) validateMetricsAddr() error {
	_, err := c.listenPort("SPHERICON_METRICS_ADDR", c.MetricsAddr)
	return err
}

// validateInternalAddr rejects a malformed SPHERICON_INTERNAL_ADDR and one sharing the public
// PORT or the metrics listener's port: the internal listener is a server of its own.
func (c *Config) validateInternalAddr() error {
	port, err := c.listenPort("SPHERICON_INTERNAL_ADDR", c.InternalAddr)
	if err != nil || port == 0 {
		return err
	}
	if metrics, err := c.listenPort("SPHERICON_METRICS_ADDR", c.MetricsAddr); err == nil && metrics == port {
		return fmt.Errorf("SPHERICON_INTERNAL_ADDR must not use the SPHERICON_METRICS_ADDR port (%d)", port)
	}
	return nil
}

// listenPort is the port of an opt-in listener address, 0 when it is empty (no
// listener). A malformed address and one on the public PORT are refused (the public
// server binds every interface, so any host collides).
func (c *Config) listenPort(name, addr string) (int, error) {
	if addr == "" {
		return 0, nil
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("%s must be host:port: %w", name, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s port %q is not a valid port", name, portStr)
	}
	if public, err := strconv.Atoi(c.Port); err == nil && public == port {
		return 0, fmt.Errorf("%s must not use the public PORT (%d)", name, port)
	}
	return port, nil
}

// minJWTSecretLength is the shortest SPHERICON_JWT_SECRET accepted outside development
// (32 characters = 256 bits when the secret is hex/random, the HS256 key size).
const minJWTSecretLength = 32

// placeholderSecretMarkers are lowercase fragments of documented example and
// development secrets; a secret containing one was copied, not generated.
var placeholderSecretMarkers = []string{"change-me", "changeme", "change-in-production", "dev-secret", "a-strong-secret"}

func validateJWTSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("SPHERICON_JWT_SECRET is required outside development")
	}
	if len(secret) < minJWTSecretLength {
		return fmt.Errorf("SPHERICON_JWT_SECRET must be at least %d characters outside development (e.g. `openssl rand -hex 32`)", minJWTSecretLength)
	}
	lower := strings.ToLower(secret)
	for _, m := range placeholderSecretMarkers {
		if strings.Contains(lower, m) {
			return fmt.Errorf("SPHERICON_JWT_SECRET looks like a placeholder; generate one with `openssl rand -hex 32`")
		}
	}
	return nil
}

// MCPOrigin is the public origin the MCP resource is served from: SPHERICON_MCP_URL when the
// deployment gives MCP its own host, otherwise AppURL.
func (c *Config) MCPOrigin() string {
	if c.MCPURL != "" {
		return c.MCPURL
	}
	return c.AppURL
}

// UseListener points the public URL and PORT at a listener the caller opened, so
// links built from AppURL reach that listener.
func (c *Config) UseListener(addr net.Addr) {
	c.AppURL = "http://" + addr.String()
	if _, port, err := net.SplitHostPort(addr.String()); err == nil {
		c.Port = port
	}
}

// EnvE2E is the end-to-end profile: a non-production env whose per-policy rate
// limits default to disabled (0), so a polling test client never trips ADR 0018/0025
// budgets. An explicit RATE_LIMIT_* variable still wins. It tolerates a missing
// SPHERICON_JWT_SECRET like development does, but is not IsDev: the dev DKIM lookup stays off
// and the harness injects its own.
const EnvE2E = "e2e"

// isE2E reports whether the env is the end-to-end profile.
func isE2E(envName string) bool { return envName == EnvE2E }

// isDevEnv reports whether the env is a non-production one where missing
// security secrets are tolerated (so local dev and tests boot without ceremony).
func isDevEnv(envName string) bool {
	return envName == "" || envName == "development" || envName == "test"
}

func envFiles(rootDir, envName string) []string {
	files := []string{
		filepath.Join(rootDir, ".env"),
		filepath.Join(rootDir, ".env."+envName),
	}
	if envName != "test" {
		files = append(files, filepath.Join(rootDir, ".env.local"))
	}
	files = append(files, filepath.Join(rootDir, ".env."+envName+".local"))
	return files
}

// parseExplicitBool is nil unless v is "true" or "false" (case-insensitive): a typo must
// not switch anything on or off.
func parseExplicitBool(v string) *bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		on := true
		return &on
	case "false":
		off := false
		return &off
	}
	return nil
}
