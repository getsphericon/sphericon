// Package config loads and validates the Instance's configuration from the process environment,
// with the .env files layered under it.
package config

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/mail"
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

// RateLimits are the per-policy request budgets (ADR 0025). Every limit has a default and 0
// disables it. They are core, never gated by the EE licence. Each field's doc comment is the
// variable's description in the configuration reference.
type RateLimits struct {
	// Requests per minute per client IP and endpoint on the public human-facing endpoints
	// (signup, invitation accept, consent confirm); over it `429` with `Retry-After`.
	Human int `env:"SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE" validate:"gte=0"`
	// Requests per second per Workspace on `/api` and `/mcp` (one shared budget); over it `429`
	// with `Retry-After`.
	APIBurst int `env:"SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND" validate:"gte=0"`
	// Requests per minute per Workspace on `/api` and `/mcp`, stacked on the burst limit.
	APIPerMinute int `env:"SPHERICON_RATE_LIMIT_API_PER_MINUTE" validate:"gte=0"`
	// Failed credential checks (bearer token, collect key) per minute per client IP; successful
	// ones are not counted.
	FailedAuth int `env:"SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE" validate:"gte=0"`
	// Opens and clicks recorded per minute per client IP. It never refuses a recipient: over it the
	// redirect or pixel is still served and the event is not recorded.
	Tracking int `env:"SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE" validate:"gte=0"`
	// Failed logins one account may have within 15 minutes before login answers `429` with an
	// exponentially growing delay, even for a correct password (no lockout).
	LoginFailures int `env:"SPHERICON_RATE_LIMIT_LOGIN_FAILURES" validate:"gte=0"`
	// Login requests per minute per client IP.
	LoginIP int `env:"SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE" validate:"gte=0"`
	// Requests per minute per Workspace on `/collect`, a budget of its own apart from `/api` and
	// `/mcp`.
	Collect int `env:"SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE" validate:"gte=0"`
	// Requests per minute per client IP on `/collect`.
	CollectIP int `env:"SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE" validate:"gte=0"`
	// Requests per minute per Service credential on the internal listener's service endpoints.
	ServicePerMinute int `env:"SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE" validate:"gte=0"`
	// Mails one address may be sent per hour by each of forgot-password, registration and an
	// email-change request (ADR 0031). Over it the request is still answered `202` and nothing is
	// sent, so the answer never tells whether the address has an account.
	ForgotAddress int `env:"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR" validate:"gte=0"`
	// Forgot-password requests per client IP per hour; over it `429`.
	ForgotIP int `env:"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR" validate:"gte=0"`
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
	// RiverMaxConns bounds river's own queries (fetch, completion, leader election); job
	// workers use the database/sql pool, so it need not match river's MaxWorkers sum. The
	// LISTEN connection river is told of new jobs on is one more, outside this bound
	// (db.OpenRiverListener). LISTEN needs a session of its own: behind PgBouncer, DATABASE_URL
	// must reach Postgres directly or a pool in session mode, never transaction mode.
	RiverMaxConns int32
}

// telemetryReceiverURL is the address of the Telemetry report receiver, fixed in the binary.
const telemetryReceiverURL = "https://telemetry.getsphericon.com"

// telemetrySchemaURL is the page that documents the Telemetry report's schema.
const telemetrySchemaURL = "https://getsphericon.com/docs/operations/telemetry-schema"

type Config struct {
	DatabaseURL    string
	Port           string
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
	// ADR 0026, ADR 0063). Empty turns the Staff sign-in off; set, it must differ from
	// SPHERICON_JWT_SECRET (staffmembers.New validates it). The `staff` license feature and the
	// saas Mode require it.
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
	// TestInboxRetention is how long a Test inbox keeps a Captured message
	// (SPHERICON_TEST_INBOX_RETENTION_DAYS, default 7, at least 1; ADR 0058).
	TestInboxRetention time.Duration
	// TestInboxMaxMessages is how many Captured messages a Workspace keeps, the newest ones
	// (SPHERICON_TEST_INBOX_MAX_MESSAGES, default 200, at least 1; ADR 0058).
	TestInboxMaxMessages int
	DBPool               DBPool
	// IsDev is true for non-production envs (development/test). Used to relax
	// production-only behaviour locally.
	IsDev bool
	// IsE2E is true for the end-to-end profile (EnvE2E), a non-production env of its own.
	IsE2E bool
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

	// Mode is the kind of deployment this Instance is (SPHERICON_MODE, ADR 0060). The settings
	// below it that the Mode decides are derived from it in Load and have no variable of their own.
	Mode Mode

	// Telemetry is false in the saas Mode, which sends no Telemetry report and locks the setting
	// off (ADR 0032, 0060). Nil in selfhosted leaves the decision to the Instance's Owner.
	Telemetry *bool
	// DoNotTrack is DO_NOT_TRACK=1: it switches the Telemetry report off and never on.
	DoNotTrack bool
	// TelemetryReceiverURL is where the daily Telemetry report goes. It is a constant of the
	// binary with no environment variable (ADR 0032): a test points it at its own receiver.
	TelemetryReceiverURL string
	// TelemetrySchemaURL is the page that documents the Telemetry report's schema; the SPA
	// links to it. A constant of the binary with no environment variable.
	TelemetrySchemaURL string
	// ReleaseCheck is the daily Release check for a newer release (ADR 0032): on in selfhosted,
	// off in saas (ADR 0060). It sends nothing about the Instance and is independent of the
	// Telemetry report and DO_NOT_TRACK.
	ReleaseCheck bool
	// GitHubAPIURL is the API root the Release check asks; empty is api.github.com. It has
	// no environment variable: tests point it at a server of their own.
	GitHubAPIURL string

	// RequireEmailVerification is the email verification gate (ADR 0030): on in saas, not
	// required in selfhosted (ADR 0060). It is never inferred from the SMTP settings.
	RequireEmailVerification bool

	// SentryDSN is the Sentry-compatible DSN the browser app reports its errors to
	// (SPHERICON_SENTRY_DSN). Empty, the default, leaves the browser reporting off: the SPA
	// sends nothing anywhere. The DSN reaches the page at serve time (internal/server/spa/spa.go),
	// never at build time, so one image serves every Instance.
	SentryDSN string

	// SignupSurvey asks a User who signed up the Sign-up survey once: on in saas, off in
	// selfhosted (ADR 0055, 0060).
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
	// DNSAddr (host:port) is the DNS server that verifies Sending domains and Join domains
	// instead of the host's resolver (SPHERICON_DNS_ADDR). Empty by default; the dev stack
	// points it at its own server, which publishes the Brightpath demo records.
	DNSAddr string

	// InternalAddr (host:port) is where the opt-in internal listener binds: the only
	// place the service endpoints of the Staff surface are served (ADR 0026). Empty
	// (the default) means no listener and no route for them anywhere; it must differ
	// from the public PORT and from SPHERICON_METRICS_ADDR. Nothing is served either without the
	// `staff` license.
	InternalAddr string

	// TrackingCNAMETarget is the edge host a Workspace's Tracking domain CNAME must point at
	// (SPHERICON_TRACKING_CNAME_TARGET, ADR 0038, ADR 0071). Empty (the default) turns the
	// feature off: no Tracking domain can be set and none is checked.
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
	SystemEmailFrom     string // the bare address; SPHERICON_SYSTEM_EMAIL_FROM may also carry a display name
	SystemEmailFromName string // that display name, empty when none was given
	// SystemEmailReplyTo is the Reply-To of the Instance's own mail (verification, reset,
	// invitations, the System sender's Test send); empty sets no header.
	SystemEmailReplyTo string
	SESRegion          string
	SESAccessKeyID     string
	SESSecretAccessKey string

	// secrets are the values of the secret variables as Load read them (SecretValues).
	secrets []string
}

// environment is every variable Load reads, with the defaults that do not depend on
// the environment name and the rules each value must hold on its own (`validate`, checked by
// validateEnvironment). Those that do (the rate limits, the log format,
// SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS) are set by profileDefaults before the variables are read.
//
// Each field's doc comment is the variable's description in the configuration reference
// (docs/configuration.md, `mise run generate`), written for the operator who sets it: a field
// without one fails the generation.
type environment struct {
	// The PostgreSQL connection string, such as `postgres://user:pass@host:5432/db?sslmode=require`;
	// `sslmode=verify-full` with `sslrootcert` also verifies the server's certificate. The job queue
	// LISTENs for new jobs on a session of its own: behind PgBouncer, it must reach Postgres directly
	// or a pool in session mode, never transaction mode.
	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	// The port the public HTTP server listens on, on every interface.
	Port string `env:"PORT" envDefault:"3000"`
	// The public origin of the Instance, `https://` in production: sessions, mailed links and
	// tracking and unsubscribe links are built on it, an `https://` origin makes the session cookie
	// Secure, and its host is the Passkey relying party id (ADR 0052), so changing the host
	// invalidates every registered Passkey. An Instance served on an IP address cannot use Passkeys.
	AppURL string `env:"SPHERICON_APP_URL" envDefault:"http://localhost:3000"`
	// The public origin of a dedicated MCP host, such as `https://mcp.example.com`: the MCP resource
	// is then served there while the OAuth issuer and consent screen stay on `SPHERICON_APP_URL`.
	// Empty serves MCP on `SPHERICON_APP_URL`.
	MCPURL string `env:"SPHERICON_MCP_URL"`
	// The public origin the tracker snippet loads `/t.js` from and posts `/collect` to, such as
	// `https://t.example.com`, when the tracker has a host of its own routed to the same Instance.
	// Empty uses `SPHERICON_APP_URL`.
	TrackerURL string `env:"SPHERICON_TRACKER_URL"`

	// The token the external API's bootstrap operation takes in `X-Bootstrap-Token` to mint an API
	// token of the oldest Workspace without a caller token. Empty refuses every bootstrap.
	BootstrapToken string `env:"SPHERICON_BOOTSTRAP_TOKEN"`
	// Comma- or space-separated origins allowed credentialed CORS on the cookie-authenticated
	// surfaces (`/site`, `/staff`). Empty is same-origin only, which the bundled app needs; the
	// bearer-token surfaces answer any origin without credentials either way.
	CORSOrigins string `env:"SPHERICON_CORS_ORIGINS"`
	// Signs sessions and mailed links. Outside development and test the start is refused when it is
	// empty, shorter than 32 characters or a documented placeholder; generate one with
	// `openssl rand -hex 32`.
	JWTSecret string `env:"SPHERICON_JWT_SECRET"`
	// The second secret of a JWT secret rotation: accepted when a session or a mailed link is
	// verified, never used to sign. It must differ from `SPHERICON_JWT_SECRET` and, outside
	// development and test, holds the same rules. Empty outside a rotation.
	JWTSecretSecondary string `env:"SPHERICON_JWT_SECRET_SECONDARY"`
	// The lifetime of a site session, a Go duration such as `12h`: both the token's expiry and the
	// cookie's, with no refresh (ADR 0020).
	SessionTTL time.Duration `env:"SPHERICON_SESSION_TTL" envDefault:"24h" validate:"gt=0"`
	// The base64 Tink keyset that encrypts stored provider credentials; the server does not start
	// without one. Generate it with `sphericon genkey`.
	EncryptionKey string `env:"SPHERICON_ENCRYPTION_KEY"`
	// The Enterprise Edition license key (ADR 0014), a JWT signed with Ed25519. Empty runs the open
	// core. An expired key also runs the open core and logs a warning (ADR 0035); a malformed key or
	// one whose signature does not verify stops the start.
	LicenseKey string `env:"SPHERICON_LICENSE_KEY"`

	// The host of the SMTP relay the Instance's own mail goes through when
	// `SPHERICON_SYSTEM_EMAIL_PROVIDER` is `smtp`. Empty leaves the Instance without outbound mail.
	SMTPHost string `env:"SPHERICON_SMTP_HOST"`
	// The port of the SMTP relay.
	SMTPPort int `env:"SPHERICON_SMTP_PORT" envDefault:"1025"`
	// The user the SMTP relay is authenticated as; empty sends without authentication.
	SMTPUser string `env:"SPHERICON_SMTP_USER"`
	// The password of `SPHERICON_SMTP_USER`.
	SMTPPass string `env:"SPHERICON_SMTP_PASS"`

	// The address of an external account page. With `SPHERICON_ACCOUNT_LINK_SIGNING_KEY`, a
	// Workspace Owner sees an Account link that opens it with a signed token. The app never calls
	// this address.
	AccountLinkURL string `env:"SPHERICON_ACCOUNT_LINK_URL" validate:"required_with=AccountLinkSigningKey"`
	// The base64 Ed25519 seed that signs the Account link token (`openssl rand -base64 32`). The
	// matching public key is published at `/.well-known/jwks.json`, so the receiver needs no secret.
	AccountLinkSigningKey string `env:"SPHERICON_ACCOUNT_LINK_SIGNING_KEY" validate:"required_with=AccountLinkURL"`

	// Signs the Staff member session and login challenge (ADR 0026, ADR 0063). Empty turns the Staff
	// sign-in off; set, it must differ from `SPHERICON_JWT_SECRET`. The `staff` license feature and
	// `SPHERICON_MODE=saas` require it. Generate one with `openssl rand -hex 32`.
	StaffJWTSecret string `env:"SPHERICON_STAFF_JWT_SECRET"`
	// The lifetime of a Staff member session, a Go duration: no refresh and no "remember me".
	StaffSessionTTL time.Duration `env:"SPHERICON_STAFF_SESSION_TTL" envDefault:"4h" validate:"gt=0"`
	// How far ahead a service may set a Staff hold's expiry, a Go duration; a later expiry is
	// refused.
	StaffHoldMaxExpiry time.Duration `env:"SPHERICON_STAFF_HOLD_MAX_EXPIRY" envDefault:"720h" validate:"gt=0"`

	// The fewest sends in the trailing 24 hours, per Sending domain, for the abuse detector (`staff`
	// license, ADR 0026) to judge its rates; below it nothing is warned or frozen.
	AbuseVolumeFloor int `env:"SPHERICON_ABUSE_VOLUME_FLOOR" envDefault:"1000" validate:"gt=0"`
	// The complaint rate, in percent, at which the abuse detector warns the Workspace's Owners by
	// email, freezing nothing.
	AbuseWarnComplaintPercent float64 `env:"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT" envDefault:"0.1" validate:"gt=0,ltfield=AbuseFreezeComplaintPercent"`
	// The bounce rate, in percent, at which the abuse detector warns the Workspace's Owners.
	AbuseWarnBouncePercent float64 `env:"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT" envDefault:"5" validate:"gt=0,ltfield=AbuseFreezeBouncePercent"`
	// The complaint rate, in percent, at which the abuse detector suspends the whole Workspace and
	// tells its Owners.
	AbuseFreezeComplaintPercent float64 `env:"SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT" envDefault:"0.3" validate:"lte=100"`
	// The bounce rate, in percent, at which the abuse detector suspends the whole Workspace.
	AbuseFreezeBouncePercent float64 `env:"SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT" envDefault:"10" validate:"lte=100"`

	// The support address shown to a Workspace's Owners with a suspension (banner and notice email),
	// where they appeal. Empty shows none.
	SupportEmail string `env:"SPHERICON_SUPPORT_EMAIL"`
	// Applies the embedded migrations at start, convenient for a single replica.
	AutoMigrate bool `env:"SPHERICON_DB_AUTO_MIGRATE"`

	// With `SPHERICON_DB_AUTO_MIGRATE` on, the longest a start waits for the migration lock and
	// the run before it fails, a Go duration; `0` is no limit. `sphericon migrate` has no limit.
	MigrateTimeout time.Duration `env:"SPHERICON_DB_MIGRATE_TIMEOUT" envDefault:"10m"`

	// The largest request body, in bytes, accepted on every surface but `/collect`; a larger one is
	// answered `413`.
	MaxBodyBytes int64 `env:"SPHERICON_MAX_BODY_BYTES" envDefault:"1048576" validate:"gt=0"`
	// The largest `/collect` batch (`POST /collect/events`), in bytes; a larger one is answered `413`.
	CollectMaxBodyBytes int64 `env:"SPHERICON_COLLECT_MAX_BODY_BYTES" envDefault:"512000" validate:"gt=0"`
	// The largest single `/collect` event, in bytes: an identify body, or each event of a batch; a
	// larger one is answered `413`.
	CollectMaxEventBytes int64 `env:"SPHERICON_COLLECT_MAX_EVENT_BYTES" envDefault:"32768" validate:"gt=0"`

	// RateLimits carry their variables, rules and doc comments on their own fields.
	RateLimits RateLimits

	// The minimum age, in days, of a domain-event outbox row every consumer has processed before
	// the prune job, which runs every 10 minutes, may delete it (ADR 0019).
	OutboxFloorDays int `env:"SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS" envDefault:"7" validate:"gte=0"`
	// The age, in days, past which analytical Events are deleted by a daily job; `0` disables it
	// (ADR 0019). Evidentiary Events (`marketing.confirmed`, `email.complained`,
	// `email.unsubscribed`, a permanent `email.bounced`) are never deleted by age.
	EventsRetentionDays int `env:"SPHERICON_EVENTS_RETENTION_DAYS" envDefault:"400" validate:"gte=0"`
	// The age, in days, past which an Unverified User with no Membership is deleted by a daily job,
	// freeing the address; `0` disables it (ADR 0030).
	UnverifiedUserRetentionDays int `env:"SPHERICON_UNVERIFIED_USER_RETENTION_DAYS" envDefault:"7" validate:"gte=0"`
	// The age, in hours, past which the external API refuses a live Event as history that belongs
	// in a Backfill. A Backfill and `/collect` are not limited.
	EventLiveWindowHours int `env:"SPHERICON_EVENT_LIVE_WINDOW_HOURS" envDefault:"72" validate:"gte=1"`
	// How many days a Test inbox keeps a Captured message (ADR 0058).
	TestInboxRetentionDays int `env:"SPHERICON_TEST_INBOX_RETENTION_DAYS" envDefault:"7" validate:"gte=1"`
	// How many Captured messages a Workspace's Test inbox keeps, the newest ones (ADR 0058).
	TestInboxMaxMessages int `env:"SPHERICON_TEST_INBOX_MAX_MESSAGES" envDefault:"200" validate:"gte=1"`

	// The most open connections of one replica's `database/sql` pool (ent, the event bus, job
	// workers). The defaults fit two replicas under Postgres' `max_connections=100`.
	DBMaxOpenConns int `env:"SPHERICON_DB_MAX_OPEN_CONNS" envDefault:"15" validate:"gt=0"`
	// The most idle connections that pool keeps. Empty is `SPHERICON_DB_MAX_OPEN_CONNS`.
	DBMaxIdleConns *int `env:"SPHERICON_DB_MAX_IDLE_CONNS" validate:"omitnil,gte=0,ltefield=DBMaxOpenConns"`
	// The longest a pooled connection lives, a Go duration.
	DBConnMaxLifetime time.Duration `env:"SPHERICON_DB_CONN_MAX_LIFETIME" envDefault:"30m" validate:"gt=0"`
	// The most connections of the job queue's own pool (fetch, completion, leader election), plus
	// one LISTEN connection outside it. Job workers use the `database/sql` pool, so it need not
	// match their number.
	RiverMaxConns int32 `env:"SPHERICON_RIVER_DB_MAX_CONNS" envDefault:"25" validate:"gt=0"`

	// Lets a Workspace's SMTP Integration reach a loopback or private host on any port. Off, it may
	// only reach a public host on port 25, 465, 587 or 2525. The Instance's own
	// `SPHERICON_SMTP_HOST` is exempt.
	SMTPAllowPrivateHosts bool `env:"SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS"`
	// Comma-separated `host:port` pairs (a name or an IP address, one port each, no wildcards) that
	// an outbound call to a third-party URL, such as a Webhook endpoint delivery, may reach although
	// the address is private or loopback, such as `n8n:5678` (ADR 0039). Redirects are still never
	// followed; a malformed value stops the start. Leave it empty on a multi-tenant host.
	OutboundAllow string `env:"SPHERICON_OUTBOUND_ALLOW"`
	// The Instance's language for the UI and its own mail. `en` is the only one; any other value
	// becomes `en`.
	Locale string `env:"SPHERICON_APP_LOCALE" envDefault:"en"`
	// `1` switches the Telemetry report off and locks the Owner's setting; it never switches the
	// report on (ADR 0032).
	DoNotTrack string `env:"DO_NOT_TRACK"`
	// The kind of deployment, `selfhosted` or `saas` (ADR 0060): it alone decides the Release check,
	// the Telemetry report, the email verification gate and the Sign-up survey. `saas` also requires
	// `SPHERICON_STAFF_JWT_SECRET`, `SPHERICON_INTERNAL_ADDR`, `SPHERICON_TRACKING_CNAME_TARGET`
	// and a system email provider. Any other value stops the start.
	Mode string `env:"SPHERICON_MODE" envDefault:"selfhosted"`
	// The Sentry-compatible DSN the browser app reports its errors to, handed to the page when it is
	// served. Empty leaves the browser reporting off.
	SentryDSN string `env:"SPHERICON_SENTRY_DSN"`

	// The OAuth client id of the Instance's Google Cloud app: with its secret, enables "Continue with
	// Google" (ADR 0048). Register `<SPHERICON_APP_URL>/site/auth/signin/google/callback` as its
	// redirect URI. One of the pair without the other stops the start.
	SigninGoogleClientID string `env:"SPHERICON_SIGNIN_GOOGLE_CLIENT_ID"`
	// The OAuth client secret that goes with `SPHERICON_SIGNIN_GOOGLE_CLIENT_ID`.
	SigninGoogleClientSecret string `env:"SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET"`
	// The OAuth client id of the Instance's GitHub OAuth app: with its secret, enables "Continue with
	// GitHub" (ADR 0048). Set `<SPHERICON_APP_URL>/site/auth/signin/github/callback` as its
	// callback URL. One of the pair without the other stops the start.
	SigninGitHubClientID string `env:"SPHERICON_SIGNIN_GITHUB_CLIENT_ID"`
	// The OAuth client secret that goes with `SPHERICON_SIGNIN_GITHUB_CLIENT_ID`.
	SigninGitHubClientSecret string `env:"SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET"`

	// The log level: `debug`, `info`, `warn` or `error`; anything else is `info`.
	LogLevel string `env:"SPHERICON_LOG_LEVEL" envDefault:"info"`
	// The log format: `json`, or `text` for coloured human-readable lines.
	LogFormat string `env:"SPHERICON_LOG_FORMAT"`
	// The `service.name` reported on OpenTelemetry traces and metrics.
	OtelServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"sphericon"`
	// The `host:port` of the opt-in Prometheus listener. Empty runs none; the public port never
	// serves `/metrics` (ADR 0025). It must differ from `PORT`.
	MetricsAddr string `env:"SPHERICON_METRICS_ADDR"`
	// The `host:port` of a DNS server that verifies Sending domains and Join domains instead of the
	// host's resolver, for a development stack that publishes its own records. Leave it empty in
	// production.
	DNSAddr string `env:"SPHERICON_DNS_ADDR"`
	// The `host:port` of the opt-in internal listener: the edge's on-demand TLS `ask` and, with the
	// `staff` license, the service endpoints of the Staff surface (ADR 0026, ADR 0038). Empty runs
	// none. It must differ from `PORT` and `SPHERICON_METRICS_ADDR`.
	InternalAddr string `env:"SPHERICON_INTERNAL_ADDR"`
	// A directory of custom Presets (ADR 0037): every `*.yaml` and `*.yml` file at its root loads
	// beside the built-in ones at start, and a file that does not load, or reuses a built-in name,
	// stops the start. Empty loads only the built-in Presets.
	PresetsDir string `env:"SPHERICON_PRESETS_DIR"`

	// The edge host a Workspace's Tracking domain (e.g. `t.example.com`) is a CNAME to, such as
	// `edge.example.com` (ADR 0038, ADR 0071). Empty turns Tracking domains off: the section is
	// hidden on Settings, Tracking and no host is set or checked.
	TrackingCNAMETarget string `env:"SPHERICON_TRACKING_CNAME_TARGET"`
	// The request header the edge sets to the customer's host, such as `X-Forwarded-Host` (ADR
	// 0038), believed only from a peer inside `SPHERICON_TRACKING_EDGE_CIDRS`. Empty uses the
	// connection's `Host`. Have the proxy of the Instance's own host drop this header.
	TrackingHostHeader string `env:"SPHERICON_TRACKING_HOST_HEADER"`
	// Comma- or space-separated CIDRs the edge connects from: the only peers whose
	// `SPHERICON_TRACKING_HOST_HEADER` is believed. A proxy on another machine must be listed.
	TrackingEdgeCIDRs string `env:"SPHERICON_TRACKING_EDGE_CIDRS" envDefault:"127.0.0.0/8,::1/128"`

	// The provider of the Instance's own mail (verification, password reset, invitations): `smtp`,
	// through `SPHERICON_SMTP_HOST`, or `ses`, through the `SPHERICON_SES_*` credentials.
	SystemEmailProvider string `env:"SPHERICON_SYSTEM_EMAIL_PROVIDER" envDefault:"smtp"`
	// The From of the Instance's own mail: a bare address or `Name <noreply@example.com>`. An
	// invalid value stops the start.
	SystemEmailFrom string `env:"SPHERICON_SYSTEM_EMAIL_FROM" envDefault:"noreply@sphericon.localhost"`
	// One bare address, such as `replies@example.com`, put in the `Reply-To` of the Instance's own
	// mail. Empty sets no header; an invalid value stops the start.
	SystemEmailReplyTo string `env:"SPHERICON_SYSTEM_EMAIL_REPLY_TO"`
	// The region of the SES-compatible service of the `ses` provider.
	SESRegion string `env:"SPHERICON_SES_REGION"`
	// The access key id of the SES-compatible service of the `ses` provider.
	SESAccessKeyID string `env:"SPHERICON_SES_ACCESS_KEY_ID"`
	// The secret access key of the SES-compatible service of the `ses` provider.
	SESSecretAccessKey string `env:"SPHERICON_SES_SECRET_ACCESS_KEY"`
}

// loadVars reads the variables of the process: the env files, then the environment over
// them, then the `_FILE` siblings of the secrets.
func loadVars(envName string) (map[string]string, error) {
	rootDir, err := envRoot()
	if err != nil {
		return nil, err
	}

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

// Variables are the names of every environment variable Load reads.
func Variables() []string {
	params, err := env.GetFieldParams(&environment{})
	if err != nil {
		// The struct's tags are fixed at compile time; a malformed one fails every Load too.
		panic(fmt.Sprintf("config: read the environment tags: %v", err))
	}
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Key)
	}
	return names
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

// parseEnvironment reads the variables over the defaults of the envName profile and refuses a
// value that breaks its field's rule.
func parseEnvironment(vars map[string]string, envName string) (environment, error) {
	e := profileDefaults(envName)
	if err := env.ParseWithOptions(&e, env.Options{Environment: vars}); err != nil {
		return environment{}, fmt.Errorf("read environment: %w", err)
	}
	if err := validateEnvironment(&e); err != nil {
		return environment{}, err
	}
	return e, nil
}

// Load reads the configuration from the process environment layered over the .env
// files (see envFiles): a variable the process sets wins over every file.
func Load(envName string) (*Config, error) {
	vars, err := loadVars(envName)
	if err != nil {
		return nil, err
	}
	e, err := parseEnvironment(vars, envName)
	if err != nil {
		return nil, err
	}

	mode, err := ParseMode(e.Mode)
	if err != nil {
		return nil, err
	}

	outboundAllow, err := webhook.ParseAllowlist(e.OutboundAllow)
	if err != nil {
		return nil, fmt.Errorf("SPHERICON_OUTBOUND_ALLOW: %w", err)
	}

	systemFrom, err := parseSystemEmailFrom(e.SystemEmailFrom)
	if err != nil {
		return nil, err
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
		RateLimits: e.RateLimits,
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
		TestInboxRetention:      time.Duration(e.TestInboxRetentionDays) * 24 * time.Hour,
		TestInboxMaxMessages:    e.TestInboxMaxMessages,
		IsDev:                   isDevEnv(envName),
		IsE2E:                   isE2E(envName),
		SMTPAllowPrivateHosts:   e.SMTPAllowPrivateHosts,
		OutboundAllow:           outboundAllow,
		Locale:                  i18n.Normalize(e.Locale),
		DoNotTrack:              strings.TrimSpace(e.DoNotTrack) == "1",
		TelemetryReceiverURL:    telemetryReceiverURL,
		TelemetrySchemaURL:      telemetrySchemaURL,

		RequireEmailVerification: false,
		SentryDSN:                strings.TrimSpace(e.SentryDSN),
		Signin:                   signin,
		LogLevel:                 e.LogLevel,
		LogFormat:                e.LogFormat,

		OtelServiceName: e.OtelServiceName,
		MetricsAddr:     e.MetricsAddr,
		DNSAddr:         e.DNSAddr,
		InternalAddr:    e.InternalAddr,
		PresetsDir:      e.PresetsDir,

		TrackingCNAMETarget: strings.ToLower(strings.TrimSuffix(strings.TrimSpace(e.TrackingCNAMETarget), ".")),
		TrackingHostHeader:  strings.TrimSpace(e.TrackingHostHeader),
		TrackingEdgeCIDRs:   edgeCIDRs,

		SystemEmailProvider: e.SystemEmailProvider,
		SystemEmailFrom:     systemFrom.Address,
		SystemEmailFromName: systemFrom.Name,
		SystemEmailReplyTo:  strings.TrimSpace(e.SystemEmailReplyTo),
		SESRegion:           e.SESRegion,
		SESAccessKeyID:      e.SESAccessKeyID,
		SESSecretAccessKey:  e.SESSecretAccessKey,

		secrets: secretsOf(vars),
	}

	cfg.ApplyMode(mode)
	cfg.isolateE2E(envName)
	return cfg.validated(envName)
}

// validated is the Config when it holds its invariants, nil and the refusal when it does not.
func (c *Config) validated(envName string) (*Config, error) {
	if err := c.validate(envName); err != nil {
		return nil, err
	}
	return c, nil
}

// isolateE2E drops the listeners a global variable would point at, so a run of the end-to-end suite
// never collides with a running dev stack on a port. The suite scrapes no metrics, and the internal
// listener is its own only in the saas Mode, which sets one.
func (c *Config) isolateE2E(envName string) {
	if !isE2E(envName) {
		return
	}
	c.MetricsAddr = ""
	if !c.Mode.SaaS() {
		c.InternalAddr = ""
	}
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

// secretVariables are the variables whose values are secrets: each also accepts a `<NAME>_FILE`
// sibling, the path of a file holding the value (Docker and Kubernetes mount secrets as files),
// and `sphericon doctor` redacts each value from everything it prints. This is the one list of
// them; TestEverySecretVariableIsRedacted fails when a secret-looking variable is missing.
var secretVariables = []string{
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
	"SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET",
	"SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET",
}

// secretsOf is the value of every secret variable in vars.
func secretsOf(vars map[string]string) []string {
	values := make([]string, 0, len(secretVariables))
	for _, name := range secretVariables {
		values = append(values, vars[name])
	}
	return values
}

// readSecretFiles fills each secret variable from its `_FILE` sibling, trimming the trailing
// newline. A value set directly wins and its file is never read; an unreadable or empty
// file is an error naming the `_FILE` variable.
func readSecretFiles(vars map[string]string) error {
	for _, name := range secretVariables {
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

// profileDefaults is the environment before the variables are read, holding the defaults that
// depend on the environment name; a variable that is set (not empty) replaces its default. The
// e2e profile has no rate limits (it polls freely), development and tests log as text, and the
// non-production profiles may reach a private SMTP host (a local Mailpit), which production
// refuses.
func profileDefaults(envName string) environment {
	e := environment{RateLimits: DefaultRateLimits, LogFormat: "json"}
	if isE2E(envName) {
		e.RateLimits = RateLimits{}
	}
	if isDevEnv(envName) {
		e.LogFormat = "text"
	}
	e.SMTPAllowPrivateHosts = isDevEnv(envName) || isE2E(envName)
	return e
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
	if err := c.validateMode(); err != nil {
		return err
	}
	if err := c.validateSystemEmailReplyTo(); err != nil {
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

// parseSystemEmailFrom splits SPHERICON_SYSTEM_EMAIL_FROM into its address and optional display
// name ("Name <noreply@example.com>"); an empty value is the zero address.
func parseSystemEmailFrom(value string) (mail.Address, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return mail.Address{}, nil
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return mail.Address{}, errors.New("SPHERICON_SYSTEM_EMAIL_FROM must be one email address, optionally with a display name: Name <noreply@example.com>")
	}
	return *parsed, nil
}

// validateSystemEmailReplyTo rejects a SPHERICON_SYSTEM_EMAIL_REPLY_TO that is not one bare
// email address (no display name, no list), so a bad value stops the start rather than every
// mail of the Instance.
func (c *Config) validateSystemEmailReplyTo() error {
	if c.SystemEmailReplyTo == "" {
		return nil
	}
	addr, err := mail.ParseAddress(c.SystemEmailReplyTo)
	if err != nil || addr.Address != c.SystemEmailReplyTo {
		return errors.New("SPHERICON_SYSTEM_EMAIL_REPLY_TO must be a single email address such as replies@example.com")
	}
	return nil
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

// envRoot is the directory the .env files are read from: the repository root, found from this
// file's source path.
func envRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate the config package's source to find the .env files")
	}
	return filepath.Join(filepath.Dir(file), ".."), nil
}

// envFiles are the .env files of an environment, in the order they override each other. The
// automated profiles (test, e2e) read only their committed .env.<name>: a developer's own
// (gitignored) .env, .env.local and .env.<name>.local would fill in what a run leaves empty.
func envFiles(rootDir, envName string) []string {
	if envName == "test" || isE2E(envName) {
		return []string{filepath.Join(rootDir, ".env."+envName)}
	}
	return []string{
		filepath.Join(rootDir, ".env"),
		filepath.Join(rootDir, ".env."+envName),
		filepath.Join(rootDir, ".env.local"),
		filepath.Join(rootDir, ".env."+envName+".local"),
	}
}

// SecretValues lists every secret value of the process: the ones the configuration read (none
// when cfg is nil, a load that failed) and the ones still in the environment, plus the password
// inside DATABASE_URL, raw and as the URL spells it. `sphericon doctor` redacts them from
// everything it prints.
func SecretValues(cfg *Config) []string {
	var values []string
	for _, name := range secretVariables {
		values = append(values, os.Getenv(name))
	}
	if cfg != nil {
		values = append(values, cfg.secrets...)
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
