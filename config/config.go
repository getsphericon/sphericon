// Package config loads and validates the Instance's configuration from the process environment,
// with the .env files layered under it.
package config

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	// RiverMaxConns bounds river's own queries (fetch, completion, leader election). River polls
	// and holds no LISTEN connection; job workers use the database/sql pool, so it need not
	// match river's MaxWorkers sum.
	RiverMaxConns int32
}

// telemetryReceiverURL is the address of the Telemetry report receiver, fixed in the binary.
const telemetryReceiverURL = "https://telemetry.getsphericon.com"

// telemetrySchemaURL is the page that documents the Telemetry report's schema.
const telemetrySchemaURL = "https://getsphericon.com/docs/operations/telemetry-schema"

type Config struct {
	DatabaseURL    string
	Port           string
	CollectSiteKey string
	BootstrapToken string
	CORSOrigins    []string
	JWTSecret      string
	// JWTSecretSecondary is the second JWT secret of a rotation (SPHERICON_JWT_SECRET_SECONDARY):
	// accepted when verifying a session or a mailed link, never used to sign. Empty outside one.
	JWTSecretSecondary string
	// SessionTTL is the lifetime of a site session: both the token's exp and the
	// cookie's Max-Age (SPHERICON_SESSION_TTL, default 24h, no refresh; ADR 0020).
	SessionTTL time.Duration
	AppURL     string
	// MCPURL is the public origin of a dedicated MCP host (SPHERICON_MCP_URL); empty means the
	// MCP resource lives on AppURL. Read it through MCPOrigin.
	MCPURL string
	// TrackerURL is the public origin the tracker snippet loads t.js from and posts /collect to
	// (SPHERICON_TRACKER_URL); empty means AppURL. Read it through TrackerOrigin.
	TrackerURL    string
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
	AutoMigrate  bool
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
	// EventLiveWindow is how old a live Event may be before the external API refuses it
	// as history that belongs in a Backfill (SPHERICON_EVENT_LIVE_WINDOW_HOURS, default 72, at least 1).
	EventLiveWindow time.Duration
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

	// SignupOpen lets anyone who reaches the Instance register. Only an explicit
	// SPHERICON_SIGNUP_OPEN=false closes it (ADR 0042): a new User then comes only through an
	// Invitation or `sphericon user create-owner`.
	SignupOpen bool

	// SignupSurvey asks a User who signed up the Sign-up survey once. Only an explicit
	// SPHERICON_SIGNUP_SURVEY=true turns it on (as ADR 0030 and 0042); it also needs the
	// `staff` license, and without it the survey stays off (signupsurvey.Enabled).
	SignupSurvey bool

	// Signin holds the credentials of the Sign-in providers the Instance enabled, by registry
	// name (ADR 0048): SPHERICON_SIGNIN_<PROVIDER>_CLIENT_ID and _CLIENT_SECRET, both or
	// neither (a half-configured provider stops the start). A provider not in the map is off.
	Signin map[string]SigninCredentials

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

	// TrackingCNAMETarget is the edge host a Sending domain's Tracking domain CNAME
	// (track.<domain>) must point at (SPHERICON_TRACKING_CNAME_TARGET, ADR 0038). Empty (the
	// default) turns the feature off: no tracking record is shown and none is checked.
	TrackingCNAMETarget string

	// TrackingHostHeader names the request header the edge sets to the customer's
	// host (SPHERICON_TRACKING_HOST_HEADER, ADR 0038). It is read only from a peer inside
	// TrackingEdgeCIDRs; empty (the default) means the connection's Host is the only source.
	TrackingHostHeader string

	// TrackingEdgeCIDRs are the peer address ranges the edge connects from
	// (SPHERICON_TRACKING_EDGE_CIDRS): the only peers whose TrackingHostHeader is believed.
	// The default is loopback only; a proxy on another address must be listed, because a
	// private range as a default would let any peer of a shared network forge the host.
	TrackingEdgeCIDRs []netip.Prefix

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
	TrackerURL  string `env:"SPHERICON_TRACKER_URL"`

	CollectSiteKey     string        `env:"SPHERICON_COLLECT_SITE_KEY"`
	BootstrapToken     string        `env:"SPHERICON_BOOTSTRAP_TOKEN"`
	CORSOrigins        string        `env:"SPHERICON_CORS_ORIGINS"`
	JWTSecret          string        `env:"SPHERICON_JWT_SECRET"`
	JWTSecretSecondary string        `env:"SPHERICON_JWT_SECRET_SECONDARY"`
	SessionTTL         time.Duration `env:"SPHERICON_SESSION_TTL" envDefault:"24h"`
	EncryptionKey      string        `env:"SPHERICON_ENCRYPTION_KEY"`
	LicenseKey         string        `env:"SPHERICON_LICENSE_KEY"`

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
	EventLiveWindowHours        int `env:"SPHERICON_EVENT_LIVE_WINDOW_HOURS" envDefault:"72"`

	DBMaxOpenConns    int           `env:"SPHERICON_DB_MAX_OPEN_CONNS" envDefault:"15"`
	DBMaxIdleConns    *int          `env:"SPHERICON_DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime time.Duration `env:"SPHERICON_DB_CONN_MAX_LIFETIME" envDefault:"30m"`
	RiverMaxConns     int32         `env:"SPHERICON_RIVER_DB_MAX_CONNS" envDefault:"25"`

	SMTPAllowPrivateHosts    bool   `env:"SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS"`
	OutboundAllow            string `env:"SPHERICON_OUTBOUND_ALLOW"`
	Locale                   string `env:"SPHERICON_APP_LOCALE" envDefault:"en"`
	Telemetry                string `env:"SPHERICON_TELEMETRY"`
	DoNotTrack               string `env:"DO_NOT_TRACK"`
	ReleaseCheck             bool   `env:"SPHERICON_RELEASE_CHECK" envDefault:"true"`
	RequireEmailVerification string `env:"SPHERICON_REQUIRE_EMAIL_VERIFICATION"`
	SignupOpen               string `env:"SPHERICON_SIGNUP_OPEN"`
	SignupSurvey             string `env:"SPHERICON_SIGNUP_SURVEY"`

	SigninGoogleClientID     string `env:"SPHERICON_SIGNIN_GOOGLE_CLIENT_ID"`
	SigninGoogleClientSecret string `env:"SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET"`
	SigninGitHubClientID     string `env:"SPHERICON_SIGNIN_GITHUB_CLIENT_ID"`
	SigninGitHubClientSecret string `env:"SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET"`

	LogLevel        string `env:"SPHERICON_LOG_LEVEL" envDefault:"info"`
	LogFormat       string `env:"SPHERICON_LOG_FORMAT"`
	OtelServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"sphericon"`
	MetricsAddr     string `env:"SPHERICON_METRICS_ADDR"`
	InternalAddr    string `env:"SPHERICON_INTERNAL_ADDR"`
	PresetsDir      string `env:"SPHERICON_PRESETS_DIR"`

	TrackingCNAMETarget string `env:"SPHERICON_TRACKING_CNAME_TARGET"`
	TrackingHostHeader  string `env:"SPHERICON_TRACKING_HOST_HEADER"`
	TrackingEdgeCIDRs   string `env:"SPHERICON_TRACKING_EDGE_CIDRS" envDefault:"127.0.0.0/8,::1/128"`

	SystemEmailProvider string `env:"SPHERICON_SYSTEM_EMAIL_PROVIDER" envDefault:"smtp"`
	SystemEmailFrom     string `env:"SPHERICON_SYSTEM_EMAIL_FROM" envDefault:"noreply@sphericon.localhost"`
	SESRegion           string `env:"SPHERICON_SES_REGION"`
	SESAccessKeyID      string `env:"SPHERICON_SES_ACCESS_KEY_ID"`
	SESSecretAccessKey  string `env:"SPHERICON_SES_SECRET_ACCESS_KEY"`
}

// loadVars reads the variables of the process: the env files, then the environment over
// them, then the `_FILE` siblings of the secrets.
func loadVars(envName string) (map[string]string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("locate the config package's source to find the .env files")
	}
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
	if err := readSecretFiles(vars); err != nil {
		return nil, err
	}
	return vars, nil
}

// EncryptionKey is SPHERICON_ENCRYPTION_KEY as the process sees it (a `_FILE` sibling
// included), without loading the rest of the config: the key commands need no database.
func EncryptionKey(envName string) (string, error) {
	vars, err := loadVars(envName)
	if err != nil {
		return "", err
	}
	return vars["SPHERICON_ENCRYPTION_KEY"], nil
}

// Port is PORT as the process sees it, without loading the rest of the config: the
// container health check needs no database and no secrets.
func Port(envName string) (string, error) {
	vars, err := loadVars(envName)
	if err != nil {
		return "", err
	}
	var e struct {
		Port string `env:"PORT" envDefault:"3000"` // the same default as environment.Port
	}
	if err := env.ParseWithOptions(&e, env.Options{Environment: vars}); err != nil {
		return "", fmt.Errorf("read environment: %w", err)
	}
	return e.Port, nil
}

// Load reads the configuration from the process environment layered over the .env
// files (see envFiles): a variable the process sets wins over every file.
func Load(envName string) (*Config, error) {
	vars, err := loadVars(envName)
	if err != nil {
		return nil, err
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

	edgeCIDRs, err := parseCIDRs("SPHERICON_TRACKING_EDGE_CIDRS", e.TrackingEdgeCIDRs)
	if err != nil {
		return nil, err
	}

	signin, err := signinCredentials([]signinVars{
		{name: "google", id: e.SigninGoogleClientID, secret: e.SigninGoogleClientSecret, idName: "SPHERICON_SIGNIN_GOOGLE_CLIENT_ID", secretName: "SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET"},
		{name: "github", id: e.SigninGitHubClientID, secret: e.SigninGitHubClientSecret, idName: "SPHERICON_SIGNIN_GITHUB_CLIENT_ID", secretName: "SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET"},
	})
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		DatabaseURL:        e.DatabaseURL,
		Port:               e.Port,
		CollectSiteKey:     e.CollectSiteKey,
		BootstrapToken:     e.BootstrapToken,
		CORSOrigins:        strings.FieldsFunc(e.CORSOrigins, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }),
		JWTSecret:          e.JWTSecret,
		JWTSecretSecondary: e.JWTSecretSecondary,
		SessionTTL:         e.SessionTTL,
		AppURL:             e.AppURL,
		MCPURL:             e.MCPURL,
		TrackerURL:         e.TrackerURL,
		SMTPHost:           e.SMTPHost,
		SMTPPort:           e.SMTPPort,
		SMTPUser:           e.SMTPUser,
		SMTPPass:           e.SMTPPass,
		SMTPFrom:           e.SMTPFrom,
		EncryptionKey:      e.EncryptionKey,
		LicenseKey:         e.LicenseKey,

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
			RiverMaxConns:   e.RiverMaxConns,
		},
		OutboxFloor:             time.Duration(e.OutboxFloorDays) * 24 * time.Hour,
		EventsRetention:         time.Duration(e.EventsRetentionDays) * 24 * time.Hour,
		EventLiveWindow:         time.Duration(e.EventLiveWindowHours) * time.Hour,
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
		SignupOpen:               !strings.EqualFold(strings.TrimSpace(e.SignupOpen), "false"),
		SignupSurvey:             strings.EqualFold(strings.TrimSpace(e.SignupSurvey), "true"),
		Signin:                   signin,
		LogLevel:                 e.LogLevel,
		LogFormat:                e.LogFormat,

		OtelServiceName: e.OtelServiceName,
		MetricsAddr:     e.MetricsAddr,
		InternalAddr:    e.InternalAddr,
		PresetsDir:      e.PresetsDir,

		TrackingCNAMETarget: strings.ToLower(strings.TrimSuffix(strings.TrimSpace(e.TrackingCNAMETarget), ".")),
		TrackingHostHeader:  strings.TrimSpace(e.TrackingHostHeader),
		TrackingEdgeCIDRs:   edgeCIDRs,

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

// parseCIDRs reads a comma/space-separated list of CIDR prefixes.
func parseCIDRs(name, list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		p, err := netip.ParsePrefix(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
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

// secretVars are the variables that also accept a `<NAME>_FILE` sibling, the path of a file
// holding the value (Docker and Kubernetes mount secrets as files).
var secretVars = []string{
	"DATABASE_URL",
	"SPHERICON_JWT_SECRET",
	"SPHERICON_JWT_SECRET_SECONDARY",
	"SPHERICON_ENCRYPTION_KEY",
	"SPHERICON_SMTP_PASS",
	"SPHERICON_LICENSE_KEY",
}

// readSecretFiles fills each secret variable from its `_FILE` sibling, trimming the trailing
// newline. A value set directly wins and its file is never read; an unreadable or empty
// file is an error naming the `_FILE` variable.
func readSecretFiles(vars map[string]string) error {
	for _, name := range secretVars {
		fileVar := name + "_FILE"
		path := vars[fileVar]
		if path == "" || vars[name] != "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", fileVar, err)
		}
		value := strings.TrimRight(string(raw), "\r\n")
		if value == "" {
			return fmt.Errorf("%s: %s is empty", fileVar, path)
		}
		vars[name] = value
	}
	return nil
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

// JWTVerifySecrets are the secrets a session or a mailed link is also accepted under besides
// JWTSecret: the secondary key of a rotation, none outside one.
func (c *Config) JWTVerifySecrets() []string {
	if c.JWTSecretSecondary == "" {
		return nil
	}
	return []string{c.JWTSecretSecondary}
}

// JWTSecrets is every JWT secret the instance accepts: JWTSecret first, then the secondary one.
func (c *Config) JWTSecrets() []string {
	return append([]string{c.JWTSecret}, c.JWTVerifySecrets()...)
}

// SecureCookies reports whether session cookies carry the Secure attribute: whenever the
// instance is served over HTTPS (SPHERICON_APP_URL reflects the public scheme, even though a proxy
// terminates TLS and the Go server listens on plain HTTP). Local http dev stays false,
// where a Secure cookie would never be sent back.
func (c *Config) SecureCookies() bool { return strings.HasPrefix(c.AppURL, "https://") }

// SystemMailConfigured reports whether the Instance has outbound mail, that is, a System
// sender: the SES provider, or SMTP with a host. Without it there is none (ADR 0041).
func (c *Config) SystemMailConfigured() bool {
	return c.SystemEmailProvider == "ses" || c.SMTPHost != ""
}

// validate enforces invariants that depend on the deployment environment.
func (c *Config) validate(envName string) error {
	if err := c.validateSecrets(envName); err != nil {
		return err
	}
	if err := firstViolation(c.requirements()); err != nil {
		return err
	}
	if err := c.validateMetricsAddr(); err != nil {
		return err
	}
	return c.validateInternalAddr()
}

// validateSecrets refuses the signing secrets that would sign with a footgun. Outside
// development/test, an empty SPHERICON_JWT_SECRET silently signs auth tokens with an empty key:
// refuse to boot rather than ship that.
func (c *Config) validateSecrets(envName string) error {
	strict := !isDevEnv(envName) && !isE2E(envName)
	if strict {
		if err := ValidateJWTSecret(c.JWTSecret); err != nil {
			return err
		}
	}
	if c.JWTSecretSecondary == "" {
		return nil
	}
	if c.JWTSecretSecondary == c.JWTSecret {
		return errors.New("SPHERICON_JWT_SECRET_SECONDARY must differ from SPHERICON_JWT_SECRET")
	}
	if strict {
		if err := ValidateJWTSecret(c.JWTSecretSecondary); err != nil {
			return fmt.Errorf("SPHERICON_JWT_SECRET_SECONDARY: %w", err)
		}
	}
	return nil
}

// requirement is one invariant of the configuration: ok says it holds, msg is the refusal when
// it does not.
type requirement struct {
	ok  bool
	msg string
}

// firstViolation is the refusal of the first requirement that does not hold, nil when all do.
func firstViolation(rs []requirement) error {
	for _, r := range rs {
		if !r.ok {
			return errors.New(r.msg)
		}
	}
	return nil
}

// requirements lists the invariants of the settings that stand alone, in the order they are
// reported.
func (c *Config) requirements() []requirement {
	const noNegative = " must not be negative (0 disables)"
	rl := c.RateLimits
	pool := c.DBPool
	return []requirement{
		{c.SessionTTL > 0, "SPHERICON_SESSION_TTL must be positive"},
		{c.StaffSessionTTL > 0, "SPHERICON_STAFF_SESSION_TTL must be positive"},
		{c.StaffHoldMaxExpiry > 0, "SPHERICON_STAFF_HOLD_MAX_EXPIRY must be positive"},
		{c.AbuseVolumeFloor > 0, "SPHERICON_ABUSE_VOLUME_FLOOR must be positive"},
		{0 < c.AbuseWarnComplaintPercent && c.AbuseWarnComplaintPercent < c.AbuseFreezeComplaintPercent && c.AbuseFreezeComplaintPercent <= 100,
			"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT must be above 0 and below SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT, which is at most 100"},
		{0 < c.AbuseWarnBouncePercent && c.AbuseWarnBouncePercent < c.AbuseFreezeBouncePercent && c.AbuseFreezeBouncePercent <= 100,
			"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT must be above 0 and below SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT, which is at most 100"},
		{(c.AccountLinkURL == "") == (c.AccountLinkSigningKey == ""), "SPHERICON_ACCOUNT_LINK_URL and SPHERICON_ACCOUNT_LINK_SIGNING_KEY must be set together"},
		{c.BodyLimits.Default > 0, "SPHERICON_MAX_BODY_BYTES must be positive"},
		{c.BodyLimits.Collect > 0, "SPHERICON_COLLECT_MAX_BODY_BYTES must be positive"},
		{c.BodyLimits.CollectEvent > 0, "SPHERICON_COLLECT_MAX_EVENT_BYTES must be positive"},
		{rl.Human >= 0, "SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE" + noNegative},
		{rl.APIBurst >= 0, "SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND" + noNegative},
		{rl.APIPerMinute >= 0, "SPHERICON_RATE_LIMIT_API_PER_MINUTE" + noNegative},
		{rl.FailedAuth >= 0, "SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE" + noNegative},
		{rl.Tracking >= 0, "SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE" + noNegative},
		{rl.LoginFailures >= 0, "SPHERICON_RATE_LIMIT_LOGIN_FAILURES" + noNegative},
		{rl.LoginIP >= 0, "SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE" + noNegative},
		{rl.Collect >= 0, "SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE" + noNegative},
		{rl.CollectIP >= 0, "SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE" + noNegative},
		{rl.ServicePerMinute >= 0, "SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE" + noNegative},
		{rl.ForgotAddress >= 0, "SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR" + noNegative},
		{rl.ForgotIP >= 0, "SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR" + noNegative},
		{c.OutboxFloor >= 0, "SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS must not be negative"},
		{c.EventsRetention >= 0, "SPHERICON_EVENTS_RETENTION_DAYS must not be negative"},
		{c.EventLiveWindow >= time.Hour, "SPHERICON_EVENT_LIVE_WINDOW_HOURS must be at least 1"},
		{c.UnverifiedUserRetention >= 0, "SPHERICON_UNVERIFIED_USER_RETENTION_DAYS must not be negative"},
		{pool.MaxOpenConns > 0, "SPHERICON_DB_MAX_OPEN_CONNS must be positive"},
		{pool.MaxIdleConns >= 0 && pool.MaxIdleConns <= pool.MaxOpenConns, "SPHERICON_DB_MAX_IDLE_CONNS must be between 0 and SPHERICON_DB_MAX_OPEN_CONNS"},
		{pool.ConnMaxLifetime > 0, "SPHERICON_DB_CONN_MAX_LIFETIME must be positive"},
		{pool.RiverMaxConns > 0, "SPHERICON_RIVER_DB_MAX_CONNS must be positive"},
	}
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
// contract-copy: a secret-strength floor for SPHERICON_JWT_SECRET; no client sees it
const minJWTSecretLength = 32

// placeholderSecretMarkers are lowercase fragments of documented example and
// development secrets; a secret containing one was copied, not generated.
var placeholderSecretMarkers = []string{"change-me", "changeme", "change-in-production", "dev-secret", "a-strong-secret"}

// ValidateJWTSecret applies the production rule for SPHERICON_JWT_SECRET: present, long
// enough and not a documented placeholder. Load applies it outside development; `sphericon
// doctor` applies it everywhere.
func ValidateJWTSecret(secret string) error {
	if secret == "" {
		return errors.New("SPHERICON_JWT_SECRET is required outside development")
	}
	if len(secret) < minJWTSecretLength {
		return fmt.Errorf("SPHERICON_JWT_SECRET must be at least %d characters outside development (e.g. `openssl rand -hex 32`)", minJWTSecretLength)
	}
	lower := strings.ToLower(secret)
	for _, m := range placeholderSecretMarkers {
		if strings.Contains(lower, m) {
			return errors.New("SPHERICON_JWT_SECRET looks like a placeholder; generate one with `openssl rand -hex 32`")
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

// TrackerOrigin is the public origin of the tracker: SPHERICON_TRACKER_URL when the deployment serves
// /t.js and /collect on a host of its own, otherwise AppURL.
func (c *Config) TrackerOrigin() string {
	if c.TrackerURL != "" {
		return strings.TrimRight(c.TrackerURL, "/")
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

// secretEnvNames are the variables whose values must never be printed.
var secretEnvNames = []string{
	"DATABASE_URL",
	"SPHERICON_JWT_SECRET",
	"SPHERICON_JWT_SECRET_SECONDARY",
	"SPHERICON_STAFF_JWT_SECRET",
	"SPHERICON_ENCRYPTION_KEY",
	"SPHERICON_SMTP_PASS",
	"SPHERICON_LICENSE_KEY",
	"SPHERICON_SES_ACCESS_KEY_ID",
	"SPHERICON_SES_SECRET_ACCESS_KEY",
	"SPHERICON_ACCOUNT_LINK_SIGNING_KEY",
	"SPHERICON_BOOTSTRAP_TOKEN",
}

// SecretValues lists every secret value of the process: the ones in cfg (nil when the load
// failed) and the ones still in the environment, plus the password inside DATABASE_URL, raw
// and as the URL spells it. `sphericon doctor` redacts them from everything it prints.
func SecretValues(cfg *Config) []string {
	var values []string
	for _, name := range secretEnvNames {
		values = append(values, os.Getenv(name))
	}
	if cfg != nil {
		values = append(values, cfg.DatabaseURL, cfg.JWTSecret, cfg.JWTSecretSecondary, cfg.StaffJWTSecret, cfg.EncryptionKey, cfg.SMTPPass,
			cfg.LicenseKey, cfg.SESAccessKeyID, cfg.SESSecretAccessKey, cfg.AccountLinkSigningKey, cfg.BootstrapToken)
	}
	for _, v := range slices.Clone(values) {
		if u, err := url.Parse(v); err == nil && u.User != nil {
			if pw, ok := u.User.Password(); ok {
				values = append(values, pw, url.QueryEscape(pw), url.PathEscape(pw))
			}
		}
	}
	return slices.DeleteFunc(values, func(v string) bool { return v == "" })
}
