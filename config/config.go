package config

import (
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/spf13/viper"
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
	// ForgotAddress is how many password-reset mails one address may be sent per
	// hour. Over it the request is still answered 202 and nothing is sent.
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

type Config struct {
	DatabaseURL    string
	Port           string
	CollectSiteKey string
	BootstrapToken string
	CORSOrigins    []string
	JWTSecret      string
	// SessionTTL is the lifetime of a site session: both the token's exp and the
	// cookie's Max-Age (SESSION_TTL, default 24h, no refresh; ADR 0020).
	SessionTTL time.Duration
	AppURL     string
	// MCPURL is the public origin of a dedicated MCP host (MCP_URL); empty means the
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
	// link opens (ACCOUNT_LINK_URL). The link is enabled only with AccountLinkSigningKey too.
	AccountLinkURL string
	// AccountLinkSigningKey is the base64 Ed25519 seed that signs the Account link
	// token (ACCOUNT_LINK_SIGNING_KEY).
	AccountLinkSigningKey string
	// OperatorJWTSecret signs the Operator session and login challenge (OPERATOR_JWT_SECRET,
	// ADR 0026). It is required, and must differ from JWT_SECRET, only when the license
	// has the `operator` feature; ee.New validates it there.
	OperatorJWTSecret string
	// OperatorSessionTTL is the lifetime of an Operator session (OPERATOR_SESSION_TTL,
	// default 4h, no refresh, no remember-me): shorter than a customer's.
	OperatorSessionTTL time.Duration
	// OperatorHoldMaxExpiry is how far ahead a service may set an Operator hold's expiry
	// (OPERATOR_HOLD_MAX_EXPIRY, default 720h): it is refused beyond it.
	OperatorHoldMaxExpiry time.Duration
	// AbuseVolumeFloor is the fewest sends in the 24-hour window, per Sending domain, for
	// the abuse detector to judge a rate (ABUSE_VOLUME_FLOOR, default 1000; ADR 0026).
	AbuseVolumeFloor int
	// The abuse detector's limits, in percent (ADR 0026): a warning at the first pair, a
	// freeze at the second. Defaults: warn 0.1% complaints / 5% bounces, freeze 0.3% / 10%.
	AbuseWarnComplaintPercent, AbuseWarnBouncePercent     float64
	AbuseFreezeComplaintPercent, AbuseFreezeBouncePercent float64
	// SupportEmail is the support contact shown with a suspension and in the owner notices
	// (SUPPORT_EMAIL): where an owner appeals. Empty shows none.
	SupportEmail string
	// BillingURL is the billing site a Workspace member under a hold is pointed to
	// (BILLING_URL). Empty (the self-host default) shows no link and no billing text.
	BillingURL  string
	AutoMigrate bool
	BodyLimits  BodyLimits
	RateLimits  RateLimits
	// OutboxFloor is the minimum age of a domain-event outbox row before the
	// prune job may delete it (OUTBOX_RETENTION_FLOOR_DAYS, default 7; ADR 0019).
	OutboxFloor time.Duration
	// EventsRetention is the age past which analytical Events are deleted
	// (EVENTS_RETENTION_DAYS, default 400, 0 disables; ADR 0019).
	EventsRetention time.Duration
	// UnverifiedUserRetention is the age past which an Unverified User with no Membership
	// is deleted (UNVERIFIED_USER_RETENTION_DAYS, default 7, 0 disables; ADR 0030).
	UnverifiedUserRetention time.Duration
	DBPool                  DBPool
	// IsDev is true for non-production envs (development/test). Used to relax
	// production-only behaviour locally — e.g. the sending-domain DKIM re-check
	// trusts seeded domains instead of hitting real DNS (ADR 0010).
	IsDev bool
	// SMTPAllowPrivateHosts lets a Workspace's SMTP Integration point at a loopback or
	// private address and any port (SMTP_ALLOW_PRIVATE_HOSTS). Off in production by
	// default, so a tenant cannot use the server to reach internal hosts; on in
	// development and tests, where the relay is a local Mailpit.
	SMTPAllowPrivateHosts bool

	// Locale is the instance-wide UI/email language (a SaaS deployment runs per
	// country). One of the supported locales; anything else is coerced to "en".
	// Drives both the SPA (injected into index.html) and the backend's own
	// system emails and validation messages (internal/i18n).
	Locale string

	// RequireEmailVerification keeps the email verification gate on (ADR 0030). Only an
	// explicit REQUIRE_EMAIL_VERIFICATION=false turns it off, for a self-hosted install
	// without outbound mail; the default and any other value keep it on. It is never
	// inferred from the SMTP settings.
	RequireEmailVerification bool

	// Logging: level is one of debug|info|warn|error; format is text|json.
	// Dev defaults to a human-readable coloured text handler, prod to JSON.
	LogLevel  string
	LogFormat string

	// OtelServiceName is the service.name reported on OTel traces/metrics. The
	// OTLP export target itself comes from the standard OTEL_EXPORTER_OTLP_* env
	// vars (read by the SDK).
	OtelServiceName string

	// MetricsAddr (host:port) is where the opt-in Prometheus listener binds. Empty
	// (the default) means no listener; the public port never serves /metrics
	// (ADR 0025).
	MetricsAddr string

	// InternalAddr (host:port) is where the opt-in internal listener binds: the only
	// place the service endpoints of the Operator surface are served (ADR 0026). Empty
	// (the default) means no listener and no route for them anywhere; it must differ
	// from the public PORT and from METRICS_ADDR. Nothing is served either without the
	// `operator` license.
	InternalAddr string

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

func Load(envName string) (*Config, error) {
	_, file, _, _ := runtime.Caller(0)
	rootDir := filepath.Join(filepath.Dir(file), "..")

	v := viper.New()
	v.SetDefault("PORT", "3000")
	v.SetDefault("APP_URL", "http://localhost:3000")
	v.SetDefault("SESSION_TTL", 24*time.Hour)
	v.SetDefault("OPERATOR_SESSION_TTL", 4*time.Hour)
	v.SetDefault("OPERATOR_HOLD_MAX_EXPIRY", 30*24*time.Hour)
	v.SetDefault("ABUSE_VOLUME_FLOOR", 1000)
	v.SetDefault("ABUSE_WARN_COMPLAINT_PERCENT", 0.1)
	v.SetDefault("ABUSE_WARN_BOUNCE_PERCENT", 5.0)
	v.SetDefault("ABUSE_FREEZE_COMPLAINT_PERCENT", 0.3)
	v.SetDefault("ABUSE_FREEZE_BOUNCE_PERCENT", 10.0)
	v.SetDefault("SMTP_PORT", 1025)
	v.SetDefault("SYSTEM_EMAIL_PROVIDER", "smtp")
	v.SetDefault("SYSTEM_EMAIL_FROM", "noreply@sphericon.localhost")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("OTEL_SERVICE_NAME", "sphericon")
	v.SetDefault("APP_LOCALE", "en")
	v.SetDefault("MAX_BODY_BYTES", 1<<20)
	v.SetDefault("COLLECT_MAX_BODY_BYTES", 500<<10)
	v.SetDefault("COLLECT_MAX_EVENT_BYTES", 32<<10)
	limits := DefaultRateLimits
	if isE2E(envName) {
		limits = RateLimits{} // the end-to-end profile polls freely: every budget is disabled
	}
	v.SetDefault("RATE_LIMIT_HUMAN_PER_MINUTE", limits.Human)
	v.SetDefault("RATE_LIMIT_API_BURST_PER_SECOND", limits.APIBurst)
	v.SetDefault("RATE_LIMIT_API_PER_MINUTE", limits.APIPerMinute)
	v.SetDefault("RATE_LIMIT_FAILED_AUTH_PER_MINUTE", limits.FailedAuth)
	v.SetDefault("RATE_LIMIT_TRACKING_PER_MINUTE", limits.Tracking)
	v.SetDefault("RATE_LIMIT_LOGIN_FAILURES", limits.LoginFailures)
	v.SetDefault("RATE_LIMIT_LOGIN_IP_PER_MINUTE", limits.LoginIP)
	v.SetDefault("RATE_LIMIT_COLLECT_PER_MINUTE", limits.Collect)
	v.SetDefault("RATE_LIMIT_COLLECT_IP_PER_MINUTE", limits.CollectIP)
	v.SetDefault("RATE_LIMIT_SERVICE_PER_MINUTE", limits.ServicePerMinute)
	v.SetDefault("RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR", limits.ForgotAddress)
	v.SetDefault("RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR", limits.ForgotIP)
	v.SetDefault("OUTBOX_RETENTION_FLOOR_DAYS", 7)
	v.SetDefault("EVENTS_RETENTION_DAYS", 400)
	v.SetDefault("UNVERIFIED_USER_RETENTION_DAYS", 7)
	v.SetDefault("DB_MAX_OPEN_CONNS", 15)
	v.SetDefault("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	v.SetDefault("PGX_MAX_CONNS", 25)
	// Human-readable logs in dev, structured JSON everywhere else.
	if isDevEnv(envName) {
		v.SetDefault("LOG_FORMAT", "text")
	} else {
		v.SetDefault("LOG_FORMAT", "json")
	}

	for _, file := range envFiles(rootDir, envName) {
		sub := viper.New()
		sub.SetConfigFile(file)
		sub.SetConfigType("env")
		if err := sub.ReadInConfig(); err == nil {
			if err := v.MergeConfigMap(sub.AllSettings()); err != nil {
				return nil, fmt.Errorf("merge config %s: %w", file, err)
			}
		}
	}

	v.AutomaticEnv()

	if !v.IsSet("DATABASE_URL") {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	// Idle defaults to the open cap so connections are reused, not churned.
	maxOpen := v.GetInt("DB_MAX_OPEN_CONNS")
	maxIdle := maxOpen
	if v.IsSet("DB_MAX_IDLE_CONNS") {
		maxIdle = v.GetInt("DB_MAX_IDLE_CONNS")
	}

	cfg := &Config{
		DatabaseURL:    v.GetString("DATABASE_URL"),
		Port:           v.GetString("PORT"),
		CollectSiteKey: v.GetString("COLLECT_SITE_KEY"),
		BootstrapToken: v.GetString("BOOTSTRAP_TOKEN"),
		CORSOrigins:    v.GetStringSlice("CORS_ORIGINS"),
		JWTSecret:      v.GetString("JWT_SECRET"),
		SessionTTL:     v.GetDuration("SESSION_TTL"),
		AppURL:         v.GetString("APP_URL"),
		MCPURL:         v.GetString("MCP_URL"),
		SMTPHost:       v.GetString("SMTP_HOST"),
		SMTPPort:       v.GetInt("SMTP_PORT"),
		SMTPUser:       v.GetString("SMTP_USER"),
		SMTPPass:       v.GetString("SMTP_PASS"),
		SMTPFrom:       v.GetString("SMTP_FROM"),
		EncryptionKey:  v.GetString("ENCRYPTION_KEY"),
		LicenseKey:     v.GetString("LICENSE_KEY"),

		AccountLinkURL:        v.GetString("ACCOUNT_LINK_URL"),
		AccountLinkSigningKey: v.GetString("ACCOUNT_LINK_SIGNING_KEY"),

		OperatorJWTSecret:     v.GetString("OPERATOR_JWT_SECRET"),
		OperatorSessionTTL:    v.GetDuration("OPERATOR_SESSION_TTL"),
		OperatorHoldMaxExpiry: v.GetDuration("OPERATOR_HOLD_MAX_EXPIRY"),

		AbuseVolumeFloor:            v.GetInt("ABUSE_VOLUME_FLOOR"),
		AbuseWarnComplaintPercent:   v.GetFloat64("ABUSE_WARN_COMPLAINT_PERCENT"),
		AbuseWarnBouncePercent:      v.GetFloat64("ABUSE_WARN_BOUNCE_PERCENT"),
		AbuseFreezeComplaintPercent: v.GetFloat64("ABUSE_FREEZE_COMPLAINT_PERCENT"),
		AbuseFreezeBouncePercent:    v.GetFloat64("ABUSE_FREEZE_BOUNCE_PERCENT"),
		SupportEmail:                strings.TrimSpace(v.GetString("SUPPORT_EMAIL")),
		BillingURL:                  strings.TrimSpace(v.GetString("BILLING_URL")),
		AutoMigrate:                 v.GetBool("AUTO_MIGRATE"),

		BodyLimits: BodyLimits{
			Default:      v.GetInt64("MAX_BODY_BYTES"),
			Collect:      v.GetInt64("COLLECT_MAX_BODY_BYTES"),
			CollectEvent: v.GetInt64("COLLECT_MAX_EVENT_BYTES"),
		},
		RateLimits: RateLimits{
			Human:            v.GetInt("RATE_LIMIT_HUMAN_PER_MINUTE"),
			APIBurst:         v.GetInt("RATE_LIMIT_API_BURST_PER_SECOND"),
			APIPerMinute:     v.GetInt("RATE_LIMIT_API_PER_MINUTE"),
			FailedAuth:       v.GetInt("RATE_LIMIT_FAILED_AUTH_PER_MINUTE"),
			Tracking:         v.GetInt("RATE_LIMIT_TRACKING_PER_MINUTE"),
			LoginFailures:    v.GetInt("RATE_LIMIT_LOGIN_FAILURES"),
			LoginIP:          v.GetInt("RATE_LIMIT_LOGIN_IP_PER_MINUTE"),
			Collect:          v.GetInt("RATE_LIMIT_COLLECT_PER_MINUTE"),
			CollectIP:        v.GetInt("RATE_LIMIT_COLLECT_IP_PER_MINUTE"),
			ServicePerMinute: v.GetInt("RATE_LIMIT_SERVICE_PER_MINUTE"),
			ForgotAddress:    v.GetInt("RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR"),
			ForgotIP:         v.GetInt("RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR"),
		},
		DBPool: DBPool{
			MaxOpenConns:    maxOpen,
			MaxIdleConns:    maxIdle,
			ConnMaxLifetime: v.GetDuration("DB_CONN_MAX_LIFETIME"),
			PGXMaxConns:     v.GetInt32("PGX_MAX_CONNS"),
		},
		OutboxFloor:             time.Duration(v.GetInt("OUTBOX_RETENTION_FLOOR_DAYS")) * 24 * time.Hour,
		EventsRetention:         time.Duration(v.GetInt("EVENTS_RETENTION_DAYS")) * 24 * time.Hour,
		UnverifiedUserRetention: time.Duration(v.GetInt("UNVERIFIED_USER_RETENTION_DAYS")) * 24 * time.Hour,
		IsDev:                   isDevEnv(envName),
		SMTPAllowPrivateHosts:   smtpAllowPrivateHosts(v, envName),
		Locale:                  i18n.Normalize(v.GetString("APP_LOCALE")),

		RequireEmailVerification: strings.TrimSpace(v.GetString("REQUIRE_EMAIL_VERIFICATION")) != "false",
		LogLevel:                 v.GetString("LOG_LEVEL"),
		LogFormat:                v.GetString("LOG_FORMAT"),

		OtelServiceName: v.GetString("OTEL_SERVICE_NAME"),
		MetricsAddr:     v.GetString("METRICS_ADDR"),
		InternalAddr:    v.GetString("INTERNAL_ADDR"),

		SystemEmailProvider: v.GetString("SYSTEM_EMAIL_PROVIDER"),
		SystemEmailFrom:     v.GetString("SYSTEM_EMAIL_FROM"),
		SESRegion:           v.GetString("SES_REGION"),
		SESAccessKeyID:      v.GetString("SES_ACCESS_KEY_ID"),
		SESSecretAccessKey:  v.GetString("SES_SECRET_ACCESS_KEY"),
	}

	if isE2E(envName) {
		// The suite never scrapes metrics; a global METRICS_ADDR (the dev stack's) would
		// make concurrent runs, or a running dev stack, collide on the port.
		cfg.MetricsAddr = ""
		cfg.InternalAddr = "" // likewise a global INTERNAL_ADDR
	}

	if err := cfg.validate(envName); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SecureCookies reports whether session cookies carry the Secure attribute: whenever the
// instance is served over HTTPS (APP_URL reflects the public scheme, even though a proxy
// terminates TLS and the Go server listens on plain HTTP). Local http dev stays false,
// where a Secure cookie would never be sent back.
func (c *Config) SecureCookies() bool { return strings.HasPrefix(c.AppURL, "https://") }

// validate enforces invariants that depend on the deployment environment.
func (c *Config) validate(envName string) error {
	// Outside development/test, an empty JWT_SECRET silently signs auth tokens
	// with an empty key — refuse to boot rather than ship that footgun.
	if !isDevEnv(envName) && !isE2E(envName) {
		if err := validateJWTSecret(c.JWTSecret); err != nil {
			return err
		}
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("SESSION_TTL must be positive")
	}
	if c.OperatorSessionTTL <= 0 {
		return fmt.Errorf("OPERATOR_SESSION_TTL must be positive")
	}
	if c.OperatorHoldMaxExpiry <= 0 {
		return fmt.Errorf("OPERATOR_HOLD_MAX_EXPIRY must be positive")
	}
	if c.AbuseVolumeFloor <= 0 {
		return fmt.Errorf("ABUSE_VOLUME_FLOOR must be positive")
	}
	if !(0 < c.AbuseWarnComplaintPercent && c.AbuseWarnComplaintPercent < c.AbuseFreezeComplaintPercent && c.AbuseFreezeComplaintPercent <= 100) {
		return fmt.Errorf("ABUSE_WARN_COMPLAINT_PERCENT must be above 0 and below ABUSE_FREEZE_COMPLAINT_PERCENT, which is at most 100")
	}
	if !(0 < c.AbuseWarnBouncePercent && c.AbuseWarnBouncePercent < c.AbuseFreezeBouncePercent && c.AbuseFreezeBouncePercent <= 100) {
		return fmt.Errorf("ABUSE_WARN_BOUNCE_PERCENT must be above 0 and below ABUSE_FREEZE_BOUNCE_PERCENT, which is at most 100")
	}
	if (c.AccountLinkURL == "") != (c.AccountLinkSigningKey == "") {
		return fmt.Errorf("ACCOUNT_LINK_URL and ACCOUNT_LINK_SIGNING_KEY must be set together")
	}
	if c.BodyLimits.Default <= 0 {
		return fmt.Errorf("MAX_BODY_BYTES must be positive")
	}
	if c.BodyLimits.Collect <= 0 {
		return fmt.Errorf("COLLECT_MAX_BODY_BYTES must be positive")
	}
	if c.BodyLimits.CollectEvent <= 0 {
		return fmt.Errorf("COLLECT_MAX_EVENT_BYTES must be positive")
	}
	for name, limit := range map[string]int{
		"RATE_LIMIT_HUMAN_PER_MINUTE":       c.RateLimits.Human,
		"RATE_LIMIT_API_BURST_PER_SECOND":   c.RateLimits.APIBurst,
		"RATE_LIMIT_API_PER_MINUTE":         c.RateLimits.APIPerMinute,
		"RATE_LIMIT_FAILED_AUTH_PER_MINUTE": c.RateLimits.FailedAuth,
	} {
		if limit < 0 {
			return fmt.Errorf("%s must not be negative (0 disables)", name)
		}
	}
	if c.RateLimits.Tracking < 0 {
		return fmt.Errorf("RATE_LIMIT_TRACKING_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.LoginFailures < 0 {
		return fmt.Errorf("RATE_LIMIT_LOGIN_FAILURES must not be negative (0 disables)")
	}
	if c.RateLimits.LoginIP < 0 {
		return fmt.Errorf("RATE_LIMIT_LOGIN_IP_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.Collect < 0 {
		return fmt.Errorf("RATE_LIMIT_COLLECT_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.CollectIP < 0 {
		return fmt.Errorf("RATE_LIMIT_COLLECT_IP_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.ServicePerMinute < 0 {
		return fmt.Errorf("RATE_LIMIT_SERVICE_PER_MINUTE must not be negative (0 disables)")
	}
	if c.RateLimits.ForgotAddress < 0 {
		return fmt.Errorf("RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR must not be negative (0 disables)")
	}
	if c.RateLimits.ForgotIP < 0 {
		return fmt.Errorf("RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR must not be negative (0 disables)")
	}
	if c.OutboxFloor < 0 {
		return fmt.Errorf("OUTBOX_RETENTION_FLOOR_DAYS must not be negative")
	}
	if c.EventsRetention < 0 {
		return fmt.Errorf("EVENTS_RETENTION_DAYS must not be negative")
	}
	if c.UnverifiedUserRetention < 0 {
		return fmt.Errorf("UNVERIFIED_USER_RETENTION_DAYS must not be negative")
	}
	if c.DBPool.MaxOpenConns <= 0 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be positive")
	}
	if c.DBPool.MaxIdleConns < 0 || c.DBPool.MaxIdleConns > c.DBPool.MaxOpenConns {
		return fmt.Errorf("DB_MAX_IDLE_CONNS must be between 0 and DB_MAX_OPEN_CONNS")
	}
	if c.DBPool.ConnMaxLifetime <= 0 {
		return fmt.Errorf("DB_CONN_MAX_LIFETIME must be positive")
	}
	if c.DBPool.PGXMaxConns <= 0 {
		return fmt.Errorf("PGX_MAX_CONNS must be positive")
	}
	if err := c.validateMetricsAddr(); err != nil {
		return err
	}
	return c.validateInternalAddr()
}

// validateMetricsAddr rejects a malformed METRICS_ADDR and one sharing the public
// PORT (the public server binds every interface, so any host collides).
func (c *Config) validateMetricsAddr() error {
	_, err := c.listenPort("METRICS_ADDR", c.MetricsAddr)
	return err
}

// validateInternalAddr rejects a malformed INTERNAL_ADDR and one sharing the public
// PORT or the metrics listener's port: the internal listener is a server of its own.
func (c *Config) validateInternalAddr() error {
	port, err := c.listenPort("INTERNAL_ADDR", c.InternalAddr)
	if err != nil || port == 0 {
		return err
	}
	if metrics, err := c.listenPort("METRICS_ADDR", c.MetricsAddr); err == nil && metrics == port {
		return fmt.Errorf("INTERNAL_ADDR must not use the METRICS_ADDR port (%d)", port)
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

// minJWTSecretLength is the shortest JWT_SECRET accepted outside development
// (32 characters = 256 bits when the secret is hex/random, the HS256 key size).
const minJWTSecretLength = 32

// placeholderSecretMarkers are lowercase fragments of documented example and
// development secrets; a secret containing one was copied, not generated.
var placeholderSecretMarkers = []string{"change-me", "changeme", "change-in-production", "dev-secret", "a-strong-secret"}

func validateJWTSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("JWT_SECRET is required outside development")
	}
	if len(secret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d characters outside development (e.g. `openssl rand -hex 32`)", minJWTSecretLength)
	}
	lower := strings.ToLower(secret)
	for _, m := range placeholderSecretMarkers {
		if strings.Contains(lower, m) {
			return fmt.Errorf("JWT_SECRET looks like a placeholder; generate one with `openssl rand -hex 32`")
		}
	}
	return nil
}

// MCPOrigin is the public origin the MCP resource is served from: MCP_URL when the
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
// JWT_SECRET like development does, but is not IsDev: the dev DKIM lookup stays off
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

// smtpAllowPrivateHosts reads SMTP_ALLOW_PRIVATE_HOSTS; unset, it follows the environment:
// allowed in development, tests and the e2e suite (a local Mailpit), refused in production.
func smtpAllowPrivateHosts(v *viper.Viper, envName string) bool {
	if v.IsSet("SMTP_ALLOW_PRIVATE_HOSTS") {
		return v.GetBool("SMTP_ALLOW_PRIVATE_HOSTS")
	}
	return isDevEnv(envName) || isE2E(envName)
}
