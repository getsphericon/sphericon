// Package config reads the Instance's configuration from the process environment into Config.
//
// The environment is the only source: the binary reads no `.env` file and no `_FILE` sibling, and
// the defaults do not depend on SPHERICON_ENV. A developer's checkout gets its variables from mise,
// a deployment from its orchestrator. caarlos0/env parses the variables by the `env` tags of
// Config, and envdoc writes the configuration reference (docs/configuration.md) from the same tags
// and the fields' doc comments, which are written for the operator who sets the variable.
package config

import (
	"cmp"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

//go:generate go tool envdoc -types Environment -output ../docs/configuration.md -edit

// Config is every setting of the Instance: the variables of its Environment, and what Load
// derives from them or a test sets. envdoc prints Environment's doc comments in the reference, so
// each one is written for the operator who sets the variable.
type Config struct {
	Environment

	// IsDev is true in the development and test environments (SPHERICON_ENV): it relaxes
	// production-only behaviour locally.
	IsDev bool
	// IsE2E is true in the end-to-end environment (EnvE2E), a non-production env of its own.
	IsE2E bool

	// Telemetry is false in the saas Mode, which sends no Telemetry report and locks the setting
	// off (ADR 0032, 0060). Nil in selfhosted leaves the decision to the Instance's Owner.
	Telemetry *bool
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
	// SignupSurvey asks a User who signed up the Sign-up survey once: on in saas, off in
	// selfhosted (ADR 0055, 0060).
	SignupSurvey bool
}

// Environment is every variable the Instance reads, with its default.
type Environment struct {
	Listener
	Keyset

	// The PostgreSQL connection string, such as `postgres://user:pass@host:5432/db?sslmode=require`;
	// `sslmode=verify-full` with `sslrootcert` also verifies the server's certificate. The job queue
	// LISTENs for new jobs on a session of its own: behind PgBouncer, it must reach Postgres directly
	// or a pool in session mode, never transaction mode.
	DatabaseURL string `env:"SPHERICON_DATABASE_URL,required,notEmpty"`
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
	// Comma-separated origins allowed credentialed CORS on the cookie-authenticated surfaces
	// (`/site`, `/staff`). Empty is same-origin only, which the bundled app needs; the bearer-token
	// surfaces answer any origin without credentials either way.
	CORSOrigins []string `env:"SPHERICON_CORS_ORIGINS" envSeparator:","`
	// Signs sessions and mailed links; the start is refused without it. Generate one with
	// `openssl rand -hex 32`.
	JWTSecret string `env:"SPHERICON_JWT_SECRET,required,notEmpty"`
	// The second secret of a JWT secret rotation: accepted when a session or a mailed link is
	// verified, never used to sign. Empty outside a rotation.
	JWTSecretSecondary string `env:"SPHERICON_JWT_SECRET_SECONDARY"`
	// The lifetime of a site session, a Go duration such as `12h`: both the token's expiry and the
	// cookie's, with no refresh (ADR 0020).
	SessionTTL time.Duration `env:"SPHERICON_SESSION_TTL" envDefault:"24h"`
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
	AccountLinkURL string `env:"SPHERICON_ACCOUNT_LINK_URL"`
	// The base64 Ed25519 seed that signs the Account link token (`openssl rand -base64 32`). The
	// matching public key is published at `/.well-known/jwks.json`, so the receiver needs no secret.
	AccountLinkSigningKey string `env:"SPHERICON_ACCOUNT_LINK_SIGNING_KEY"`

	// Signs the Staff member session and login challenge (ADR 0026, ADR 0063). Empty turns the Staff
	// sign-in off; set, it must differ from `SPHERICON_JWT_SECRET`. The `staff` license feature
	// requires it. Generate one with `openssl rand -hex 32`.
	StaffJWTSecret string `env:"SPHERICON_STAFF_JWT_SECRET"`
	// The lifetime of a Staff member session, a Go duration: no refresh and no "remember me".
	StaffSessionTTL time.Duration `env:"SPHERICON_STAFF_SESSION_TTL" envDefault:"4h"`
	// How far ahead a service may set a Staff hold's expiry, a Go duration; a later expiry is
	// refused.
	StaffHoldMaxExpiry time.Duration `env:"SPHERICON_STAFF_HOLD_MAX_EXPIRY" envDefault:"720h"`

	// The fewest sends in the trailing 24 hours, per Sending domain, for the abuse detector (`staff`
	// license, ADR 0026) to judge its rates; below it nothing is warned or frozen.
	AbuseVolumeFloor int `env:"SPHERICON_ABUSE_VOLUME_FLOOR" envDefault:"1000"`
	// The complaint rate, in percent, at which the abuse detector warns the Workspace's Owners by
	// email, freezing nothing.
	AbuseWarnComplaintPercent float64 `env:"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT" envDefault:"0.1"`
	// The bounce rate, in percent, at which the abuse detector warns the Workspace's Owners.
	AbuseWarnBouncePercent float64 `env:"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT" envDefault:"5"`
	// The complaint rate, in percent, at which the abuse detector suspends the whole Workspace and
	// tells its Owners.
	AbuseFreezeComplaintPercent float64 `env:"SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT" envDefault:"0.3"`
	// The bounce rate, in percent, at which the abuse detector suspends the whole Workspace.
	AbuseFreezeBouncePercent float64 `env:"SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT" envDefault:"10"`

	// The support address shown to a Workspace's Owners with a suspension (banner and notice email),
	// where they appeal. Empty shows none.
	SupportEmail string `env:"SPHERICON_SUPPORT_EMAIL"`
	// Applies the embedded migrations at start, convenient for a single replica.
	AutoMigrate bool `env:"SPHERICON_DB_AUTO_MIGRATE"`
	// With `SPHERICON_DB_AUTO_MIGRATE` on, the longest a start waits for the migration lock and
	// the run before it fails, a Go duration; `0` is no limit. `sphericon migrate` has no limit.
	MigrateTimeout time.Duration `env:"SPHERICON_DB_MIGRATE_TIMEOUT" envDefault:"10m"`

	// The request body limits.
	BodyLimits BodyLimits
	// The per-policy rate limits (ADR 0025); `0` disables a limit.
	RateLimits RateLimits

	// The age past which analytical Events are deleted by a daily job, a Go duration such as
	// `9600h` (400 days); `0` disables it (ADR 0019). Evidentiary Events (`marketing.confirmed`,
	// `sms.confirmed`, the `*.engagement_tracking_granted` and `*.engagement_tracking_denied` Events,
	// `email.complained`, `email.unsubscribed`, `email.resubscribed`, a permanent `email.bounced`) are
	// never deleted by age. Rule Firings and the receipts that reject a redelivered external fact
	// end with the same window.
	EventsRetention time.Duration `env:"SPHERICON_EVENTS_RETENTION" envDefault:"9600h"`
	// The age past which a daily job deletes Outbound messages, Broadcast recipients and tracked
	// links, keeping the Broadcast and Automation figures they made (ADR 0143), a Go duration; `0`
	// disables it. A finished Broadcast goes whole once it has been finished that long. The
	// analytics overview reads 180 days (`4320h`) back, so keep at least that.
	MessagesRetention time.Duration `env:"SPHERICON_MESSAGES_RETENTION" envDefault:"9600h"`
	// The age past which an Unverified User with no Membership is deleted by a daily job, freeing
	// the address, a Go duration; `0` disables it (ADR 0030).
	UnverifiedUserRetention time.Duration `env:"SPHERICON_UNVERIFIED_USER_RETENTION" envDefault:"168h"`
	// The age past which the external API refuses a live Event as history that belongs in a
	// Backfill, a Go duration. A Backfill and `/collect` are not limited.
	EventLiveWindow time.Duration `env:"SPHERICON_EVENT_LIVE_WINDOW" envDefault:"72h"`
	// How long a Test inbox keeps a Captured message, a Go duration (ADR 0058).
	TestInboxRetention time.Duration `env:"SPHERICON_TEST_INBOX_RETENTION" envDefault:"168h"`
	// How many Captured messages a Workspace's Test inbox keeps, the newest ones (ADR 0058).
	TestInboxMaxMessages int `env:"SPHERICON_TEST_INBOX_MAX_MESSAGES" envDefault:"200"`

	// The Postgres connection pools of one replica.
	DBPool DBPool

	// Lets a Workspace's SMTP Integration reach a loopback or private host on any port. Off, it may
	// only reach a public host on port 25, 465, 587 or 2525. The Instance's own
	// `SPHERICON_SMTP_HOST` is exempt.
	SMTPAllowPrivateHosts bool `env:"SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS"`
	// Comma-separated `host:port` pairs (a name or an IP address, one port each, no wildcards) that
	// an outbound call to a third-party URL, such as a Webhook endpoint delivery, may reach although
	// the address is private or loopback, such as `n8n:5678` (ADR 0039). Redirects are still never
	// followed; a malformed value stops the start. Leave it empty on a multi-tenant host.
	OutboundAllow webhook.Allowlist `env:"SPHERICON_OUTBOUND_ALLOW"`
	// The Instance's language for the UI and its own mail. `en` is the only one.
	Locale string `env:"SPHERICON_APP_LOCALE" envDefault:"en"`
	// `true` (or `1`) switches the Telemetry report off and locks the Owner's setting; it never
	// switches the report on (ADR 0032).
	DoNotTrack bool `env:"DO_NOT_TRACK"`
	// The kind of deployment, `selfhosted` or `saas` (ADR 0060): it alone decides the Release check,
	// the Telemetry report, the email verification gate and the Sign-up survey. Any other value
	// stops the start.
	Mode Mode `env:"SPHERICON_MODE" envDefault:"selfhosted"`
	// The Sentry-compatible DSN the browser app reports its errors to, handed to the page when it is
	// served. Empty leaves the browser reporting off.
	SentryDSN string `env:"SPHERICON_SENTRY_DSN"`

	// The Sign-in providers' OAuth clients (ADR 0048).
	Signin SigninProviders

	// The log level: `debug`, `info`, `warn` or `error`, in any case; a value that names no level
	// stops the start.
	LogLevel slog.Level `env:"SPHERICON_LOG_LEVEL" envDefault:"info"`
	// The log format: `json`, or `text` for coloured human-readable lines; any other value stops the
	// start.
	LogFormat LogFormat `env:"SPHERICON_LOG_FORMAT" envDefault:"json"`
	// The `service.name` reported on OpenTelemetry traces and metrics. The export destinations come
	// from the declarative configuration file `OTEL_CONFIG_FILE` names.
	OtelServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"sphericon"`
	// The `host:port` of the opt-in Prometheus listener. Empty runs none; the public port never
	// serves `/metrics` (ADR 0025). Give it a port other than `PORT`.
	MetricsAddr string `env:"SPHERICON_METRICS_ADDR"`
	// The `host:port` of a DNS server that verifies Sending domains and Join domains instead of the
	// host's resolver, for a development stack that publishes its own records. Leave it empty in
	// production.
	DNSAddr string `env:"SPHERICON_DNS_ADDR"`
	// The `host:port` of the opt-in internal listener: the edge's on-demand TLS `ask` and, with the
	// `staff` license, the service endpoints of the Staff surface (ADR 0026, ADR 0038). Empty runs
	// none. Give it a port other than `PORT` and `SPHERICON_METRICS_ADDR`'s.
	InternalAddr string `env:"SPHERICON_INTERNAL_ADDR"`
	// A directory of custom Presets (ADR 0037): every `*.yaml` and `*.yml` file at its root loads
	// beside the built-in ones at start, and a file that does not load, or reuses a built-in name,
	// stops the start. Empty loads only the built-in Presets.
	PresetsDir string `env:"SPHERICON_PRESETS_DIR"`
	// The base URL, without a trailing slash, of the Telegram Bot API every Workspace's Telegram bot
	// is reached at: Telegram's own server by default, or a local `telegram-bot-api` server
	// (https://github.com/tdlib/telegram-bot-api) that runs the Bot API on your own infrastructure,
	// such as `http://telegram-bot-api:8081`. Calls go to `<url>/bot<token>/<method>`.
	TelegramBotAPIURL string `env:"SPHERICON_TELEGRAM_BOT_API_URL" envDefault:"https://api.telegram.org"`
	// The base URL, without a trailing slash, of Twilio's REST API every Workspace's SMS Integration
	// is reached at: Twilio's own API by default, or a regional edge such as
	// `https://api.dublin.ie1.twilio.com`. The Messaging Services API is reached at the same host
	// with its first label `messaging` instead of `api`; a base whose host does not start with
	// `api.` (a proxy, a test's fake) takes both APIs' calls.
	TwilioAPIURL string `env:"SPHERICON_TWILIO_API_URL" envDefault:"https://api.twilio.com"`

	// The edge host a Workspace's Tracking domain (e.g. `t.example.com`) is a CNAME to, such as
	// `edge.example.com`, in lowercase and without a trailing dot (ADR 0038, ADR 0071). Empty turns
	// Tracking domains off: the section is hidden on Settings, Tracking and no host is set or
	// checked.
	TrackingCNAMETarget string `env:"SPHERICON_TRACKING_CNAME_TARGET"`
	// The request header the edge sets to the customer's host, such as `X-Forwarded-Host` (ADR
	// 0038), believed only from a peer inside `SPHERICON_TRACKING_EDGE_CIDRS`. Empty uses the
	// connection's `Host`. Have the proxy of the Instance's own host drop this header.
	TrackingHostHeader string `env:"SPHERICON_TRACKING_HOST_HEADER"`
	// Comma-separated CIDRs the edge connects from: the only peers whose
	// `SPHERICON_TRACKING_HOST_HEADER` is believed. A proxy on another machine must be listed.
	TrackingEdgeCIDRs []netip.Prefix `env:"SPHERICON_TRACKING_EDGE_CIDRS" envSeparator:"," envDefault:"127.0.0.0/8,::1/128"`
	// Comma-separated CIDRs of the reverse proxies in front of the Instance (ADR 0175): a request's
	// `X-Forwarded-For` is read only when it comes from one of them, and its rightmost address
	// outside them is the client's, the one the rate limits key on and consent records keep. The
	// default trusts loopback and the private networks, where a proxy on the same host, in a
	// container network or in a cluster connects from; list a proxy on a public address. Empty trusts
	// none: the connection's own address is the client's.
	TrustedProxyCIDRs []netip.Prefix `env:"SPHERICON_TRUSTED_PROXY_CIDRS" envSeparator:"," envDefault:"127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"`

	// The provider of the Instance's own mail (verification, password reset, invitations): `smtp`,
	// through `SPHERICON_SMTP_HOST`, or `ses`, through the `SPHERICON_SES_*` credentials.
	SystemEmailProvider providers.Provider `env:"SPHERICON_SYSTEM_EMAIL_PROVIDER" envDefault:"smtp"`
	// The bare From address of the Instance's own mail, such as `noreply@example.com`.
	SystemEmailFrom string `env:"SPHERICON_SYSTEM_EMAIL_FROM" envDefault:"noreply@sphericon.localhost"`
	// The display name of the From of the Instance's own mail; empty sends the bare address.
	SystemEmailFromName string `env:"SPHERICON_SYSTEM_EMAIL_FROM_NAME"`
	// One bare address, such as `replies@example.com`, put in the `Reply-To` of the Instance's own
	// mail. Empty sets no header.
	SystemEmailReplyTo string `env:"SPHERICON_SYSTEM_EMAIL_REPLY_TO"`
	// The region of the SES-compatible service of the `ses` provider.
	SESRegion string `env:"SPHERICON_SES_REGION"`
	// The access key id of the SES-compatible service of the `ses` provider.
	SESAccessKeyID string `env:"SPHERICON_SES_ACCESS_KEY_ID"`
	// The secret access key of the SES-compatible service of the `ses` provider.
	SESSecretAccessKey string `env:"SPHERICON_SES_SECRET_ACCESS_KEY"`
}

// Listener is the public listener's variable, which the container health check reads on its own
// (Port).
type Listener struct {
	// The port the public HTTP server listens on, on every interface.
	Port string `env:"PORT" envDefault:"3000"`
}

// Keyset is the encryption key's variable, which the key commands read on its own (EncryptionKey):
// they need no database.
type Keyset struct {
	// The base64 Tink keyset that encrypts stored provider credentials; the start is refused without
	// it. Generate it with `sphericon genkey`.
	EncryptionKey string `env:"SPHERICON_ENCRYPTION_KEY,required,notEmpty"`
}

// BodyLimits are the request body caps in bytes. /collect is the public tracking
// ingestion (its key ships in customer pages, so anyone can post to it): Collect
// caps a batch (POST /collect/events), CollectEvent one event (an identify body, or
// each event inside a batch). CSVImport caps the upload of a CSV import (ADR 0138). Default
// applies to every other surface.
type BodyLimits struct {
	// The largest request body, in bytes, accepted on every surface but `/collect`; a larger one is
	// answered `413`.
	Default int64 `env:"SPHERICON_MAX_BODY_BYTES" envDefault:"1048576"`
	// The largest `/collect` batch (`POST /collect/events`), in bytes; a larger one is answered `413`.
	Collect int64 `env:"SPHERICON_COLLECT_MAX_BODY_BYTES" envDefault:"512000"`
	// The largest single `/collect` event, in bytes: an identify body, or each event of a batch; a
	// larger one is answered `413`.
	CollectEvent int64 `env:"SPHERICON_COLLECT_MAX_EVENT_BYTES" envDefault:"32768"`
	// The largest CSV file of Contacts a Member uploads to import, in bytes (50 MiB by default); a
	// larger one is answered `413` with the limit. Raise it for a large migration.
	CSVImport int64 `env:"SPHERICON_CSV_IMPORT_MAX_BYTES" envDefault:"52428800"`
}

// RateLimits are the per-policy request budgets (ADR 0025). Every limit has a default and 0
// disables it. They are core, never gated by the EE licence.
type RateLimits struct {
	// Requests per minute per client IP and endpoint on the public human-facing endpoints
	// (signup, invitation accept, consent confirm); over it `429` with `Retry-After`.
	Human int `env:"SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE" envDefault:"60"`
	// Requests per second per Workspace on `/api` and `/mcp` (one shared budget); over it `429`
	// with `Retry-After`.
	APIBurst int `env:"SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND" envDefault:"20"`
	// Requests per minute per Workspace on `/api` and `/mcp`, stacked on the burst limit.
	APIPerMinute int `env:"SPHERICON_RATE_LIMIT_API_PER_MINUTE" envDefault:"600"`
	// Failed credential checks (bearer token, collect key) per minute per client IP; successful
	// ones are not counted.
	FailedAuth int `env:"SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE" envDefault:"30"`
	// Opens and clicks recorded per minute per client IP. It never refuses a recipient: over it the
	// redirect or pixel is still served and the event is not recorded.
	Tracking int `env:"SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE" envDefault:"600"`
	// Failed logins one account may have within 15 minutes before login answers `429` with an
	// exponentially growing delay, even for a correct password (no lockout).
	LoginFailures int `env:"SPHERICON_RATE_LIMIT_LOGIN_FAILURES" envDefault:"5"`
	// Login requests per minute per client IP.
	LoginIP int `env:"SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE" envDefault:"20"`
	// Requests per minute per Workspace on `/collect`, a budget of its own apart from `/api` and
	// `/mcp`.
	Collect int `env:"SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE" envDefault:"6000"`
	// Requests per minute per client IP on `/collect`.
	CollectIP int `env:"SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE" envDefault:"300"`
	// Requests per minute per Service credential on the internal listener's service endpoints.
	ServicePerMinute int `env:"SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE" envDefault:"600"`
	// Mails one address may be sent per hour by each of forgot-password, registration and an
	// email-change request (ADR 0031). Over it the request is still answered `202` and nothing is
	// sent, so the answer never tells whether the address has an account.
	ForgotAddress int `env:"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR" envDefault:"3"`
	// Forgot-password requests per client IP per hour; over it `429`.
	ForgotIP int `env:"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR" envDefault:"10"`
}

// DBPool bounds the Postgres connections one replica may open: the
// database/sql pool (ent, pubsub) and the pgx pool river runs on. Defaults
// assume at most two replicas against max_connections=100:
// (15+25) x 2 x 1.15 = 92 <= 97.
type DBPool struct {
	// The most open connections of one replica's `database/sql` pool (ent, the event bus, job
	// workers). The defaults fit two replicas under Postgres' `max_connections=100`.
	MaxOpenConns int `env:"SPHERICON_DB_MAX_OPEN_CONNS" envDefault:"15"`
	// The most idle connections that pool keeps. Empty is `SPHERICON_DB_MAX_OPEN_CONNS`.
	// It is declared after MaxOpenConns: the expansion reads that field's parsed value.
	MaxIdleConns int `env:"SPHERICON_DB_MAX_IDLE_CONNS,expand" envDefault:"${SPHERICON_DB_MAX_OPEN_CONNS}"`
	// The longest a pooled connection lives, a Go duration.
	ConnMaxLifetime time.Duration `env:"SPHERICON_DB_CONN_MAX_LIFETIME" envDefault:"30m"`
	// The most connections of the job queue's own pool (fetch, completion, leader election), plus
	// one LISTEN connection outside it. Job workers use the `database/sql` pool, so it need not
	// match their number.
	RiverMaxConns int32 `env:"SPHERICON_RIVER_DB_MAX_CONNS" envDefault:"25"`
}

// telemetryReceiverURL is the address of the Telemetry report receiver, fixed in the binary.
const telemetryReceiverURL = "https://telemetry.getsphericon.com"

// telemetrySchemaURL is the page that documents the Telemetry report's schema.
const telemetrySchemaURL = "https://getsphericon.com/docs/operations/telemetry-schema"

// Load reads the configuration from the process environment. envName is the environment the
// process runs as (SPHERICON_ENV, read by the caller); it only sets IsDev and IsE2E.
func Load(envName string) (*Config, error) {
	cfg := &Config{
		IsDev:                envName == EnvDevelopment || envName == EnvTest,
		IsE2E:                envName == EnvE2E,
		TelemetryReceiverURL: telemetryReceiverURL,
		TelemetrySchemaURL:   telemetrySchemaURL,
	}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("read the environment: %w", err)
	}
	cfg.ApplyMode(cfg.Mode)
	return cfg, nil
}

// Port is PORT as the process sees it, without loading the rest of the config: the
// container health check needs no database and no secrets.
func Port() (string, error) {
	l, err := env.ParseAs[Listener]()
	if err != nil {
		return "", fmt.Errorf("read the environment: %w", err)
	}
	return l.Port, nil
}

// EncryptionKey is SPHERICON_ENCRYPTION_KEY as the process sees it, without loading the rest of
// the config: the key commands need no database.
func EncryptionKey() (string, error) {
	k, err := env.ParseAs[Keyset]()
	if err != nil {
		return "", fmt.Errorf("read the environment: %w", err)
	}
	return k.EncryptionKey, nil
}

// Variables are the names of every environment variable Load reads.
func Variables() []string {
	params, err := env.GetFieldParams(&Environment{})
	if err != nil {
		// The struct's tags are fixed at compile time; a malformed one fails every Load too.
		panic("config: read the env tags: " + err.Error())
	}
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Key)
	}
	return names
}

// JWTVerifySecrets are the secrets a session or a mailed link is also accepted under besides
// JWTSecret: the secondary key of a rotation, none outside one.
func (c *Config) JWTVerifySecrets() []string {
	if c.JWTSecretSecondary == "" {
		return nil
	}
	return []string{c.JWTSecretSecondary}
}

// JWTSecrets are every secret a session or a mailed link is accepted under: JWTSecret first,
// then the verify-only ones.
func (c *Config) JWTSecrets() []string {
	return append([]string{c.JWTSecret}, c.JWTVerifySecrets()...)
}

// SecureCookies reports whether the session cookie is Secure: the public origin is https.
func (c *Config) SecureCookies() bool { return strings.HasPrefix(c.AppURL, "https://") }

// SystemMailConfigured reports whether the Instance has outbound mail, that is, a System
// sender: the SES provider, or SMTP with a host. Without it there is none (ADR 0041).
func (c *Config) SystemMailConfigured() bool {
	return c.SystemEmailProvider == providers.SES || c.SMTPHost != ""
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

// EnvVar is the variable that names the environment a process runs as; unset is EnvProduction.
const EnvVar = "SPHERICON_ENV"

// Env is the environment this process runs as: the value of EnvVar, EnvProduction when it is unset.
func Env() string { return cmp.Or(os.Getenv(EnvVar), EnvProduction) }

// The environments a process runs as.
const (
	// EnvProduction is a deployed Instance, the environment when EnvVar is unset.
	EnvProduction = "production"
	// EnvDevelopment is a developer's checkout.
	EnvDevelopment = "development"
	// EnvTest is the Go test suite's.
	EnvTest = "test"
	// EnvE2E is the end-to-end suite's. It is not IsDev: the dev DKIM lookup stays off and the
	// harness injects its own.
	EnvE2E = "e2e"
)

// secretVariables are the variables whose values `sphericon doctor` redacts.
var secretVariables = []string{
	"SPHERICON_DATABASE_URL",
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

// SecretValues lists every secret value of the process environment, plus the password inside
// SPHERICON_DATABASE_URL, raw and as the URL spells it. `sphericon doctor` redacts them from
// everything it prints.
func SecretValues() []string {
	var values []string
	for _, name := range secretVariables {
		values = append(values, os.Getenv(name))
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
